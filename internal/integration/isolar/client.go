// Package isolar is the iSolarCloud (Sungrow) adapter: plant/device
// discovery, minute production series and fault alarms (06-integrations.md
// §6). Unlike the meter adapters (internal/integration/osos, .../gridbox,
// .../aril, .../pm5340), it does not implement integration.MeterDataSource —
// it feeds plant_production, power_plant_devices and plant alarms instead
// of meter_readings, so its Client exposes its own bespoke method set.
//
// This package is PURE (06 §1 rule 2): no import of internal/store,
// internal/ingest or github.com/jackc/pgx — internal/arch's
// TestAdaptersDoNotImportTheStore enforces it. The blocker-aware store path
// that consumes this package's ProductionSample lives in
// internal/ingest/production, a separate package one layer up.
package isolar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

// resultSuccess is the ONE value of envelope.ResultCode that means success
// (06 §6: "\"1\" means success, otherwise R21").
const resultSuccess = "1"

// defaultMinuteInterval is the legacy default (06 §6's brief: "minute
// series requests use minute_interval \"60\" (legacy default)").
const defaultMinuteInterval = "60"

// maxPages bounds every paginated call in this package (queryPowerStationList,
// getDeviceListByPsId, getFaultAlarmInfo), the same "page budget" R4 gives
// the meter adapters, so a provider bug (rowCount that never shrinks, an
// empty page that is not the last one) cannot loop forever.
const maxPages = 10

// pointIDList is the fixed set of measurement points every minute-series
// call asks for (06 §6): 1 yield today (Wh), 24 total active power (W),
// 2001 daily horizontal irradiation (Wh/m²), 2009 ambient temperature (°C),
// 2010 module temperature (°C).
const pointIDList = "1,24,2001,2009,2010"

// regionCloudID is 06 §6's Regions table's Cloud id column, keyed by
// Credentials.Region. It is fixed platform knowledge, not something any
// integration_definitions row carries, so it is a constant here rather than
// read from creds.Endpoints.
var regionCloudID = map[string]int{
	"EU": 3,
	"CN": 1,
	"AU": 2,
}

// authFailureMsgPattern matches a result_msg that names an authentication
// problem (R21's mapping is isolated to classifyResult below, the one place
// touching it). An HTTP-level 401/403 is already ErrAuth before a response
// body is ever read (httpx's classifyStatus); this pattern handles the
// other half of R21 — a 200 response whose JSON envelope carries a
// non-success result_code.
var authFailureMsgPattern = regexp.MustCompile(`(?i)token|auth|appkey|access`)

// classifyResult maps a non-success (envelope.ResultCode != "1") result to
// an integration.Error Kind. This is the ONE place that mapping lives (per
// the task brief: "isolate the code→Kind mapping in one place with a
// comment"). iSolar's actual non-success codes are UNVERIFIED against a
// real response (R21, open question Q4) — the brief's own worked example
// (result_code "E00003", result_msg "er_token_login_invalid") matches via
// the "token" word in result_msg, which is the mechanism R21 specifies:
// "a result_msg matching (?i)token|auth|appkey|access, is ErrAuth; anything
// else is ErrUpstreamUnavailable." Correcting this mapping against real
// iSolar codes in F14 touches only this function.
func classifyResult(_ string, msg string) error {
	if authFailureMsgPattern.MatchString(msg) {
		return integration.ErrAuth
	}
	return integration.ErrUpstreamUnavailable
}

// Options configures a Client.
type Options struct {
	Clock clock.Clock
	// MinuteInterval overrides defaultMinuteInterval ("60") for every
	// minute-series call. Empty: the legacy default applies.
	MinuteInterval string
}

// Client is the iSolarCloud adapter. It is built fresh from a *httpx.Pool
// per the adapter contract (06 §1 rule 2: adapters never construct their
// own *http.Client) and carries no per-call state — every method takes its
// own integration.Credentials, so one Client serves every company's calls.
type Client struct {
	pool           *httpx.Pool
	clock          clock.Clock
	minuteInterval string
}

// New builds a Client from pool.
func New(pool *httpx.Pool, o Options) *Client {
	c := o.Clock
	if c == nil {
		c = clock.System()
	}
	interval := o.MinuteInterval
	if interval == "" {
		interval = defaultMinuteInterval
	}
	return &Client{pool: pool, clock: c, minuteInterval: interval}
}

// httpClient builds the *httpx.Client for one call. EVERY client this
// package builds — auth (token/refreshToken) AND data (plants, devices,
// series, faults) — carries the SAME LimiterKey/SerializeKey, both
// "isolar:<company id>" (adapter-patterns.md item 15: serialisation and
// rate limiting apply to every client an adapter builds, not only its data
// calls). Values are the Provider defaults table's isolar row: 30s
// timeout, 1 request/second, burst 1, serialised per company.
func (c *Client) httpClient(creds integration.Credentials) *httpx.Client {
	key := "isolar:" + creds.CompanyID.String()
	return c.pool.Client(httpx.ClientConfig{
		Provider:       integration.ProviderISolar,
		LimiterKey:     key,
		Every:          time.Second,
		Burst:          1,
		RequestTimeout: 30 * time.Second,
		SerializeKey:   key,
	})
}

// callOptions is one call's shape: which endpoint, whether it carries the
// bearer access-token header (every call except token/refreshToken — 06 §6:
// "Every platform call carries the app key, the signed secret header and
// the access token" — token/refreshToken are the two exceptions, since the
// access token does not exist yet, or is being replaced), whether it must
// go through the R32 NoRetry path, and the request-specific body fields
// (appkey is added by call itself, never by a caller).
type callOptions struct {
	op      string
	bearer  bool
	noRetry bool
	body    map[string]any
}

// call is the ONE place every iSolarCloud request is built and sent: it
// resolves the endpoint template from creds.Endpoints (never anywhere
// else — "Endpoints come from creds.Endpoints"), attaches appkey to the
// body and x-access-key/Authorization to the headers, and decodes the
// {result_code, result_msg, result_data} envelope. It returns the raw
// result_data for the caller to unmarshal into its own wire type.
func (c *Client) call(ctx context.Context, creds integration.Credentials, o callOptions) (json.RawMessage, error) {
	tmpl, ok := creds.Endpoints[o.op]
	if !ok || tmpl == "" {
		return nil, c.configError(o.op)
	}

	appKey, secretKey, err := c.appAndSecretKey(creds)
	if err != nil {
		return nil, err
	}

	body := make(map[string]any, len(o.body)+1)
	body["appkey"] = appKey
	for k, v := range o.body {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderISolar, Op: o.op}
	}

	header := make(http.Header)
	header.Set("x-access-key", secretKey)
	if o.bearer {
		token := creds.Extra["access_token"]
		if token.IsZero() {
			return nil, c.configError(o.op)
		}
		header.Set("Authorization", "Bearer "+token.Reveal())
	}

	resp, err := c.httpClient(creds).Do(ctx, httpx.Request{
		Op:          o.op,
		Method:      http.MethodPost,
		Template:    tmpl,
		Header:      header,
		Body:        raw,
		ContentType: "application/json",
		NoRetry:     o.noRetry,
	})
	if err != nil {
		return nil, err
	}
	return c.decodeEnvelope(o.op, resp)
}

// configError is returned for every configuration/credential-incompleteness
// failure this package detects before ever sending a request: a missing
// endpoint template, a missing app_key/secret_key/access_token. CHOSEN
// KIND: ErrAuth. No dedicated config Kind exists in internal/integration
// today (a planned follow-up per the task brief); ErrMalformedPayload is
// documented as the wrong choice for a config error (adapter-patterns.md
// item 9), and ErrAuth is the closest existing sentinel to "this call
// cannot be authenticated/addressed without configuration that is
// missing" — every case this function covers is exactly that. Revisit this
// choice once a dedicated Kind exists.
func (c *Client) configError(op string) error {
	return &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderISolar, Op: op}
}

// appAndSecretKey resolves the two credential-shaped Extra values every
// call needs, or a configError.
func (c *Client) appAndSecretKey(creds integration.Credentials) (appKey, secretKey string, err error) {
	ak := creds.Extra["app_key"]
	sk := creds.Extra["secret_key"]
	if ak.IsZero() || sk.IsZero() {
		return "", "", c.configError("credentials")
	}
	return ak.Reveal(), sk.Reveal(), nil
}

// decodeEnvelope enforces 06 §6's response contract: the body must be
// application/json (else ErrMalformedPayload) and shaped
// {result_code, result_msg, result_data} with result_code present and
// non-null (else ErrMalformedPayload, adapter-patterns.md item 6 — a
// missing/null top-level key is never treated as an empty success).
// result_code == "1" is success; any other value goes through
// classifyResult.
func (c *Client) decodeEnvelope(op string, resp httpx.Response) (json.RawMessage, error) {
	if mt, _, mimeErr := mime.ParseMediaType(resp.Header.Get("Content-Type")); mimeErr != nil || mt != "application/json" {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderISolar, Op: op, HTTPStatus: resp.Status}
	}

	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	dec.UseNumber()
	var env envelope
	if err := dec.Decode(&env); err != nil || env.ResultCode == nil {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderISolar, Op: op, HTTPStatus: resp.Status}
	}
	if *env.ResultCode != resultSuccess {
		return nil, &integration.Error{Kind: classifyResult(*env.ResultCode, env.ResultMsg), Provider: integration.ProviderISolar, Op: op, HTTPStatus: resp.Status}
	}
	return env.ResultData, nil
}

// formatIsolarTime renders t (any instant) as Europe/Istanbul local
// "yyyy-MM-dd HH:mm:ss" — the inverse of parseIsolarTime in series.go/
// alarms.go, and the format every from/to request parameter in this
// package uses.
func formatIsolarTime(t time.Time) string {
	return t.In(normalize.Istanbul).Format("2006-01-02 15:04:05")
}

// malformedErr wraps a json.Unmarshal failure into ErrMalformedPayload,
// never the bare decode error (which could otherwise echo request/response
// data). Deliberately not a generic helper: internal/arch's
// TestIntegrationTreesDoNotParseFloats treats every type-parameter decode
// target as an unverifiable hazard by construction (it cannot see through a
// generic function's own body to each call site's concrete instantiation),
// so every result_data decode in this package unmarshals into its own
// concrete, named wire type inline and calls this helper only to wrap the
// resulting error.
func malformedErr(op string) error {
	return &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderISolar, Op: op}
}

// requireRegion resolves creds.Region to its Cloud id, or a configError for
// an unrecognised region.
func (c *Client) requireRegion(creds integration.Credentials) (int, error) {
	id, ok := regionCloudID[creds.Region]
	if !ok {
		return 0, c.configError(fmt.Sprintf("region:%s", creds.Region))
	}
	return id, nil
}

// Package isolar is the iSolarCloud (Sungrow) adapter: plant/device
// discovery, minute production series and fault alarms (06-integrations.md
// §6). Unlike the meter adapters (internal/integration/osos, .../gridbox,
// .../aril, .../pm5340), it does not implement integration.MeterDataSource —
// it feeds plant production, power_plant_devices and plant faults instead
// of meter_readings, so its Client exposes its own bespoke method set.
//
// This package is PURE (06 §1 rule 2): no import of internal/store,
// internal/ingest or github.com/jackc/pgx — internal/arch's
// TestAdaptersDoNotImportTheStore enforces it. internal/service/solar
// (F9) is the one consumer that stores what this package fetches.
package isolar

import (
	"bytes"
	"context"
	"encoding/json"
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

// defaultMinuteInterval is the legacy default (isolarClient.ts:614,643:
// "minute_interval: options.minuteInterval || \"60\"").
const defaultMinuteInterval = "60"

// isolarTimestampLayout is every timestamp this package sends or parses on
// the wire: request start_time_stamp/end_time_stamp and response
// time_stamp are documented "yyyyMMddHHmmss" (isolarClient.ts:594,622's own
// comments: "timestamp format: yyyyMMddHHmmss"). getFaultAlarmInfo's
// create_time has no format documented anywhere in the legacy source; this
// package assumes the same family convention (R41 allows "one F14 verify
// against real responses note" for exactly this kind of gap) rather than
// inventing an unrelated format — flagged for F14 verification alongside
// R21/R35's other unverified-against-a-real-response items.
const isolarTimestampLayout = "20060102150405"

// maxPageBudget bounds every non-cursor paginated call in this package
// (queryPowerStationList, getDeviceListByPsId, getFaultAlarmInfo) — R44's
// "hard cap (e.g. 100)". Unlike round 1's maxPages=10 silent-truncation
// loop, exceeding this budget is a non-retryable *integration.Error
// (pageBudgetExceeded below), never a quietly-truncated result: a provider
// bug (rowCount that never shrinks, a rowCount far larger than this
// credential could plausibly have) must be visible, not swallowed.
const maxPageBudget = 100

// MaxWindowMinute is R42/Provider-defaults' isolar row: "1 d minute
// series". DeviceMinuteSeries and PlantMinuteSeries refuse a request whose
// [from, to) exceeds this BEFORE any call, and internally split an
// accepted window into contiguous <=3h sub-windows (legacyMaxSubWindow in
// series.go) — the legacy API's own documented per-request cap
// (isolarClient.ts:593,621: "API supports max 3-hour time intervals per
// request").
const MaxWindowMinute = 24 * time.Hour

// MaxWindowDay is Provider-defaults' isolar row: "31 d day series";
// PlantDailySeries enforces it.
const MaxWindowDay = 31 * 24 * time.Hour

// Minute-series points (06 §6): an inverter reports 1 yield today (Wh) and
// 24 active power (W); a plant reports 83022 daily yield (Wh) and 83025
// active power (W). F2 asked plants for the inverter points (F9 finding).
const (
	devicePoints = "1,24"
	plantPoints  = "83022,83025"
)

// Endpoints keys this package never hard-codes a value for (R40): every
// call resolves its path from creds.Endpoints, and the gateway origin,
// OAuth authorize origin and cloud id come from creds.Endpoints too — all
// four are integration_definitions.json's isolar row, verbatim
// (internal/seed/data/integration_definitions.json). No endpoint table, no
// per-region constant map, lives in this package.
const (
	endpointGateway         = "gateway"
	endpointAuthorizeOrigin = "authorize_origin"
	endpointCloudID         = "cloud_id"
)

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

// callOptions is one call's shape: which endpoint (an
// integration_definitions.json key — R40), whether it carries the bearer
// access-token header (every call except token/refreshToken — 06 §6:
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

// call is the ONE place every iSolarCloud request is built and sent (R40):
// it builds the URL as creds.Endpoints["gateway"] + creds.Endpoints[o.op]
// (the relative path integration_definitions.json's isolar row carries for
// this operation — never a hard-coded endpoint table), attaches appkey to
// the body and x-access-key/Authorization to the headers, and decodes the
// {result_code, result_msg, result_data} envelope. It returns the raw
// result_data for the caller to unmarshal into its own wire type.
func (c *Client) call(ctx context.Context, creds integration.Credentials, o callOptions) (json.RawMessage, error) {
	gateway, ok := creds.Endpoints[endpointGateway]
	if !ok || gateway == "" {
		return nil, c.configError(endpointGateway)
	}
	path, ok := creds.Endpoints[o.op]
	if !ok || path == "" {
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
		Template:    gateway + path,
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
// gateway/endpoint template, a missing app_key/secret_key/access_token/
// cloud_id, or an empty required identifier (psID/psKeys — adapter-
// patterns.md item 12). R48/I5: this used to report ErrAuth (round 1's
// documented stop-gap, before integration.ErrConfig existed) — non-
// retryable like a rejected password, but NOT an authentication failure;
// F3's credential-health logic must never treat it as one. Use ErrConfig.
func (c *Client) configError(op string) error {
	return &integration.Error{Kind: integration.ErrConfig, Provider: integration.ProviderISolar, Op: op}
}

// pageBudgetExceeded is returned when a non-cursor list (plants, devices,
// faults) reports, via rowCount, more rows than maxPageBudget pages can
// cover (R44) — a provider-shape anomaly distinct from configError's
// caller-input/config problems, so it gets ErrMalformedPayload rather than
// reusing configError's ErrConfig. Both are non-retryable.
func (c *Client) pageBudgetExceeded(op string) error {
	return &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderISolar, Op: op}
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
// classifyResult. I4: on a success code, result_data itself missing or
// JSON null is ALSO ErrMalformedPayload — never silently decoded into a
// zero-value struct/empty map downstream (json.Unmarshal(null, &x) is a
// documented no-op in encoding/json, so without this check a null
// result_data on "result_code":"1" would produce an empty-but-valid Token/
// Plant/Device list instead of surfacing the shape violation).
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
	if isNullOrEmpty(env.ResultData) {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderISolar, Op: op, HTTPStatus: resp.Status}
	}
	return env.ResultData, nil
}

// isNullOrEmpty reports whether raw is absent (nil/zero-length, the key
// was missing entirely) or the literal JSON token "null".
func isNullOrEmpty(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || string(trimmed) == "null"
}

// formatIsolarTime renders t (any instant) as Europe/Istanbul local
// yyyyMMddHHmmss — the inverse of parsing with isolarTimestampLayout, and
// the format every from/to request parameter in this package uses
// (isolarClient.ts:594,622).
func formatIsolarTime(t time.Time) string {
	return t.In(normalize.Istanbul).Format(isolarTimestampLayout)
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

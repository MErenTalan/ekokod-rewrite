// Package epias is the EPİAŞ transparency-platform client: day-ahead PTF
// (MCP) and YEKDEM unit-cost, both read-only. It is a pure client per 06
// §1 rule 2 — it never touches the store; internal/marketdata (Task 12's
// other half) is what persists what this package fetches.
package epias

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

// Defaults (task-12 brief / Provider defaults table).
const (
	defaultCASURL    = "https://giris.epias.com.tr/cas/v1/tickets"
	defaultBaseURL   = "https://seffaflik.epias.com.tr/electricity-service"
	defaultTicketTTL = 90 * time.Minute // R24

	// epiasKey is the ONE global limiter/serialise key EPİAŞ uses (Provider
	// defaults table: "global lock `epias`" — unlike every other provider,
	// which serialises per company, EPİAŞ credentials are a single
	// platform-wide config value, so there is exactly one caller to
	// serialise against). CAS and the data endpoints share it: the
	// platform is limited to one EPİAŞ request per epiasEvery, full stop,
	// regardless of which endpoint it is.
	epiasKey = "epias"

	epiasEvery      = 3 * time.Second
	epiasBurst      = 1
	epiasCASTimeout = 15 * time.Second // Provider defaults table: "20s (CAS 15s)"
	epiasTimeout    = 20 * time.Second

	// epiasDefaultRateLimitWait is the legacy default wait for a 429 that
	// carries no Retry-After header (Provider defaults table / Common:
	// "EPİAŞ 429 without Retry-After waits 65s").
	epiasDefaultRateLimitWait = 65 * time.Second

	ptfMaxWindow = 30 * 24 * time.Hour // R36/S7: 30-day PTF chunking

	// yekdemMaxWindow is normalize.Chunk's max argument for YEKDEM: "3
	// months" per the Provider defaults table, expressed as Chunk requires
	// — a whole-calendar-day count (floor(max/24h)), not a calendar-month
	// count, since Chunk (R36) does not do calendar-month arithmetic. 90
	// days is the standard approximation of "3 months" and errs, if at
	// all, toward fetching a few extra already-published days rather than
	// missing any.
	yekdemMaxWindow = 90 * 24 * time.Hour
)

// Options configures a Client. Every field has a default except Username
// and Password, which New refuses to leave empty.
type Options struct {
	CASURL    string // default defaultCASURL
	BaseURL   string // default defaultBaseURL
	Username  string
	Password  integration.Secret
	TicketTTL time.Duration // default 90m (R24)
	Clock     clock.Clock
	// Locker serialises TGT acquisition across processes under the fixed
	// key "epias:tgt" (see ticket.go's getTicket) — distinct from the
	// httpx-level "epias" SerializeKey this client's own requests carry
	// (that one comes from the Pool passed to New, not from here). nil is
	// valid only in a single-process unit test: without it, getTicket
	// falls back to its own in-process mutex only, which is exactly what
	// every one of this package's tests below actually exercises unless a
	// Locker is deliberately supplied.
	Locker httpx.Locker
}

// Client is a fully-configured EPİAŞ client: one httpx.Client per endpoint
// family (CAS vs. data), plus the in-process TGT cache ticket.go manages.
type Client struct {
	opts       Options
	casClient  *httpx.Client
	dataClient *httpx.Client

	mu       sync.Mutex
	ticket   string
	ticketAt time.Time

	// yekdemDropped is the count of YEKDEM rows dropped as unparseable in
	// the most recent YekdemUnitCost call — M2 (task-12-fix1-findings.md,
	// fix round 1). See YekdemDropped's and parseUnitCostResponse's doc
	// comments for why this exists instead of a Warning return.
	yekdemDropped atomic.Int32
}

// YekdemDropped returns the number of YEKDEM rows silently dropped as
// unparseable during the most recent YekdemUnitCost call (0 if none, or if
// YekdemUnitCost has not been called yet).
//
// This is an OPTIONAL capability, not part of the marketdata.PriceSource
// interface: YekdemUnitCost's own return signature is pinned by the
// task-12 brief as `([]model.YekdemMonthly, error)`, with no
// []integration.Warning slot (unlike HourlyPTF's), so a bad YEKDEM row
// cannot be reported that way without breaking that pinned contract.
// marketdata.Syncer type-asserts its PriceSource for this method and,
// when present, folds the count into the sync run's `detail` JSON — so a
// systematically malformed YEKDEM feed is still visible to an operator
// reading job_runs, per M2's "surface the drop count in the sync run's
// detail" resolution.
func (c *Client) YekdemDropped() int32 { return c.yekdemDropped.Load() }

// New builds a Client. An empty Username or Password is refused before any
// network call, naming the environment variable the platform config reads
// it from (never the value — there is no value to name, it is empty) —
// EKOKOD_EPIAS_USERNAME / EKOKOD_EPIAS_PASSWORD (.env.example).
//
// Kind chosen for this configuration error: integration.ErrAuth. There is
// no dedicated configuration-error Kind yet (a planned follow-up per the
// task brief); ErrAuth is the closest existing sentinel — "this client
// cannot authenticate" is true whether the credential is wrong or simply
// absent — and, unlike ErrMalformedPayload, it is not misleading about
// what happened.
func New(pool *httpx.Pool, o Options) (*Client, error) {
	if o.CASURL == "" {
		o.CASURL = defaultCASURL
	}
	if o.BaseURL == "" {
		o.BaseURL = defaultBaseURL
	}
	if o.TicketTTL <= 0 {
		o.TicketTTL = defaultTicketTTL
	}
	if o.Clock == nil {
		o.Clock = clock.System()
	}
	if o.Username == "" {
		return nil, &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderEPIAS, Op: "config: EKOKOD_EPIAS_USERNAME is required"}
	}
	if o.Password.IsZero() {
		return nil, &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderEPIAS, Op: "config: EKOKOD_EPIAS_PASSWORD is required"}
	}

	cfg := httpx.ClientConfig{
		Provider:             integration.ProviderEPIAS,
		LimiterKey:           epiasKey,
		Every:                epiasEvery,
		Burst:                epiasBurst,
		SerializeKey:         epiasKey,
		DefaultRateLimitWait: epiasDefaultRateLimitWait,
	}
	casCfg := cfg
	casCfg.RequestTimeout = epiasCASTimeout
	dataCfg := cfg
	dataCfg.RequestTimeout = epiasTimeout

	return &Client{
		opts:       o,
		casClient:  pool.Client(casCfg),
		dataClient: pool.Client(dataCfg),
	}, nil
}

// formatEpiasDate renders t as the Istanbul-local "YYYY-MM-DDTHH:mm:ss+ZZ:ZZ"
// timestamp EPİAŞ's data endpoints take. The offset is computed from t's
// own Istanbul-local zone at that instant, not hardcoded to +03:00, so a
// pre-2016 winter date (Turkey observed DST until then) renders +02:00
// correctly — R6.
func formatEpiasDate(t time.Time) string {
	return t.In(normalize.Istanbul).Format("2006-01-02T15:04:05-07:00")
}

// doData obtains a ticket, sends one data request via dataClient, and — on
// exactly one ErrAuth response — invalidates the cached ticket and retries
// exactly once with a freshly obtained one (task-12 brief: "A data call
// returning 401 invalidates the cache and retries once with a new
// ticket").
func (c *Client) doData(ctx context.Context, build func(ticket string) httpx.Request) (httpx.Response, error) {
	ticket, err := c.getTicket(ctx)
	if err != nil {
		return httpx.Response{}, err
	}
	resp, err := c.dataClient.Do(ctx, build(ticket))
	if err == nil {
		return resp, nil
	}
	// *integration.Error.Unwrap exposes Kind directly, so errors.Is against
	// the sentinel is exact — no need to type-assert *integration.Error.
	if !errors.Is(err, integration.ErrAuth) {
		return httpx.Response{}, err
	}

	c.invalidateTicket()
	ticket, err = c.getTicket(ctx)
	if err != nil {
		return httpx.Response{}, err
	}
	return c.dataClient.Do(ctx, build(ticket))
}

func (c *Client) mcpRequest(ticket string, from, to time.Time) httpx.Request {
	body, _ := json.Marshal(mcpRequestBody{StartDate: formatEpiasDate(from), EndDate: formatEpiasDate(to)})
	return httpx.Request{
		Op:          "mcp",
		Method:      http.MethodPost,
		Template:    c.opts.BaseURL + "/v1/markets/dam/data/mcp",
		Header:      http.Header{"TGT": []string{ticket}},
		Body:        body,
		ContentType: "application/json",
	}
}

func (c *Client) unitCostRequest(ticket string, from, to time.Time) httpx.Request {
	body, _ := json.Marshal(unitCostRequestBody{StartDate: formatEpiasDate(from), EndDate: formatEpiasDate(to)})
	return httpx.Request{
		Op:          "unit-cost",
		Method:      http.MethodPost,
		Template:    c.opts.BaseURL + "/v1/renewables/data/unit-cost",
		Header:      http.Header{"TGT": []string{ticket}},
		Body:        body,
		ContentType: "application/json",
	}
}

// HourlyPTF returns one model.MarketPrice per published hour in [from, to),
// UTC, TL/MWh. Requests are chunked to 30 days via normalize.Chunk (R20
// pagination; S7). Missing hours are NOT filled — an hour EPİAŞ has not
// published simply does not appear in the result.
func (c *Client) HourlyPTF(ctx context.Context, from, to time.Time) ([]model.MarketPrice, []integration.Warning, error) {
	var prices []model.MarketPrice
	var warnings []integration.Warning

	for _, w := range normalize.Chunk(from, to, ptfMaxWindow) {
		resp, err := c.doData(ctx, func(ticket string) httpx.Request {
			return c.mcpRequest(ticket, w.From, w.To)
		})
		if err != nil {
			return nil, nil, err
		}
		rows, warns, err := parseMCPResponse(resp.Body)
		if err != nil {
			return nil, nil, err
		}
		fetchedAt := c.opts.Clock.Now()
		for _, row := range rows {
			prices = append(prices, model.MarketPrice{Ts: row.Ts, PTF: row.Price, FetchedAt: fetchedAt})
		}
		warnings = append(warnings, warns...)
	}
	return prices, warnings, nil
}

// YekdemUnitCost returns one model.YekdemMonthly per published (year,
// month) intersecting [from, to). Requests are chunked to 3 months via
// normalize.Chunk (R36/S7); a (year, month) reported by more than one
// chunk (possible when a chunk boundary falls mid-month, since EPİAŞ
// returns whole months intersecting the queried window) is deduplicated
// here, keeping the last chunk's value — the two are expected to agree,
// since they describe the same published month.
func (c *Client) YekdemUnitCost(ctx context.Context, from, to time.Time) ([]model.YekdemMonthly, error) {
	type key struct{ year, month int16 }
	seen := make(map[key]model.YekdemMonthly)
	var order []key
	var dropped int32

	for _, w := range normalize.Chunk(from, to, yekdemMaxWindow) {
		resp, err := c.doData(ctx, func(ticket string) httpx.Request {
			return c.unitCostRequest(ticket, w.From, w.To)
		})
		if err != nil {
			return nil, err
		}
		rows, drop, err := parseUnitCostResponse(resp.Body)
		if err != nil {
			return nil, err
		}
		dropped += int32(drop)
		fetchedAt := c.opts.Clock.Now()
		for _, row := range rows {
			k := key{row.Year, row.Month}
			if _, ok := seen[k]; !ok {
				order = append(order, k)
			}
			seen[k] = model.YekdemMonthly{Year: row.Year, Month: row.Month, Value: row.Value, FetchedAt: fetchedAt}
		}
	}
	// Stored even on a zero-drop call, so a stale nonzero count from an
	// earlier call never leaks into a later, clean one (M2).
	c.yekdemDropped.Store(dropped)

	out := make([]model.YekdemMonthly, 0, len(order))
	for _, k := range order {
		out = append(out, seen[k])
	}
	return out, nil
}

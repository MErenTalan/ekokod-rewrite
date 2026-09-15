package pm5340

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

const (
	opFetchReadings = "fetch_readings"
	opVerify        = "verify"

	fetchLimit  = "500"
	verifyLimit = "1"

	defaultPageBudget = 10

	maxWindow = 7 * 24 * time.Hour

	rateEvery      = time.Second / 5
	rateBurst      = 5
	requestTimeout = 30 * time.Second
)

// Options configures a Source, per the shared adapter template.
type Options struct {
	Clock      clock.Clock
	PageBudget int
}

// Source is the PM5340 integration.Adapter.
type Source struct {
	pool       *httpx.Pool
	clock      clock.Clock
	pageBudget int
}

var _ integration.Adapter = (*Source)(nil)

// New builds a Source drawing its HTTP client from pool, per call, since
// each call's rate-limiter and body-cap policy is keyed off that call's own
// Credentials.CompanyID.
func New(pool *httpx.Pool, o Options) *Source {
	c := o.Clock
	if c == nil {
		c = clock.System()
	}
	pb := o.PageBudget
	if pb <= 0 {
		pb = defaultPageBudget
	}
	return &Source{pool: pool, clock: c, pageBudget: pb}
}

// Provider reports ProviderPM5340.
func (s *Source) Provider() integration.Provider { return integration.ProviderPM5340 }

// Kinds reports the one reading kind PM5340 ever supplies (Provider
// defaults table: "load_profile").
func (s *Source) Kinds(_ integration.Credentials) []model.ReadingKind {
	return []model.ReadingKind{model.ReadingKindLoadProfile}
}

// MaxWindow reports 7 days (Provider defaults table) for every kind: PM5340
// only ever fetches load_profile, but Planner.MaxWindow is queried
// per-kind generically.
func (s *Source) MaxWindow(_ model.ReadingKind) time.Duration { return maxWindow }

func (s *Source) client(creds integration.Credentials) *httpx.Client {
	return s.pool.Client(httpx.ClientConfig{
		Provider:       integration.ProviderPM5340,
		LimiterKey:     "pm5340:" + creds.CompanyID.String(),
		Every:          rateEvery,
		Burst:          rateBurst,
		RequestTimeout: requestTimeout,
	})
}

// configError reports a deliberately-classified configuration problem: a
// missing/blank PM5340 base URL, or (wrapDo) a malformed request template.
// R48/I5: this used to report ErrAuth (round 1's documented stop-gap,
// before integration.ErrConfig existed) — non-retryable like a rejected
// password, but NOT an authentication failure; F3's credential-health
// logic must never treat it as one. Use ErrConfig.
func configError(op string) error {
	return &integration.Error{Kind: integration.ErrConfig, Provider: integration.ProviderPM5340, Op: op}
}

func wrapDo(err error, op string) error {
	if err == nil {
		return nil
	}
	var ierr *integration.Error
	if errors.As(err, &ierr) {
		return err
	}
	return configError(op)
}

// Verify probes verifyTemplate (06 §5 / task-9 brief S5): the ONLY param is
// {limit}="1", and — unlike FetchReadings' template — there is no
// &sort=asc, &start=, &end= or &cursor= at all, so a Verify call can never
// be confused with a real fetch by an operator reading recorded requests
// (TestPM5340VerifyUsesItsOwnMinimalTemplate).
func (s *Source) Verify(ctx context.Context, creds integration.Credentials) error {
	base, ok := baseURL(creds)
	if !ok {
		return configError(opVerify)
	}

	_, err := s.client(creds).Do(ctx, httpx.Request{
		Op:       opVerify,
		Method:   http.MethodGet,
		Template: base + "/api/v1/readings?limit={limit}",
		Params:   map[string]httpx.Param{"limit": {Value: verifyLimit}},
	})
	return wrapDo(err, opVerify)
}

// DiscoverMeteringPoints always returns (nil, nil): PM5340 has no discovery
// API (spec silent — task-9 brief's ruling). The credential service (Task
// 14) creates the single analyzer from the configured installation number
// instead of discovering it here.
func (s *Source) DiscoverMeteringPoints(_ context.Context, _ integration.Credentials) ([]integration.MeteringPoint, error) {
	return nil, nil
}

// FetchReadings follows fetchTemplate (06 §5 / task-9 brief S5) — the
// four-parameter template {limit, sort=asc (literal), start, end}, plus
// &cursor={cursor} once a cursorNext has been returned — across as many
// pages as Options.PageBudget allows, mapping every row through mapRow and
// dropping anything outside [req.From, req.To). If the page budget is
// exhausted while the provider's own hasMore is still true, NextCursor is
// the last fully covered reading's Ts (R34); otherwise NextCursor is nil
// (R4: the window is exhausted).
func (s *Source) FetchReadings(ctx context.Context, creds integration.Credentials, req integration.FetchRequest) (integration.FetchResult, error) {
	base, ok := baseURL(creds)
	if !ok {
		return integration.FetchResult{}, configError(opFetchReadings)
	}
	if req.Multiplier.IsZero() {
		return integration.FetchResult{}, configError(opFetchReadings)
	}

	client := s.client(creds)
	start := req.From.UTC().Format(time.RFC3339)
	end := req.To.UTC().Format(time.RFC3339)

	var (
		readings []model.MeterReading
		warnings []integration.Warning
		seen     = make(map[time.Time]model.MeterReading)
		cursor   string
	)

	for page := 1; ; page++ {
		tmpl := base + "/api/v1/readings?limit={limit}&sort=asc&start={start}&end={end}"
		params := map[string]httpx.Param{
			"limit": {Value: fetchLimit},
			"start": {Value: start},
			"end":   {Value: end},
		}
		if cursor != "" {
			tmpl += "&cursor={cursor}"
			params["cursor"] = httpx.Param{Value: cursor}
		}

		resp, err := client.Do(ctx, httpx.Request{Op: opFetchReadings, Method: http.MethodGet, Template: tmpl, Params: params})
		if err != nil {
			return integration.FetchResult{}, wrapDo(err, opFetchReadings)
		}

		env, ok := decodeEnvelope(resp.Body)
		if !ok {
			return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderPM5340, Op: opFetchReadings, HTTPStatus: resp.Status}
		}

		for i, raw := range *env.Items {
			row, decodeErr := decodeRow(raw)
			if decodeErr != nil {
				warnings = append(warnings, unparseableRowWarning("row", i))
				continue
			}
			reading, field, mapped := mapRow(row, raw, req)
			if !mapped {
				warnings = append(warnings, unparseableRowWarning(field, i))
				continue
			}
			if reading.Ts.Before(req.From) || !reading.Ts.Before(req.To) {
				continue
			}
			reading.IngestedAt = s.clock.Now().UTC()

			if prior, dup := seen[reading.Ts]; dup && !registersEqual(prior, reading) {
				warnings = append(warnings, duplicateTimestampWarning(i))
			}
			seen[reading.Ts] = reading

			if reading.IntervalGenerationKwh == nil {
				warnings = append(warnings, generationNilWarning(i))
			}

			readings = append(readings, reading)
		}

		hasMore := env.HasMore != nil && *env.HasMore
		next := env.CursorNext
		if !hasMore || next == nil || *next == "" {
			return integration.FetchResult{Readings: readings, Warnings: warnings}, nil
		}
		if page >= s.pageBudget {
			return integration.FetchResult{Readings: readings, Warnings: warnings, NextCursor: lastCoveredTs(readings)}, nil
		}
		cursor = *next
	}
}

func lastCoveredTs(readings []model.MeterReading) *time.Time {
	if len(readings) == 0 {
		return nil
	}
	t := readings[len(readings)-1].Ts
	return &t
}

func baseURL(creds integration.Credentials) (string, bool) {
	trimmed := strings.TrimSpace(creds.BaseURL)
	if trimmed == "" {
		return "", false
	}
	return strings.TrimSuffix(trimmed, "/"), true
}

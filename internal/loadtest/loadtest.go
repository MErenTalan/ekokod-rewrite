// Package loadtest drives the API with concurrent signed-in users and reports
// latency per endpoint against a p95 budget (F15b R467).
package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Options are the run's parameters.
type Options struct {
	Base            string // e.g. http://127.0.0.1:8080
	Email, Password string
	Users           int
	Duration        time.Duration
	P95Budget       time.Duration
	MaxErrorRatio   float64 // default 0.01
	Now             time.Time
}

// EndpointStats is one endpoint's latency summary.
type EndpointStats struct {
	Name          string
	Count, Errors int
	P50, P95, P99 time.Duration
}

// MarshalJSON prints durations as milliseconds.
func (s EndpointStats) MarshalJSON() ([]byte, error) {
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	return json.Marshal(map[string]any{"name": s.Name, "count": s.Count, "errors": s.Errors, "p50_ms": ms(s.P50), "p95_ms": ms(s.P95), "p99_ms": ms(s.P99)})
}

// Result is the run's outcome.
type Result struct {
	Requests  int             `json:"requests"`
	PerSecond float64         `json:"requests_per_second"`
	Endpoints []EndpointStats `json:"endpoints"`
	Pass      bool            `json:"pass"`
	Failures  []string        `json:"failures"`
}

type endpoint struct {
	name   string
	weight int
	path   func(r *rand.Rand) string
}

type sample struct {
	name string
	took time.Duration
	ok   bool
}

type client struct {
	base   string
	http   *http.Client
	cookie string
}

func (c *client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ekokod-loadtest")
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	return c.http.Do(req)
}

// login keeps the session cookies by hand: they are Secure, and a cookie jar
// would not send them over the plain HTTP a local run uses.
func (c *client) login(ctx context.Context, email, password string) error {
	res, err := c.do(ctx, http.MethodPost, "/api/v1/auth/login", map[string]any{"email": email, "password": password})
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("login: %d %s", res.StatusCode, b)
	}
	var parts []string
	for _, sc := range res.Header.Values("Set-Cookie") {
		pair, _, _ := strings.Cut(sc, ";")
		parts = append(parts, pair)
	}
	c.cookie = strings.Join(parts, "; ")
	return nil
}

func (c *client) ids(ctx context.Context, path string) ([]string, error) {
	res, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, it.ID)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: nothing to load-test (seed the scale dataset)", path)
	}
	return out, nil
}

// Run is R467.
func Run(ctx context.Context, o Options, progress io.Writer) (Result, error) {
	if o.MaxErrorRatio == 0 {
		o.MaxErrorRatio = 0.01
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	httpClient := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	first := &client{base: strings.TrimRight(o.Base, "/"), http: httpClient}
	if err := first.login(ctx, o.Email, o.Password); err != nil {
		return Result{}, err
	}
	buildings, err := first.ids(ctx, "/api/v1/buildings?limit=200")
	if err != nil {
		return Result{}, err
	}
	analyzers, err := first.ids(ctx, "/api/v1/analyzers?limit=200")
	if err != nil {
		return Result{}, err
	}
	day := o.Now.AddDate(0, 0, -2).Format(time.DateOnly)
	month := o.Now.AddDate(0, 0, -31).Format(time.DateOnly)
	year := o.Now.AddDate(-1, 0, 0).Format(time.DateOnly)
	today := o.Now.Format(time.DateOnly)
	pick := func(r *rand.Rand, ids []string) string { return url.QueryEscape(ids[r.IntN(len(ids))]) }
	mix := []endpoint{
		{"consumption hourly (building, day)", 4, func(r *rand.Rand) string {
			return fmt.Sprintf("/api/v1/consumption?building_id=%s&granularity=hourly&from=%s&to=%s", pick(r, buildings), day, today)
		}},
		{"consumption daily (building, month)", 3, func(r *rand.Rand) string {
			return fmt.Sprintf("/api/v1/consumption?building_id=%s&granularity=daily&from=%s&to=%s", pick(r, buildings), month, today)
		}},
		{"consumption monthly (analyzer, year)", 2, func(r *rand.Rand) string {
			return fmt.Sprintf("/api/v1/consumption?analyzer_id=%s&granularity=monthly&from=%s&to=%s", pick(r, analyzers), year, today)
		}},
		{"buildings", 1, func(*rand.Rand) string { return "/api/v1/buildings" }},
		{"analyzers", 1, func(*rand.Rand) string { return "/api/v1/analyzers" }},
		{"bills", 1, func(*rand.Rand) string { return "/api/v1/bills" }},
		{"alarms", 1, func(*rand.Rand) string { return "/api/v1/alarms" }},
		{"reports", 1, func(*rand.Rand) string { return "/api/v1/reports" }},
	}
	var wheel []endpoint
	for _, e := range mix {
		for range e.weight {
			wheel = append(wheel, e)
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, o.Duration)
	defer cancel()
	samples := make(chan sample, 1024)
	var wg sync.WaitGroup
	var loginErr error
	var loginOnce sync.Once
	for u := range max(o.Users, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := &client{base: first.base, http: httpClient}
			if err := c.login(runCtx, o.Email, o.Password); err != nil {
				loginOnce.Do(func() { loginErr = err })
				return
			}
			r := rand.New(rand.NewPCG(uint64(u)+1, 42)) //nolint:gosec // load mix, not security
			for runCtx.Err() == nil {
				e := wheel[r.IntN(len(wheel))]
				started := time.Now()
				res, err := c.do(runCtx, http.MethodGet, e.path(r), nil)
				took := time.Since(started)
				if runCtx.Err() != nil {
					if res != nil {
						_ = res.Body.Close()
					}
					return
				}
				ok := err == nil && res.StatusCode < 400
				if res != nil {
					_, _ = io.Copy(io.Discard, res.Body)
					_ = res.Body.Close()
				}
				samples <- sample{e.name, took, ok}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(samples)
	}()
	byName := map[string][]time.Duration{}
	errs := map[string]int{}
	total := 0
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for done := false; !done; {
		select {
		case s, ok := <-samples:
			if !ok {
				done = true
				continue
			}
			total++
			byName[s.name] = append(byName[s.name], s.took)
			if !s.ok {
				errs[s.name]++
			}
		case <-tick.C:
			_, _ = fmt.Fprintf(progress, "loadtest: %d requests so far\n", total)
		}
	}
	if loginErr != nil {
		return Result{}, loginErr
	}
	return summarise(byName, errs, total, o), nil
}

func quantile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(q*float64(len(sorted)-1) + 0.5)
	return sorted[i]
}

func summarise(byName map[string][]time.Duration, errs map[string]int, total int, o Options) Result {
	res := Result{Requests: total, PerSecond: float64(total) / o.Duration.Seconds(), Pass: true, Failures: []string{}}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		d := byName[n]
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		s := EndpointStats{Name: n, Count: len(d), Errors: errs[n], P50: quantile(d, 0.5), P95: quantile(d, 0.95), P99: quantile(d, 0.99)}
		res.Endpoints = append(res.Endpoints, s)
		if o.P95Budget > 0 && s.P95 > o.P95Budget {
			res.Pass = false
			res.Failures = append(res.Failures, fmt.Sprintf("%s: p95 %s over the %s budget", n, s.P95.Round(time.Millisecond), o.P95Budget))
		}
		if float64(s.Errors) > o.MaxErrorRatio*float64(s.Count) {
			res.Pass = false
			res.Failures = append(res.Failures, fmt.Sprintf("%s: %d of %d requests failed", n, s.Errors, s.Count))
		}
	}
	return res
}

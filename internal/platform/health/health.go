// Package health runs named readiness checks concurrently and aggregates them.
package health

import (
	"context"
	"sync"
	"time"
)

// Check is one named readiness probe.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// Result is the outcome of a single check.
type Result struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// Report aggregates every check.
type Report struct {
	Status string   `json:"status"`
	Checks []Result `json:"checks"`
}

// Healthy reports whether every check passed.
func (r Report) Healthy() bool { return r.Status == "ok" }

// Run executes the checks concurrently, each bounded by timeout, and returns
// results in the order the checks were given.
func Run(ctx context.Context, timeout time.Duration, checks ...Check) Report {
	results := make([]Result, len(checks))

	var wg sync.WaitGroup
	for i, check := range checks {
		wg.Add(1)
		go func(i int, check Check) {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			started := time.Now()
			err := check.Fn(checkCtx)
			res := Result{Name: check.Name, Status: "ok", DurationMS: time.Since(started).Milliseconds()}
			if err != nil {
				res.Status = "failed"
				res.Error = err.Error()
			}
			results[i] = res
		}(i, check)
	}
	wg.Wait()

	report := Report{Status: "ok", Checks: results}
	for _, r := range results {
		if r.Status != "ok" {
			report.Status = "degraded"
			break
		}
	}
	return report
}

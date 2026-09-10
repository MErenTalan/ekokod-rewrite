// Package health runs named readiness checks concurrently and aggregates them.
package health

import (
	"context"
	"fmt"
	"time"
)

// Check is one named readiness probe. Fn should observe ctx and return
// promptly when it is done: Run bounds every check by a timeout, but a check
// that ignores ctx is only reported as failed once the timeout elapses — its
// goroutine keeps running in the background until Fn itself returns.
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
// results in the order the checks were given. Run itself never blocks past
// (approximately) timeout, even if a check ignores its context and keeps
// running in the background: each check gets its own buffered result
// channel, so an abandoned goroutine's eventual send never blocks and never
// touches the shared results slice, which only Run's own goroutine writes.
func Run(ctx context.Context, timeout time.Duration, checks ...Check) Report {
	results := make([]Result, len(checks))
	deadline := time.Now().Add(timeout)

	channels := make([]chan Result, len(checks))
	for i, check := range checks {
		ch := make(chan Result, 1)
		channels[i] = ch
		go func(check Check, ch chan<- Result) {
			checkCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			started := time.Now()
			err := check.Fn(checkCtx)
			res := Result{Name: check.Name, Status: "ok", DurationMS: time.Since(started).Milliseconds()}
			if err != nil {
				res.Status = "failed"
				res.Error = err.Error()
			}
			ch <- res
		}(check, ch)
	}

	for i, check := range checks {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}
		timer := time.NewTimer(remaining)
		select {
		case res := <-channels[i]:
			timer.Stop()
			results[i] = res
		case <-timer.C:
			results[i] = Result{
				Name:       check.Name,
				Status:     "failed",
				Error:      fmt.Sprintf("check did not return within its %s timeout", timeout),
				DurationMS: timeout.Milliseconds(),
			}
		}
	}

	report := Report{Status: "ok", Checks: results}
	for _, r := range results {
		if r.Status != "ok" {
			report.Status = "degraded"
			break
		}
	}
	return report
}

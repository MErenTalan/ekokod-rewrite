package cli

// consecutiveHealthCheckFailureLimit is how many consecutive broker health
// check failures the worker tolerates before treating the broker as dead
// and exiting non-zero. At job.HealthCheckInterval (15s) this is ~1 minute
// — long enough to ride out a brief network blip without a supervisor
// restarting the pod, short enough that a genuinely dead Redis is reported
// promptly. This is a named constant, deliberately not a new environment
// variable: a config field would drag in .env.example, cfg.Resolved() rows
// and the env-reference documentation, which is out of this task's scope.
const consecutiveHealthCheckFailureLimit = 4

// newHealthTracker returns a callback suitable for job.NewServer's
// onHealthCheck parameter. It counts consecutive failures — any nil error
// resets the count to zero — and calls onThreshold the first time the
// count reaches limit. It does not call onThreshold again for the same
// unbroken run of failures (guarded by "tripped"), but a later success
// followed by a fresh run of failures reaching the limit again does call
// it again. onHealthCheck is invoked by asynq's own healthchecker
// goroutine, which calls it serially and never concurrently with itself
// (asynq@v0.26.0 healthcheck.go's start() loop has exactly one goroutine
// per server), so the counters here need no synchronisation.
func newHealthTracker(limit int, onThreshold func()) func(error) {
	var consecutive int
	var tripped bool
	return func(err error) {
		if err == nil {
			consecutive = 0
			tripped = false
			return
		}
		consecutive++
		if consecutive >= limit && !tripped {
			tripped = true
			onThreshold()
		}
	}
}

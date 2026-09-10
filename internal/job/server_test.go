package job_test

import (
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/stretchr/testify/require"
)

// TestShutdownTimeoutIsBoundedByMaxShutdownGrace pins Important-2 from the
// task 9 review: EKOKOD_JOB_TIMEOUT (cfg.Worker.Timeout) bounds how long a
// single task may run, not how long the process may take to exit on
// shutdown. Passing it straight through as asynq's ShutdownTimeout (as the
// original implementation did) means a worker configured with the 30
// minute default can block a SIGTERM drain for 30 minutes — far past
// Kubernetes' default 30s terminationGracePeriodSeconds or compose's 10s
// stop_grace_period, both of which SIGKILL the process anyway, truncating
// the drain ungracefully regardless. ShutdownTimeout must return the
// smaller of the two so the drain window itself never exceeds
// job.MaxShutdownGrace. This is a pure function so it is provable without a
// broker: internal/job's own integration test cannot run without Docker,
// which is down for this phase.
func TestShutdownTimeoutIsBoundedByMaxShutdownGrace(t *testing.T) {
	require.Equal(t, 30*time.Second, job.MaxShutdownGrace,
		"the bound itself must be exactly 30s to fit inside a 30s k8s terminationGracePeriodSeconds")

	require.Equal(t, 10*time.Second, job.ShutdownTimeout(10*time.Second),
		"a task timeout shorter than the grace bound must pass through unchanged")
	require.Equal(t, 30*time.Second, job.ShutdownTimeout(30*time.Minute),
		"the default 30 minute task timeout must be clamped down to the 30s grace bound")
	require.Equal(t, 30*time.Second, job.ShutdownTimeout(30*time.Second),
		"a task timeout exactly at the bound must not be altered")
	require.Equal(t, 1*time.Second, job.ShutdownTimeout(1*time.Second),
		"a task timeout shorter than the grace bound must never be extended")
}

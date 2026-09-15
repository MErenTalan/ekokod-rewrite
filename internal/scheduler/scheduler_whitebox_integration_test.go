//go:build integration

package scheduler

// White-box (package scheduler) integration test for Carried-forward
// defect 3 (F1 plan). It needs a real Redis to observe the server's own
// connected_clients count, and needs unexported access to runTerm to feed
// it a deliberately invalid cron entry — config.Load's own cronExpr
// validator (internal/platform/config/load.go) would reject a malformed
// cron before it ever reached Scheduler.entries(), so this test bypasses
// entries() entirely and calls runTerm directly with a hand-built Entry.

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// connectedClients reads Redis's own INFO clients: connected_clients,
// the server-side ground truth for "how many client connections are open
// right now" — independent of, and unable to be fooled by, whichever
// *goredis.Client object this process happens to hold a reference to.
func connectedClients(t *testing.T, c *goredis.Client) int {
	t.Helper()
	info, err := c.Info(context.Background(), "clients").Result()
	require.NoError(t, err)
	for _, line := range strings.Split(info, "\r\n") {
		if n, ok := strings.CutPrefix(line, "connected_clients:"); ok {
			v, err := strconv.Atoi(strings.TrimSpace(n))
			require.NoError(t, err)
			return v
		}
	}
	t.Fatal("connected_clients not found in INFO clients output")
	return 0
}

// TestSchedulerReleasesRedisClientOnRegisterFailure pins Carried-forward
// defect 3: a bad cron entry failing asynqScheduler.Register must not leak
// the goredis client runTerm opened for that term. Before the fix,
// scheduler.go built its *asynq.Scheduler with the opt-based
// asynq.NewScheduler (which opens and owns its own client internally, one
// per call) and NEVER closed anything on the Register-failure return path,
// so a leader stuck retrying against one permanently-bad cron entry would
// open one new pooled Redis client connection per retry, forever.
func TestSchedulerReleasesRedisClientOnRegisterFailure(t *testing.T) {
	redisCfg := testfixtures.RedisConfig(t)
	log := testfixtures.DiscardLogger()

	monitorOpts, err := goredis.ParseURL(redisCfg.URL)
	require.NoError(t, err)
	monitor := goredis.NewClient(monitorOpts)
	t.Cleanup(func() { _ = monitor.Close() })

	baseline := connectedClients(t, monitor)

	s := &Scheduler{cfg: &config.Config{Redis: redisCfg, Timezone: time.UTC}, log: log}

	badTask := asynq.NewTask(job.TypeNoop, []byte("{}"))
	err = s.runTerm(context.Background(), []Entry{{Cron: "not a valid cron expression", Task: badTask}})
	require.Error(t, err, "an invalid cron entry must fail Register")
	require.Contains(t, err.Error(), "register")

	require.Eventually(t, func() bool {
		return connectedClients(t, monitor) <= baseline
	}, 5*time.Second, 50*time.Millisecond,
		"runTerm must close its own redis client on a Register failure, not leak it")
}

package job_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestNewNoopTaskRoundTripsThroughDecode(t *testing.T) {
	task, err := job.NewNoopTask("hi")
	require.NoError(t, err)
	require.Equal(t, job.TypeNoop, task.Type())

	payload, err := job.DecodeNoop(task)
	require.NoError(t, err)
	require.Equal(t, "hi", payload.Message)
}

// TestRegisterWiresNoopOntoTheMux exercises the Produces surface the
// integration test bypasses (it registers its own inline handler instead of
// job.Register), so this unit test is the only place job.Register and
// Handlers.Noop are verified together, without needing a live Redis.
func TestRegisterWiresNoopOntoTheMux(t *testing.T) {
	mux := asynq.NewServeMux()
	h := &job.Handlers{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	job.Register(mux, h)

	task, err := job.NewNoopTask("routed through Register")
	require.NoError(t, err)
	require.NoError(t, mux.ProcessTask(context.Background(), task))
}

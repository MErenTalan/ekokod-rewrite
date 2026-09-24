package metrics_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/metrics"
)

func TestServeExposesMetricsOnItsOwnAddress(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, metrics.Serve(ctx, addr, slog.New(slog.DiscardHandler)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/metrics", nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Contains(t, string(body), "go_goroutines")

	require.Error(t, metrics.Serve(ctx, addr, slog.New(slog.DiscardHandler)), "a taken address fails fast")
	require.NoError(t, metrics.Serve(ctx, "", slog.New(slog.DiscardHandler)), "empty disables")
}

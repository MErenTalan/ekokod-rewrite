// Package metrics is the processes' Prometheus surface (F15b): an internal
// listener, the job middleware and the queue and integration collectors.
package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Serve exposes /metrics on an internal address until ctx ends (R460); the
// public router never serves it. An empty addr disables it. It fails fast when
// the address cannot be bound.
func Serve(ctx context.Context, addr string, log *slog.Logger) error {
	if addr == "" {
		return nil
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics listener stopped", slog.String("addr", addr), slog.Any("error", err))
		}
	}()
	log.Info("metrics listening", slog.String("addr", ln.Addr().String()))
	return nil
}

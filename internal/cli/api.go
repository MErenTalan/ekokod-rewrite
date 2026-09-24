package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/metrics"

	"github.com/spf13/cobra"

	"github.com/MErenTalan/ekokod-rewrite/internal/api"
	"github.com/MErenTalan/ekokod-rewrite/internal/apiwire"
	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	ekoredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
)

// shutdownGrace bounds how long the api process waits for in-flight
// requests to drain once it is asked to stop.
const shutdownGrace = 30 * time.Second

// apiMaxHeaderBytes bounds the total size of a request's header block.
// middleware.RequestID (internal/api/middleware/requestid.go) echoes
// whatever X-Request-Id a caller supplies verbatim, so header size cannot
// be left to Go's default (net/http.DefaultMaxHeaderBytes, 1 MiB): an
// unauthenticated caller could otherwise send an enormous header to consume
// memory per connection. 32 KiB comfortably covers real traffic (a bearer
// JWT, cookies, a request id, standard proxy headers) while keeping the
// worst case far below the 1 MiB a caller would otherwise be allowed.
const apiMaxHeaderBytes = 32 << 10

func newAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "api",
		Short: "Run the HTTP API server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := newCommandLogger(cfg, os.Stderr)
			ctx := cmd.Context()

			pool, err := postgres.NewPool(ctx, cfg.DB, log)
			if err != nil {
				return err
			}
			defer pool.Close()
			if err := metrics.Serve(ctx, cfg.Metrics.APIAddr, log); err != nil {
				return fmt.Errorf("metrics listener: %w", err)
			}

			cache, err := ekoredis.New(ctx, cfg.Redis, log)
			if err != nil {
				return err
			}
			defer func() { _ = cache.Close() }()

			surface, err := apiwire.Build(ctx, cfg, pool, log, apiwire.Options{})
			if err != nil {
				return err
			}
			defer surface.Close()

			router := api.NewRouter(api.Deps{
				V1:    surface.V1,
				Cfg:   cfg,
				Log:   log,
				Build: buildinfo.Get(),
				ReadyChecks: []health.Check{
					postgres.PoolCheck(pool),
					ekoredis.Check(cache),
					postgres.MigrationsCheck(pool),
				},
			})

			server := newAPIServer(cfg.HTTP.Addr, router)

			// Bind synchronously, before starting the serve goroutine or
			// entering the shutdown select (task 9 review, Minor-3). Two
			// defects shared this one root cause: (1) if ListenAndServe's
			// bind failure and a SIGTERM arrived at the same instant, the
			// select below had two ready cases and Go's pseudo-random
			// choice could pick ctx.Done(), running Shutdown on a server
			// that never bound (a no-op returning nil) — RunE then
			// returned nil, exit 0, and a Restart=on-failure supervisor
			// would never restart a dead API; and (2) "api listening" was
			// logged before ListenAndServe was even called, so a bind
			// failure always logged a successful-sounding line first.
			// Binding here makes a bind failure a deterministic, immediate
			// non-nil return with nothing yet to shut down.
			//
			// ctx here governs only address resolution during this Listen
			// call; it does not bind the returned listener's lifetime to
			// context cancellation. Cancelling ctx after this call returns
			// does not close ln out from under the running server.
			ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTP.Addr)
			if err != nil {
				return fmt.Errorf("listen %s: %w", cfg.HTTP.Addr, err)
			}
			log.Info("api listening", slog.String("addr", ln.Addr().String()),
				slog.String("env", string(cfg.Env)))

			errCh := make(chan error, 1)
			go func() {
				if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- fmt.Errorf("http server: %w", err)
				}
			}()

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				log.Info("api shutting down")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
				defer cancel()
				return server.Shutdown(shutdownCtx)
			}
		},
	}
}

// newAPIServer builds the HTTP server with every timeout and size bound set
// explicitly, so none of them silently falls back to net/http's defaults.
func newAPIServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		MaxHeaderBytes:    apiMaxHeaderBytes,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
}

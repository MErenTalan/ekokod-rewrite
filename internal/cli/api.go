package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/api"
	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	ekoredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
	"github.com/spf13/cobra"
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
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)
			ctx := cmd.Context()

			pool, err := postgres.NewPool(ctx, cfg.DB, log)
			if err != nil {
				return err
			}
			defer pool.Close()

			cache, err := ekoredis.New(ctx, cfg.Redis, log)
			if err != nil {
				return err
			}
			defer func() { _ = cache.Close() }()

			router := api.NewRouter(api.Deps{
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

			errCh := make(chan error, 1)
			go func() {
				log.Info("api listening", slog.String("addr", cfg.HTTP.Addr),
					slog.String("env", string(cfg.Env)))
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

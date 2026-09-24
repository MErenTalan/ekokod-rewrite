package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/loadtest"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// newToolSeedScaleCmd is F15b R466: the load-test dataset, never in production.
func newToolSeedScaleCmd() *cobra.Command {
	var o seed.ScaleOptions
	cmd := &cobra.Command{
		Use:   "seed-scale",
		Short: "Build the load-test dataset (one company, buildings, analyzers, hourly readings); refuses production",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			if cfg.Env == config.EnvProduction {
				return errors.New("refusing to build the scale dataset in production")
			}
			if o.Password = os.Getenv("EKOKOD_SCALE_PASSWORD"); o.Password == "" {
				return errors.New("EKOKOD_SCALE_PASSWORD is required (the load test logs in with it)")
			}
			ctx := cmd.Context()
			pool, err := postgres.NewPool(ctx, cfg.DB, newCommandLogger(cfg, os.Stderr))
			if err != nil {
				return err
			}
			defer pool.Close()
			started := time.Now()
			res, err := seed.Scale(ctx, pool, auth.Hasher{Pepper: cfg.Security.PasswordPepper, Cost: cfg.Security.BcryptCost}, o, time.Now())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "scale dataset: company %s, %d readings written, login %s, %s\n", res.CompanyID, res.Readings,
				seed.ScaleEmail, time.Since(started).Round(time.Second))
			return err
		},
	}
	cmd.Flags().IntVar(&o.Analyzers, "analyzers", 200, "analyzers (Q-M1: 2× production)")
	cmd.Flags().IntVar(&o.Buildings, "buildings", 40, "buildings")
	cmd.Flags().IntVar(&o.Days, "days", 400, "days of hourly readings up to now")
	return cmd
}

// newToolLoadTestCmd is F15b R467.
func newToolLoadTestCmd() *cobra.Command {
	var o loadtest.Options
	var report string
	cmd := &cobra.Command{
		Use:   "loadtest",
		Short: "Drive the API with concurrent signed-in users; fail on the p95 budget or > 1 % errors",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.Password == "" {
				o.Password = os.Getenv("EKOKOD_SCALE_PASSWORD")
			}
			res, err := loadtest.Run(cmd.Context(), o, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			b, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(b)); err != nil {
				return err
			}
			if report != "" {
				if err := os.WriteFile(report, append(b, '\n'), 0o600); err != nil {
					return err
				}
			}
			if !res.Pass {
				return fmt.Errorf("loadtest failed: %v", res.Failures)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&o.Base, "base", "http://127.0.0.1:8080", "the API's base URL")
	cmd.Flags().StringVar(&o.Email, "email", seed.ScaleEmail, "login email")
	cmd.Flags().StringVar(&o.Password, "password", "", "login password (default $EKOKOD_SCALE_PASSWORD)")
	cmd.Flags().IntVar(&o.Users, "users", 10, "concurrent users (Q-M1)")
	cmd.Flags().DurationVar(&o.Duration, "duration", 2*time.Minute, "how long to run")
	cmd.Flags().DurationVar(&o.P95Budget, "p95-budget", 500*time.Millisecond, "per-endpoint p95 budget (Q-M2)")
	cmd.Flags().StringVar(&report, "report", "", "also write the JSON result here")
	return cmd
}

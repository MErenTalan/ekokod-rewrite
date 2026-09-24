package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/recompute"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/worker"
)

// newRecomputeCmd is 08 §7: derived data regenerated after a migration load,
// through the worker's own handlers (F14c R430–R435).
func newRecomputeCmd() *cobra.Command {
	var report string
	cmd := &cobra.Command{Use: "recompute", Short: "Regenerate derived data: consumption, bills, reports, carbon, forecasts (08 §7)"}
	cmd.PersistentFlags().StringVar(&report, "report", "", "also write the JSON summary to this file")

	var from, to string
	consumption := &cobra.Command{Use: "consumption", Short: "Refresh every continuous aggregate over the history",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, t, err := dayRange(from, to)
			if err != nil {
				return err
			}
			return withGraph(cmd, report, func(ctx context.Context, pool *pgxpool.Pool, _ *job.Handlers) (recompute.Summary, error) {
				return recompute.Consumption(ctx, admin.NewAggregateRepository(pool), f, t, cmd.ErrOrStderr()), nil
			})
		}}
	consumption.Flags().StringVar(&from, "from", "", "first day, YYYY-MM-DD (required)")
	consumption.Flags().StringVar(&to, "to", "", "day after the last, YYYY-MM-DD (default: now)")

	var billsFrom, billsTo string
	var force bool
	var parallel int
	bills := &cobra.Command{Use: "bills", Short: "Generate building, analyzer and company bills for every period",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if billsFrom == "" {
				return errors.New("--from YYYY-MM is required")
			}
			return withGraph(cmd, report, func(ctx context.Context, pool *pgxpool.Pool, h *job.Handlers) (recompute.Summary, error) {
				return recompute.Bills(ctx, admin.NewBillingRepository(pool), postgres.NewAnalyzerRepository(pool), h.BillingGenerate,
					recompute.BillsOptions{From: billsFrom, To: billsTo, Force: force, Parallel: parallel, Now: time.Now()}, cmd.ErrOrStderr())
			})
		}}
	bills.Flags().StringVar(&billsFrom, "from", "", "first period, YYYY-MM (required)")
	bills.Flags().StringVar(&billsTo, "to", "", "last period, YYYY-MM (default: each building's latest closed period)")
	bills.Flags().BoolVar(&force, "force", false, "supersede bills that already exist (Q-K1); without it a rerun converges")
	bills.Flags().IntVar(&parallel, "parallel", 2, "companies computed at once")

	var reportsFrom string
	reports := &cobra.Command{Use: "reports", Short: "Generate monthly and yearly reports for every building",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if reportsFrom == "" {
				return errors.New("--from YYYY-MM is required")
			}
			return withGraph(cmd, report, func(ctx context.Context, pool *pgxpool.Pool, h *job.Handlers) (recompute.Summary, error) {
				return recompute.Reports(ctx, admin.NewBillingRepository(pool), h.ReportGenerate, reportsFrom, time.Now(), cmd.ErrOrStderr())
			})
		}}
	reports.Flags().StringVar(&reportsFrom, "from", "", "first month, YYYY-MM (required)")

	var carbonFrom string
	carbon := &cobra.Command{Use: "carbon", Short: "Rebuild the daily carbon accruals from a date to yesterday",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, _, err := dayRange(carbonFrom, "")
			if err != nil {
				return err
			}
			return withGraph(cmd, report, func(ctx context.Context, _ *pgxpool.Pool, h *job.Handlers) (recompute.Summary, error) {
				c, ok := h.Carbon.(recompute.CarbonRecomputer)
				if !ok {
					return recompute.Summary{}, errors.New("the carbon service cannot recompute")
				}
				return recompute.Carbon(ctx, c, f, time.Now(), cmd.ErrOrStderr()), nil
			})
		}}
	carbon.Flags().StringVar(&carbonFrom, "from", "", "first day, YYYY-MM-DD (required)")

	forecasts := &cobra.Command{Use: "forecasts", Short: "Run the forecast for every analyzer (current horizon)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withGraph(cmd, report, func(ctx context.Context, _ *pgxpool.Pool, h *job.Handlers) (recompute.Summary, error) {
				return recompute.Forecasts(ctx, h.Forecast, cmd.ErrOrStderr()), nil
			})
		}}

	cmd.AddCommand(consumption, bills, reports, carbon, forecasts)
	return cmd
}

func dayRange(from, to string) (time.Time, time.Time, error) {
	if from == "" {
		return time.Time{}, time.Time{}, errors.New("--from YYYY-MM-DD is required")
	}
	f, err := time.ParseInLocation(time.DateOnly, from, istanbulLoc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("--from: %w", err)
	}
	t := time.Now()
	if to != "" {
		if t, err = time.ParseInLocation(time.DateOnly, to, istanbulLoc); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--to: %w", err)
		}
	}
	return f, t, nil
}

var istanbulLoc = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// withGraph opens the pool and the worker graph, runs fn, prints the summary
// and fails the command when anything failed (R435).
func withGraph(cmd *cobra.Command, report string, fn func(context.Context, *pgxpool.Pool, *job.Handlers) (recompute.Summary, error)) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	log := newCommandLogger(cfg, os.Stderr)
	pool, err := postgres.NewPool(ctx, cfg.DB, log)
	if err != nil {
		return err
	}
	defer pool.Close()
	built, err := worker.Build(ctx, cfg, pool, log)
	if err != nil {
		return err
	}
	defer built.Close()
	sum, err := fn(ctx, pool, built.Handlers)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(sum, "", "  ")
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
	if sum.Failed > 0 {
		return fmt.Errorf("recompute: %d of %d failed (see the failures above)", sum.Failed, sum.Failed+sum.Processed)
	}
	return nil
}

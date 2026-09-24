package cli

import (
	"net/http"
	"os"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	apperrors "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/spf13/cobra"
)

var errMigrateDownNeedsAll = apperrors.New("migrate_down_needs_all", http.StatusBadRequest,
	"errors.migrate.downNeedsAll")

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply, roll back or inspect database migrations",
	}

	up := &cobra.Command{
		Use:   "up",
		Short: "Apply every pending migration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)
			return postgres.MigrateUp(cmd.Context(), cfg.DB.URL, log)
		},
	}

	var all bool
	down := &cobra.Command{
		Use:   "down",
		Short: "Roll migrations back (requires --all)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !all {
				return errMigrateDownNeedsAll
			}
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)
			return postgres.MigrateDownAll(cmd.Context(), cfg.DB.URL, log)
		},
	}
	down.Flags().BoolVar(&all, "all", false, "roll back every migration")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show which migrations have been applied",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			return postgres.MigrateStatus(cmd.Context(), cfg.DB.URL, cmd.OutOrStdout())
		},
	}

	cmd.AddCommand(up, down, status)
	cmd.AddCommand(newMigrateLegacyCmd())
	return cmd
}

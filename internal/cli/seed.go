package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/spf13/cobra"
)

// seedDatasets grows in F1 (emission factors, GHG↔ISO mapping, integration
// definitions, national tariff schedule, ISO 50001 clause texts). Each
// loader must be idempotent: seed may be run more than once against the
// same database.
var seedDatasets = map[string]func(cmd *cobra.Command, cfg *config.Config) error{}

func newSeedCmd() *cobra.Command {
	var only string
	var yes bool
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Load reference datasets idempotently",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}

			// Print exactly what this is about to run against before doing
			// anything else, and before ever opening a connection: seed
			// writes to a database, must never silently run against the
			// wrong one, and F0 has zero datasets to seed, so there is no
			// reason to pay for a connection when there is nothing to load.
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "target environment: %s\n", cfg.Env)
			fmt.Fprintf(out, "database: %s\n", dbURLDisplay(cfg))

			if cfg.Env == config.EnvProduction && !yes {
				return fmt.Errorf("refusing to seed the production database without --yes")
			}

			if only != "" {
				loader, ok := seedDatasets[only]
				if !ok {
					return fmt.Errorf("unknown dataset %q; available: %s", only, availableDatasets())
				}
				return loader(cmd, cfg)
			}
			if len(seedDatasets) == 0 {
				fmt.Fprintln(out, "no reference datasets are defined yet")
				return nil
			}
			// Iterate in sorted-name order, not map order (Go map
			// iteration order is randomised on every run), so seed's
			// output and load order are deterministic.
			for _, name := range sortedNames() {
				fmt.Fprintf(out, "seeding %s\n", name)
				if err := seedDatasets[name](cmd, cfg); err != nil {
					return fmt.Errorf("seed %s: %w", name, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "seed a single dataset by name")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm seeding the production database")
	return cmd
}

func availableDatasets() string {
	names := sortedNames()
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

func sortedNames() []string {
	names := make([]string, 0, len(seedDatasets))
	for name := range seedDatasets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// dbURLDisplay returns the already-masked EKOKOD_DB_URL row from
// cfg.Resolved(). This deliberately reuses config's own redactDSN
// (internal/platform/config/resolved.go), which is already tested
// (parse_test.go) and already printed verbatim by `ekokod config:check`,
// instead of re-parsing the DSN here.
//
// An earlier version of this function (maskedDBHost) hand-parsed cfg.DB.URL
// with net/url and ran the extracted host through secret.Redact against
// password fragments. Task 9's review (Minor-1, Minor-2) found that pass
// both pointless and actively harmful: net/url splits the authority at the
// LAST unescaped '@', so (*url.URL).Host is disjoint from the userinfo
// password by construction — the redaction pass never removed real
// credential bytes, it only risked corrupting the displayed host whenever
// a password fragment happened to also be a substring of the host or port
// (e.g. "postgres://u:host@myhost:5432/db" printed "my••••••••:5432"). It
// also duplicated DSN-scrubbing logic config already solved, and degraded
// to "(unknown)" for DSN forms cfg.DB.URL may legally hold (pgx
// keyword/value pairs, unix-socket DSNs) that config's own loader accepts.
// Reusing the Resolved row avoids a second hand-rolled DSN parser — the
// exact code class behind two Criticals earlier in this phase — and shows
// the database name as well as the host.
func dbURLDisplay(cfg *config.Config) string {
	for _, r := range cfg.Resolved() {
		if r.Name == "EKOKOD_DB_URL" {
			return r.Value
		}
	}
	return "(unknown)"
}

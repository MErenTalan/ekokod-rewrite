package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
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
			fmt.Fprintf(out, "database host: %s\n", maskedDBHost(cfg.DB.URL))

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

// maskedDBHost extracts the host:port from a DSN for display, without ever
// printing the credential. url.Parse already keeps the password out of
// (*url.URL).Host, but the extracted host is additionally passed through
// secret.Redact against every fragment of the userinfo password as a
// defence-in-depth measure, in case the DSN is malformed in a way that
// causes credential material to land somewhere unexpected.
func maskedDBHost(rawDSN string) string {
	u, err := url.Parse(rawDSN)
	if err != nil {
		return "(unparseable DSN)"
	}
	host := u.Host
	if host == "" {
		return "(unknown)"
	}
	if u.User != nil {
		if pw, ok := u.User.Password(); ok {
			host = secret.Redact(host, secret.Fragments(pw))
		}
	}
	return host
}

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
// also duplicated DSN-scrubbing logic config already solved.
//
// CORRECTION (task 9 review round 2, Critical): an earlier version of this
// comment claimed that reusing the Resolved row also "fixes" the
// "(unknown)" degradation maskedDBHost showed for pgx keyword/value DSNs
// and unix-socket DSNs. That claim was false, and dangerously so: at the
// time it was written, config.redactDSN's fallback for exactly those
// shapes was a bare `return raw`, which — combined with dsnDisplay
// unconditionally prepending the mask glyph — meant this function printed
// the database password in clear behind what looked like a redacted
// value. redactDSN has since been fixed to fail closed (never return an
// unredacted value on any path); see its doc comment for the mandated
// shape. The accurate claim now is narrower: a URL-shaped DSN that carries
// its credential in the query string (a common unix-socket form, e.g.
// "postgres:///db?host=/var/run/postgresql&password=...") does now show
// the host and database name safely, with the password stripped from the
// query string. A true pgx keyword/value DSN with no scheme at all
// ("host=db user=x password=... dbname=y") is still withheld entirely —
// redactDSN does not attempt to tokenise it, on the grounds that a
// fallible redactor over a credential is worse than withholding — so that
// case is exactly as opaque as maskedDBHost's old "(unknown)", just now
// guaranteed safe by construction rather than safe by the old function's
// bare-return accident.
func dbURLDisplay(cfg *config.Config) string {
	for _, r := range cfg.Resolved() {
		if r.Name == "EKOKOD_DB_URL" {
			return r.Value
		}
	}
	return "(unknown)"
}

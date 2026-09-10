package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/spf13/cobra"
)

func newConfigCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "config:check",
		Aliases: []string{"config-check"},
		Short:   "Validate the configuration and print every resolved value with secrets masked",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "VARIABLE\tVALUE\tSOURCE"); err != nil {
				return err
			}
			for _, r := range cfg.Resolved() {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Value, r.Source); err != nil {
					return err
				}
			}
			return w.Flush()
		},
	}
}

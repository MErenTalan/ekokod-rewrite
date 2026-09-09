package cli

import (
	"encoding/json"
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Get()
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				return enc.Encode(info)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "ekokod %s (commit %s, built %s, %s)\n",
				info.Version, info.Commit, info.Date, info.Go)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	return cmd
}

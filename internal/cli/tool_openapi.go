package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
	"github.com/MErenTalan/ekokod-rewrite/internal/apidoc"
)

// newToolOpenAPICmd prints the OpenAPI document generated from the route table
// (R185), or its Markdown reference (F15c R482).
func newToolOpenAPICmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "openapi",
		Short: "Print the /api/v1 OpenAPI 3.1 document (--format markdown: the API reference)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := v1.BuildOpenAPI(v1.Table())
			if err != nil {
				return err
			}
			switch format {
			case "json":
			case "markdown":
				if doc, err = apidoc.Markdown(doc); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown --format %q (json or markdown)", format)
			}
			_, err = cmd.OutOrStdout().Write(doc)
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "json or markdown")
	return cmd
}

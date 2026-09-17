package cli

import (
	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
	"github.com/spf13/cobra"
)

// newToolOpenAPICmd prints the OpenAPI document generated from the route table (R185).
func newToolOpenAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "openapi",
		Short: "Print the /api/v1 OpenAPI 3.1 document",
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := v1.BuildOpenAPI(v1.Table())
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(doc)
			return err
		},
	}
}

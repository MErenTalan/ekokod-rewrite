package cli

import "github.com/spf13/cobra"

// newToolCmd groups operator tools that need no running services.
func newToolCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tool", Short: "Operator tools"}
	cmd.AddCommand(newCompareInvoiceCmd())
	return cmd
}

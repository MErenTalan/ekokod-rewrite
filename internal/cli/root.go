// Package cli wires the ekokod subcommands. It contains no business logic.
package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// NewRoot builds the root command with every subcommand attached.
func NewRoot(out io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "ekokod",
		Short:         "ekokod energy management platform",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(out)
	root.SetErr(out)
	root.AddCommand(
		newVersionCmd(),
		newConfigCheckCmd(),
		newMigrateCmd(),
		newAPICmd(),
		newWorkerCmd(),
		newSchedulerCmd(),
		newSeedCmd(),
	)
	return root
}

// Execute runs the CLI with the given arguments, writing output to out.
func Execute(ctx context.Context, args []string, out io.Writer) error {
	root := NewRoot(out)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

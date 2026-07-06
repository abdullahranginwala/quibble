package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Injected via -ldflags at build time; see Makefile.
var (
	version = "dev"
	commit  = "unknown"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the quibble version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "quibble %s (%s)\n", version, commit)
			return nil
		},
	}
}

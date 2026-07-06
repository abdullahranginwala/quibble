// Package cli wires the quibble command tree.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	flagJSON bool
	flagDir  string
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "quibble",
		Short: "Review-grade markdown docs for humans and AI agents",
		Long: `Quibble renders markdown into readable HTML and manages anchored,
threaded comments on those docs, stored as files in git so agents
see them natively. See DESIGN.md in the quibble repository.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable JSON output")
	root.PersistentFlags().StringVar(&flagDir, "dir", ".", "project root to operate on")

	root.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newBuildCmd(),
		newServeCmd(),
		newCommentsCmd(),
		newDoctorCmd(),
	)
	return root
}

// Execute runs the CLI and returns a process exit code.
func Execute() int {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "quibble: %v\n", err)
		if code, ok := exitCodeOf(err); ok {
			return code
		}
		return 1
	}
	return 0
}

// codedError carries a specific process exit code (see plan/06-m4-cli.md).
type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &codedError{code: code, err: err}
}

func exitCodeOf(err error) (int, bool) {
	var ce *codedError
	if ok := asCoded(err, &ce); ok {
		return ce.code, true
	}
	return 0, false
}

func asCoded(err error, target **codedError) bool {
	for err != nil {
		if ce, ok := err.(*codedError); ok {
			*target = ce
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

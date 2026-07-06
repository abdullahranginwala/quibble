package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/abdullahranginwala/quibble/internal/server"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the local review app (comment on rendered docs in your browser)",
		Long: `Renders the configured docs and serves them at http://127.0.0.1:<port>
with the Google-Docs-style comment UI. Selecting text and commenting writes
thread files into .quibble/comments/, which you commit like any other change.
Binds loopback only; there is no daemon and no state outside the repo.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return withExitCode(1, err)
			}
			srv, err := server.New(root)
			if err != nil {
				return withExitCode(1, err)
			}
			ln, err := server.Listen(port)
			if err != nil {
				return withExitCode(1, err)
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			fmt.Fprintf(cmd.OutOrStdout(), "quibble serve → http://%s\n", ln.Addr().String())
			fmt.Fprintln(cmd.OutOrStdout(), "watching for changes; press Ctrl-C to stop")

			if err := srv.Serve(ctx, ln); err != nil {
				return withExitCode(1, err)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 4747, "loopback port to listen on")
	return cmd
}

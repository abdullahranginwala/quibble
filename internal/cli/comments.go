package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/store"
	"github.com/spf13/cobra"
)

func newCommentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "comments",
		Short:   "List and manage comment threads",
		Aliases: []string{"comment"},
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(
		newListCmd(),
		newShowCmd(),
		newAddCmd(),
		newReplyCmd(),
		newAddressCmd(),
		newResolveCmd(),
		newReopenCmd(),
		newRepinCmd(),
	)
	return cmd
}

func newListCmd() *cobra.Command {
	var (
		doc       string
		open      bool
		addressed bool
		resolved  bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List comment threads (default: open + addressed)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return withExitCode(1, err)
			}
			if _, err := loadConfig(root); err != nil {
				return err
			}
			s, err := openStore(root)
			if err != nil {
				return err
			}
			var statuses []comment.Status
			if open {
				statuses = append(statuses, comment.StatusOpen)
			}
			if addressed {
				statuses = append(statuses, comment.StatusAddressed)
			}
			if resolved {
				statuses = append(statuses, comment.StatusResolved)
			}
			if len(statuses) == 0 {
				// The actionable default — deliberately NOT resolved.
				statuses = []comment.Status{comment.StatusOpen, comment.StatusAddressed}
			}
			threads, err := s.List(context.Background(), store.Filter{Doc: doc, Statuses: statuses})
			if err != nil {
				return withExitCode(1, err)
			}
			return writeList(cmd.OutOrStdout(), threads)
		},
	}
	cmd.Flags().StringVar(&doc, "doc", "", "only threads on this document (relative path)")
	cmd.Flags().BoolVar(&open, "open", false, "include open threads")
	cmd.Flags().BoolVar(&addressed, "addressed", false, "include addressed threads")
	cmd.Flags().BoolVar(&resolved, "resolved", false, "include resolved threads")
	return cmd
}

func writeList(w io.Writer, threads []*comment.Thread) error {
	if flagJSON {
		views := make([]threadJSON, 0, len(threads))
		for _, t := range threads {
			views = append(views, toThreadJSON(t))
		}
		return writeJSON(w, views)
	}
	if len(threads) == 0 {
		fmt.Fprintln(w, "no matching threads")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tDOC\tAGE\tBODY")
	for _, t := range threads {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			t.ID, t.Status, t.Doc, ageString(t.Created), firstRunes(t.Body, 60))
	}
	return tw.Flush()
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a single thread with its replies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := projectRoot()
			if err != nil {
				return withExitCode(1, err)
			}
			if _, err := loadConfig(root); err != nil {
				return err
			}
			s, err := openStore(root)
			if err != nil {
				return err
			}
			t, err := mustGet(context.Background(), s, args[0])
			if err != nil {
				return err
			}
			return writeShow(cmd.OutOrStdout(), t)
		},
	}
}

func writeShow(w io.Writer, t *comment.Thread) error {
	if flagJSON {
		return writeJSON(w, toThreadJSON(t))
	}
	fmt.Fprintf(w, "%s  [%s]  %s\n", t.ID, t.Status, t.Doc)
	fmt.Fprintf(w, "author: %s   created: %s\n", t.Author, t.Created.Format(time.RFC3339))
	fmt.Fprintf(w, "quote:  %q\n", t.Anchor.Exact)
	if len(t.Anchor.Heading) > 0 {
		fmt.Fprintf(w, "under:  %v\n", t.Anchor.Heading)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, t.Body)
	for _, r := range t.Replies {
		fmt.Fprintf(w, "\n--- reply by %s at %s ---\n%s\n", r.Author, r.Time.Format(time.RFC3339), r.Body)
	}
	if t.Status == comment.StatusResolved && t.ResolvedBy != "" {
		at := ""
		if t.ResolvedAt != nil {
			at = t.ResolvedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "\nresolved by %s at %s\n", t.ResolvedBy, at)
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return withExitCode(1, fmt.Errorf("encoding JSON: %w", err))
	}
	return nil
}

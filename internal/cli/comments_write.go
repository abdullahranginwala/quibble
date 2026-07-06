package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var (
		doc     string
		quote   string
		message string
		author  string
	)
	cmd := &cobra.Command{
		Use:   "add --doc D --quote \"...\" [-m msg]",
		Short: "Anchor a new comment thread to a quote in a document",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return withExitCode(1, err)
			}
			cfg, err := loadConfig(root)
			if err != nil {
				return err
			}
			if doc == "" || quote == "" {
				return withExitCode(1, errors.New("both --doc and --quote are required"))
			}
			s, err := openStore(root)
			if err != nil {
				return err
			}
			src, err := readDoc(root, doc)
			if err != nil {
				return withExitCode(1, err)
			}
			ac, err := docAnchorContext(src, doc)
			if err != nil {
				return withExitCode(1, err)
			}
			anchor, err := anchorForQuote(ac, quote)
			if err != nil {
				return err
			}
			t := &comment.Thread{
				ID:      comment.NewID(),
				Doc:     doc,
				Status:  comment.StatusOpen,
				Created: time.Now(),
				Author:  resolveAuthor(author, cfg.Authors.Human),
				Anchor:  anchor,
				Body:    message,
			}
			if err := s.Create(context.Background(), t); err != nil {
				return withExitCode(1, err)
			}
			return writeCreated(cmd, t)
		},
	}
	cmd.Flags().StringVar(&doc, "doc", "", "document to comment on (relative path)")
	cmd.Flags().StringVar(&quote, "quote", "", "verbatim text from the document to anchor to")
	cmd.Flags().StringVarP(&message, "message", "m", "", "comment body")
	cmd.Flags().StringVar(&author, "author", "", "override the comment author")
	return cmd
}

func writeCreated(cmd *cobra.Command, t *comment.Thread) error {
	if flagJSON {
		return writeJSON(cmd.OutOrStdout(), toThreadJSON(t))
	}
	fmt.Fprintln(cmd.OutOrStdout(), t.ID)
	return nil
}

func newReplyCmd() *cobra.Command {
	var (
		message string
		author  string
	)
	cmd := &cobra.Command{
		Use:   "reply <id> -m msg",
		Short: "Append a reply to a thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, s, err := setupMutation()
			if err != nil {
				return err
			}
			if message == "" {
				return withExitCode(1, errors.New("--message is required for a reply"))
			}
			if _, err := mustGet(context.Background(), s, args[0]); err != nil {
				return err
			}
			r := comment.Reply{
				Author: resolveAuthor(author, cfg.Authors.Human),
				Time:   time.Now(),
				Body:   message,
			}
			if err := s.Reply(context.Background(), args[0], r); err != nil {
				return withExitCode(1, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "replied to %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "reply body")
	cmd.Flags().StringVar(&author, "author", "", "override the reply author")
	return cmd
}

func newAddressCmd() *cobra.Command {
	var (
		message string
		author  string
	)
	cmd := &cobra.Command{
		Use:   "address <id> [-m msg]",
		Short: "Mark a thread addressed (the agent's verb)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, s, err := setupMutation()
			if err != nil {
				return err
			}
			actor := resolveAuthor(author, cfg.Authors.Agent)
			ctx := context.Background()
			if _, err := mustGet(ctx, s, args[0]); err != nil {
				return err
			}
			if message != "" {
				r := comment.Reply{Author: actor, Time: time.Now(), Body: message}
				if err := s.Reply(ctx, args[0], r); err != nil {
					return withExitCode(1, err)
				}
			}
			if err := s.SetStatus(ctx, args[0], comment.StatusAddressed, actor); err != nil {
				return withExitCode(1, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "addressed %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "optional reply to append before addressing")
	cmd.Flags().StringVar(&author, "author", "", "override the author")
	return cmd
}

func newResolveCmd() *cobra.Command {
	var author string
	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Resolve a thread (the human's verb; moves it to the archive)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, s, err := setupMutation()
			if err != nil {
				return err
			}
			actor := resolveAuthor(author, cfg.Authors.Human)
			// Policy gate: agents address, humans resolve (DESIGN.md §4).
			if actor == cfg.Authors.Agent {
				return withExitCode(3, errors.New("agents address; humans resolve"))
			}
			ctx := context.Background()
			if _, err := mustGet(ctx, s, args[0]); err != nil {
				return err
			}
			if err := s.SetStatus(ctx, args[0], comment.StatusResolved, actor); err != nil {
				return withExitCode(1, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "resolved %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&author, "author", "", "override the resolver (agents may not, per the contract)")
	return cmd
}

func newReopenCmd() *cobra.Command {
	var (
		message string
		author  string
	)
	cmd := &cobra.Command{
		Use:   "reopen <id> [-m msg]",
		Short: "Reopen an addressed or resolved thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, s, err := setupMutation()
			if err != nil {
				return err
			}
			actor := resolveAuthor(author, cfg.Authors.Human)
			ctx := context.Background()
			if _, err := mustGet(ctx, s, args[0]); err != nil {
				return err
			}
			if message != "" {
				r := comment.Reply{Author: actor, Time: time.Now(), Body: message}
				if err := s.Reply(ctx, args[0], r); err != nil {
					return withExitCode(1, err)
				}
			}
			if err := s.SetStatus(ctx, args[0], comment.StatusOpen, actor); err != nil {
				return withExitCode(1, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reopened %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "optional reply to append while reopening")
	cmd.Flags().StringVar(&author, "author", "", "override the author")
	return cmd
}

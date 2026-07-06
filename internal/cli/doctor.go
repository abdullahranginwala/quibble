package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/abdullahranginwala/quibble/internal/config"
	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/render"
	"github.com/abdullahranginwala/quibble/pkg/store"
	"github.com/spf13/cobra"
)

// doctorRow is one thread's anchor-health report.
type doctorRow struct {
	ID         string  `json:"id"`
	Doc        string  `json:"doc"`
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
	Fixed      bool    `json:"fixed,omitempty"`
}

func newDoctorCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check anchor health across all non-resolved threads",
		Long: `Re-anchors every open/addressed thread against its current document and
reports the method (exact/context/fuzzy/orphan) and confidence. With --fix it
rewrites fuzzy anchors (confidence >= 0.75) to self-heal them. Exit codes: 0 all
exact/context, 2 any fuzzy, 3 any orphan or corrupt thread file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return withExitCode(1, err)
			}
			cfg, err := loadConfig(root)
			if err != nil {
				return err
			}
			return runDoctor(cmd, root, cfg, fix)
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "rewrite healable fuzzy anchors in place")
	return cmd
}

func runDoctor(cmd *cobra.Command, root string, cfg *config.Config, fix bool) error {
	s, err := openStore(root)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// All non-resolved threads.
	threads, err := s.List(ctx, store.Filter{
		Statuses: []comment.Status{comment.StatusOpen, comment.StatusAddressed},
	})
	if err != nil {
		return withExitCode(1, err)
	}
	warnings := s.Warnings()

	// Cache per-doc anchor context so each document is rendered once.
	ctxCache := map[string]*anchorContext{}
	getCtx := func(doc string) (*anchorContext, bool) {
		if ac, ok := ctxCache[doc]; ok {
			return ac, ac != nil
		}
		src, err := readDoc(root, doc)
		if err != nil {
			ctxCache[doc] = nil
			return nil, false
		}
		ac, err := docAnchorContext(src, doc)
		if err != nil {
			ctxCache[doc] = nil
			return nil, false
		}
		ctxCache[doc] = &ac
		return &ac, true
	}

	var (
		rows      []doctorRow
		anyFuzzy  bool
		anyOrphan bool
	)
	for _, t := range threads {
		ac, ok := getCtx(t.Doc)
		if !ok {
			rows = append(rows, doctorRow{ID: t.ID, Doc: t.Doc, Method: string(comment.MethodOrphan), Confidence: 0})
			anyOrphan = true
			continue
		}
		p := comment.Resolve(ac.text, ac.sections, t.Anchor)
		row := doctorRow{ID: t.ID, Doc: t.Doc, Method: string(p.Method), Confidence: p.Confidence}
		switch p.Method {
		case comment.MethodFuzzy:
			anyFuzzy = true
			if fix {
				newAnchor := comment.NewAnchor(ac.text, ac.sections, p.Start, p.End)
				if err := rewriteAnchor(root, t, newAnchor); err != nil {
					return withExitCode(1, err)
				}
				row.Fixed = true
			}
		case comment.MethodOrphan:
			anyOrphan = true
		}
		rows = append(rows, row)
	}

	// Validate configured theme overrides against render's token contract by
	// letting the renderer reject unknown tokens (it owns the list).
	var themeErr error
	if len(cfg.Theme.Overrides) > 0 {
		if _, err := render.New(render.Options{Theme: render.Paper(), Overrides: cfg.Theme.Overrides}); err != nil {
			themeErr = err
		}
	}

	writeDoctor(cmd.OutOrStdout(), rows, warnings, themeErr)

	switch {
	case anyOrphan || len(warnings) > 0 || themeErr != nil:
		return withExitCode(3, fmt.Errorf("doctor found orphaned or corrupt threads"))
	case anyFuzzy:
		return withExitCode(2, fmt.Errorf("doctor found fuzzy anchors"))
	default:
		return nil
	}
}

func writeDoctor(w io.Writer, rows []doctorRow, warnings []string, themeErr error) {
	if flagJSON {
		out := struct {
			Threads  []doctorRow `json:"threads"`
			Warnings []string    `json:"warnings,omitempty"`
			Theme    string      `json:"theme_error,omitempty"`
		}{Threads: rows, Warnings: warnings}
		if themeErr != nil {
			out.Theme = themeErr.Error()
		}
		_ = writeJSON(w, out)
		return
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no threads to check")
	} else {
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tDOC\tMETHOD\tCONFIDENCE\tFIXED")
		for _, r := range rows {
			fixed := ""
			if r.Fixed {
				fixed = "yes"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%.2f\t%s\n", r.ID, r.Doc, r.Method, r.Confidence, fixed)
		}
		tw.Flush()
	}
	for _, wmsg := range warnings {
		fmt.Fprintf(w, "corrupt: %s\n", wmsg)
	}
	if themeErr != nil {
		fmt.Fprintf(w, "theme: %v\n", themeErr)
	}
}

func newRepinCmd() *cobra.Command {
	var quote string
	cmd := &cobra.Command{
		Use:   "repin <id> --quote \"...\"",
		Short: "Re-anchor an orphaned thread to a fresh verbatim quote",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, s, err := setupMutation()
			if err != nil {
				return err
			}
			if quote == "" {
				return withExitCode(1, fmt.Errorf("--quote is required"))
			}
			ctx := context.Background()
			t, err := mustGet(ctx, s, args[0])
			if err != nil {
				return err
			}
			src, err := readDoc(root, t.Doc)
			if err != nil {
				return withExitCode(1, err)
			}
			ac, err := docAnchorContext(src, t.Doc)
			if err != nil {
				return withExitCode(1, err)
			}
			anchor, err := anchorForQuote(ac, quote)
			if err != nil {
				return err
			}
			if err := rewriteAnchor(root, t, anchor); err != nil {
				return withExitCode(1, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "repinned %s\n", t.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&quote, "quote", "", "verbatim text from the document to re-anchor to")
	return cmd
}

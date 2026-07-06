package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abdullahranginwala/quibble/internal/config"
	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/render"
	"github.com/abdullahranginwala/quibble/pkg/store"
)

// projectRoot returns the absolute project root selected by the global --dir
// flag.
func projectRoot() (string, error) {
	abs, err := filepath.Abs(flagDir)
	if err != nil {
		return "", fmt.Errorf("resolving --dir %q: %w", flagDir, err)
	}
	return abs, nil
}

// inGitWorkTree reports whether dir (or any ancestor) contains a .git entry.
// Comments only make sense inside a repository (DESIGN.md §3), so `init`
// refuses to run outside one.
func inGitWorkTree(dir string) bool {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// loadConfig loads the project config, requiring an initialized project.
func loadConfig(root string) (*config.Config, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, withExitCode(1, err)
	}
	return cfg, nil
}

// openStore builds the filesystem comment store for an initialized project.
func openStore(root string) (*store.FSStore, error) {
	s, err := store.NewFS(root)
	if err != nil {
		return nil, withExitCode(1, err)
	}
	return s, nil
}

// setupMutation is the shared preamble for the write subcommands: resolve the
// project root, require an initialized config, and open the store.
func setupMutation() (root string, cfg *config.Config, s *store.FSStore, err error) {
	root, err = projectRoot()
	if err != nil {
		return "", nil, nil, withExitCode(1, err)
	}
	cfg, err = loadConfig(root)
	if err != nil {
		return "", nil, nil, err
	}
	s, err = openStore(root)
	if err != nil {
		return "", nil, nil, err
	}
	return root, cfg, s, nil
}

// resolveAuthor applies the attribution precedence: --author flag, then the
// QUIBBLE_AUTHOR environment variable, then the supplied config default.
func resolveAuthor(flagAuthor, cfgDefault string) string {
	if flagAuthor != "" {
		return flagAuthor
	}
	if env := os.Getenv("QUIBBLE_AUTHOR"); env != "" {
		return env
	}
	return cfgDefault
}

// anchorContext holds the normalized text and heading sections a document
// contributes to anchoring. Both come straight from pkg/render so add/doctor/
// repin all anchor against the identical normalized text the renderer emits.
type anchorContext struct {
	text     string
	sections []comment.Section
}

// docAnchorContext renders src (the raw markdown of relPath) and bridges the
// render types into the pkg/comment types used for anchoring. The bridge lives
// here in internal/cli (see plan/DECISIONS.md).
func docAnchorContext(src []byte, relPath string) (anchorContext, error) {
	r, err := render.New(render.Options{Theme: render.Paper()})
	if err != nil {
		return anchorContext{}, fmt.Errorf("preparing renderer: %w", err)
	}
	doc, err := r.RenderDoc(src, relPath)
	if err != nil {
		return anchorContext{}, err
	}
	headings := make([]comment.Heading, len(doc.Outline))
	for i, h := range doc.Outline {
		headings[i] = comment.Heading{Level: h.Level, Text: h.Text, Anchor: h.Anchor}
	}
	blocks := make([]comment.Block, len(doc.Blocks))
	for i, b := range doc.Blocks {
		blocks[i] = comment.Block{ID: b.ID, Text: b.Text, Start: b.Start, End: b.End}
	}
	return anchorContext{
		text:     doc.Text,
		sections: comment.Sectionize(headings, doc.Text, blocks),
	}, nil
}

// readDoc reads a document's source given the project root and its relative
// path, returning a friendly error when the file is absent.
func readDoc(root, relDoc string) ([]byte, error) {
	p := filepath.Join(root, filepath.FromSlash(relDoc))
	src, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("document %s not found under the project root", relDoc)
		}
		return nil, fmt.Errorf("reading %s: %w", relDoc, err)
	}
	return src, nil
}

// quoteResult is the outcome of locating a quote in a document's normalized
// text. Offsets are rune offsets into that text.
type quoteResult struct {
	start, end int
	count      int
}

// findQuote counts occurrences of quote within docText (both compared as rune
// sequences) and returns the rune span of the first occurrence. The count lets
// callers enforce the "exactly once" uniqueness rule that `add` and `repin`
// require.
func findQuote(docText, quote string) quoteResult {
	doc := []rune(docText)
	q := []rune(quote)
	if len(q) == 0 || len(q) > len(doc) {
		return quoteResult{}
	}
	first, count := -1, 0
	last := len(doc) - len(q)
	for i := 0; i <= last; i++ {
		match := true
		for j := 0; j < len(q); j++ {
			if doc[i+j] != q[j] {
				match = false
				break
			}
		}
		if match {
			if first < 0 {
				first = i
			}
			count++
		}
	}
	return quoteResult{start: first, end: first + len(q), count: count}
}

// anchorForQuote enforces the uniqueness rule and builds an anchor. A missing
// quote or an ambiguous (>1) quote is a user error with exit code 1.
func anchorForQuote(ac anchorContext, quote string) (comment.Anchor, error) {
	q := strings.TrimSpace(quote)
	if q == "" {
		return comment.Anchor{}, withExitCode(1, errors.New("--quote must not be empty"))
	}
	res := findQuote(ac.text, q)
	switch res.count {
	case 1:
		return comment.NewAnchor(ac.text, ac.sections, res.start, res.end), nil
	case 0:
		return comment.Anchor{}, withExitCode(1, errors.New(
			"quote not found; copy it verbatim from the document's rendered text"))
	default:
		return comment.Anchor{}, withExitCode(1, fmt.Errorf(
			"quote occurs %d times; add surrounding words to make it unique", res.count))
	}
}

// findThreadFile locates a thread's on-disk file by scanning the comment
// directories (open/addressed slugs first, then the resolved archive), mirroring
// FSStore's own lookup. Used by doctor --fix and repin to rewrite anchors in
// place, since CommentStore has no anchor-update method.
func findThreadFile(root, id string) (string, error) {
	commentsDir := filepath.Join(root, ".quibble", "comments")
	name := id + ".md"
	entries, err := os.ReadDir(commentsDir)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", commentsDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "_resolved" {
			continue
		}
		p := filepath.Join(commentsDir, e.Name(), name)
		if fileExists(p) {
			return p, nil
		}
	}
	resolvedRoot := filepath.Join(commentsDir, "_resolved")
	if rentries, err := os.ReadDir(resolvedRoot); err == nil {
		for _, e := range rentries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(resolvedRoot, e.Name(), name)
			if fileExists(p) {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("thread %s not found on disk", id)
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// rewriteAnchor persists a new anchor onto a thread's file, preserving its
// location. It writes atomically (temp + rename) to match FSStore's durability.
func rewriteAnchor(root string, t *comment.Thread, a comment.Anchor) error {
	p, err := findThreadFile(root, t.ID)
	if err != nil {
		return err
	}
	t.Anchor = a
	data, err := t.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling thread %s: %w", t.ID, err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming %s: %w", tmp, err)
	}
	return nil
}

// mustGet fetches a thread, translating not-found into an exit-1 user error.
func mustGet(ctx context.Context, s *store.FSStore, id string) (*comment.Thread, error) {
	t, err := s.Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, withExitCode(1, fmt.Errorf("no thread with id %s", id))
		}
		return nil, withExitCode(1, err)
	}
	return t, nil
}

// ageString renders the elapsed time since t as a compact, human-friendly
// token (e.g. "3d", "2h", "5m", "10s", "now").
func ageString(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 0:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// firstRunes returns the first n runes of s on a single line, collapsing inner
// newlines to spaces so table rows stay intact.
func firstRunes(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

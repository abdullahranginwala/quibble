package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abdullahranginwala/quibble/internal/config"
	"github.com/fsnotify/fsnotify"
)

// debounceInterval coalesces bursts of filesystem events (e.g. editor
// save-then-chmod, or a store's temp-write + rename) into one update.
const debounceInterval = 150 * time.Millisecond

// Watch starts watching the docs tree and .quibble/comments for changes,
// re-rendering changed docs and broadcasting SSE events. It returns once the
// watcher is set up; the watch loop runs until ctx is cancelled. A setup
// failure (e.g. fsnotify unavailable) is returned.
func (s *Server) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("server: creating watcher: %w", err)
	}
	for _, dir := range s.watchDirs() {
		// Best-effort: a missing dir is not fatal (it may be created later).
		_ = w.Add(dir)
	}
	go s.watchLoop(ctx, w)
	return nil
}

// watchDirs collects the directories to watch: the project root, every
// directory containing a configured doc, and the whole .quibble/comments tree.
func (s *Server) watchDirs() []string {
	set := map[string]struct{}{s.root: {}}

	if paths, err := config.MatchDocs(os.DirFS(s.root), s.cfg.Docs); err == nil {
		for _, rel := range paths {
			set[filepath.Dir(filepath.Join(s.root, filepath.FromSlash(rel)))] = struct{}{}
		}
	}
	commentsRoot := filepath.Join(s.root, ".quibble", "comments")
	// Ensure the comments dir exists so it can be watched from the start; the
	// first-ever comment then fires a Create we catch (test row 11).
	_ = os.MkdirAll(commentsRoot, 0o755)
	_ = filepath.WalkDir(commentsRoot, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			set[p] = struct{}{}
		}
		return nil
	})
	set[commentsRoot] = struct{}{}

	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	return out
}

func (s *Server) watchLoop(ctx context.Context, w *fsnotify.Watcher) {
	defer w.Close()

	timer := time.NewTimer(debounceInterval)
	if !timer.Stop() {
		<-timer.C
	}
	timerActive := false

	pendingDocs := map[string]string{}       // slug -> relPath
	pendingComments := map[string]struct{}{} // slug

	arm := func() {
		if !timerActive {
			timer.Reset(debounceInterval)
			timerActive = true
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounceInterval)
		}
	}

	flush := func() {
		timerActive = false
		for slug, rel := range pendingDocs {
			s.reRender(rel)
			s.hub.broadcast("doc-changed", map[string]string{"slug": slug})
			delete(pendingDocs, slug)
		}
		for slug := range pendingComments {
			s.hub.broadcast("comments-changed", map[string]string{"slug": slug})
			delete(pendingComments, slug)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			// A newly created directory (e.g. a comment slug dir) must be
			// watched so files created inside it are seen.
			if ev.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = w.Add(ev.Name)
				}
			}
			if slug, rel, kind := s.classify(ev.Name); kind == kindDoc {
				pendingDocs[slug] = rel
				arm()
			} else if kind == kindComment {
				pendingComments[slug] = struct{}{}
				arm()
			}
		case <-w.Errors:
			// Ignore watcher errors; the next event re-syncs state.
		case <-timer.C:
			flush()
		}
	}
}

type changeKind int

const (
	kindNone changeKind = iota
	kindDoc
	kindComment
)

// classify decides whether an absolute path is a doc change, a comment change,
// or irrelevant, and returns the doc slug it affects.
func (s *Server) classify(abs string) (slug, rel string, kind changeKind) {
	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return "", "", kindNone
	}
	rel = filepath.ToSlash(rel)

	const commentsPrefix = ".quibble/comments/"
	if strings.HasPrefix(rel, commentsPrefix) {
		parts := strings.Split(strings.TrimPrefix(rel, commentsPrefix), "/")
		if len(parts) == 0 || parts[0] == "" {
			return "", "", kindNone
		}
		// _resolved/<slug>/... vs <slug>/...
		if parts[0] == "_resolved" {
			if len(parts) < 2 || parts[1] == "" {
				return "", "", kindNone
			}
			return parts[1], "", kindComment
		}
		return parts[0], "", kindComment
	}

	// A doc change: the path must be a .md matching the configured globs.
	if !strings.HasSuffix(rel, ".md") {
		return "", "", kindNone
	}
	if strings.HasPrefix(rel, ".quibble/") {
		return "", "", kindNone
	}
	for _, g := range s.cfg.Docs {
		if ok, _ := matchGlob(g, rel); ok {
			return slugForRel(rel), rel, kindDoc
		}
	}
	return "", "", kindNone
}

// slugForRel derives a doc slug: drop the extension, replace "/" with "--"
// (conventions §Naming — same pure function used by the renderer and store).
func slugForRel(rel string) string {
	if i := strings.LastIndex(rel, "."); i >= 0 {
		if !strings.Contains(rel[i:], "/") {
			rel = rel[:i]
		}
	}
	return strings.ReplaceAll(rel, "/", "--")
}

// matchGlob mirrors config's `**`-aware matcher for a single path.
func matchGlob(glob, name string) (bool, error) {
	return config.MatchGlob(glob, name)
}

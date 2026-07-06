package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abdullahranginwala/quibble/pkg/comment"
)

// resolvedDirName is the archive subdirectory under .quibble/comments/ where
// resolved threads live.
const resolvedDirName = "_resolved"

// FSStore is the reference CommentStore: one markdown file per thread under
// <projectRoot>/.quibble/comments/. Open and addressed threads live in
// comments/<doc-slug>/; resolving a thread moves its file to
// comments/_resolved/<doc-slug>/ (and reopening moves it back), so an agent
// listing a doc's directory sees only actionable threads (DESIGN.md §3).
//
// Concurrency: an in-process per-ID mutex serializes Create, Reply and
// SetStatus for a given thread, so concurrent callers within one process are
// safe. Cross-process safety is deliberately NOT provided here — the source of
// truth is git, and concurrent writers on different machines/processes reconcile
// through ordinary git merges (DESIGN.md §3). FSStore is not safe to share
// across processes writing the same repo simultaneously.
type FSStore struct {
	root        string // project root (contains .quibble/)
	commentsDir string // <root>/.quibble/comments

	mu       sync.Mutex             // guards muxes and warnings
	muxes    map[string]*sync.Mutex // per-ID locks
	warnings []string               // corrupt paths from the most recent List
}

// compile-time check that FSStore implements the interface.
var _ CommentStore = (*FSStore)(nil)

// NewFS returns a store rooted at projectRoot. It requires <projectRoot>/.quibble/
// to already exist (created by `quibble init`); per-doc comment directories are
// created lazily on first write.
func NewFS(projectRoot string) (*FSStore, error) {
	qdir := filepath.Join(projectRoot, ".quibble")
	fi, err := os.Stat(qdir)
	if err != nil {
		return nil, fmt.Errorf("store: .quibble not found in %s: %w", projectRoot, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("store: %s exists but is not a directory", qdir)
	}
	return &FSStore{
		root:        projectRoot,
		commentsDir: filepath.Join(qdir, "comments"),
		muxes:       make(map[string]*sync.Mutex),
	}, nil
}

// Warnings returns the corrupt/unreadable paths encountered during the most
// recent List call. It is an FSStore extra (not part of CommentStore) consumed
// by `quibble doctor`; the interface stays clean.
func (s *FSStore) Warnings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.warnings))
	copy(out, s.warnings)
	return out
}

// lockID acquires (creating if needed) the per-ID mutex and returns its unlock.
func (s *FSStore) lockID(id string) func() {
	s.mu.Lock()
	m := s.muxes[id]
	if m == nil {
		m = &sync.Mutex{}
		s.muxes[id] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func (s *FSStore) addWarning(msg string) {
	s.mu.Lock()
	s.warnings = append(s.warnings, msg)
	s.mu.Unlock()
}

// dirFor returns the directory that holds a thread for the given doc and status.
func (s *FSStore) dirFor(doc string, st comment.Status) string {
	slug := slugFor(doc)
	if st == comment.StatusResolved {
		return filepath.Join(s.commentsDir, resolvedDirName, slug)
	}
	return filepath.Join(s.commentsDir, slug)
}

// pathFor returns the on-disk path a thread would occupy for the given
// doc/id/status.
func (s *FSStore) pathFor(doc, id string, st comment.Status) string {
	return filepath.Join(s.dirFor(doc, st), id+".md")
}

// locate finds a thread's file by ID, searching open doc directories first and
// then the _resolved archive. Returns ErrNotFound if absent.
func (s *FSStore) locate(id string) (string, error) {
	name := id + ".md"

	entries, err := os.ReadDir(s.commentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("store: reading %s: %w", s.commentsDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == resolvedDirName {
			continue
		}
		p := filepath.Join(s.commentsDir, e.Name(), name)
		if fileExists(p) {
			return p, nil
		}
	}

	resolvedRoot := filepath.Join(s.commentsDir, resolvedDirName)
	rentries, err := os.ReadDir(resolvedRoot)
	if err == nil {
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
	return "", ErrNotFound
}

// Create implements CommentStore.
func (s *FSStore) Create(ctx context.Context, t *comment.Thread) error {
	if err := t.Validate(); err != nil {
		return err
	}
	unlock := s.lockID(t.ID)
	defer unlock()

	if _, err := s.locate(t.ID); err == nil {
		return ErrExists
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	return writeThread(s.pathFor(t.Doc, t.ID, t.Status), t)
}

// Get implements CommentStore.
func (s *FSStore) Get(ctx context.Context, id string) (*comment.Thread, error) {
	p, err := s.locate(id)
	if err != nil {
		return nil, err
	}
	return readThread(p)
}

// Reply implements CommentStore.
func (s *FSStore) Reply(ctx context.Context, id string, r comment.Reply) error {
	unlock := s.lockID(id)
	defer unlock()

	p, err := s.locate(id)
	if err != nil {
		return err
	}
	t, err := readThread(p)
	if err != nil {
		return err
	}
	t.Replies = append(t.Replies, r)
	return writeThread(p, t)
}

// SetStatus implements CommentStore.
func (s *FSStore) SetStatus(ctx context.Context, id string, st comment.Status, actor string) error {
	unlock := s.lockID(id)
	defer unlock()

	p, err := s.locate(id)
	if err != nil {
		return err
	}
	t, err := readThread(p)
	if err != nil {
		return err
	}

	from := t.Status
	if from == st {
		return nil // same-status no-op success
	}
	if !legalTransition(from, st) {
		return ErrTransition
	}

	t.Status = st
	switch {
	case st == comment.StatusResolved:
		now := time.Now()
		t.ResolvedBy = actor
		t.ResolvedAt = &now
	case from == comment.StatusResolved: // reopen clears the resolution stamp
		t.ResolvedBy = ""
		t.ResolvedAt = nil
	}

	newPath := s.pathFor(t.Doc, id, st)
	if newPath == p {
		return writeThread(p, t)
	}
	// Physical move: write to the new location, then remove the old file.
	if err := writeThread(newPath, t); err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("store: removing old thread %s: %w", p, err)
	}
	return nil
}

// List implements CommentStore.
func (s *FSStore) List(ctx context.Context, f Filter) ([]*comment.Thread, error) {
	s.mu.Lock()
	s.warnings = nil
	s.mu.Unlock()

	var all []*comment.Thread
	corrupt := 0

	scanDir := func(dir string) {
		files, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, fe := range files {
			if fe.IsDir() || !strings.HasSuffix(fe.Name(), ".md") {
				continue
			}
			p := filepath.Join(dir, fe.Name())
			data, err := os.ReadFile(p)
			if err != nil {
				s.addWarning(fmt.Sprintf("%s: %v", p, err))
				corrupt++
				continue
			}
			t, err := comment.ParseThread(data)
			if err != nil {
				s.addWarning(fmt.Sprintf("%s: %v", p, err))
				corrupt++
				continue
			}
			all = append(all, t)
		}
	}

	entries, err := os.ReadDir(s.commentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*comment.Thread{}, nil
		}
		return nil, fmt.Errorf("store: reading %s: %w", s.commentsDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == resolvedDirName {
			resolvedRoot := filepath.Join(s.commentsDir, resolvedDirName)
			rentries, err := os.ReadDir(resolvedRoot)
			if err != nil {
				continue
			}
			for _, re := range rentries {
				if re.IsDir() {
					scanDir(filepath.Join(resolvedRoot, re.Name()))
				}
			}
			continue
		}
		scanDir(filepath.Join(s.commentsDir, e.Name()))
	}

	// Error only if nothing at all was readable (files existed but all corrupt).
	if len(all) == 0 && corrupt > 0 {
		return nil, fmt.Errorf("store: no readable threads (%d corrupt)", corrupt)
	}

	out := make([]*comment.Thread, 0, len(all))
	for _, t := range all {
		if matches(t, f) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.Before(out[j].Created)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// matches reports whether t satisfies filter f.
func matches(t *comment.Thread, f Filter) bool {
	if f.Doc != "" && t.Doc != f.Doc {
		return false
	}
	if len(f.Statuses) > 0 {
		ok := false
		for _, st := range f.Statuses {
			if t.Status == st {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// legalTransition reports whether from→to is an allowed lifecycle move. It
// assumes from != to (same-status is handled by the caller as a no-op).
func legalTransition(from, to comment.Status) bool {
	switch from {
	case comment.StatusOpen:
		return to == comment.StatusAddressed || to == comment.StatusResolved
	case comment.StatusAddressed:
		return to == comment.StatusResolved || to == comment.StatusOpen
	case comment.StatusResolved:
		return to == comment.StatusOpen
	}
	return false
}

// slugFor maps a relative doc path to its comment-directory slug per
// conventions §Naming: drop the extension and replace "/" with "--".
//
// Collision note (logged in plan/DECISIONS.md): the mapping is a pure,
// non-injective function. A path segment that already contains "--" can collide
// with a nested path — "a--b.md" and "a/b.md" both slug to "a--b". This is
// acceptable-but-deterministic: identical input always yields identical slug,
// so a doc's threads land consistently; disambiguation is a future concern if
// real repos hit it.
func slugFor(doc string) string {
	doc = strings.TrimSuffix(doc, path.Ext(doc))
	return strings.ReplaceAll(doc, "/", "--")
}

// fileExists reports whether path names an existing regular file.
func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// readThread reads and parses a thread file.
func readThread(p string) (*comment.Thread, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("store: reading thread %s: %w", p, err)
	}
	t, err := comment.ParseThread(data)
	if err != nil {
		return nil, fmt.Errorf("store: parsing thread %s: %w", p, err)
	}
	return t, nil
}

// writeThread atomically writes t to path: marshal, write to "<path>.tmp" in the
// same directory, then rename over. A crash never leaves a half-written thread
// visible, and no ".tmp" remains after success.
func writeThread(p string, t *comment.Thread) error {
	data, err := t.Marshal()
	if err != nil {
		return fmt.Errorf("store: marshaling thread %s: %w", t.ID, err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("store: creating dir for %s: %w", p, err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("store: writing temp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("store: renaming %s to %s: %w", tmp, p, err)
	}
	return nil
}

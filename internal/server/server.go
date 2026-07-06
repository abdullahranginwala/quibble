// Package server implements `quibble serve`: a local, loopback-only HTTP app
// that renders the configured docs, serves the embedded comment UI, and turns
// every UI action into a comment-file write. It watches the docs and the
// .quibble/comments tree and pushes live updates over SSE. No daemon, no state
// outside the repo (DESIGN.md §7).
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing/fstest"
	"unicode/utf8"

	"github.com/abdullahranginwala/quibble/internal/config"
	"github.com/abdullahranginwala/quibble/internal/docbridge"
	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/render"
	"github.com/abdullahranginwala/quibble/pkg/store"
)

// Server holds the rendered-doc cache, the comment store, and the SSE hub. It
// is safe for concurrent use: the doc cache is guarded by an RWMutex and the
// store has its own per-thread locking.
type Server struct {
	root         string
	cfg          *config.Config
	renderer     *render.Renderer
	store        *store.FSStore
	human, agent string

	assets map[string][]byte // shared theme/highlight CSS, keyed "qbl/<name>"

	mu     sync.RWMutex
	bySlug map[string]*docState
	byRel  map[string]*docState

	hub *hub
}

// docState is one rendered document and the anchoring model derived from it.
type docState struct {
	slug     string
	relPath  string
	title    string
	text     string // normalized plain text (rune-offset space)
	blocks   []comment.Block
	sections []comment.Section
	page     []byte
	version  string // fingerprint of text; the docVersion etag
}

// New builds a server rooted at an initialized project. It loads config, builds
// the renderer, and renders every configured doc into the cache.
func New(root string) (*Server, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("server: resolving root %q: %w", root, err)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		return nil, err
	}
	if cfg.Theme.Name != "paper" {
		return nil, fmt.Errorf("server: unknown theme %q (only the built-in \"paper\" theme is available)", cfg.Theme.Name)
	}
	r, err := render.New(render.Options{
		Theme:     render.Paper(),
		Overrides: cfg.Theme.Overrides,
		TOC:       true,
		Title:     "Docs",
	})
	if err != nil {
		return nil, err
	}
	st, err := store.NewFS(abs)
	if err != nil {
		return nil, err
	}
	// Capture the theme/highlight CSS the pages reference, via an empty render.
	site, err := r.RenderDir(fstest.MapFS{})
	if err != nil {
		return nil, err
	}
	s := &Server{
		root:     abs,
		cfg:      cfg,
		renderer: r,
		store:    st,
		assets:   site.Assets,
		human:    cfg.Authors.Human,
		agent:    cfg.Authors.Agent,
		bySlug:   map[string]*docState{},
		byRel:    map[string]*docState{},
		hub:      newHub(),
	}
	if err := s.loadAll(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadAll globs and renders every configured doc into the cache.
func (s *Server) loadAll() error {
	paths, err := config.MatchDocs(os.DirFS(s.root), s.cfg.Docs)
	if err != nil {
		return fmt.Errorf("server: globbing docs: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bySlug = map[string]*docState{}
	s.byRel = map[string]*docState{}
	for _, rel := range paths {
		ds, err := s.renderState(rel)
		if err != nil {
			// A single bad doc is skipped, not fatal — serve keeps running.
			continue
		}
		s.bySlug[ds.slug] = ds
		s.byRel[ds.relPath] = ds
	}
	return nil
}

// renderState reads and renders one doc (by relative path) into a docState.
func (s *Server) renderState(rel string) (*docState, error) {
	src, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("server: reading %s: %w", rel, err)
	}
	rd, err := docbridge.Render(s.renderer, src, rel)
	if err != nil {
		return nil, err
	}
	return &docState{
		slug:     rd.Slug,
		relPath:  rd.RelPath,
		title:    rd.Title,
		text:     rd.Text,
		blocks:   rd.Blocks,
		sections: rd.Sections,
		page:     rd.Page,
		version:  docVersion(rd.Text),
	}, nil
}

// reRender re-reads a doc from disk and swaps it into the cache. A read/render
// failure drops the doc from the cache rather than serving a stale copy.
func (s *Server) reRender(rel string) {
	ds, err := s.renderState(rel)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		if old, ok := s.byRel[rel]; ok {
			delete(s.byRel, rel)
			delete(s.bySlug, old.slug)
		}
		return
	}
	s.bySlug[ds.slug] = ds
	s.byRel[ds.relPath] = ds
}

func (s *Server) docBySlug(slug string) (*docState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ds, ok := s.bySlug[slug]
	return ds, ok
}

func (s *Server) docByRel(rel string) (*docState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ds, ok := s.byRel[rel]
	return ds, ok
}

// docStates returns a snapshot of every cached doc.
func (s *Server) docStates() []*docState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*docState, 0, len(s.bySlug))
	for _, ds := range s.bySlug {
		out = append(out, ds)
	}
	return out
}

// docVersion is the fingerprint used as the optimistic-concurrency etag: the
// first 12 hex chars of sha256(normalized text). Any doc edit changes it, so a
// stale POST (409) is detected without tracking timestamps.
func docVersion(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}

// placementFor maps a rune-span Placement in the doc's text to a block ID and
// in-block rune offsets the client can walk. Orphans return (_, false).
func (ds *docState) placementFor(p comment.Placement) (blockID string, start, end int, ok bool) {
	if p.Method == comment.MethodOrphan || p.Start < 0 {
		return "", 0, 0, false
	}
	for _, b := range ds.blocks {
		if p.Start >= b.Start && p.Start < b.End {
			inStart := p.Start - b.Start
			inEnd := p.End - b.Start
			if bl := utf8.RuneCountInString(b.Text); inEnd > bl {
				inEnd = bl // clamp cross-block/fuzzy overshoot to this block
			}
			return b.ID, inStart, inEnd, true
		}
	}
	return "", 0, 0, false
}

// blockByID returns the block with the given ID, if present.
func (ds *docState) blockByID(id string) (comment.Block, bool) {
	for _, b := range ds.blocks {
		if b.ID == id {
			return b, true
		}
	}
	return comment.Block{}, false
}

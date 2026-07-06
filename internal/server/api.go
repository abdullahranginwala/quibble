package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/store"
	"github.com/abdullahranginwala/quibble/web"
)

// Handler returns the HTTP handler for the app, wrapped in the CSRF guard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /d/{slug}", s.handleDocPage)
	mux.HandleFunc("GET /qbl/{file}", s.handleAsset)
	mux.HandleFunc("GET /d/qbl/{file}", s.handleAsset)
	mux.HandleFunc("GET /api/docs", s.handleDocs)
	mux.HandleFunc("GET /api/docs/{slug}/comments", s.handleComments)
	mux.HandleFunc("POST /api/comments", s.handleCreate)
	mux.HandleFunc("POST /api/comments/{id}/reply", s.handleReply)
	mux.HandleFunc("POST /api/comments/{id}/status", s.handleStatus)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	return csrfGuard(mux)
}

// csrfGuard rejects any state-changing request that lacks the X-Qbl header the
// embedded UI sets. The server writes files, so an unadorned cross-origin POST
// must never reach a handler (test row 8).
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("X-Qbl") != "1" {
				writeError(w, http.StatusForbidden, "missing X-Qbl header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// --- JSON error + decode helpers ---

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSON enforces the JSON content type and decodes the body. A bad content
// type or malformed body is a 400.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	ct := r.Header.Get("Content-Type")
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	if strings.TrimSpace(ct) != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "reading body: "+err.Error())
		return false
	}
	if err := json.Unmarshal(body, dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// --- assets ---

func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if data, ok := s.assets["qbl/"+file]; ok {
		serveBytes(w, file, data)
		return
	}
	switch file {
	case "comments.js", "anchor-render.js", "ui.css":
		data, err := web.Assets.ReadFile(file)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		serveBytes(w, file, data)
	default:
		http.NotFound(w, r)
	}
}

func serveBytes(w http.ResponseWriter, name string, data []byte) {
	w.Header().Set("Content-Type", contentType(name))
	_, _ = w.Write(data)
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// --- pages ---

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	docs := s.docStates()
	// Stable order by slug.
	sortDocStates(docs)
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString(`<title>Docs · quibble</title>`)
	b.WriteString(`<link rel="stylesheet" href="/qbl/tokens.css"><link rel="stylesheet" href="/qbl/theme.css">`)
	b.WriteString(`<link rel="stylesheet" href="/qbl/quibble.css"><link rel="stylesheet" href="/qbl/ui.css">`)
	b.WriteString(`</head><body><header class="qbl-header"><div class="qbl-header-inner">`)
	b.WriteString(`<span class="qbl-site">Docs</span></div></header>`)
	b.WriteString(`<main class="qbl-main"><article class="qbl-prose"><h1>Documents</h1><ul class="qbl-index">`)
	for _, ds := range docs {
		o, a := s.counts(ds.relPath)
		fmt.Fprintf(&b,
			`<li><a href="/d/%s">%s</a> <span class="qbl-index-path">%s</span> `+
				`<span class="qbl-badge">%d open</span> <span class="qbl-badge">%d addressed</span></li>`,
			htmlEscape(ds.slug), htmlEscape(ds.title), htmlEscape(ds.relPath), o, a)
	}
	b.WriteString(`</ul></article></main></body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) handleDocPage(w http.ResponseWriter, r *http.Request) {
	ds, ok := s.docBySlug(r.PathValue("slug"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("ETag", `"`+ds.version+`"`)
	_, _ = w.Write(ds.page)
}

// --- /api/docs ---

type docSummary struct {
	Slug           string `json:"slug"`
	RelPath        string `json:"relPath"`
	Title          string `json:"title"`
	OpenCount      int    `json:"openCount"`
	AddressedCount int    `json:"addressedCount"`
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	docs := s.docStates()
	sortDocStates(docs)
	out := make([]docSummary, 0, len(docs))
	for _, ds := range docs {
		o, a := s.counts(ds.relPath)
		out = append(out, docSummary{
			Slug:           ds.slug,
			RelPath:        ds.relPath,
			Title:          ds.title,
			OpenCount:      o,
			AddressedCount: a,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// counts returns the open and addressed thread counts for a doc.
func (s *Server) counts(relPath string) (open, addressed int) {
	threads, err := s.store.List(context.Background(), store.Filter{Doc: relPath})
	if err != nil {
		return 0, 0
	}
	for _, t := range threads {
		switch t.Status {
		case comment.StatusOpen:
			open++
		case comment.StatusAddressed:
			addressed++
		}
	}
	return open, addressed
}

// --- /api/docs/{slug}/comments ---

type placementJSON struct {
	BlockID    string  `json:"blockId"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
}

type threadWithPlacement struct {
	threadJSON
	Placement *placementJSON `json:"placement"`
}

type commentsResponse struct {
	Slug       string                `json:"slug"`
	RelPath    string                `json:"relPath"`
	DocVersion string                `json:"docVersion"`
	Open       int                   `json:"open"`
	Addressed  int                   `json:"addressed"`
	Threads    []threadWithPlacement `json:"threads"`
}

func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	ds, ok := s.docBySlug(r.PathValue("slug"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such document")
		return
	}
	includeResolved := r.URL.Query().Get("include") == "resolved"

	threads, err := s.store.List(context.Background(), store.Filter{Doc: ds.relPath})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := commentsResponse{
		Slug:       ds.slug,
		RelPath:    ds.relPath,
		DocVersion: ds.version,
		Threads:    []threadWithPlacement{},
	}
	for _, t := range threads {
		switch t.Status {
		case comment.StatusOpen:
			resp.Open++
		case comment.StatusAddressed:
			resp.Addressed++
		case comment.StatusResolved:
			if !includeResolved {
				continue // resolved excluded from the default view
			}
		}
		item := threadWithPlacement{threadJSON: toThreadJSON(t)}
		// Resolved threads carry no placement (they leave the reading view).
		if t.Status != comment.StatusResolved {
			if p := s.place(ds, t); p != nil {
				item.Placement = p
			}
		}
		resp.Threads = append(resp.Threads, item)
	}
	writeJSON(w, http.StatusOK, resp)
}

// place re-anchors a thread against the current doc text and maps it to a
// block. Returns nil for orphans (client renders them in the orphan panel).
func (s *Server) place(ds *docState, t *comment.Thread) *placementJSON {
	p := comment.Resolve(ds.text, ds.sections, t.Anchor)
	blockID, start, end, ok := ds.placementFor(p)
	if !ok {
		return nil
	}
	return &placementJSON{
		BlockID:    blockID,
		Start:      start,
		End:        end,
		Method:     string(p.Method),
		Confidence: p.Confidence,
	}
}

// --- POST /api/comments ---

type createRequest struct {
	Doc        string `json:"doc"`
	BlockID    string `json:"blockId"`
	QuoteStart int    `json:"quoteStart"`
	QuoteEnd   int    `json:"quoteEnd"`
	Quote      string `json:"quote"`
	Body       string `json:"body"`
	Author     string `json:"author"`
	DocVersion string `json:"docVersion"`
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Doc == "" || req.BlockID == "" {
		writeError(w, http.StatusBadRequest, "doc and blockId are required")
		return
	}
	ds, ok := s.docByRel(req.Doc)
	if !ok {
		writeError(w, http.StatusNotFound, "no such document")
		return
	}
	// Optimistic concurrency: the client anchored against a specific doc
	// version; if the doc changed underneath, make it re-fetch (row 4).
	if req.DocVersion != "" && req.DocVersion != ds.version {
		writeError(w, http.StatusConflict, "document changed; re-fetch and retry")
		return
	}
	block, ok := ds.blockByID(req.BlockID)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown blockId")
		return
	}
	blockLen := utf8.RuneCountInString(block.Text)
	if req.QuoteStart < 0 || req.QuoteEnd > blockLen || req.QuoteStart >= req.QuoteEnd {
		writeError(w, http.StatusBadRequest, "quote offsets out of range")
		return
	}
	absStart := block.Start + req.QuoteStart
	absEnd := block.Start + req.QuoteEnd

	// Never trust client offsets: the text the offsets select must equal the
	// quote the client claims to have selected (row 5).
	runes := []rune(ds.text)
	selected := string(runes[absStart:absEnd])
	if selected != req.Quote {
		writeError(w, http.StatusBadRequest, "quote does not match the selected offsets")
		return
	}

	author := req.Author
	if author == "" {
		author = s.human
	}
	anchor := comment.NewAnchor(ds.text, ds.sections, absStart, absEnd)
	t := &comment.Thread{
		ID:      comment.NewID(),
		Doc:     ds.relPath,
		Status:  comment.StatusOpen,
		Created: time.Now(),
		Author:  author,
		Anchor:  anchor,
		Body:    req.Body,
	}
	if err := s.store.Create(context.Background(), t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.broadcast("comments-changed", map[string]string{"slug": ds.slug})

	item := threadWithPlacement{threadJSON: toThreadJSON(t)}
	if p := s.place(ds, t); p != nil {
		item.Placement = p
	}
	writeJSON(w, http.StatusCreated, item)
}

// --- POST /api/comments/{id}/reply ---

type replyRequest struct {
	Body   string `json:"body"`
	Author string `json:"author"`
}

func (s *Server) handleReply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req replyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "reply body is required")
		return
	}
	ctx := context.Background()
	t, err := s.store.Get(ctx, id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	author := req.Author
	if author == "" {
		author = s.human
	}
	if err := s.store.Reply(ctx, id, comment.Reply{Author: author, Time: time.Now(), Body: req.Body}); err != nil {
		s.storeError(w, err)
		return
	}
	updated, err := s.store.Get(ctx, id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	s.broadcastForDoc(t.Doc)
	writeJSON(w, http.StatusOK, toThreadJSON(updated))
}

// --- POST /api/comments/{id}/status ---

type statusRequest struct {
	Status string `json:"status"`
	Author string `json:"author"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req statusRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	st := comment.Status(req.Status)
	switch st {
	case comment.StatusOpen, comment.StatusAddressed, comment.StatusResolved:
	default:
		writeError(w, http.StatusBadRequest, "unknown status")
		return
	}
	author := req.Author
	if author == "" {
		author = s.human
	}
	// Policy gate mirrors the CLI: agents address, humans resolve (DESIGN §4).
	if st == comment.StatusResolved && author == s.agent {
		writeError(w, http.StatusForbidden, "agents address; humans resolve")
		return
	}
	ctx := context.Background()
	t, err := s.store.Get(ctx, id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.store.SetStatus(ctx, id, st, author); err != nil {
		s.storeError(w, err)
		return
	}
	updated, err := s.store.Get(ctx, id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	s.broadcastForDoc(t.Doc)
	writeJSON(w, http.StatusOK, toThreadJSON(updated))
}

// broadcastForDoc notifies clients that a doc's comments changed, keyed by slug.
func (s *Server) broadcastForDoc(relPath string) {
	if ds, ok := s.docByRel(relPath); ok {
		s.hub.broadcast("comments-changed", map[string]string{"slug": ds.slug})
	}
}

// storeError maps store sentinel errors to HTTP statuses.
func (s *Server) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such thread")
	case errors.Is(err, store.ErrTransition):
		writeError(w, http.StatusConflict, "illegal status transition")
	case errors.Is(err, store.ErrExists):
		writeError(w, http.StatusConflict, "thread already exists")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// htmlEscape escapes text for embedding in the index HTML.
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

func sortDocStates(docs []*docState) {
	for i := 1; i < len(docs); i++ {
		for j := i; j > 0 && docs[j-1].slug > docs[j].slug; j-- {
			docs[j-1], docs[j] = docs[j], docs[j-1]
		}
	}
}

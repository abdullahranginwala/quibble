package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/store"
)

const testConfig = `docs:
  - "*.md"
theme:
  name: paper
  overrides: {}
authors:
  human: human
  agent: agent
`

const docOne = `# First Doc

The quick brown fox jumps over the lazy dog.

Second paragraph with unique marker zebra content.
`

// newProject writes a 3-doc initialized project and returns its root.
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, ".quibble"))
	mustWrite(t, filepath.Join(root, ".quibble", "config.yml"), testConfig)
	mustWrite(t, filepath.Join(root, "one.md"), docOne)
	mustWrite(t, filepath.Join(root, "two.md"), "# Second Doc\n\nSome body text here.\n")
	mustWrite(t, filepath.Join(root, "three.md"), "# Third Doc\n\nMore content lives here.\n")
	return root
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newServer(t *testing.T, root string) *Server {
	t.Helper()
	s, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// addThread writes an open thread anchored to exact on doc, via the store.
func addThread(t *testing.T, root, doc, author string, status comment.Status, exact string) *comment.Thread {
	t.Helper()
	st, err := store.NewFS(root)
	if err != nil {
		t.Fatal(err)
	}
	th := &comment.Thread{
		ID:      comment.NewID(),
		Doc:     doc,
		Status:  comment.StatusOpen,
		Created: time.Now(),
		Author:  author,
		Anchor:  comment.Anchor{Exact: exact},
		Body:    "comment on " + exact,
	}
	if err := st.Create(context.Background(), th); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if status != comment.StatusOpen {
		if err := st.SetStatus(context.Background(), th.ID, status, author); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
	}
	return th
}

func doGET(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doPOST(t *testing.T, h http.Handler, path string, body any, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if csrf {
		req.Header.Set("X-Qbl", "1")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func countThreadFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(filepath.Join(root, ".quibble", "comments"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
			n++
		}
		return nil
	})
	return n
}

// --- Row 1 ---

func TestRow01_DocsList(t *testing.T) {
	root := newProject(t)
	addThread(t, root, "one.md", "human", comment.StatusOpen, "quick brown fox")
	addThread(t, root, "one.md", "agent", comment.StatusAddressed, "lazy dog")
	addThread(t, root, "three.md", "human", comment.StatusOpen, "More content")
	s := newServer(t, root)

	rec := doGET(t, s.Handler(), "/api/docs")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var docs []docSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("got %d docs, want 3", len(docs))
	}
	by := map[string]docSummary{}
	for _, d := range docs {
		by[d.Slug] = d
	}
	if by["one"].Title != "First Doc" {
		t.Errorf("one title = %q", by["one"].Title)
	}
	if by["one"].OpenCount != 1 || by["one"].AddressedCount != 1 {
		t.Errorf("one counts = %d/%d, want 1/1", by["one"].OpenCount, by["one"].AddressedCount)
	}
	if by["two"].OpenCount != 0 || by["two"].AddressedCount != 0 {
		t.Errorf("two counts = %d/%d, want 0/0", by["two"].OpenCount, by["two"].AddressedCount)
	}
	if by["three"].OpenCount != 1 {
		t.Errorf("three open = %d, want 1", by["three"].OpenCount)
	}
	if by["three"].RelPath != "three.md" {
		t.Errorf("three relPath = %q", by["three"].RelPath)
	}
}

// --- Row 2 ---

func TestRow02_Placements(t *testing.T) {
	root := newProject(t)
	exact := addThread(t, root, "one.md", "human", comment.StatusOpen, "quick brown fox")
	fuzzy := addThread(t, root, "one.md", "human", comment.StatusOpen, "quick brown fax jumps")
	orphan := addThread(t, root, "one.md", "human", comment.StatusOpen, "this text does not appear anywhere")
	s := newServer(t, root)

	rec := doGET(t, s.Handler(), "/api/docs/one/comments")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp commentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	pl := map[string]*placementJSON{}
	for _, tw := range resp.Threads {
		pl[tw.ID] = tw.Placement
	}
	if len(resp.Threads) != 3 {
		t.Fatalf("got %d threads, want 3", len(resp.Threads))
	}
	if p := pl[exact.ID]; p == nil || p.Method != "exact" {
		t.Errorf("exact placement = %+v, want method exact", p)
	} else {
		ds, _ := s.docBySlug("one")
		block, _ := ds.blockByID(p.BlockID)
		got := sliceRunesStr(block.Text, p.Start, p.End)
		if got != "quick brown fox" {
			t.Errorf("exact placement text = %q", got)
		}
	}
	if p := pl[fuzzy.ID]; p == nil || p.Method != "fuzzy" {
		t.Errorf("fuzzy placement = %+v, want method fuzzy", p)
	} else if p.Confidence <= 0.75 || p.Confidence > 1.0 {
		t.Errorf("fuzzy confidence = %v", p.Confidence)
	}
	if p, ok := pl[orphan.ID]; !ok || p != nil {
		t.Errorf("orphan placement = %+v, want null", p)
	}
}

// --- Row 3 ---

func TestRow03_CreateComment(t *testing.T) {
	root := newProject(t)
	s := newServer(t, root)
	ds, _ := s.docBySlug("one")
	block := blockContaining(t, ds, "quick brown fox")
	qStart, qEnd, quote := runeSpan(block.Text, "quick brown fox")

	before := countThreadFiles(t, root)
	rec := doPOST(t, s.Handler(), "/api/comments", map[string]any{
		"doc":        "one.md",
		"blockId":    block.ID,
		"quoteStart": qStart,
		"quoteEnd":   qEnd,
		"quote":      quote,
		"body":       "is this fox lazy too?",
		"docVersion": ds.version,
	}, true)
	if rec.Code != 201 {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if countThreadFiles(t, root) != before+1 {
		t.Fatalf("no thread file written")
	}
	var item threadWithPlacement
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Anchor.Exact != quote {
		t.Errorf("anchor.exact = %q, want %q", item.Anchor.Exact, quote)
	}
	// Verify the on-disk thread matches too.
	st, _ := store.NewFS(root)
	got, err := st.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Anchor.Exact != quote {
		t.Errorf("stored anchor.exact = %q", got.Anchor.Exact)
	}
}

// --- Row 4 ---

func TestRow04_StaleDocVersion(t *testing.T) {
	root := newProject(t)
	s := newServer(t, root)
	ds, _ := s.docBySlug("one")
	block := blockContaining(t, ds, "quick brown fox")
	qStart, qEnd, quote := runeSpan(block.Text, "quick brown fox")

	before := countThreadFiles(t, root)
	rec := doPOST(t, s.Handler(), "/api/comments", map[string]any{
		"doc":        "one.md",
		"blockId":    block.ID,
		"quoteStart": qStart,
		"quoteEnd":   qEnd,
		"quote":      quote,
		"body":       "x",
		"docVersion": "staleversion",
	}, true)
	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if countThreadFiles(t, root) != before {
		t.Fatalf("stale POST wrote a file")
	}
}

// --- Row 5 ---

func TestRow05_QuoteMismatch(t *testing.T) {
	root := newProject(t)
	s := newServer(t, root)
	ds, _ := s.docBySlug("one")
	block := blockContaining(t, ds, "quick brown fox")
	qStart, qEnd, _ := runeSpan(block.Text, "quick brown fox")

	before := countThreadFiles(t, root)
	rec := doPOST(t, s.Handler(), "/api/comments", map[string]any{
		"doc":        "one.md",
		"blockId":    block.ID,
		"quoteStart": qStart,
		"quoteEnd":   qEnd,
		"quote":      "totally different text", // offsets don't match this
		"body":       "x",
		"docVersion": ds.version,
	}, true)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if countThreadFiles(t, root) != before {
		t.Fatalf("mismatched POST wrote a file")
	}
}

// --- Row 6 ---

func TestRow06_ResolveAsHuman(t *testing.T) {
	root := newProject(t)
	th := addThread(t, root, "one.md", "human", comment.StatusOpen, "quick brown fox")
	s := newServer(t, root)

	rec := doPOST(t, s.Handler(), "/api/comments/"+th.ID+"/status",
		map[string]any{"status": "resolved", "author": "human"}, true)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	moved := filepath.Join(root, ".quibble", "comments", "_resolved", "one", th.ID+".md")
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("thread not moved to _resolved: %v", err)
	}
	old := filepath.Join(root, ".quibble", "comments", "one", th.ID+".md")
	if _, err := os.Stat(old); err == nil {
		t.Fatalf("thread still in open dir")
	}
}

// --- Row 7 ---

func TestRow07_ResolveAsAgentForbidden(t *testing.T) {
	root := newProject(t)
	th := addThread(t, root, "one.md", "human", comment.StatusOpen, "quick brown fox")
	s := newServer(t, root)

	rec := doPOST(t, s.Handler(), "/api/comments/"+th.ID+"/status",
		map[string]any{"status": "resolved", "author": "agent"}, true)
	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	// Unchanged: still in the open directory.
	old := filepath.Join(root, ".quibble", "comments", "one", th.ID+".md")
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("thread should be unchanged in open dir: %v", err)
	}
}

// --- Row 8 ---

func TestRow08_MissingCSRF(t *testing.T) {
	root := newProject(t)
	th := addThread(t, root, "one.md", "human", comment.StatusOpen, "quick brown fox")
	s := newServer(t, root)

	rec := doPOST(t, s.Handler(), "/api/comments/"+th.ID+"/reply",
		map[string]any{"body": "hi"}, false) // no X-Qbl
	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (CSRF)", rec.Code)
	}
}

// --- Row 9 ---

func TestRow09_LoopbackBind(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr type %T", ln.Addr())
	}
	if !tcp.IP.IsLoopback() {
		t.Fatalf("bound to non-loopback %s", tcp.IP)
	}
}

// --- Row 10 ---

func TestRow10_DocEditSSE(t *testing.T) {
	root := newProject(t)
	s := newServer(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	// Cancel (which unblocks the SSE handler) must run before Close, or Close
	// waits forever on the open event-stream connection.
	defer func() { cancel(); ts.Close() }()

	events := sseClient(t, ctx, ts.URL)
	waitForClient(t, s)

	mustWrite(t, filepath.Join(root, "two.md"), "# Second Doc\n\nEDITED body text now.\n")

	ev := waitEvent(t, events, "doc-changed", 2*time.Second)
	if ev["slug"] != "two" {
		t.Fatalf("doc-changed slug = %q", ev["slug"])
	}
	// GET reflects the edit.
	rec := doGET(t, s.Handler(), "/d/two")
	if !strings.Contains(rec.Body.String(), "EDITED body text now.") {
		t.Fatalf("doc page not updated")
	}
}

// --- Row 11 ---

func TestRow11_ThreadAddedSSE(t *testing.T) {
	root := newProject(t)
	s := newServer(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	// Cancel (which unblocks the SSE handler) must run before Close, or Close
	// waits forever on the open event-stream connection.
	defer func() { cancel(); ts.Close() }()

	events := sseClient(t, ctx, ts.URL)
	waitForClient(t, s)

	th := addThread(t, root, "one.md", "human", comment.StatusOpen, "quick brown fox")

	waitEvent(t, events, "comments-changed", 2*time.Second)

	rec := doGET(t, s.Handler(), "/api/docs/one/comments")
	var resp commentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tw := range resp.Threads {
		if tw.ID == th.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("new thread not in comments listing")
	}
}

// --- Row 12 ---

func TestRow12_DebounceSingleEvent(t *testing.T) {
	root := newProject(t)
	s := newServer(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	// Cancel (which unblocks the SSE handler) must run before Close, or Close
	// waits forever on the open event-stream connection.
	defer func() { cancel(); ts.Close() }()

	events := sseClient(t, ctx, ts.URL)
	waitForClient(t, s)

	// Two rapid saves within the debounce window.
	mustWrite(t, filepath.Join(root, "two.md"), "# Second Doc\n\nfirst quick save.\n")
	mustWrite(t, filepath.Join(root, "two.md"), "# Second Doc\n\nsecond quick save.\n")

	// Collect doc-changed events for 'two' over ~700ms; expect exactly one.
	deadline := time.After(700 * time.Millisecond)
	count := 0
loop:
	for {
		select {
		case ev := <-events:
			if ev.name == "doc-changed" && ev.data["slug"] == "two" {
				count++
			}
		case <-deadline:
			break loop
		}
	}
	if count != 1 {
		t.Fatalf("got %d doc-changed events, want 1 (debounce)", count)
	}
}

// --- Row 13 ---

func TestRow13_ConcurrentReplyAndRerender(t *testing.T) {
	root := newProject(t)
	th := addThread(t, root, "one.md", "human", comment.StatusOpen, "quick brown fox")
	s := newServer(t, root)

	var wg sync.WaitGroup
	// Re-render the doc repeatedly (write lock on the cache).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			s.reRender("one.md")
		}
	}()
	// Concurrently post a reply (store path).
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec := doPOST(t, s.Handler(), "/api/comments/"+th.ID+"/reply",
			map[string]any{"body": "concurrent reply"}, true)
		if rec.Code != 200 {
			t.Errorf("reply status = %d", rec.Code)
		}
	}()
	// And concurrent reads.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			doGET(t, s.Handler(), "/api/docs/one/comments")
		}
	}()
	wg.Wait()

	st, _ := store.NewFS(root)
	got, err := st.Get(context.Background(), th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Replies) != 1 || got.Replies[0].Body != "concurrent reply" {
		t.Fatalf("reply did not land: %+v", got.Replies)
	}
	if _, ok := s.docBySlug("one"); !ok {
		t.Fatalf("doc missing after concurrent re-render")
	}
}

// --- extra: content-type + doc etag ---

func TestExtra_ContentTypeEnforced(t *testing.T) {
	root := newProject(t)
	s := newServer(t, root)
	req := httptest.NewRequest(http.MethodPost, "/api/comments", strings.NewReader("{}"))
	req.Header.Set("X-Qbl", "1")
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

// --- helpers for placement math ---

func sliceRunesStr(s string, start, end int) string {
	r := []rune(s)
	if start < 0 {
		start = 0
	}
	if end > len(r) {
		end = len(r)
	}
	return string(r[start:end])
}

func blockContaining(t *testing.T, ds *docState, sub string) comment.Block {
	t.Helper()
	for _, b := range ds.blocks {
		if strings.Contains(b.Text, sub) {
			return b
		}
	}
	t.Fatalf("no block contains %q", sub)
	return comment.Block{}
}

// runeSpan returns the rune offsets of sub within text plus the substring.
func runeSpan(text, sub string) (start, end int, quote string) {
	bi := strings.Index(text, sub)
	if bi < 0 {
		return 0, 0, ""
	}
	start = utf8.RuneCountInString(text[:bi])
	end = start + utf8.RuneCountInString(sub)
	return start, end, sub
}

// --- SSE test client ---

type sseEvent struct {
	name string
	data map[string]string
}

func sseClient(t *testing.T, ctx context.Context, base string) chan sseEvent {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan sseEvent, 32)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		var name string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				var m map[string]string
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m)
				select {
				case ch <- sseEvent{name: name, data: m}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch
}

func waitForClient(t *testing.T, s *Server) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if s.hub.clientCount() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("SSE client never subscribed")
}

func waitEvent(t *testing.T, ch chan sseEvent, name string, d time.Duration) map[string]string {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case ev := <-ch:
			if ev.name == name {
				return ev.data
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s event", name)
			return nil
		}
	}
}

var _ = fmt.Sprintf

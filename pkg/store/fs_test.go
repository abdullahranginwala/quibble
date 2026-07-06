package store_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/store"
	"github.com/abdullahranginwala/quibble/pkg/store/storetest"
)

// TestFSStoreConformance runs the full CommentStore conformance suite against
// the filesystem reference implementation (M3 rows 1-10, 12-16, 18, 19).
func TestFSStoreConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) store.CommentStore {
		s, _ := newFS(t)
		return s
	})
}

// newFS creates a fresh FSStore in a temp project root with .quibble/ present.
func newFS(t *testing.T) (*store.FSStore, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".quibble"), 0o755); err != nil {
		t.Fatalf("mkdir .quibble: %v", err)
	}
	s, err := store.NewFS(root)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	return s, root
}

var testTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func fsThread(id, doc string) *comment.Thread {
	return &comment.Thread{
		ID:      id,
		Doc:     doc,
		Status:  comment.StatusOpen,
		Created: testTime,
		Author:  "abdullah",
		Anchor:  comment.Anchor{Exact: "some quoted text", Position: 7},
		Body:    "body of " + id,
	}
}

func TestNewFSRequiresQuibbleDir(t *testing.T) {
	root := t.TempDir() // no .quibble/
	if _, err := store.NewFS(root); err == nil {
		t.Fatal("NewFS without .quibble/: got nil error, want error")
	}
}

// Row 11 (fs-only): a resolved thread's file lives physically under
// _resolved/<slug>/ and is gone from the open directory.
func TestRow11_ResolvedThreadLocation(t *testing.T) {
	s, root := newFS(t)
	ctx := context.Background()
	th := fsThread("qbl-aaaaaa", "docs/plan.md")
	if err := s.Create(ctx, th); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetStatus(ctx, "qbl-aaaaaa", comment.StatusResolved, "abdullah"); err != nil {
		t.Fatalf("SetStatus resolved: %v", err)
	}

	openPath := filepath.Join(root, ".quibble", "comments", "docs--plan", "qbl-aaaaaa.md")
	resolvedPath := filepath.Join(root, ".quibble", "comments", "_resolved", "docs--plan", "qbl-aaaaaa.md")
	if _, err := os.Stat(resolvedPath); err != nil {
		t.Fatalf("resolved file not at %s: %v", resolvedPath, err)
	}
	if _, err := os.Stat(openPath); !os.IsNotExist(err) {
		t.Fatalf("open-dir file still present at %s (err=%v)", openPath, err)
	}
}

// Row 12 (fs-only assert): reopening a resolved thread moves the file back to
// the open directory; the archived copy is removed. (ResolvedBy/At clearing is
// asserted store-agnostically in storetest.Run.)
func TestRow12_ReopenMovesFileBack(t *testing.T) {
	s, root := newFS(t)
	ctx := context.Background()
	if err := s.Create(ctx, fsThread("qbl-aaaaaa", "docs/plan.md")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetStatus(ctx, "qbl-aaaaaa", comment.StatusResolved, "abdullah"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := s.SetStatus(ctx, "qbl-aaaaaa", comment.StatusOpen, "abdullah"); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	openPath := filepath.Join(root, ".quibble", "comments", "docs--plan", "qbl-aaaaaa.md")
	resolvedPath := filepath.Join(root, ".quibble", "comments", "_resolved", "docs--plan", "qbl-aaaaaa.md")
	if _, err := os.Stat(openPath); err != nil {
		t.Fatalf("reopened file not back at %s: %v", openPath, err)
	}
	if _, err := os.Stat(resolvedPath); !os.IsNotExist(err) {
		t.Fatalf("archived copy still present at %s (err=%v)", resolvedPath, err)
	}
	// The reopened file on disk must not carry a resolution stamp.
	data, err := os.ReadFile(openPath)
	if err != nil {
		t.Fatalf("read reopened file: %v", err)
	}
	if strings.Contains(string(data), "resolved_by") || strings.Contains(string(data), "resolved_at") {
		t.Fatalf("reopened file still contains resolution keys:\n%s", data)
	}
}

// Row 17 (fs-only): a corrupt file is skipped by List, the good threads are
// returned, and Warnings() names the bad path.
func TestRow17_CorruptFileWarnings(t *testing.T) {
	s, root := newFS(t)
	ctx := context.Background()
	if err := s.Create(ctx, fsThread("qbl-aaaaaa", "docs/plan.md")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	badPath := filepath.Join(root, ".quibble", "comments", "docs--plan", "qbl-broken.md")
	if err := os.WriteFile(badPath, []byte("not a thread file at all"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	got, err := s.List(ctx, store.Filter{})
	if err != nil {
		t.Fatalf("List with corrupt file: %v", err)
	}
	if len(got) != 1 || got[0].ID != "qbl-aaaaaa" {
		t.Fatalf("List: got %d threads, want just qbl-aaaaaa", len(got))
	}
	warns := s.Warnings()
	if len(warns) != 1 {
		t.Fatalf("Warnings: got %v, want exactly one entry", warns)
	}
	if !strings.Contains(warns[0], badPath) {
		t.Fatalf("Warnings[0] = %q, want it to name %s", warns[0], badPath)
	}

	// A clean subsequent List resets the warnings.
	if err := os.Remove(badPath); err != nil {
		t.Fatalf("remove corrupt file: %v", err)
	}
	if _, err := s.List(ctx, store.Filter{}); err != nil {
		t.Fatalf("second List: %v", err)
	}
	if warns := s.Warnings(); len(warns) != 0 {
		t.Fatalf("Warnings after clean List: got %v, want empty", warns)
	}
}

// List must error when files exist but none are readable.
func TestListErrorsWhenNothingReadable(t *testing.T) {
	s, root := newFS(t)
	dir := filepath.Join(root, ".quibble", "comments", "docs--plan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qbl-broken.md"), []byte("garbage"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.List(context.Background(), store.Filter{}); err == nil {
		t.Fatal("List with only corrupt files: got nil error, want error")
	}
}

// Row 20 (fs-only): writes are atomic — no *.tmp files remain after Create
// (and after a resolve, which also writes).
func TestRow20_NoTmpLitter(t *testing.T) {
	s, root := newFS(t)
	ctx := context.Background()
	if err := s.Create(ctx, fsThread("qbl-aaaaaa", "docs/plan.md")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(ctx, fsThread("qbl-aaaaab", "docs/other.md")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetStatus(ctx, "qbl-aaaaaa", comment.StatusResolved, "abdullah"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	assertNoTmp(t, filepath.Join(root, ".quibble"))
}

func assertNoTmp(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".tmp") {
			t.Errorf("temp file left behind: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// Row 21 (fs-only): doc path → slug edge cases, asserted via the physical
// directory each thread lands in. The slug function is pure: extension dropped,
// "/" → "--". Names already containing "--" may collide with nested paths
// ("a--b.md" vs "a/b.md" → both "a--b"); that is acceptable but deterministic
// (rule logged in plan/DECISIONS.md) — threads stay distinct by ID and the
// frontmatter doc field remains authoritative for filtering.
func TestRow21_SlugEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		slug string
	}{
		{"nested path", "docs/payments/plan.md", "docs--payments--plan"},
		{"root README", "README.md", "README"},
		{"single level", "docs/plan.md", "docs--plan"},
		{"name already containing --", "a--b.md", "a--b"},
		{"nested path colliding with -- name", "a/b.md", "a--b"},
		{"no extension", "docs/NOTES", "docs--NOTES"},
	}
	s, root := newFS(t)
	ctx := context.Background()
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "qbl-slug" + string(rune('a'+i)) + "a"
			th := fsThread(id, tc.doc)
			if err := s.Create(ctx, th); err != nil {
				t.Fatalf("Create(%s): %v", tc.doc, err)
			}
			want := filepath.Join(root, ".quibble", "comments", tc.slug, id+".md")
			if _, err := os.Stat(want); err != nil {
				t.Fatalf("thread for %q not at slug path %s: %v", tc.doc, want, err)
			}
			got, err := s.Get(ctx, id)
			if err != nil {
				t.Fatalf("Get(%s): %v", id, err)
			}
			if got.Doc != tc.doc {
				t.Fatalf("Doc round trip: got %q, want %q", got.Doc, tc.doc)
			}
		})
	}

	// Collision determinism: the two colliding docs coexist in one slug dir and
	// List still separates them by their frontmatter doc paths.
	both, err := s.List(ctx, store.Filter{Doc: "a/b.md"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(both) != 1 || both[0].Doc != "a/b.md" {
		t.Fatalf("doc filter across slug collision: got %d threads, want the a/b.md one", len(both))
	}
}

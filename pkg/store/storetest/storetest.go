// Package storetest is the CommentStore conformance suite. Every store
// implementation — the filesystem reference store today, cloud adapters
// tomorrow — must pass Run unchanged; that is quibble's provider-independence
// guarantee (DESIGN.md §9.2).
package storetest

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/store"
)

// Run exercises every behavior of the CommentStore contract that is not tied to
// the filesystem layout. newStore must return a fresh, empty store on each call.
//
// Filesystem-specific rows from the M3 test table (physical file location,
// corrupt-file handling, .tmp cleanup, slug edge cases) live in FSStore's own
// tests alongside a call to Run.
func Run(t *testing.T, newStore func(t *testing.T) store.CommentStore) {
	t.Helper()

	// Row 1: Create → Get round-trips deep-equal.
	t.Run("Row01_CreateGetRoundTrip", func(t *testing.T) {
		s := newStore(t)
		want := mkThread("qbl-aaaaaa", "docs/plan.md", comment.StatusOpen, baseTime)
		want.Replies = []comment.Reply{
			{Author: "claude", Time: baseTime.Add(time.Hour), Body: "a reply"},
		}
		mustCreate(t, s, want)
		got := mustGet(t, s, want.ID)
		assertThread(t, got, want)
	})

	// Row 2: Create with a duplicate ID → ErrExists.
	t.Run("Row02_CreateDup", func(t *testing.T) {
		s := newStore(t)
		a := mkThread("qbl-aaaaaa", "docs/plan.md", comment.StatusOpen, baseTime)
		mustCreate(t, s, a)
		b := mkThread("qbl-aaaaaa", "docs/other.md", comment.StatusOpen, baseTime)
		if err := s.Create(ctx(), b); err != store.ErrExists {
			t.Fatalf("Create dup: got %v, want ErrExists", err)
		}
	})

	// Row 3: Create an invalid thread → validation error, nothing written.
	t.Run("Row03_CreateInvalid", func(t *testing.T) {
		s := newStore(t)
		bad := mkThread("qbl-aaaaaa", "docs/plan.md", comment.Status("bogus"), baseTime)
		if err := s.Create(ctx(), bad); err == nil {
			t.Fatal("Create invalid: got nil error, want validation error")
		}
		got, err := s.List(ctx(), store.Filter{})
		if err != nil {
			t.Fatalf("List after failed Create: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("failed Create wrote %d threads, want 0", len(got))
		}
	})

	// Row 4: List with no threads → empty slice, nil error.
	t.Run("Row04_ListEmpty", func(t *testing.T) {
		s := newStore(t)
		got, err := s.List(ctx(), store.Filter{})
		if err != nil {
			t.Fatalf("List empty: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("List empty: got %d threads, want 0", len(got))
		}
	})

	// Row 5: List filtered by doc returns only that doc's threads.
	t.Run("Row05_ListFilterDoc", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		mustCreate(t, s, mkThread("qbl-aaaaab", "docs/a.md", comment.StatusOpen, baseTime))
		mustCreate(t, s, mkThread("qbl-aaaaac", "docs/b.md", comment.StatusOpen, baseTime))
		got := mustList(t, s, store.Filter{Doc: "docs/a.md"})
		if len(got) != 2 {
			t.Fatalf("filter by doc: got %d, want 2", len(got))
		}
		for _, th := range got {
			if th.Doc != "docs/a.md" {
				t.Fatalf("filter by doc leaked %q", th.Doc)
			}
		}
	})

	// Row 6: List filtered by [open, addressed] excludes resolved.
	t.Run("Row06_ListFilterStatus", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		mustCreate(t, s, mkThread("qbl-aaaaab", "docs/a.md", comment.StatusOpen, baseTime))
		mustCreate(t, s, mkThread("qbl-aaaaac", "docs/a.md", comment.StatusOpen, baseTime))
		// Move the third to resolved.
		mustSetStatus(t, s, "qbl-aaaaac", comment.StatusResolved, "abdullah")
		got := mustList(t, s, store.Filter{
			Statuses: []comment.Status{comment.StatusOpen, comment.StatusAddressed},
		})
		if len(got) != 2 {
			t.Fatalf("status filter: got %d, want 2", len(got))
		}
		for _, th := range got {
			if th.Status == comment.StatusResolved {
				t.Fatalf("status filter leaked a resolved thread %s", th.ID)
			}
		}
	})

	// Row 7: List ordering — Created ascending, ties broken by ID.
	t.Run("Row07_ListOrdering", func(t *testing.T) {
		s := newStore(t)
		t1 := baseTime.Add(1 * time.Hour)
		t2 := baseTime.Add(2 * time.Hour)
		// Insert out of order; C and B share t1 (tie → ID order), A is later.
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, t2))
		mustCreate(t, s, mkThread("qbl-bbbbbb", "docs/a.md", comment.StatusOpen, t1))
		mustCreate(t, s, mkThread("qbl-aaaaac", "docs/a.md", comment.StatusOpen, t1))
		got := mustList(t, s, store.Filter{})
		wantOrder := []string{"qbl-aaaaac", "qbl-bbbbbb", "qbl-aaaaaa"}
		if len(got) != len(wantOrder) {
			t.Fatalf("ordering: got %d threads, want %d", len(got), len(wantOrder))
		}
		for i, id := range wantOrder {
			if got[i].ID != id {
				t.Fatalf("ordering[%d]: got %s, want %s", i, got[i].ID, id)
			}
		}
	})

	// Row 8: Reply to a missing ID → ErrNotFound.
	t.Run("Row08_ReplyMissing", func(t *testing.T) {
		s := newStore(t)
		err := s.Reply(ctx(), "qbl-zzzzzz", comment.Reply{Author: "claude", Time: baseTime, Body: "hi"})
		if err != store.ErrNotFound {
			t.Fatalf("Reply missing: got %v, want ErrNotFound", err)
		}
	})

	// Row 9: Reply appends; Get shows replies in order with timestamps preserved.
	t.Run("Row09_ReplyAppends", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		r1 := comment.Reply{Author: "claude", Time: baseTime.Add(1 * time.Hour), Body: "first"}
		r2 := comment.Reply{Author: "abdullah", Time: baseTime.Add(2 * time.Hour), Body: "second"}
		mustReply(t, s, "qbl-aaaaaa", r1)
		mustReply(t, s, "qbl-aaaaaa", r2)
		got := mustGet(t, s, "qbl-aaaaaa")
		if len(got.Replies) != 2 {
			t.Fatalf("replies: got %d, want 2", len(got.Replies))
		}
		assertReply(t, got.Replies[0], r1)
		assertReply(t, got.Replies[1], r2)
	})

	// Row 10: open→addressed→resolved leaves status resolved with ResolvedBy/At.
	t.Run("Row10_ResolveLifecycle", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusAddressed, "claude")
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusResolved, "abdullah")
		got := mustGet(t, s, "qbl-aaaaaa")
		if got.Status != comment.StatusResolved {
			t.Fatalf("status: got %s, want resolved", got.Status)
		}
		if got.ResolvedBy != "abdullah" {
			t.Fatalf("ResolvedBy: got %q, want abdullah", got.ResolvedBy)
		}
		if got.ResolvedAt == nil {
			t.Fatal("ResolvedAt is nil, want set")
		}
	})

	// Row 12 (non-fs part): reopening a resolved thread clears ResolvedBy/At.
	t.Run("Row12_ReopenClearsResolution", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusResolved, "abdullah")
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusOpen, "abdullah")
		got := mustGet(t, s, "qbl-aaaaaa")
		if got.Status != comment.StatusOpen {
			t.Fatalf("status after reopen: got %s, want open", got.Status)
		}
		if got.ResolvedBy != "" {
			t.Fatalf("ResolvedBy after reopen: got %q, want empty", got.ResolvedBy)
		}
		if got.ResolvedAt != nil {
			t.Fatalf("ResolvedAt after reopen: got %v, want nil", got.ResolvedAt)
		}
	})

	// Row 13: addressed→addressed is a no-op success.
	t.Run("Row13_AddressedNoOp", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusAddressed, "claude")
		if err := s.SetStatus(ctx(), "qbl-aaaaaa", comment.StatusAddressed, "claude"); err != nil {
			t.Fatalf("addressed→addressed: got %v, want nil", err)
		}
		if got := mustGet(t, s, "qbl-aaaaaa"); got.Status != comment.StatusAddressed {
			t.Fatalf("status: got %s, want addressed", got.Status)
		}
	})

	// Row 14: open→open after a reopen is a no-op success.
	t.Run("Row14_OpenNoOpAfterReopen", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusAddressed, "claude")
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusResolved, "abdullah")
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusOpen, "abdullah") // reopen
		if err := s.SetStatus(ctx(), "qbl-aaaaaa", comment.StatusOpen, "abdullah"); err != nil {
			t.Fatalf("open→open: got %v, want nil", err)
		}
		if got := mustGet(t, s, "qbl-aaaaaa"); got.Status != comment.StatusOpen {
			t.Fatalf("status: got %s, want open", got.Status)
		}
	})

	// Row 15: resolved→addressed → ErrTransition.
	t.Run("Row15_IllegalTransition", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusResolved, "abdullah")
		if err := s.SetStatus(ctx(), "qbl-aaaaaa", comment.StatusAddressed, "claude"); err != store.ErrTransition {
			t.Fatalf("resolved→addressed: got %v, want ErrTransition", err)
		}
	})

	// Row 16: Get finds resolved threads without a status hint.
	t.Run("Row16_GetFindsResolved", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		mustSetStatus(t, s, "qbl-aaaaaa", comment.StatusResolved, "abdullah")
		got := mustGet(t, s, "qbl-aaaaaa")
		if got.Status != comment.StatusResolved {
			t.Fatalf("Get resolved: status %s, want resolved", got.Status)
		}
	})

	// Row 18: concurrent Creates of distinct threads (10×20) all succeed & list.
	t.Run("Row18_ConcurrentCreates", func(t *testing.T) {
		s := newStore(t)
		const goroutines, per = 10, 20
		var wg sync.WaitGroup
		errs := make(chan error, goroutines*per)
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for k := 0; k < per; k++ {
					id := idFromInt(g*per + k)
					th := mkThread(id, "docs/a.md", comment.StatusOpen, baseTime)
					if err := s.Create(ctx(), th); err != nil {
						errs <- fmt.Errorf("create %s: %w", id, err)
					}
				}
			}(g)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		got := mustList(t, s, store.Filter{})
		if len(got) != goroutines*per {
			t.Fatalf("concurrent creates: listed %d, want %d", len(got), goroutines*per)
		}
	})

	// Row 19: concurrent Replies to the same thread ×10 — all present.
	t.Run("Row19_ConcurrentReplies", func(t *testing.T) {
		s := newStore(t)
		mustCreate(t, s, mkThread("qbl-aaaaaa", "docs/a.md", comment.StatusOpen, baseTime))
		const n = 10
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				r := comment.Reply{
					Author: "claude",
					Time:   baseTime.Add(time.Duration(i) * time.Second),
					Body:   fmt.Sprintf("reply %d", i),
				}
				if err := s.Reply(ctx(), "qbl-aaaaaa", r); err != nil {
					errs <- err
				}
			}(i)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		got := mustGet(t, s, "qbl-aaaaaa")
		if len(got.Replies) != n {
			t.Fatalf("concurrent replies: got %d, want %d", len(got.Replies), n)
		}
		seen := map[string]bool{}
		for _, r := range got.Replies {
			seen[r.Body] = true
		}
		for i := 0; i < n; i++ {
			if !seen[fmt.Sprintf("reply %d", i)] {
				t.Fatalf("missing reply %d among %v", i, seen)
			}
		}
	})
}

// baseTime is a fixed, second-precision, UTC instant so round trips are exact.
var baseTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func ctx() context.Context { return context.Background() }

// mkThread builds a valid thread for tests.
func mkThread(id, doc string, st comment.Status, created time.Time) *comment.Thread {
	return &comment.Thread{
		ID:      id,
		Doc:     doc,
		Status:  st,
		Created: created,
		Author:  "abdullah",
		Anchor: comment.Anchor{
			Exact:    "the retry loop re-attempts every 30 minutes",
			Prefix:   "guest is charged ",
			Suffix:   " and marked failed",
			Position: 42,
		},
		Body: "body of " + id,
	}
}

// idFromInt encodes n as a valid 6-char base32 thread id (distinct n → distinct id).
func idFromInt(n int) string {
	const al = "abcdefghijklmnopqrstuvwxyz234567"
	var b [6]byte
	for i := 5; i >= 0; i-- {
		b[i] = al[n&31]
		n >>= 5
	}
	return "qbl-" + string(b[:])
}

func mustCreate(t *testing.T, s store.CommentStore, th *comment.Thread) {
	t.Helper()
	if err := s.Create(ctx(), th); err != nil {
		t.Fatalf("Create(%s): %v", th.ID, err)
	}
}

func mustGet(t *testing.T, s store.CommentStore, id string) *comment.Thread {
	t.Helper()
	got, err := s.Get(ctx(), id)
	if err != nil {
		t.Fatalf("Get(%s): %v", id, err)
	}
	return got
}

func mustList(t *testing.T, s store.CommentStore, f store.Filter) []*comment.Thread {
	t.Helper()
	got, err := s.List(ctx(), f)
	if err != nil {
		t.Fatalf("List(%+v): %v", f, err)
	}
	return got
}

func mustReply(t *testing.T, s store.CommentStore, id string, r comment.Reply) {
	t.Helper()
	if err := s.Reply(ctx(), id, r); err != nil {
		t.Fatalf("Reply(%s): %v", id, err)
	}
}

func mustSetStatus(t *testing.T, s store.CommentStore, id string, st comment.Status, actor string) {
	t.Helper()
	if err := s.SetStatus(ctx(), id, st, actor); err != nil {
		t.Fatalf("SetStatus(%s, %s): %v", id, st, err)
	}
}

// assertThread compares two threads field-by-field, using Time.Equal for
// instants so equal times with differing internal representations still match.
func assertThread(t *testing.T, got, want *comment.Thread) {
	t.Helper()
	if got.ID != want.ID || got.Doc != want.Doc || got.Status != want.Status ||
		got.Author != want.Author || got.Body != want.Body {
		t.Fatalf("scalar mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if !got.Created.Equal(want.Created) {
		t.Fatalf("Created: got %v, want %v", got.Created, want.Created)
	}
	if !reflect.DeepEqual(got.Anchor, want.Anchor) {
		t.Fatalf("Anchor:\n got=%+v\nwant=%+v", got.Anchor, want.Anchor)
	}
	if got.ResolvedBy != want.ResolvedBy {
		t.Fatalf("ResolvedBy: got %q, want %q", got.ResolvedBy, want.ResolvedBy)
	}
	if (got.ResolvedAt == nil) != (want.ResolvedAt == nil) {
		t.Fatalf("ResolvedAt presence: got %v, want %v", got.ResolvedAt, want.ResolvedAt)
	}
	if got.ResolvedAt != nil && !got.ResolvedAt.Equal(*want.ResolvedAt) {
		t.Fatalf("ResolvedAt: got %v, want %v", *got.ResolvedAt, *want.ResolvedAt)
	}
	if len(got.Replies) != len(want.Replies) {
		t.Fatalf("replies len: got %d, want %d", len(got.Replies), len(want.Replies))
	}
	for i := range got.Replies {
		assertReply(t, got.Replies[i], want.Replies[i])
	}
}

func assertReply(t *testing.T, got, want comment.Reply) {
	t.Helper()
	if got.Author != want.Author || got.Body != want.Body {
		t.Fatalf("reply mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if !got.Time.Equal(want.Time) {
		t.Fatalf("reply time: got %v, want %v", got.Time, want.Time)
	}
}

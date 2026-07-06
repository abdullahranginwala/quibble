// Package store defines the CommentStore boundary that makes quibble
// provider-independent, plus the filesystem reference implementation (FSStore).
//
// The interface is deliberately small and takes a context.Context on every
// method so future cloud adapters (Cloudflare, AWS — see DESIGN.md §9) can
// implement the exact same contract. The conformance suite in
// pkg/store/storetest is the executable specification: every adapter must pass
// storetest.Run unchanged, which is the provider-independence guarantee.
package store

import (
	"context"
	"errors"

	"github.com/abdullahranginwala/quibble/pkg/comment"
)

// Filter narrows a List call. The zero Filter matches every thread.
type Filter struct {
	Doc      string           // "" = all docs (relative doc path, not slug)
	Statuses []comment.Status // nil/empty = all statuses
}

// CommentStore is the storage boundary for comment threads. Implementations
// must satisfy the behaviors exercised by storetest.Run.
type CommentStore interface {
	// List returns threads matching f, sorted by Created ascending and then by
	// ID for ties. Corrupt/unreadable entries are skipped; an error is returned
	// only when nothing at all could be read.
	List(ctx context.Context, f Filter) ([]*comment.Thread, error)

	// Get returns the thread with the given id, or ErrNotFound.
	Get(ctx context.Context, id string) (*comment.Thread, error)

	// Create validates t and stores it. It returns ErrExists if a thread with
	// the same ID already exists; on a validation failure nothing is written.
	Create(ctx context.Context, t *comment.Thread) error

	// Reply appends r to the thread with the given id. Returns ErrNotFound if
	// the thread does not exist.
	Reply(ctx context.Context, id string, r comment.Reply) error

	// SetStatus moves the thread through its lifecycle. Legal transitions are
	// open→addressed, open→resolved, addressed→resolved, addressed→open,
	// resolved→open; a same-status call is a no-op success and anything else
	// returns ErrTransition. When s is resolved, actor is recorded as the
	// resolver. The store is mechanism, not policy: it does not enforce who may
	// resolve — that is the CLI/server layer's job (DESIGN.md §4).
	SetStatus(ctx context.Context, id string, s comment.Status, actor string) error
}

// Sentinel errors. Callers should test with errors.Is.
var (
	ErrNotFound   = errors.New("thread not found")
	ErrExists     = errors.New("thread already exists")
	ErrTransition = errors.New("illegal status transition")
)

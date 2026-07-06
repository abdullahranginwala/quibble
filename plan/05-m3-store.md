# M3 — `pkg/store`: CommentStore interface, fs store, conformance suite

**Goal:** the storage boundary that makes quibble provider-independent. The fs store is the reference implementation; the conformance suite is the contract every future adapter (Cloudflare, AWS) must pass unchanged.

**Depends on:** M2.

## Public API (exact)

```go
package store

type Filter struct {
    Doc      string           // "" = all docs (rel path, not slug)
    Statuses []comment.Status // nil = all
}

type CommentStore interface {
    List(ctx context.Context, f Filter) ([]*comment.Thread, error) // sorted by Created asc, then ID
    Get(ctx context.Context, id string) (*comment.Thread, error)   // ErrNotFound
    Create(ctx context.Context, t *comment.Thread) error           // ErrExists on dup ID; validates first
    Reply(ctx context.Context, id string, r comment.Reply) error
    SetStatus(ctx context.Context, id string, s comment.Status, actor string) error
}

var ErrNotFound = errors.New("thread not found")
var ErrExists   = errors.New("thread already exists")
var ErrTransition = errors.New("illegal status transition")

func NewFS(projectRoot string) (*FSStore, error) // requires .quibble/ to exist
```

## FS layout & semantics

```
.quibble/comments/<doc-slug>/<id>.md          # open + addressed
.quibble/comments/_resolved/<doc-slug>/<id>.md # resolved
```

- `<doc-slug>` per conventions §Naming (pure function of `Thread.Doc`).
- **SetStatus to resolved** sets `resolved_by`/`resolved_at`, writes the file, then **moves** it under `_resolved/` (write new, remove old — plain `os.Rename` is fine; both paths are inside the repo). **Reopen** (resolved → open) clears those fields and moves it back.
- Legal transitions: open→addressed, open→resolved, addressed→resolved, addressed→open (reopen-before-resolve), resolved→open. Same-status is a no-op success. Everything else → `ErrTransition`.
- `SetStatus(..., StatusResolved, actor)` stamps `ResolvedBy = actor`. The *policy* that agents may not resolve is enforced at the CLI/server layer (M4/M5), *not* here — the store is mechanism, not policy.
- Writes are atomic: write to `<path>.tmp` in the same dir, then rename over. A crash never leaves a half-written thread visible.
- `Get`/`Reply`/`SetStatus` must find threads in either location (open dirs first, then `_resolved/`).
- `List` skips (and collects) corrupt files: return value is the good threads plus an `error` only if *nothing* was readable; corrupt paths are reported via a `Warnings() []string` method on FSStore checked by CLI `doctor`. (Interface stays clean; warnings are an FSStore extra.)

## Conformance suite

```go
package storetest // pkg/store/storetest

// Run exercises every CommentStore behavior. newStore returns a fresh, empty store.
func Run(t *testing.T, newStore func(t *testing.T) store.CommentStore)
```

The fs store's own tests are (almost) just `storetest.Run(t, ...)` + fs-specific cases. **Every future adapter imports and passes `storetest.Run` unmodified — this is the provider-independence guarantee, so write it as the real spec:** cover every row below inside `Run` except rows marked *(fs-only)*.

## Tests

| # | Case | Expect |
|---|------|--------|
| 1 | Create → Get | deep-equal round trip |
| 2 | Create dup ID | `ErrExists` |
| 3 | Create invalid thread (bad status) | validation error, nothing written |
| 4 | List with no threads | empty slice, nil error |
| 5 | List filter by doc | only that doc's threads |
| 6 | List filter `[open, addressed]` | excludes resolved |
| 7 | List ordering | Created asc, ties by ID |
| 8 | Reply to missing ID | `ErrNotFound` |
| 9 | Reply appends | Get shows replies in order, timestamps preserved |
| 10 | open→addressed→resolved | final Get: status resolved, ResolvedBy=actor, ResolvedAt set |
| 11 | resolved thread location *(fs-only)* | file physically under `_resolved/<slug>/`, gone from open dir |
| 12 | reopen resolved | back in open dir *(fs-only assert)*; ResolvedBy/At cleared |
| 13 | addressed→addressed | no-op success |
| 14 | open→open after reopen | no-op success |
| 15 | resolved→addressed | `ErrTransition` |
| 16 | Get finds resolved threads | works without status hint |
| 17 | Corrupt file in comments dir *(fs-only)* | List returns good threads; `Warnings()` names the bad path |
| 18 | Concurrent Creates of distinct threads (10 goroutines ×20) | all succeed, all listed; run under `-race` |
| 19 | Concurrent Reply to same thread ×10 | all 10 replies present (FSStore serializes with an in-process per-ID mutex; document that cross-process safety = git's job) |
| 20 | Atomicity *(fs-only)* | after Create, no `*.tmp` files remain; simulate by checking dir listing |
| 21 | Doc path → slug edge cases | nested paths, `README.md` at root, names already containing `--` (slug collision acceptable-but-deterministic: log rule in DECISIONS.md) |

## Acceptance

- `storetest.Run` green against `FSStore` under `-race`.
- Manually: create two threads via a scratch `main`, resolve one, inspect `.quibble/comments/` — files are human-readable, diff-friendly, and `git status` shows exactly the expected paths.

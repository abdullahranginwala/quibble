# Conventions — binding for every milestone and subagent

## Language & dependencies

- Go ≥ 1.24 (`go.mod` directive stays `1.24`; do not bump without logging in DECISIONS.md).
- **Allowed third-party deps** (do not add others without a DECISIONS.md entry and strong cause):
  - `github.com/yuin/goldmark` (+ its extension subpackages) — markdown
  - `github.com/alecthomas/chroma/v2` — syntax highlighting
  - `github.com/spf13/cobra` — CLI
  - `gopkg.in/yaml.v3` — config + thread frontmatter
  - `github.com/fsnotify/fsnotify` — fs watch for serve
  - `github.com/rogpeppe/go-internal/testscript` — CLI e2e tests (test-only)
- **Frontend: zero dependencies.** Vanilla JS (ES modules) + CSS, embedded via `go:embed`. No npm, no bundler, no framework. If you feel you need one, you're overbuilding — simplify the UI instead.

## Code style

- `gofmt`-clean, `go vet`-clean. Exported symbols in `pkg/` get doc comments; `internal/` only where non-obvious.
- Errors: wrap with `%w` and context (`fmt.Errorf("parsing thread %s: %w", path, err)`). No panics outside `main` init paths.
- No global mutable state. Constructors take explicit deps (`NewFS(root string)`, `server.New(store, renderer, docsFS)`).
- Contexts on anything that does I/O and could later go remote (store methods take `context.Context` even though fs store ignores it — the interface is shared with future cloud adapters).
- Timestamps: `time.Time` in RFC 3339 with zone offset when serialized. IDs: `qbl-` + 6 chars of lowercase base32 (`abcdefghijklmnopqrstuvwxyz234567`) from `crypto/rand`.

## Testing rules

- Standard `testing` package. Table-driven where a table exists in the milestone spec — **implement every row of every test table; the tables are requirements, not suggestions.** Add more cases freely; never remove specced ones.
- Golden files under `testdata/golden/`; regenerate only via an explicit `-update` flag pattern, and inspect diffs before committing.
- CLI e2e via testscript files under `internal/cli/testdata/script/*.txtar`.
- HTTP via `net/http/httptest`. No network access in any test.
- Every bug found during implementation gets a regression test before the fix is committed.

## Gate (run before every commit; CI enforces on push)

```sh
gofmt -l . | (! grep .)   # no unformatted files
go vet ./...
go build ./...
go test ./... -race
```

CI: `.github/workflows/ci.yml` (created in M0) runs the same gate on ubuntu-latest + macos-latest.

## Git workflow

- Work directly on `main` (single-implementer repo). One or a few logical commits per milestone, prefixed `M<n>: `.
- Stage files explicitly by path. **Never `git add -A` or `git add .`** — scratch/debug files must not land.
- Commit messages end with: `Co-Authored-By: Claude <the implementing agent>` per the environment's convention.

## Naming & layout invariants (from DESIGN.md §11)

- Public API lives only in `pkg/render`, `pkg/comment`, `pkg/store`. Anything else is `internal/`.
- The tool **never mutates user markdown** (DESIGN.md §5, §13). Any code path that writes into a docs dir (other than `.quibble/`) is a bug.
- All user-facing state lives in the target repo's `.quibble/` (and, for cloud creds later, `~/.config/quibble/`). No other dotfiles, caches, or temp dirs in the user's repo.
- Doc slugs: relative path from docs root, extension dropped, path separators replaced with `--`. Example: `docs/payments/plan.md` → `docs--payments--plan`. Slugs are pure functions of the path — same input, same slug, forever.

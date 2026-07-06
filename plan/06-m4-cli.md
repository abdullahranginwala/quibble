# M4 — CLI: init, build, comments, doctor

**Goal:** the complete terminal surface (DESIGN.md §10). After M4, the human-agent loop works end-to-end *without* the web UI: agents list/reply/address via CLI, humans can resolve via CLI.

**Depends on:** M1 + M2 + M3.

## Commands (exact behavior)

### `quibble init`

- Creates `.quibble/config.yml` (config.Default()), `.quibble/comments/.gitkeep`, and the agent contract `.quibble/AGENTS.md` (content specced in M6 — create the file in M4 with a placeholder header; M6 fills it).
- Appends a 3-line pointer block to the repo's `CLAUDE.md` **only if** that file exists AND doesn't already contain the marker `<!-- quibble -->` (idempotent). Never creates CLAUDE.md.
- Idempotent overall: re-running on an initialized repo exits 0 with "already initialized", changes nothing.
- Refuses politely (exit 1) if `--dir` is not inside a git work tree (check for `.git` walking up) — comments only make sense in git.

### `quibble build [-o dist]`

- Loads config, globs docs, `RenderDir`, `Site.WriteTo(out)`. Default out: `dist/` under project root. Exit non-zero listing each doc that failed; render the rest anyway.
- Works without `.quibble/` too (zero-config mode): defaults docs=`["**/*.md"]`, theme=paper — the "just give me pretty HTML" entry point.

### `quibble comments …`

All read commands honor global `--json` (stable field names = the Go struct yaml names; one JSON array/object per invocation, no NDJSON).

- `list [--doc D] [--open|--addressed|--resolved]` — table: ID, STATUS, DOC, AGE, first 60 runes of body. Flags combine as union; none = open+addressed (the actionable set — **not** resolved; that default is the product).
- `show <id>` — full thread: header, anchor quote, body, replies, status history line. `--json` = full struct.
- `add --doc D --quote "…" [-m msg] [--author name]` — creates a thread anchored at the (unique) occurrence of `--quote` in the doc's normalized text. Quote not found → error suggesting `--quote` must be copied verbatim from the doc; found >1× → error asking for a longer quote. Author defaults to config `authors.human`.
- `reply <id> -m msg [--author name]` — author defaults: see attribution below.
- `address <id> [-m msg]` — optional reply then status→addressed, author = config `authors.agent` by default.
- `resolve <id>` — status→resolved. **Policy gate:** if effective author == config `authors.agent`, refuse with exit 3 and message "agents address; humans resolve". `--author` can override attribution but the agent contract (M6) forbids agents doing so.
- `reopen <id> [-m msg]`.

Attribution: `--author` flag > `QUIBBLE_AUTHOR` env > config default (`authors.human` for add/reply/resolve/reopen, `authors.agent` for address).

### `quibble doctor [--fix]`

- For every non-resolved thread: load its doc, normalize, `Resolve` the anchor. Report table: ID, doc, method (exact/context/fuzzy/orphan), confidence.
- `--fix`: for fuzzy placements ≥0.75, rewrite the thread's anchor via `NewAnchor` at the found span (self-healing). Orphans are listed, never auto-fixed.
- Also surfaces `FSStore.Warnings()` (corrupt files) and validates theme overrides against the token list.
- Exit code: 0 all exact/context; 2 if any fuzzy (healed or not); 3 if any orphan or corrupt file.
- `comments repin <id> --quote "…"` lives here in spirit: re-anchors an orphan to a fresh verbatim quote (same uniqueness rules as `add`).

## Exit codes (uniform)

0 success · 1 generic/usage error · 2 warnings (doctor fuzzy) · 3 policy refusal or orphans present.

## Tests — testscript e2e (`internal/cli/testdata/script/*.txtar`)

Each row = one txtar script; scripts set up a git repo + docs in their sandbox.

| # | Script | Asserts |
|---|--------|---------|
| 1 | `init.txtar` | fresh repo → files created; second run idempotent; non-git dir refused |
| 2 | `init_claudemd.txtar` | CLAUDE.md pointer appended once, never twice |
| 3 | `build.txtar` | dist/ contains index + per-doc html; zero-config mode works without .quibble |
| 4 | `build_partial_failure.txtar` | one unreadable doc → exit ≠0, other docs still rendered, error names the file |
| 5 | `comments_lifecycle.txtar` | add → list shows open → reply → address (as agent) → list default hides nothing yet → resolve (as human) → default list now empty → `--resolved` shows it → file moved to `_resolved/` |
| 6 | `comments_add_quote.txtar` | verbatim quote ok; missing quote errors; ambiguous quote errors with count |
| 7 | `agent_cannot_resolve.txtar` | `QUIBBLE_AUTHOR=claude quibble comments resolve X` → exit 3, status unchanged |
| 8 | `json_output.txtar` | `list --json` parses (use script `stdout` regex on stable keys); `show --json` round-trips through `jq`-less grep |
| 9 | `doctor.txtar` | edit doc: typo-fix → doctor reports fuzzy, exit 2; `--fix` rewrites anchor; doctor again → exact, exit 0 |
| 10 | `doctor_orphan.txtar` | delete anchored sentence → doctor exit 3, orphan listed; `repin` with new quote → exact |
| 11 | `attribution.txtar` | flag > env > config precedence for author |

Unit tests where logic is non-trivial (quote-uniqueness finder, exit-code mapping) in ordinary `_test.go` files.

## Acceptance

- A human can run the full loop from two terminals (one `QUIBBLE_AUTHOR=claude`) with no web UI and it feels coherent.
- `quibble comments list --json` output is stable enough that M6's agent skill can rely on it (field names frozen from here on).

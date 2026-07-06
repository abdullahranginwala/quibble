# M5 — `quibble serve`: local review app

**Goal:** the Google-Docs moment. `quibble serve` opens the rendered docs in the browser; the user selects text, comments, replies, resolves — every action a file write into `.quibble/comments/`. Live reload on doc *and* comment changes. No daemon, no state outside the repo.

**Depends on:** M4.

## Architecture

- `internal/server`: `net/http` + `go:embed` frontend from `web/`. Single port (default `:4747`, flag `--port`), binds `127.0.0.1` **only** (this server writes files; never expose it by default).
- On start: load config, render all docs into memory (reuse M1), watch (fsnotify) the docs globs + `.quibble/comments/` recursively; debounce 150 ms; re-render changed docs; notify clients via SSE.
- Comment placements are computed server-side per render: for each non-resolved thread of a doc, `Resolve` against the doc's normalized text, then map rune spans → block IDs + in-block offsets (blocks carry Start/End from M1). The client receives *placements*, it never runs anchoring logic.

## HTTP API (all JSON; `Content-Type` enforced; non-GET requires header `X-Qbl: 1` as CSRF guard — the embedded UI sets it)

```
GET  /                         → index page (doc list)
GET  /d/{slug}                 → rendered doc page (Doc.Page + comment UI mounted)
GET  /api/docs                 → [{slug, relPath, title, openCount, addressedCount}]
GET  /api/docs/{slug}/comments → {threads: [{thread…, placement: {blockId, start, end, method, confidence} | null}]}
                                 (placement null = orphan; resolved threads excluded; ?include=resolved to include, placement omitted)
POST /api/comments             → body {doc, quoteStart, quoteEnd, blockId, body, author?} — server builds Anchor via
                                 NewAnchor from the block's span + offsets; 409 if the doc changed underneath
                                 (client sends docVersion etag it got on GET; mismatch → re-fetch)
POST /api/comments/{id}/reply  → {body, author?}
POST /api/comments/{id}/status → {status, author?} — same policy gate as CLI (agent author cannot resolve → 403)
GET  /api/events               → SSE: `doc-changed {slug}`, `comments-changed {slug}`
```

Author defaults on the server: `authors.human` (this is the human's UI). Errors: JSON `{error}` + correct status (400 validation, 404, 403 policy, 409 stale).

## Frontend (`web/` — vanilla ES modules, zero deps, embedded)

Files: `comments.js` (entry), `anchor-render.js` (span→DOM highlighting), `ui.css` (styles **only** via `--qbl-*` tokens — this is where the theme contract proves itself).

Behaviors:

1. **Highlights:** for each placement, wrap the target text range inside its `[data-qbl]` block with `<mark class="qbl-mark">` (walk text nodes; spans may cross inline elements — handle by wrapping each intersected text node segment). Fuzzy placements get a dashed underline modifier; a tooltip shows confidence.
2. **Margin bubbles:** one gutter indicator per thread, vertically aligned to its mark (position: absolute against article; recompute on resize). Click → thread panel.
3. **Thread panel:** slide-over showing quote, body, replies (rendered as plain text v0.1 — no client md rendering), reply box, and action buttons per status: open → [Reply] [Resolve]; addressed → [Reply] [Resolve] [Reopen]. (No "address" button — addressing is the agent's CLI act.)
4. **New comment:** on text selection inside the article, float a "💬 Comment" button; click → panel with the quoted selection; submit → POST with block ID + rune offsets within the block's normalized text (compute by normalizing the block's textContent with the *same* whitespace rule; server re-verifies the quote matches and errors if not — never trust client offsets blindly).
5. **Orphans:** "Unanchored comments (N)" panel pinned at top when N>0.
6. **Header:** open/addressed counts, link to `?include=resolved` view (read-only list of resolved threads), SSE-driven "doc updated — reload" toast (auto-reload only when no panel is open / no draft text).
7. Works in Safari + Chrome + Firefox current versions. No build step: files ship as authored.

## Tests

Server (httptest; no browser automation in v0.1 — the API carries the logic, the JS is kept thin enough to verify by hand):

| # | Case | Expect |
|---|------|--------|
| 1 | GET /api/docs on 3-doc project | slugs, titles, counts correct |
| 2 | GET comments for doc with exact/fuzzy/orphan threads | placements: spans for first two, null for orphan, method fields correct |
| 3 | POST comment with valid block+offsets | 201; thread file exists in `.quibble/comments/`; anchor exact == selected quote |
| 4 | POST comment with stale docVersion | 409; no file written |
| 5 | POST comment whose offsets don't match quote server-side | 400; no file written |
| 6 | POST status resolve as human | 200; file moved to `_resolved/` |
| 7 | POST status resolve with agent author | 403; unchanged |
| 8 | POST without `X-Qbl` header | 403 (CSRF guard) |
| 9 | Server binds 127.0.0.1 | connection from non-loopback addr impossible (assert listener addr) |
| 10 | Doc file edited on disk | SSE `doc-changed` within 1 s; GET doc reflects edit; placements recomputed |
| 11 | Thread file added via CLI while serving | SSE `comments-changed`; GET includes it |
| 12 | Two rapid saves (debounce) | single SSE event |
| 13 | Concurrent POST reply + doc re-render | no race (`-race`), both effects land |

Manual checklist (commit as `plan/checklists/m5-manual.md`, tick before closing M5):

- [ ] Select across bold/italic/link text → mark wraps correctly, no broken DOM
- [ ] Selection spanning two blocks → comment button *not* offered (v0.1 limitation: single-block anchors; document it)
- [ ] Bubbles align after window resize and TOC toggle
- [ ] Dark mode: marks, bubbles, panel all legible (tokens doing their job)
- [ ] Draft reply survives a doc-changed toast (no auto-reload while typing)
- [ ] Full loop in browser: comment → (CLI as agent) address → UI shows addressed → resolve in UI → thread leaves view, file in `_resolved/`

## Acceptance

- `quibble serve` on this repo; complete the manual checklist against `DESIGN.md` itself.
- Kill -9 the server mid-comment-write → repo never left with `.tmp` litter or corrupt thread (atomic writes from M3 doing their job).

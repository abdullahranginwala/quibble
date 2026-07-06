# Quibble — Design

> Review-grade markdown docs for humans and agents. Render beautiful HTML, comment on it like Google Docs, and let the comments live in git so every LLM agent that opens your repo sees exactly what's open, what's addressed, and what's done.

## 1. The problem

Long-running projects (weeks/months) with LLM agents produce two artifacts: code and documents (plans, RFCs, runbooks, decision logs). Code has great tooling — git, PRs, review comments. Documents don't:

- Every new agent session starts cold. The doc is the handoff medium, but there's no way to mark "this part needs work" **inside** the doc in a way both the human and the agent treat as first-class.
- Review feedback happens out-of-band (chat messages, TODOs pasted into prompts) and evaporates.
- Raw markdown in a terminal or GitHub's renderer is a poor reading experience for long documents.

## 2. The product

Two cleanly separated pieces in one Go monorepo:

| Piece | What it is | Ships as |
|---|---|---|
| **Renderer** | Markdown → beautiful, readable static HTML | Importable Go library (`pkg/render`) + `quibble build` |
| **Review layer** | Anchored, threaded, lifecycle-managed comments on those docs | `quibble serve` (local web UI) + `quibble comments` CLI + git-native storage |

The renderer must be independently useful — someone who just wants gorgeous HTML from a docs folder can import `pkg/render` or run `quibble build` and never touch comments.

## 3. Core architectural decision: git is the database

Comments are **files in the repo**, not rows in a hosted database.

```
.quibble/
  config.yml                      # project config (docs globs, theme, authors)
  comments/
    <doc-slug>/
      qbl-7f3k2a.md               # one file per thread (open / addressed)
    _resolved/
      <doc-slug>/
        qbl-19x8mm.md             # archive: resolved threads move here
```

Why this wins for the primary use case (you + agents in a repo):

- **Agents see comments natively.** An LLM reading the repo needs zero API calls — open threads for a doc are just files next to it. `quibble comments list --open` is a convenience, not a requirement.
- **History for free.** Comment history = git history. Doc versions = git commits. No second system to reconcile. "See what changed" → `git log`; "see the discussion" → the thread file or its history.
- **Works offline, zero infra.** `quibble serve` gives the full Google-Docs commenting experience with no cloud account at all — the local server writes thread files directly.
- **Branch/PR-native.** Comments travel with branches. Doc review can literally ride a PR.
- **Cloud-provider independence by construction.** Storage is behind a `CommentStore` interface. The filesystem store is the reference implementation; Cloudflare D1 / DynamoDB / Postgres become optional *sync adapters* (§9), not the foundation. This inverts Parchi's architecture (Worker + D1 required) — which we take inspiration from for the *sharing* story, not the core.

### Resolved-thread placement

Resolved threads **move** to `_resolved/` rather than getting a status flag in place. Rationale: an agent that lists `.quibble/comments/<doc>/` sees only actionable threads without any filtering logic, and the "most up-to-date view" the user asked for stays uncluttered. The move is a git-tracked rename, so identity and history are preserved, and the rendered HTML links each doc to its resolved archive.

## 4. Comment data model

One markdown file per thread, YAML frontmatter + body, replies appended as delimited sections. Human-readable, agent-readable, diff-friendly.

```markdown
---
id: qbl-7f3k2a
doc: docs/deployment-plan.md
status: open            # open | addressed | resolved
created: 2026-07-06T14:31:00+05:30
author: abdullah
anchor:
  exact: "the retry loop will re-attempt every 30 minutes"
  prefix: "guest is charged but marked failed, "
  suffix: ". This is the double-charge window"
  heading: ["Rollback plan", "Failure modes"]
  position: 14382       # byte offset hint at creation time
resolved_by: null
resolved_at: null
---

Shouldn't this be idempotency-keyed on booking id, not attempt count?

<!-- reply author=claude time=2026-07-06T15:02:00+05:30 -->

Agreed — switched the key to `bookingId + chargeType` in commit `a1b2c3d`
and updated §4.2 of this doc to match. Marking addressed.
```

### States and who can set them

```
open ──(agent replies + does the work)──▶ addressed ──(human approves)──▶ resolved
  ▲                                            │
  └────────────(human reopens)─────────────────┘
```

- **open** → created by human (usually via web UI) or agent (via CLI).
- **addressed** → set by the agent after implementing/answering. Agents may *never* resolve.
- **resolved** → humans only. Moves the file to `_resolved/`.
- **Orphaned** is not a state — it's a derived render-time condition (§6) so it never masks the real lifecycle state.

## 5. Rendering (`pkg/render` + `quibble build`)

- **Parser:** [goldmark](https://github.com/yuin/goldmark) — CommonMark + GFM (tables, task lists, strikethrough, autolinks), footnotes, extensible AST for our needs.
- **Syntax highlighting:** chroma, server-side (no JS highlighter).
- **Diagrams:** mermaid via client-side script, opt-in per project config.
- **Themes:** 2–3 built-in themes, user-extensible (see §5.1). Every theme gets the same structural features: measure-limited prose, sticky table-of-contents sidebar, light/dark via `prefers-color-scheme` + toggle, anchored headings, copy buttons on code blocks. Comment highlights render as subtle marks with margin bubbles (Google Docs-style) that expand to threads.
- **Stable block fingerprints:** at render time each block gets `data-qbl` = hash(normalized text) + occurrence index. These are *hints* for anchoring — never written back into the markdown source. **The user's markdown is never mutated.**
- **Output:** self-contained static site (`quibble build -o dist/`), assets inlined or copied — deployable to any static host with zero runtime.
- **Library API sketch:**

```go
site, err := render.New(render.Options{Theme: render.ThemePaper, TOC: true}).
    RenderDir(os.DirFS("docs"))
// or single-doc:
html, err := render.Doc(src, opts)
```

### 5.1 Theme system

The hard constraint shaping the design: **the comment layer must work on every theme**. Highlights, margin bubbles, and thread UI are owned by quibble, not by themes — so a theme is not arbitrary CSS, it's a package that fills a defined contract.

**What a theme is:** a directory (built-ins embedded via `go:embed`, custom ones on disk):

```
my-theme/
  theme.yml        # name, author, light/dark support, token values
  tokens.css       # REQUIRED: the design-token contract (CSS custom properties)
  theme.css        # the theme's own styling, built on those tokens
  layout.html      # OPTIONAL: Go html/template override of page chrome
  assets/          # OPTIONAL: fonts, extra JS
```

**The contract is the token set.** `tokens.css` must define the full quibble token vocabulary — `--qbl-bg`, `--qbl-fg`, `--qbl-accent`, `--qbl-prose-max`, `--qbl-font-body`, `--qbl-font-mono`, `--qbl-radius`, `--qbl-comment-*`, etc. — for both light and dark. The comment UI and structural chrome (TOC, code copy buttons, orphan panel) style themselves *exclusively* from these tokens, so they automatically match any theme. `quibble doctor` validates a custom theme against the token schema and fails loudly on missing tokens.

**Three escalating levels of customization** — most users never leave level 1:

1. **Pick a built-in:** `theme: ink` in `.quibble/config.yml`.
2. **Override tokens inline:** a `theme.overrides:` map in config (`--qbl-accent: "#7c3aed"`, a different font stack) — rebrand without writing a theme.
3. **Full custom theme:** `theme: ./themes/acme/` pointing at a theme directory; template override included. Shareable by copying the directory or referencing a git repo.

**Built-ins (ship with v0.1–v0.2):**

- **paper** *(default)* — quiet, editorial, serif headings, generous whitespace; optimized for long-form RFC reading.
- **ink** — dense and technical: sans everywhere, tighter type scale, sharper contrast; for runbooks and reference docs.
- **terminal** — mono-first, dark-first, feels like well-typeset man pages; for the CLI-native crowd.

**Library API:** `render.Options{Theme: render.ThemePaper}` for built-ins, `render.ThemeFromFS(fsys)` for anything implementing the contract — so `pkg/render` importers get the same extensibility as CLI users. Resolution order: built-in name → path → error listing available themes.

## 6. Anchoring — the nuanced part

Line numbers die on the first edit. Quibble uses **W3C Web Annotation-style selectors** (TextQuote + Position + heading path, as stored in the frontmatter above) and re-anchors at render time:

1. **Exact match** of `exact` within the doc → anchor.
2. Multiple matches → disambiguate with `prefix`/`suffix`, then nearest to `position`.
3. No exact match → **fuzzy match** (sliding-window similarity, threshold ~0.75) scoped to the `heading` section first, then whole doc.
4. Still nothing → thread renders in an **"Unanchored comments"** panel at the top of the doc — visible, never silently dropped. `quibble doctor` reports orphans and lets you re-pin them (`quibble comments repin <id>`), which rewrites the selector.

On successful fuzzy re-anchor, the CLI can refresh the stored selector (`quibble doctor --fix`) so anchors self-heal over a doc's lifetime.

## 7. The local app (`quibble serve`)

A single-binary local server (Go stdlib `net/http`, frontend embedded via `go:embed` — vanilla JS/CSS, no framework, no build step, dependency-free like Parchi's CLI philosophy):

- Live-renders docs (fs-watch → reload).
- Select text → floating "comment" button → thread file written to `.quibble/comments/` instantly.
- Reply, mark addressed, resolve, reopen — all buttons in the thread bubble; every action is a file write the user then commits like any other change.
- Doc header shows: open count, addressed count (needs your review), link to resolved archive.

There is deliberately **no daemon, no account, no state outside the repo**.

## 8. Agent workflow

`quibble init` installs an agent-facing contract into the repo (a Claude Code skill + a section agents discover naturally):

1. Before working from a doc, check open threads: `quibble comments list --doc <doc> --open` (or read `.quibble/comments/<slug>/`).
2. Do the work the comment asks for (doc edit, code change, or an answer).
3. Reply on the thread citing what changed, then `quibble comments address <id>`.
4. **Never resolve.** Resolution is the human reviewer's act.

The human then opens `quibble serve`, reviews everything in `addressed`, and resolves or reopens. This is the full loop the tool exists for.

## 9. Sharing & cloud (v0.3+, adapter-based)

For sharing rendered docs with people who don't have the repo — Parchi's actual sweet spot — Quibble adds a `Publisher` interface:

```go
type CommentStore interface { List, Get, Create, Reply, SetStatus, ... }
type Publisher interface { PublishSite, PullComments, PushComments }
```

- `quibble publish --to cloudflare` → static site to Pages/Workers Sites, comments API on a Worker + D1, **on the user's own account** (self-hosted, no shared backend).
- `quibble publish --to aws` → S3/CloudFront + Lambda + DynamoDB.
- `quibble sync pull` merges remote comments back into `.quibble/comments/` — **git remains the source of truth**; the cloud copy is a projection.

Provider independence is enforced by making the fs store the reference `CommentStore` and CI-testing every adapter against the same conformance suite.

## 10. CLI surface (v0.1)

```
quibble init                        # .quibble/, config, agent skill
quibble build [dir] [-o dist]       # render static site
quibble serve [dir]                 # local review app
quibble comments list [--doc D] [--open|--addressed|--resolved]
quibble comments show <id>
quibble comments add --doc D --quote "..." -m "..."
quibble comments reply <id> -m "..."
quibble comments address <id>       # agent's verb
quibble comments resolve <id>       # human's verb (moves to _resolved/)
quibble comments reopen <id>
quibble doctor [--fix]              # anchor health, orphan report, repin
```

Machine-friendly: every read command takes `--json`.

## 11. Repository layout

```
quibble/
  cmd/quibble/          # CLI entrypoint (cobra)
  pkg/render/           # PUBLIC: md → html library
  pkg/comment/          # PUBLIC: thread model, selectors, re-anchoring
  pkg/store/            # CommentStore interface + fs implementation
  internal/server/      # quibble serve: http handlers, fs-watch
  internal/cli/         # command wiring
  web/                  # embedded frontend assets (go:embed)
  skill/                # agent contract installed by `quibble init`
  docs/                 # our own docs — dogfooded with quibble itself
  DESIGN.md
```

`pkg/` is the supported public API surface (the "proper library" ask); `internal/` is app plumbing.

## 12. Roadmap

- **v0.1 — the loop works locally.** `init`, `build`, `serve` with commenting UI, fs store, full `comments` CLI, agent skill, **paper** theme on the token contract. Dogfood on this repo's own docs.
- **v0.2 — anchors that survive, themes open up.** Fuzzy re-anchoring, orphan panel, `doctor --fix`; **ink** + **terminal** themes, token overrides, custom theme dirs + `ThemeFromFS`; print styles, mermaid, search.
- **v0.3 — sharing.** `Publisher` interface + Cloudflare adapter (Pages + Worker + D1), viewer identity for shared review.
- **v0.4 — second adapter proves the interface.** AWS publisher, `sync` conflict handling, GitHub PR integration (open threads ⇄ PR comments).

## 13. Non-goals

- Not a wiki, CMS, or Notion replacement — markdown in git stays the medium.
- No WYSIWYG editing of docs in the browser (comments yes, doc edits happen in your editor).
- No shared/multi-tenant hosted backend, ever. Self-hosted only.
- No mutation of the user's markdown source by the tool.

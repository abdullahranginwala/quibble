# Quibble Implementation Plan — Orchestrator Entry Point

**You are an agent tasked with implementing quibble end-to-end. This file is your contract. Read it fully, then read `DESIGN.md` at the repo root, then execute the milestones below in order. Everything you need is in this repo — do not ask the user for design decisions; they are all settled and recorded here and in `DESIGN.md`.**

## What you are building

Quibble: a Go CLI + libraries that (a) render markdown docs into beautiful static HTML and (b) manage Google-Docs-style comment threads on those docs, stored as files in git so LLM agents see them natively. `DESIGN.md` is the authoritative product/architecture spec. This plan decomposes it into implementable, independently testable milestones.

## How to execute this plan

1. **Read order:** this file → `DESIGN.md` → `plan/01-conventions.md` → the milestone file you're about to start.
2. **Milestones run in dependency order** (graph below). M1 and M2 are independent of each other and may run in parallel via subagents. Everything else is sequential unless stated.
3. **Per milestone:** implement → make the milestone's test table pass → run the full repo gate (`plan/01-conventions.md §Gate`) → commit (one or few logical commits per milestone, message prefixed `M<n>:`) → move on. Do not start a milestone whose dependencies aren't merged.
4. **Subagent dispatch:** each milestone file is written to be self-sufficient for a subagent that has also read `DESIGN.md` and `01-conventions.md`. When dispatching, give the subagent exactly: those two files + its milestone file + this instruction: *"Implement precisely what the milestone specifies. Where the spec is silent, choose the simplest option consistent with DESIGN.md and record the choice in `plan/DECISIONS.md`."*
5. **`plan/DECISIONS.md`:** append-only log (create it on first use). Every deviation from or interpretation of the spec gets one dated line. Never silently deviate.
6. **Do not redesign.** If you believe a spec detail is wrong, implement the smallest correct alternative, log it in DECISIONS.md, and continue. The user reviews via quibble itself once M6 lands (dogfooding).

## Milestone graph

```
M0 foundations
 ├─▶ M1 pkg/render (parallel-safe with M2)
 ├─▶ M2 pkg/comment (parallel-safe with M1)
 │        └─▶ M3 pkg/store (needs M2)
 │                 └─▶ M4 CLI (needs M1+M2+M3)
 │                          └─▶ M5 serve + web UI (needs M4)
 │                                   └─▶ M6 agent skill + dogfood + docs (needs M5)
 └────────────────────────────────────────▶ M7 cloud layer (needs M6; OPTIONAL — only if user asks)
```

| # | File | Delivers | Est. size |
|---|------|----------|-----------|
| M0 | `02-m0-foundations.md` | deps, cobra skeleton, config, CI, lint gate | small |
| M1 | `03-m1-render.md` | `pkg/render`: goldmark pipeline, fingerprints, theme contract, **paper** theme, `build` | large |
| M2 | `04-m2-comment-model.md` | `pkg/comment`: thread files, selectors, re-anchoring engine | large |
| M3 | `05-m3-store.md` | `pkg/store`: `CommentStore` iface, fs store, conformance suite | medium |
| M4 | `06-m4-cli.md` | `quibble init/build/comments/doctor`, `--json`, testscript e2e | medium |
| M5 | `07-m5-server-ui.md` | `quibble serve`: HTTP API, live reload, embedded comment UI | large |
| M6 | `08-m6-agent-skill-dogfood.md` | agent skill, dogfooding on this repo's docs, user docs | small |
| M7 | `09-m7-cloud-layer.md` | `Publisher` iface, capability auth, Cloudflare adapter | large, **deferred** |

Final gate: `plan/10-acceptance.md` — the end-to-end acceptance walkthrough. **The implementation is not done until every step in that file passes as written.**

## Definition of done (v0.1)

- All milestone test tables pass; `go test ./... -race` green; gate in `01-conventions.md` green in CI on `main`.
- `plan/10-acceptance.md` executes cleanly top to bottom on a fresh clone by a fresh agent.
- This repo dogfoods itself: `.quibble/` initialized, `DESIGN.md` renders via `quibble serve`, at least one real comment thread has completed the open → addressed → resolved lifecycle and lives in `_resolved/`.
- README updated from "design phase" to working usage instructions.

## What is explicitly NOT in scope for v0.1

- M7 (cloud/publish/share) — specced, not built, unless the user explicitly asks.
- Themes beyond **paper**; token *overrides* config is in scope, extra built-ins are v0.2.
- Fuzzy re-anchoring is IN scope (M2); `doctor --fix` selector rewriting is IN scope (M4). Search, print styles, mermaid are v0.2 — do not build them.
- Editing markdown through the web UI. Never in scope (DESIGN.md §13).

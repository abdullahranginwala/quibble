# M6 — Agent contract, dogfooding, user docs

**Goal:** close the loop quibble exists for: an LLM agent, pointed at any quibble-initialized repo, knows the workflow without being told — and this repo itself becomes the first user.

**Depends on:** M5.

## Deliverables

### 1. `.quibble/AGENTS.md` (written by `quibble init`; finalize the template in `internal/cli/templates/`)

The agent contract, ~40 lines, containing exactly:

- **Discovery:** "Open comment threads for a doc live in `.quibble/comments/<slug>/` — or run `quibble comments list --doc <doc> --open --json`."
- **The loop:** read open threads before working from a doc → do what each asks (doc edit / code change / answer) → `quibble comments reply <id> -m "<what you did, with commit refs>"` → `quibble comments address <id>`.
- **The two rules:** (1) **Never resolve** — `resolve` is the human's act; do not use `--author` to impersonate a human. (2) **Never edit thread files by hand** — always go through the CLI so format and lifecycle stay valid.
- **Anchors:** if your doc edits reword an anchored sentence, run `quibble doctor` and repin orphans you created.
- Set `QUIBBLE_AUTHOR` per config `authors.agent`.

### 2. `skill/` directory — a Claude Code skill (`skill/SKILL.md` + frontmatter) teaching the same contract, installable by copying into `.claude/skills/quibble/`. `quibble init --claude` copies it automatically when `.claude/` exists.

### 3. Dogfooding this repo

- `quibble init` on this repo; docs glob covers `DESIGN.md`, `README.md`, `plan/**`.
- Seed **3 real threads** on `DESIGN.md` (genuine open questions — e.g. slug collision rule, reply-marker-in-code-fence limitation, v0.2 theme priorities), then run one full lifecycle: agent addresses one via CLI, human resolves via `serve` UI. Result: ≥1 file in `_resolved/`, ≥2 open/addressed — committed as living proof and as the acceptance fixture.

### 4. User docs

- Rewrite `README.md`: install (go install + brew formula TODO), 60-second quickstart (`init` → `serve` → comment → agent loop), CLI reference table, theme config, link to DESIGN.md. Status flips from "design phase" to working-software.
- `docs/agent-workflow.md`: the human-side guide (how to review `addressed`, resolve, reopen; how doctor fits in).

## Tests

| # | Case | Expect |
|---|------|--------|
| 1 | `init` output contains AGENTS.md with all sections (golden) | template regressions caught |
| 2 | `init --claude` with `.claude/` present | skill copied; without → skipped, message says so |
| 3 | testscript: simulated agent session (env QUIBBLE_AUTHOR=claude): list → reply → address; then resolve as claude | first three succeed, resolve exits 3 |
| 4 | README quickstart commands | extracted and run verbatim in a testscript (keep quickstart honest) |

## Acceptance

- Point a *fresh* agent session at this repo with only: "There are open comments on DESIGN.md, handle them." It finds the threads, addresses them correctly, and does not resolve. (Run this for real once.)
- `plan/10-acceptance.md` full pass.

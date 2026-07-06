# Final acceptance — v0.1

**Run top to bottom on a fresh clone, ideally by an agent that did not implement the code. Every step must pass as written. This file is the definition of "quibble v0.1 works."**

Setup: macOS or Linux, Go ≥1.24, git, a browser. `git clone <repo> && cd quibble`.

## A. Gate & self-check

1. `make gate` → all four checks green.
2. `go install ./cmd/quibble` → `quibble version` prints a version.

## B. Fresh-project loop (scratch dir, not this repo)

3. `mkdir /tmp/qbl-accept && cd /tmp/qbl-accept && git init` — create `docs/plan.md` with ≥3 sections, a table, and a code block.
4. `quibble init` → `.quibble/config.yml`, `AGENTS.md` exist; rerun → "already initialized".
5. `quibble build` → open `dist/docs--plan.html` in a browser **with network disabled**: styled, TOC, highlighted code, dark toggle works.
6. `quibble serve` → browser at `127.0.0.1:4747`: doc listed; open it.
7. In the browser: select a sentence → comment "Please clarify rollback timing." → gutter bubble appears; `.quibble/comments/docs--plan/qbl-*.md` exists and is human-readable.
8. Agent side (second terminal): `QUIBBLE_AUTHOR=claude quibble comments list --open --json` shows the thread → `reply` with a message → `address` it. Browser updates live (SSE) to addressed without manual reload.
9. `QUIBBLE_AUTHOR=claude quibble comments resolve <id>` → exit 3, refused.
10. In the browser: Resolve the thread → it leaves the view; file now under `.quibble/comments/_resolved/docs--plan/`.
11. Edit `docs/plan.md`: fix a typo inside a *newly added* commented sentence (add a second comment first, then typo-edit its sentence) → `quibble doctor` reports fuzzy, exit 2 → `quibble doctor --fix` → doctor again exits 0.
12. Delete that sentence entirely → `quibble doctor` exit 3, orphan listed → browser shows it under "Unanchored comments" → `quibble comments repin <id> --quote "<other verbatim sentence>"` → doctor exit 0.
13. `git add .quibble docs && git commit` → diff is readable; thread lifecycle visible in file moves.

## C. Dogfood check (this repo)

14. `quibble serve` in the quibble repo itself: `DESIGN.md` renders beautifully; the seeded threads from M6 are visible; ≥1 resolved thread exists in `_resolved/`.
15. Fresh-agent test: a new agent session pointed at this repo with the prompt "There are open comments on DESIGN.md — handle them" completes the address loop and does not resolve anything.

## D. Robustness spot-checks

16. `quibble build` in a dir with no `.quibble/` → renders `**/*.md` with paper theme (zero-config mode).
17. Corrupt a thread file (truncate mid-frontmatter) → `comments list` still lists others; `doctor` names the corrupt file, exit 3.
18. `kill -9` the serve process → no `.tmp` files under `.quibble/`; restart clean.
19. A doc containing Hindi text + emoji: comment on a Hindi sentence; anchor round-trips; `show` displays it intact.

Sign-off: append a dated entry to `plan/DECISIONS.md`: "v0.1 acceptance passed — <date> — <agent/human>". Then, and only then, update README status and tag `v0.1.0`.

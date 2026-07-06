# Contributing to quibble

Thanks for wanting to help! Quibble is young; issues and PRs are both welcome.

## Ground rules for changes

- **Read [DESIGN.md](DESIGN.md) first.** Non-goals (§13) are firm: no user-markdown
  mutation, no WYSIWYG editing, no shared hosted backend, and the core stays
  zero-infrastructure. PRs that cross those lines will be declined regardless
  of quality.
- The comment **file format and CLI `--json` field names are frozen**
  compatibility surfaces. Changes to them need an issue + discussion before code.
- Frontend stays **zero-dependency** vanilla JS/CSS (no npm, no framework).
  Go dependencies are deliberately minimal — propose additions in an issue first.

## Dev workflow

```sh
git clone https://github.com/abdullahranginwala/quibble && cd quibble
make gate         # gofmt + vet + build + race tests — must be green
make build        # ./bin/quibble
```

- Every PR must pass `make gate` (CI enforces it on Linux + macOS).
- Bug fixes come with a regression test; features come with tests mirroring the
  style of the milestone test tables in [plan/](plan/).
- Golden files regenerate with the package's `-update` flag; inspect the diff.
- This repo dogfoods itself: doc-facing PRs may get review comments as quibble
  threads in `.quibble/comments/` — see [docs/agent-workflow.md](docs/agent-workflow.md).

## Commits & PRs

- Small, focused PRs merge fastest. Squash-merged onto `main`.
- Explain *why* in the PR body, not just what.
- If you used an AI agent to produce part of the change, that's fine — you're
  still the author: review it, test it, and say so in the PR body.

## Releases (maintainers)

Tag `vX.Y.Z` on `main` → the release workflow runs the gate, then goreleaser
builds cross-platform binaries and publishes the GitHub Release.

# quibble

**Review-grade markdown docs for humans and AI agents.**

Quibble renders your markdown into beautiful HTML and lets you comment on it Google-Docs-style — except the comments are files in your git repo. Agents (Claude Code, etc.) see open comments natively when they read the repo, address them, and reply; you review and resolve. Doc history is git history. No cloud account, no database, no daemon.

This repo dogfoods itself: run `quibble serve` here and you'll find real review threads on [DESIGN.md](DESIGN.md).

## Install

```sh
go install github.com/abdullahranginwala/quibble/cmd/quibble@latest
```

(Homebrew formula: planned.)

## 60-second quickstart

```sh
cd your-repo            # must be a git work tree
quibble init            # creates .quibble/ (config, comments store, agent contract)
quibble serve           # opens your docs at http://127.0.0.1:4747
```

In the browser: select any text in a doc → **Comment** → type your note. The thread is now a file in `.quibble/comments/`, ready to commit.

The agent side of the loop (Claude Code or any LLM agent — see `.quibble/AGENTS.md`):

```sh
export QUIBBLE_AUTHOR=agent            # the authors.agent name from config
quibble comments list --open --json    # what needs doing
quibble comments reply <id> -m "Fixed in a1b2c3d; updated §4.2."
quibble comments address <id>          # hands it back for your review
```

You then resolve (in the UI, or `quibble comments resolve <id>`). Agents cannot resolve — the CLI refuses them. Full lifecycle: **open → addressed → resolved**, with resolved threads archived under `.quibble/comments/_resolved/`.

## CLI reference

| Command | What it does |
|---|---|
| `quibble init [--claude]` | Set up `.quibble/` in a git repo; `--claude` also installs the Claude Code skill |
| `quibble build [-o dist]` | Render docs to a self-contained static site (works with zero config, too) |
| `quibble serve [--port N]` | Local review app: live-rendering, commenting, resolving (127.0.0.1 only) |
| `quibble comments list/show` | Read threads (`--open/--addressed/--resolved`, `--doc`, `--json`) |
| `quibble comments add` | New thread anchored to a verbatim `--quote` from the doc |
| `quibble comments reply/address/resolve/reopen` | Move a thread through its lifecycle |
| `quibble comments repin` | Re-anchor an orphaned thread to a fresh quote |
| `quibble doctor [--fix]` | Anchor health for every thread; `--fix` self-heals drifted anchors |

## How anchoring works

Comments anchor to *text*, not line numbers. Each thread stores the quoted text plus context (prefix/suffix, heading path, position) and is re-located on every render: exact match → context-disambiguated match → fuzzy match (scored, shown with a dashed underline) → orphan (surfaced in an "Unanchored comments" panel, never silently dropped). `quibble doctor --fix` rewrites drifted anchors; your markdown files are **never** modified by quibble.

## Themes

One built-in theme today — **paper**, an editorial long-form reading theme with light/dark — on a design-token contract (`--qbl-*` CSS custom properties) that the comment UI styles itself from. Rebrand without writing a theme:

```yaml
# .quibble/config.yml
theme:
  name: paper
  overrides:
    --qbl-accent: "#7c3aed"
```

`ink` and `terminal` themes plus full custom theme directories are the v0.2 roadmap (see [DESIGN.md](DESIGN.md) §5.1).

## Docs

- [DESIGN.md](DESIGN.md) — architecture: git-native comment store, anchoring model, theme contract, cloud layer.
- [docs/agent-workflow.md](docs/agent-workflow.md) — the human side of reviewing agent work.
- `.quibble/AGENTS.md` (in any initialized repo) — the binding agent contract.
- [plan/](plan/) — the milestone-by-milestone implementation plan this was built from.

Inspired by [Parchi](https://github.com/shawshankkumar/Parchi); differs by making **git the comment database** so no cloud infrastructure is required for the core workflow.

## License

MIT

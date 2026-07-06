# quibble

**Review-grade markdown docs for humans and AI agents.**

Quibble renders your markdown into beautiful HTML and lets you comment on it Google-Docs-style — except the comments are files in your git repo. Agents (Claude Code, etc.) see open comments natively when they read the repo, address them, and reply; you review and resolve. Doc history is git history. No cloud account, no database, no daemon.

> Status: **design complete, implementation starting**. [DESIGN.md](DESIGN.md) is the architecture spec; [plan/00-INDEX.md](plan/00-INDEX.md) is the full milestone-by-milestone implementation plan (an agent can be pointed at that file cold and build v0.1 end-to-end).

## The loop

```
you: quibble serve → read the rendered doc → select text → drop a comment
agent: quibble comments list --open → does the work → replies → marks addressed
you: review addressed threads → resolve (or reopen)
```

Comments live in `.quibble/comments/` as one markdown file per thread, anchored to the text they reference with edit-resilient selectors. Resolved threads are archived in-repo. Everything is committed, branched, and PR'd like the code it describes.

## What's inside

- `pkg/render` — importable Go library: markdown → clean, readable static HTML (goldmark + chroma, light/dark, TOC). Built-in themes (`paper`, `ink`, `terminal`) on a design-token contract; bring your own theme as a directory that fills the same contract.
- `quibble` CLI — `build`, `serve` (local review app), `comments` (full lifecycle), `doctor` (anchor health).
- Optional publish adapters (Cloudflare, AWS — self-hosted on *your* account) for sharing docs beyond the repo. Planned, see roadmap.

Inspired by [Parchi](https://github.com/shawshankkumar/Parchi); differs by making **git the comment database** so no cloud infrastructure is required for the core workflow.

## License

MIT

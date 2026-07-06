# Reviewing agent work with quibble

The human side of the loop. (The agent side lives in `.quibble/AGENTS.md`,
written by `quibble init`.)

## The loop, from your chair

1. **Comment.** `quibble serve`, open a doc, select text, write what you want
   changed or clarified. Each comment is a thread file in
   `.quibble/comments/<doc-slug>/` — commit it like code.
2. **Hand off.** Point your agent at the repo. The contract in
   `.quibble/AGENTS.md` tells it to read open threads before working from any
   doc, do what they ask, reply with what changed, and mark them `addressed`.
3. **Review what's addressed.** The doc header in `serve` shows the addressed
   count, or run `quibble comments list --addressed`. Read the agent's reply,
   check the doc/code it cites.
4. **Resolve or reopen.**
   - Satisfied → **Resolve** (UI button, or `quibble comments resolve <id>`).
     The thread file moves to `.quibble/comments/_resolved/<doc-slug>/` — out
     of every agent's default view, but preserved with full history.
   - Not satisfied → **Reopen** with a reply saying why. It's back in the
     agent's queue.

Only humans can resolve. The CLI and server refuse resolution by the
configured agent author (exit code 3 / HTTP 403) — an agent claiming its own
work is done isn't review.

## Keeping anchors healthy

Docs change; anchors follow. After heavy edits run:

```sh
quibble doctor        # exit 0: all exact · 2: some fuzzy · 3: orphans/corrupt
quibble doctor --fix  # rewrite fuzzy-but-confident anchors in place
```

Orphans (anchored text deleted outright) surface at the top of the doc in
`serve` and in doctor's report. Re-pin one to new text with:

```sh
quibble comments repin <id> --quote "<verbatim sentence from the doc>"
```

## Attribution

Every write records an author: `--author` flag > `QUIBBLE_AUTHOR` env >
`.quibble/config.yml` defaults (`authors.human` for you, `authors.agent` for
the agent). Keep the two distinct — the resolve policy depends on it.

## Where everything lives

```
.quibble/
  config.yml               # docs globs, theme, authors
  AGENTS.md                 # the agent contract
  comments/<slug>/*.md      # open + addressed threads
  comments/_resolved/<slug>/*.md   # resolved archive
```

All of it is plain text in git: `git log -p .quibble/comments` *is* the audit
trail of your doc reviews.

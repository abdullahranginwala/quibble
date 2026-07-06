---
name: quibble
description: Work with quibble doc-review comment threads in this repo. Use when the user mentions doc comments, review threads, addressing feedback on markdown docs, or when .quibble/ exists and you are about to work from a markdown doc (plans, RFCs, runbooks).
---

# Quibble: git-native doc review

This repo uses quibble — markdown docs carry anchored comment threads stored
as files under `.quibble/comments/`. The full binding contract is in
`.quibble/AGENTS.md`; this skill is the working summary.

## Before working from any markdown doc

```sh
quibble comments list --doc <doc> --open --json
```

Open threads are review feedback addressed to you. Read them before acting on
the doc's content; they may change what the doc is asking for.

## Handling a thread

1. Do what it asks (doc edit, code change, or an answer).
2. Reply citing what changed, with commit hashes/paths:
   `quibble comments reply <id> -m "..."`
3. Mark it addressed: `quibble comments address <id>`
4. Set `QUIBBLE_AUTHOR` to the `authors.agent` value from `.quibble/config.yml`.

## Hard rules

- **Never run `quibble comments resolve`** — resolution is the human
  reviewer's act. The CLI refuses agent authors; do not bypass with `--author`.
- **Never edit `.quibble/comments/*.md` by hand** — always use the CLI.
- If you reworded anchored text in a doc, run `quibble doctor` (then
  `doctor --fix`, and `comments repin <id> --quote "..."` for orphans you made).

## Useful commands

```sh
quibble comments show <id>        # full thread with replies
quibble comments list --addressed # what awaits human review
quibble doctor                    # anchor health for all threads
quibble serve                     # human-facing review UI (don't run headless)
```

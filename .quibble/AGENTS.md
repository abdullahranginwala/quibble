# Quibble agent contract

This repository uses [quibble](https://github.com/abdullahranginwala/quibble):
markdown docs carry anchored, threaded review comments stored as files in git.
If you are an LLM agent working in this repo, this contract is binding.

## Discovery

Open comment threads for a doc live in `.quibble/comments/<doc-slug>/` — one
markdown file per thread, YAML frontmatter + body + replies. Resolved threads
are archived under `.quibble/comments/_resolved/` and need no attention.

Prefer the CLI for reading (it filters and re-anchors for you):

```sh
quibble comments list --open --json            # everything actionable
quibble comments list --doc docs/plan.md --open
quibble comments show <id>
```

## The loop

1. **Before working from a doc, check its open threads.** They are review
   feedback addressed to you; treat them as part of the task.
2. **Do what each thread asks** — edit the doc, change code, or answer the
   question asked.
3. **Reply, citing what you changed** (include commit hashes or file paths):
   `quibble comments reply <id> -m "Switched the key to bookingId in a1b2c3d; updated §4.2."`
4. **Mark it addressed:** `quibble comments address <id>`.

## The two rules

1. **Never resolve.** `quibble comments resolve` is the human reviewer's act —
   the tool refuses it for agent authors, and you must not impersonate a human
   via `--author` to get around that.
2. **Never edit thread files by hand.** Always go through the CLI so the file
   format and lifecycle stay valid.

## Anchors

If your doc edits reword a sentence that a thread is anchored to, run
`quibble doctor` afterwards; heal drifted anchors with `quibble doctor --fix`,
and repin any orphan you created: `quibble comments repin <id> --quote "<verbatim text>"`.

## Identity

Set `QUIBBLE_AUTHOR` to the agent author name from `.quibble/config.yml`
(`authors.agent`) so your comments are attributed correctly.

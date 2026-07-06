# M5 manual checklist — `quibble serve` in a real browser

The 13-row server table is covered by automated httptest tests
(`internal/server/server_test.go`). The items below need a human at a browser
(Safari + Chrome + Firefox, current versions) and are unticked until verified.

Run against this repo's own docs:

```sh
quibble serve            # then open http://127.0.0.1:4747
```

## DOM / highlighting

- [ ] Select across bold/italic/link text → the mark wraps correctly, no broken DOM
- [ ] Selection spanning two blocks → the comment button is *not* offered (v0.1 single-block-anchor limitation; documented)
- [ ] Margin bubbles align to their marks after a window resize and after toggling the TOC
- [ ] Dark mode: marks, bubbles, and the thread panel are all legible (the `--qbl-*` tokens carry the styling)

## Drafts / live updates

- [ ] A draft reply survives a `doc-changed` toast (no auto-reload while typing)

## Full loop

- [ ] Comment in the browser → (run the CLI as the agent) `quibble comments address <id>` → the UI shows the thread as *addressed* → resolve it in the UI → the thread leaves the reading view and its file is in `.quibble/comments/_resolved/`

## Acceptance (DESIGN.md dogfood)

- [ ] Complete the loop above against `DESIGN.md` itself
- [ ] `kill -9` the server mid-comment-write → the repo is never left with a `.tmp` file or a corrupt thread (atomic writes from M3)

## Notes for the verifier

- Fuzzy placements render with a dashed underline; hover shows a "N% confidence" tooltip.
- Orphaned (unanchored) comments appear in a pinned "Unanchored comments (N)" panel at the top of the article.
- The header shows open/addressed counts and a "resolved…" link to the read-only `?include=resolved` view.
- The comment button, panel, and bubbles are injected by `/qbl/comments.js`; if they do not appear, confirm the browser console shows no module-load or CSP errors.

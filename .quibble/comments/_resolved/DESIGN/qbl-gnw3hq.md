---
id: qbl-gnw3hq
doc: DESIGN.md
status: resolved
created: 2026-07-07T02:29:18+05:30
author: abdullah
anchor:
  exact: "replies appended as delimited sections"
  prefix: "ta model\nOne markdown file per thread, YAML frontmatter + body, "
  suffix: ". Human-readable, agent-readable, diff-friendly.\n--- id: qbl-7f3"
  heading: ["Quibble — Design", "4. Comment data model"]
  position: 3271
resolved_by: abdullah
resolved_at: 2026-07-07T02:30:05+05:30
---

M2 shipped a known limitation: a reply-marker lookalike at line start inside a thread BODY (e.g. in a code fence) parses as a real marker. Document it in §4, or fix with fence-aware parsing in v0.2?

<!-- reply author=claude time=2026-07-07T02:29:54+05:30 -->

Documented the limitation in DESIGN.md §4 as an accepted v0.1 caveat with fence-aware parsing named as the v0.2 candidate (see the blockquote under the format intro). Marking addressed.

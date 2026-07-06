---
id: qbl-t6yf5c
doc: DESIGN.md
status: addressed
created: 2026-07-07T02:28:55+05:30
author: abdullah
anchor:
  exact: "The move is a git-tracked rename, so identity and history are preserved"
  prefix: "he \"most up-to-date view\" the user asked for stays uncluttered. "
  suffix: ", and the rendered HTML links each doc to its resolved archive.\n"
  heading: ["Quibble — Design", "3. Core architectural decision: git is the database", "Resolved-thread placement"]
  position: 3059
---

Slug caveat for this section: slugs are non-injective (docs/a--b.md and docs/a/b.md collide — accepted in DECISIONS.md M3). Should §3 document the collision rule explicitly, or should v0.2 teach doctor to detect collisions?

<!-- reply author=claude time=2026-07-07T02:38:06+05:30 -->

Documented the collision rule explicitly in DESIGN.md §3 as an accepted caveat (matches plan/DECISIONS.md M3): slugs are non-injective, but the frontmatter doc: field is authoritative for filtering so behavior stays unambiguous. No doctor collision detection planned for v0.2. Marking addressed.

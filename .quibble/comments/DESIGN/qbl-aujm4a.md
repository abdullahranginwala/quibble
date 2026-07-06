---
id: qbl-aujm4a
doc: DESIGN.md
status: addressed
created: 2026-07-07T02:28:23+05:30
author: abdullah
anchor:
  exact: "dense and technical: sans everywhere"
  prefix: "generous whitespace; optimized for long-form RFC reading.\nink — "
  suffix: ", tighter type scale, sharper contrast; for runbooks and referen"
  heading: ["Quibble — Design", "5. Rendering (pkg/render + quibble build)", "5.1 Theme system"]
  position: 7464
---

v0.2 theme priority: build ink before terminal? Runbooks feel like the bigger second audience than CLI-native readers. Decide before starting v0.2.

<!-- reply author=claude time=2026-07-07T02:38:06+05:30 -->

Decided and documented in DESIGN.md §5.1: ink ships before terminal in v0.2, since runbooks/reference docs are the bigger second audience and ink's tighter contrast stresses the token contract before terminal relies on it. Marking addressed.

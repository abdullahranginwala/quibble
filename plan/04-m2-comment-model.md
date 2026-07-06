# M2 — `pkg/comment`: thread model, selectors, re-anchoring

**Goal:** the data heart of quibble. Thread files parse/serialize round-trip-exactly; anchors resolve against edited documents; nothing is ever silently dropped.

**Depends on:** M0. **Parallel-safe with:** M1 (uses `render.NormalizeBlocks` only in tests; coordinate via the frozen golden in M1 #15 — if M1 hasn't landed, vendor the normalization spec from `03-m1-render.md` step 4 into a local test helper and reconcile when both merge).

## Public API (exact)

```go
package comment

type Status string
const (
    StatusOpen      Status = "open"
    StatusAddressed Status = "addressed"
    StatusResolved  Status = "resolved"
)

type Anchor struct {
    Exact    string   `yaml:"exact"`
    Prefix   string   `yaml:"prefix"`             // ≤64 runes of context before
    Suffix   string   `yaml:"suffix"`             // ≤64 runes after
    Heading  []string `yaml:"heading,omitempty"`  // heading path at creation, outermost first
    Position int      `yaml:"position"`           // rune offset hint into Doc.Text at creation
}

type Reply struct {
    Author string
    Time   time.Time
    Body   string
}

type Thread struct {
    ID         string
    Doc        string   // rel path of the document, e.g. "docs/plan.md"
    Status     Status
    Created    time.Time
    Author     string
    Anchor     Anchor
    Body       string
    Replies    []Reply
    ResolvedBy string
    ResolvedAt *time.Time
}

func NewID() string                          // "qbl-" + 6 lowercase base32 chars (crypto/rand)
func ParseThread(src []byte) (*Thread, error)
func (t *Thread) Marshal() ([]byte, error)   // Parse(Marshal(t)) == t, byte-stable on re-marshal
func (t *Thread) Validate() error            // id format, doc non-empty, status known, anchor exact non-empty

// --- anchoring ---

type Method string
const (
    MethodExact   Method = "exact"
    MethodContext Method = "context"
    MethodFuzzy   Method = "fuzzy"
    MethodOrphan  Method = "orphan"
)

type Placement struct {
    Start, End int     // rune offsets into the doc's normalized text; -1,-1 when orphan
    Confidence float64 // 1.0 exact/context; fuzzy score otherwise; 0 orphan
    Method     Method
}

// Resolve locates a in docText (the render-normalized text of the whole doc).
// headings maps each heading's normalized text to its section's [start,end) span,
// as produced by the sectionizer below.
func Resolve(docText string, sections []Section, a Anchor) Placement

type Section struct {
    Path       []string // heading path, outermost first
    Start, End int      // rune span of the section in docText
}
```

## File format (frozen — this is a compatibility surface)

`Marshal` emits exactly this shape; `ParseThread` accepts it plus tolerable whitespace variance:

```markdown
---
id: qbl-7f3k2a
doc: docs/deployment-plan.md
status: open
created: 2026-07-06T14:31:00+05:30
author: abdullah
anchor:
  exact: "the retry loop will re-attempt every 30 minutes"
  prefix: "guest is charged but marked failed, "
  suffix: ". This is the double-charge window"
  heading: ["Rollback plan", "Failure modes"]
  position: 14382
---

Shouldn't this be idempotency-keyed on booking id, not attempt count?

<!-- reply author=claude time=2026-07-06T15:02:00+05:30 -->

Agreed — switched the key and updated §4.2. Marking addressed.
```

Rules:

- Frontmatter is YAML between `---` fences. `resolved_by`/`resolved_at` keys appear only when set.
- Body = everything after frontmatter up to the first reply marker, trimmed.
- Reply marker: `<!-- reply author=<token> time=<RFC3339> -->` on its own line; author token is `[a-z0-9_.-]+`. Reply body runs to the next marker or EOF, trimmed. Replies stay in file order.
- Bodies are markdown but `pkg/comment` treats them as opaque strings.
- Parse failures return errors that include the offending line number. A directory scan (M3) must be able to report "thread file X is corrupt" without dying.

## Anchoring algorithm (DESIGN.md §6, exact order)

Given `docText` (normalized, from `render`), `sections`, anchor `a`:

1. **Exact:** find all occurrences of `a.Exact` in `docText`.
   - 1 match → `Placement{start, end, 1.0, MethodExact}`.
   - >1 match → **context:** keep occurrences where prefix/suffix match (compare up to available lengths). If exactly 1 survives → `MethodContext`, confidence 1.0. If several → pick nearest to `a.Position`, `MethodContext`, confidence 1.0.
   - 0 matches → step 2.
2. **Fuzzy:** sliding window of `len(a.Exact)` runes (±20% window-length variants: 0.8×, 1.0×, 1.2×), step = max(1, len/20), scored by normalized similarity `1 - levenshtein(window, exact)/max(len)`. Search order: (a) within the section whose `Path` equals `a.Heading` (if such a section exists), (b) whole doc. First phase reaching a score ≥ **0.75** wins; take the best-scoring window of that phase → `Placement{…, score, MethodFuzzy}`.
3. **Orphan:** `Placement{-1, -1, 0, MethodOrphan}`.

Performance bar: `Resolve` on a 200 KB doc with a 200-rune anchor completes < 100 ms (fuzzy path). Use a banded/early-exit levenshtein; do not allocate per-window.

Also ship the **sectionizer**: `Sectionize(outline []render.Heading, docText string, blocks []render.Block) []Section` — derives heading-path spans; used by Resolve callers (M4/M5). If M1 is not merged yet, define the minimal local structs and reconcile.

And the **anchor factory** used when creating comments: `NewAnchor(docText string, sections []Section, start, end int) Anchor` — captures exact = selected range, ≤64-rune prefix/suffix, heading path of the containing section, position = start.

## Tests

Round-trip / format:

| # | Case | Expect |
|---|------|--------|
| 1 | Marshal→Parse→Marshal on a full thread (2 replies, resolved) | byte-identical second marshal; struct equality after parse |
| 2 | Thread with no replies, open | round-trips; no reply markers emitted |
| 3 | Body containing `---` and `<!-- reply` **inside a code fence** | body preserved intact (marker must be at line start, outside frontmatter — document the simple rule: markers are matched at line start; a marker-lookalike inside body at line start is a known limitation, log in DECISIONS.md) |
| 4 | Unknown status value | `Validate` error |
| 5 | Corrupt frontmatter (bad YAML) | error with line number, no panic |
| 6 | Reply with future/zoned timestamps (+05:30, Z) | parsed with zone preserved |
| 7 | `NewID` ×10k | all match `^qbl-[a-z2-7]{6}$`, no dupes |
| 8 | Unicode body + anchor (Hindi, emoji) | round-trips; rune offsets not byte offsets |

Anchoring (each row = one table-driven case; doc fixtures in `testdata/anchors/`):

| # | Doc mutation vs. creation time | Expect |
|---|-------------------------------|--------|
| 9 | Unchanged doc | exact, correct span |
| 10 | Text moved to a different section wholesale | exact (position hint irrelevant) |
| 11 | Anchor text appears 3× (repeated boilerplate), context unique | context picks the right one |
| 12 | Anchor text 3×, context also identical | context → nearest to position |
| 13 | One typo fixed inside anchored sentence | fuzzy ≥0.9, correct span |
| 14 | Sentence reworded ~20% | fuzzy ≥0.75, correct span |
| 15 | Sentence deleted entirely | orphan |
| 16 | Sentence rewritten beyond recognition (>25% distance) | orphan (no false re-anchor — this matters more than recall) |
| 17 | Heading renamed but sentence intact | exact still wins (heading only guides fuzzy) |
| 18 | Same sentence exists in 2 sections; anchor's heading section survives edit | fuzzy phase (a) finds in-section match first |
| 19 | Anchor at very start / very end of doc | prefix/suffix shorter than 64 runes handled |
| 20 | 200 KB doc, fuzzy path | < 100 ms (use `testing.B` + a hard assert in a regular test with generous 500 ms CI margin) |
| 21 | `NewAnchor` on a mid-doc range | prefix/suffix ≤64 runes, heading path correct; `Resolve(NewAnchor(...))` = exact at same span |

## Acceptance

- All tables green under `-race`.
- Fuzz test (`go test -fuzz=FuzzParseThread -fuzztime=30s` locally, seed corpus committed): `ParseThread` never panics on arbitrary bytes.

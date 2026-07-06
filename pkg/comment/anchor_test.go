package comment

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// --- Test document model -----------------------------------------------------
//
// pkg/comment must not import pkg/render, so these tests vendor render's frozen
// normalization rule (plan/03-m1-render.md step 4): within each top-level block
// collapse whitespace runs to a single space and trim; join blocks with "\n".
// Blocks are separated by blank lines; a block whose first line starts with '#'
// is a heading. This mirrors render.NormalizeBlocks / render.Block / .Heading
// closely enough to exercise Sectionize + Resolve end-to-end. See DECISIONS.md.

var blankLineRe = regexp.MustCompile(`\n[ \t]*\n+`)

func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func headingOf(block string) (level int, text string, ok bool) {
	first := block
	if i := strings.IndexByte(block, '\n'); i >= 0 {
		first = block[:i]
	}
	trimmed := strings.TrimLeft(first, " \t")
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return 0, "", false
	}
	if n < len(trimmed) && trimmed[n] != ' ' {
		return 0, "", false
	}
	return n, normalizeWS(trimmed[n:]), true
}

type testDoc struct {
	text     string
	blocks   []Block
	outline  []Heading
	sections []Section
}

func loadDocString(src string) testDoc {
	raw := blankLineRe.Split(strings.TrimSpace(src), -1)
	var d testDoc
	var parts []string
	offset := 0
	for _, rb := range raw {
		level, htext, isHeading := headingOf(rb)
		norm := htext
		if !isHeading {
			norm = normalizeWS(rb)
		}
		if norm == "" {
			continue
		}
		start := offset
		end := start + len([]rune(norm))
		d.blocks = append(d.blocks, Block{ID: fmt.Sprintf("b%d", len(d.blocks)), Text: norm, Start: start, End: end})
		if isHeading {
			d.outline = append(d.outline, Heading{Level: level, Text: norm})
		}
		parts = append(parts, norm)
		offset = end + 1 // "\n" join
	}
	d.text = strings.Join(parts, "\n")
	d.sections = Sectionize(d.outline, d.text, d.blocks)
	return d
}

func loadFixture(t *testing.T, name string) testDoc {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "anchors", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return loadDocString(string(b))
}

// runeIndexOf returns the rune [start,end) of the first occurrence of sub.
func runeIndexOf(t *testing.T, docText, sub string) (int, int) {
	t.Helper()
	dr := []rune(docText)
	sr := []rune(sub)
	for i := 0; i+len(sr) <= len(dr); i++ {
		ok := true
		for j := range sr {
			if dr[i+j] != sr[j] {
				ok = false
				break
			}
		}
		if ok {
			return i, i + len(sr)
		}
	}
	t.Fatalf("substring not found in docText: %q", sub)
	return 0, 0
}

func nthRuneIndexOf(t *testing.T, docText, sub string, n int) (int, int) {
	t.Helper()
	dr := []rune(docText)
	sr := []rune(sub)
	count := 0
	for i := 0; i+len(sr) <= len(dr); i++ {
		ok := true
		for j := range sr {
			if dr[i+j] != sr[j] {
				ok = false
				break
			}
		}
		if ok {
			if count == n {
				return i, i + len(sr)
			}
			count++
			i += len(sr) - 1
		}
	}
	t.Fatalf("occurrence %d of %q not found", n, sub)
	return 0, 0
}

// Row 9: unchanged doc → exact, correct span.
func TestRow9_UnchangedExact(t *testing.T) {
	d := loadFixture(t, "plan.md")
	sentence := "The retry loop will re-attempt every 30 minutes until the charge succeeds or the booking is cancelled."
	start, end := runeIndexOf(t, d.text, sentence)
	a := NewAnchor(d.text, d.sections, start, end)

	p := Resolve(d.text, d.sections, a)
	if p.Method != MethodExact {
		t.Fatalf("method = %s, want exact", p.Method)
	}
	if p.Start != start || p.End != end {
		t.Fatalf("span = [%d,%d), want [%d,%d)", p.Start, p.End, start, end)
	}
	if p.Confidence != 1.0 {
		t.Fatalf("confidence = %v, want 1.0", p.Confidence)
	}
}

// Row 10: text moved wholesale to a different section → exact, position hint irrelevant.
func TestRow10_MovedWholesale(t *testing.T) {
	base := loadFixture(t, "plan.md")
	sentence := "Operators can trigger a manual reconciliation from the admin console at any time during an incident."
	start, end := runeIndexOf(t, base.text, sentence)
	a := NewAnchor(base.text, base.sections, start, end)

	// Move the sentence from the Recovery section to the very end (Appendix).
	moved := loadDocString(`# Deployment Plan

Intro paragraph that shifts every offset in the document downward by a lot.

## Monitoring

Dashboards track charge success rate, latency, and the daily reconciliation count across every currency.

## Recovery

Nothing here anymore.

## Appendix

Operators can trigger a manual reconciliation from the admin console at any time during an incident.`)

	p := Resolve(moved.text, moved.sections, a)
	if p.Method != MethodExact {
		t.Fatalf("method = %s, want exact", p.Method)
	}
	ns, ne := runeIndexOf(t, moved.text, sentence)
	if p.Start != ns || p.End != ne {
		t.Fatalf("span = [%d,%d), want moved [%d,%d)", p.Start, p.End, ns, ne)
	}
}

// Row 11: anchor text appears 3×, context unique → context picks the right one.
func TestRow11_ContextUnique(t *testing.T) {
	d := loadFixture(t, "boilerplate.md")
	sentence := "Please review this section carefully before sign-off."
	// Anchor at the FIRST occurrence (under Alpha); NewAnchor captures its
	// unique neighbouring context.
	start, end := nthRuneIndexOf(t, d.text, sentence, 0)
	a := NewAnchor(d.text, d.sections, start, end)

	p := Resolve(d.text, d.sections, a)
	if p.Method != MethodContext {
		t.Fatalf("method = %s, want context", p.Method)
	}
	if p.Start != start {
		t.Fatalf("start = %d, want first occurrence %d", p.Start, start)
	}
}

// Row 12: anchor text 3×, context also identical → nearest to position.
func TestRow12_ContextIdenticalNearestPosition(t *testing.T) {
	d := loadFixture(t, "boilerplate_same.md")
	sentence := "Please review this section carefully before sign-off."
	// Aim at the SECOND occurrence via position, with empty context so every
	// occurrence survives the context filter.
	s2, _ := nthRuneIndexOf(t, d.text, sentence, 1)
	a := Anchor{Exact: sentence, Position: s2}

	p := Resolve(d.text, d.sections, a)
	if p.Method != MethodContext {
		t.Fatalf("method = %s, want context", p.Method)
	}
	if p.Start != s2 {
		t.Fatalf("start = %d, want nearest-to-position (2nd) %d", p.Start, s2)
	}
}

// Row 13: one typo fixed inside the sentence → fuzzy ≥0.9, correct span.
func TestRow13_OneTypoFuzzy(t *testing.T) {
	base := loadFixture(t, "plan.md")
	sentence := "Dashboards track charge success rate, latency, and the daily reconciliation count across every currency."
	start, end := runeIndexOf(t, base.text, sentence)
	a := NewAnchor(base.text, base.sections, start, end)

	mutated := loadDocString(strings.Replace(readFixture(t, "plan.md"),
		"reconciliation count across every currency", "reconcilliation count across every currency", 1))

	p := Resolve(mutated.text, mutated.sections, a)
	if p.Method != MethodFuzzy {
		t.Fatalf("method = %s, want fuzzy", p.Method)
	}
	if p.Confidence < 0.9 {
		t.Fatalf("confidence = %v, want ≥0.9", p.Confidence)
	}
	ts, _ := runeIndexOf(t, mutated.text, "Dashboards track charge success rate")
	if abs(p.Start-ts) > len([]rune(sentence))/5 {
		t.Fatalf("start = %d not near true start %d", p.Start, ts)
	}
	_ = end
}

// Row 14: sentence reworded ~20% → fuzzy ≥0.75, correct span.
func TestRow14_RewordFuzzy(t *testing.T) {
	base := loadFixture(t, "plan.md")
	sentence := "Operators can trigger a manual reconciliation from the admin console at any time during an incident."
	start, end := runeIndexOf(t, base.text, sentence)
	a := NewAnchor(base.text, base.sections, start, end)

	reword := "Operators can start a manual reconciliation from the admin panel at any moment during an incident."
	mutated := loadDocString(strings.Replace(readFixture(t, "plan.md"), sentence, reword, 1))

	p := Resolve(mutated.text, mutated.sections, a)
	if p.Method != MethodFuzzy {
		t.Fatalf("method = %s, want fuzzy", p.Method)
	}
	if p.Confidence < 0.75 {
		t.Fatalf("confidence = %v, want ≥0.75", p.Confidence)
	}
	ts, _ := runeIndexOf(t, mutated.text, "Operators can start a manual reconciliation")
	if abs(p.Start-ts) > len([]rune(sentence))/4 {
		t.Fatalf("start = %d not near true start %d", p.Start, ts)
	}
	_ = end
}

// Row 15: sentence deleted entirely → orphan.
func TestRow15_DeletedOrphan(t *testing.T) {
	base := loadFixture(t, "plan.md")
	sentence := "Refer to the runbook for the full escalation matrix and on-call rotation details."
	start, end := runeIndexOf(t, base.text, sentence)
	a := NewAnchor(base.text, base.sections, start, end)

	mutated := loadDocString(strings.Replace(readFixture(t, "plan.md"), sentence, "", 1))

	p := Resolve(mutated.text, mutated.sections, a)
	if p.Method != MethodOrphan || p.Start != -1 || p.End != -1 {
		t.Fatalf("placement = %+v, want orphan (-1,-1)", p)
	}
}

// Row 16: sentence rewritten beyond recognition (>25% distance) → orphan (no false re-anchor).
func TestRow16_RewrittenOrphan(t *testing.T) {
	base := loadFixture(t, "plan.md")
	sentence := "Refer to the runbook for the full escalation matrix and on-call rotation details."
	start, end := runeIndexOf(t, base.text, sentence)
	a := NewAnchor(base.text, base.sections, start, end)

	replacement := "Bananas ripen a little faster next to apples in a warm sunny kitchen."
	mutated := loadDocString(strings.Replace(readFixture(t, "plan.md"), sentence, replacement, 1))

	p := Resolve(mutated.text, mutated.sections, a)
	if p.Method != MethodOrphan {
		t.Fatalf("method = %s, want orphan — false re-anchor is worse than an orphan", p.Method)
	}
}

// Row 17: heading renamed but sentence intact → exact still wins.
func TestRow17_HeadingRenamedExact(t *testing.T) {
	base := loadFixture(t, "plan.md")
	sentence := "Guest is charged but marked failed, which opens a double-charge window that must be closed by reconciliation."
	start, end := runeIndexOf(t, base.text, sentence)
	a := NewAnchor(base.text, base.sections, start, end)
	if len(a.Heading) == 0 {
		t.Fatal("expected a heading path on the anchor")
	}

	mutated := loadDocString(strings.Replace(readFixture(t, "plan.md"), "## Rollback plan", "## Backout plan", 1))

	p := Resolve(mutated.text, mutated.sections, a)
	if p.Method != MethodExact {
		t.Fatalf("method = %s, want exact (heading only guides fuzzy)", p.Method)
	}
	ns, ne := runeIndexOf(t, mutated.text, sentence)
	if p.Start != ns || p.End != ne {
		t.Fatalf("span = [%d,%d), want [%d,%d)", p.Start, p.End, ns, ne)
	}
}

// Row 18: same sentence in two sections; anchor's heading section survives edit
// → fuzzy phase (a) finds the in-section match first, even though the other
// section holds a higher-scoring window.
func TestRow18_InSectionFuzzyPrecedence(t *testing.T) {
	base := loadFixture(t, "two_sections.md")
	sentence := "The reconciliation job runs nightly and settles all pending charges before dawn."
	// Anchor the Alpha copy.
	aStart, aEnd := nthRuneIndexOf(t, base.text, sentence, 0)
	a := NewAnchor(base.text, base.sections, aStart, aEnd)
	if !pathEqual(a.Heading, []string{"Doc", "Alpha"}) {
		t.Fatalf("anchor heading = %v, want [Doc Alpha]", a.Heading)
	}

	// Mutate: Alpha reworded (score ~0.85), Beta a single-char typo (~0.98).
	src := readFixture(t, "two_sections.md")
	// Replace only the first occurrence (Alpha) with a reword.
	alphaReword := "The reconciliation task executes nightly and settles all pending charges before dawn."
	src = strings.Replace(src, sentence, alphaReword, 1)
	// Now replace the remaining (Beta) occurrence with a small typo.
	betaTypo := "The reconciliation job runs nightly and settles all pending charges before dwan."
	src = strings.Replace(src, sentence, betaTypo, 1)
	mutated := loadDocString(src)

	// Find the Alpha section span in the mutated doc.
	var alpha Section
	for _, s := range mutated.sections {
		if pathEqual(s.Path, []string{"Doc", "Alpha"}) {
			alpha = s
		}
	}

	p := Resolve(mutated.text, mutated.sections, a)
	if p.Method != MethodFuzzy {
		t.Fatalf("method = %s, want fuzzy", p.Method)
	}
	if p.Start < alpha.Start || p.Start >= alpha.End {
		t.Fatalf("match at %d is outside Alpha section [%d,%d) — phase (a) precedence broken",
			p.Start, alpha.Start, alpha.End)
	}
}

// Row 19: anchor at very start / very end of doc → short prefix/suffix handled.
func TestRow19_BoundaryContext(t *testing.T) {
	d := loadFixture(t, "plan.md")

	// Very start: first block, prefix must be empty (shorter than 64).
	firstStart, firstEnd := runeIndexOf(t, d.text, "Deployment Plan")
	if firstStart != 0 {
		t.Fatalf("first block not at offset 0: %d", firstStart)
	}
	aStart := NewAnchor(d.text, d.sections, firstStart, firstEnd)
	if aStart.Prefix != "" {
		t.Fatalf("start-of-doc prefix should be empty, got %q", aStart.Prefix)
	}
	if len([]rune(aStart.Suffix)) > 64 {
		t.Fatalf("suffix exceeds 64 runes: %d", len([]rune(aStart.Suffix)))
	}
	if p := Resolve(d.text, d.sections, aStart); p.Method != MethodExact || p.Start != firstStart {
		t.Fatalf("start anchor did not resolve exact: %+v", p)
	}

	// Very end: last block, suffix must be empty.
	last := "Refer to the runbook for the full escalation matrix and on-call rotation details."
	lStart, lEnd := runeIndexOf(t, d.text, last)
	if lEnd != len([]rune(d.text)) {
		t.Fatalf("last block does not end at EOF: end=%d len=%d", lEnd, len([]rune(d.text)))
	}
	aEnd := NewAnchor(d.text, d.sections, lStart, lEnd)
	if aEnd.Suffix != "" {
		t.Fatalf("end-of-doc suffix should be empty, got %q", aEnd.Suffix)
	}
	if len([]rune(aEnd.Prefix)) > 64 {
		t.Fatalf("prefix exceeds 64 runes: %d", len([]rune(aEnd.Prefix)))
	}
	if p := Resolve(d.text, d.sections, aEnd); p.Method != MethodExact || p.Start != lStart {
		t.Fatalf("end anchor did not resolve exact: %+v", p)
	}
}

// Row 21: NewAnchor on a mid-doc range → bounded context, correct heading path,
// and Resolve(NewAnchor(...)) == exact at the same span.
func TestRow21_NewAnchorMidDoc(t *testing.T) {
	d := loadFixture(t, "plan.md")
	sentence := "Guest is charged but marked failed, which opens a double-charge window that must be closed by reconciliation."
	start, end := runeIndexOf(t, d.text, sentence)
	a := NewAnchor(d.text, d.sections, start, end)

	if len([]rune(a.Prefix)) > 64 || len([]rune(a.Suffix)) > 64 {
		t.Fatalf("context exceeds 64 runes: prefix=%d suffix=%d", len([]rune(a.Prefix)), len([]rune(a.Suffix)))
	}
	if a.Exact != sentence {
		t.Fatalf("exact = %q, want %q", a.Exact, sentence)
	}
	if a.Position != start {
		t.Fatalf("position = %d, want %d", a.Position, start)
	}
	if !pathEqual(a.Heading, []string{"Deployment Plan", "Rollback plan", "Failure modes"}) {
		t.Fatalf("heading path = %v", a.Heading)
	}
	p := Resolve(d.text, d.sections, a)
	if p.Method != MethodExact || p.Start != start || p.End != end {
		t.Fatalf("resolve = %+v, want exact [%d,%d)", p, start, end)
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "anchors", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

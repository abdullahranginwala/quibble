package comment

import (
	"strings"
	"testing"
	"time"
)

// big200KB builds a ~200 KB document whose target sentence is present only in a
// mutated form, forcing Resolve down the whole-doc fuzzy path. It returns the
// doc plus the anchor (built against the pristine sentence) and the true span.
func big200KB(tb testing.TB) (testDoc, Anchor) {
	tb.Helper()
	const sentence = "The nightly reconciliation job settles every pending payment charge across all supported currencies and regions, retrying transient gateway failures with backoff before escalating any discrepancy to the on-call engineer."
	const mutated = "The nightly reconciliation job settles every pending payment charge across all supported currencies and regions, retrying transient gateway failures with backoff before escalating any discrepancy to the on-call operator." // one word changed

	filler := "Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua ut enim ad minim veniam quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat."

	var b strings.Builder
	b.WriteString("# Big Document\n\n")
	// ~450 filler paragraphs of ~230 bytes ≈ 100 KB before the anchor.
	for i := 0; i < 450; i++ {
		b.WriteString(filler)
		b.WriteString("\n\n")
	}
	b.WriteString("## Target\n\n")
	b.WriteString(mutated)
	b.WriteString("\n\n")
	for i := 0; i < 450; i++ {
		b.WriteString(filler)
		b.WriteString("\n\n")
	}

	// Build the anchor from a pristine doc that still contains `sentence`, so the
	// stored exact will not match the mutated doc (fuzzy path is exercised).
	pristine := loadDocString("# Big Document\n\n## Target\n\n" + sentence + "\n")
	ps, pe := runeIndexOf2(pristine.text, sentence)
	a := NewAnchor(pristine.text, pristine.sections, ps, pe)
	a.Heading = nil // force phase (a) skip → whole-doc scan (worst case)

	d := loadDocString(b.String())
	if sz := len(d.text); sz < 180_000 {
		tb.Fatalf("fixture too small: %d bytes", sz)
	}
	return d, a
}

func runeIndexOf2(docText, sub string) (int, int) {
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
	return -1, -1
}

// Row 20: 200 KB doc, fuzzy path completes well under budget. Hard assert at
// 500 ms for CI headroom; the design bar is < 100 ms locally.
func TestRow20_LargeDocFuzzyPerf(t *testing.T) {
	d, a := big200KB(t)

	start := time.Now()
	p := Resolve(d.text, d.sections, a)
	elapsed := time.Since(start)

	if p.Method != MethodFuzzy {
		t.Fatalf("method = %s, want fuzzy", p.Method)
	}
	if p.Confidence < 0.9 {
		t.Fatalf("confidence = %v, want the near-identical mutated sentence", p.Confidence)
	}
	// The match must land in the Target section, not somewhere in the filler.
	ts, _ := runeIndexOf2(d.text, "The nightly reconciliation job settles")
	if abs(p.Start-ts) > 200 {
		t.Fatalf("match at %d not near target %d", p.Start, ts)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Resolve took %v, want < 500ms", elapsed)
	}
	t.Logf("200KB fuzzy Resolve: %v (doc %d runes)", elapsed, len([]rune(d.text)))
}

func BenchmarkResolveFuzzy200KB(b *testing.B) {
	d, a := big200KB(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p := Resolve(d.text, d.sections, a); p.Method != MethodFuzzy {
			b.Fatalf("method = %s", p.Method)
		}
	}
}

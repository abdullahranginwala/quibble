package comment

// Method records how a Placement was found.
type Method string

const (
	MethodExact   Method = "exact"
	MethodContext Method = "context"
	MethodFuzzy   Method = "fuzzy"
	MethodOrphan  Method = "orphan"
)

// Placement is the result of re-anchoring: a rune span in the doc's normalized
// text (or -1,-1 when orphaned), the confidence, and the method used.
type Placement struct {
	Start, End int     // rune offsets into the doc's normalized text; -1,-1 when orphan
	Confidence float64 // 1.0 exact/context; fuzzy score otherwise; 0 orphan
	Method     Method
}

// fuzzyThreshold is the minimum normalized similarity for a fuzzy re-anchor.
// Below it, a comment orphans rather than risk a false re-anchor — orphans are
// visible and recoverable; a wrong anchor silently misleads (DESIGN.md §6).
const fuzzyThreshold = 0.75

var orphan = Placement{Start: -1, End: -1, Confidence: 0, Method: MethodOrphan}

// Resolve locates anchor a within docText (the render-normalized text of the
// whole document). It tries exact match, then prefix/suffix + position
// disambiguation, then sliding-window fuzzy matching scoped to the anchor's
// heading section before the whole doc, and finally orphans. All offsets are
// rune offsets. See DESIGN.md §6.
func Resolve(docText string, sections []Section, a Anchor) Placement {
	if a.Exact == "" {
		return orphan
	}
	doc := []rune(docText)
	exact := []rune(a.Exact)

	// --- Step 1: exact occurrences ---
	occ := findAll(doc, exact)
	switch len(occ) {
	case 0:
		// fall through to fuzzy
	case 1:
		return Placement{Start: occ[0], End: occ[0] + len(exact), Confidence: 1.0, Method: MethodExact}
	default:
		start := disambiguate(doc, occ, len(exact), a)
		return Placement{Start: start, End: start + len(exact), Confidence: 1.0, Method: MethodContext}
	}

	// --- Step 2: fuzzy ---
	if p, ok := fuzzy(doc, exact, sections, a); ok {
		return p
	}

	// --- Step 3: orphan ---
	return orphan
}

// findAll returns the rune offsets of every occurrence of pat in doc.
func findAll(doc, pat []rune) []int {
	if len(pat) == 0 || len(pat) > len(doc) {
		return nil
	}
	var out []int
	last := len(doc) - len(pat)
	for i := 0; i <= last; i++ {
		match := true
		for j := 0; j < len(pat); j++ {
			if doc[i+j] != pat[j] {
				match = false
				break
			}
		}
		if match {
			out = append(out, i)
		}
	}
	return out
}

// disambiguate picks the best occurrence among several exact matches using
// prefix/suffix context, then nearest-to-position as a tie-break.
func disambiguate(doc []rune, occ []int, exactLen int, a Anchor) int {
	prefix := []rune(a.Prefix)
	suffix := []rune(a.Suffix)

	var kept []int
	for _, start := range occ {
		if contextMatches(doc, start, exactLen, prefix, suffix) {
			kept = append(kept, start)
		}
	}
	switch len(kept) {
	case 1:
		return kept[0]
	case 0:
		// No context survived; fall back to nearest among all occurrences.
		return nearest(occ, a.Position)
	default:
		return nearest(kept, a.Position)
	}
}

// contextMatches reports whether the runes immediately before/after an
// occurrence agree with the stored prefix/suffix, comparing only the overlap
// available at each end (context may be shorter near document boundaries).
func contextMatches(doc []rune, start, exactLen int, prefix, suffix []rune) bool {
	// Suffix of stored prefix must equal the doc runes ending at start.
	n := len(prefix)
	if n > start {
		n = start
	}
	for k := 1; k <= n; k++ {
		if doc[start-k] != prefix[len(prefix)-k] {
			return false
		}
	}
	// Prefix of stored suffix must equal the doc runes starting after the match.
	end := start + exactLen
	m := len(suffix)
	if end+m > len(doc) {
		m = len(doc) - end
	}
	for k := 0; k < m; k++ {
		if doc[end+k] != suffix[k] {
			return false
		}
	}
	return true
}

// nearest returns the candidate offset closest to pos.
func nearest(cands []int, pos int) int {
	best := cands[0]
	bestDist := abs(best - pos)
	for _, c := range cands[1:] {
		if d := abs(c - pos); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// fuzzy runs the sliding-window search: phase (a) within the anchor's heading
// section, then phase (b) the whole doc. The first phase whose best window
// meets the threshold wins.
func fuzzy(doc, exact []rune, sections []Section, a Anchor) (Placement, bool) {
	// Scratch buffers reused across every window comparison — no per-window
	// allocation. Sized for the longest possible b operand (exact).
	prev := make([]int, len(exact)+2)
	cur := make([]int, len(exact)+2)

	// Phase (a): the section whose Path equals the anchor's heading.
	if len(a.Heading) > 0 {
		for _, s := range sections {
			if pathEqual(s.Path, a.Heading) {
				lo, hi := s.Start, s.End
				if lo < 0 {
					lo = 0
				}
				if hi > len(doc) {
					hi = len(doc)
				}
				if p, ok := scanWindows(doc, exact, lo, hi, prev, cur); ok {
					return p, true
				}
				break
			}
		}
	}

	// Phase (b): whole doc.
	return scanWindows(doc, exact, 0, len(doc), prev, cur)
}

// scanWindows slides windows of length 0.8×, 1.0×, 1.2× the anchor length
// across doc[lo:hi], scoring each by normalized similarity, and returns the
// best-scoring window if that best meets the threshold.
//
// Two optimizations keep the hot loop fast without changing the result:
//   - A rolling multiset (bag) distance is a lower bound on edit distance and
//     is maintained incrementally as the length-L window slides, so windows
//     that cannot possibly beat the working threshold are skipped before any DP.
//   - The band shrinks to the best score found so far: once a strong match is
//     seen, later windows are compared against a much tighter band (they only
//     matter if they beat the incumbent), so dissimilar text early-exits fast.
func scanWindows(doc, exact []rune, lo, hi int, prev, cur []int) (Placement, bool) {
	L := len(exact)
	if L == 0 || lo >= hi {
		return Placement{}, false
	}
	step := L / 20
	if step < 1 {
		step = 1
	}
	widths := [3]int{(L * 8) / 10, L, (L * 12) / 10}

	// Character multiset of the anchor.
	ec := make(map[rune]int, L)
	for _, r := range exact {
		ec[r]++
	}

	bestScore := 0.0
	bestStart, bestEnd := -1, -1

	// Rolling length-L window multiset and its L1 distance D to ec. With an
	// empty window, D == sum(ec) == L. winLo is the window's current start.
	wc := make(map[rune]int, L)
	D := L
	add := func(r rune) {
		old := abs(wc[r] - ec[r])
		wc[r]++
		D += abs(wc[r]-ec[r]) - old
	}
	remove := func(r rune) {
		old := abs(wc[r] - ec[r])
		wc[r]--
		if wc[r] == 0 {
			delete(wc, r)
		}
		D += abs((wc[r])-ec[r]) - old
	}
	winLo := -1

	runDP := func(start int) {
		// Adaptive band: a window only matters if it can beat bestScore.
		for _, w := range widths {
			if w < 1 {
				continue
			}
			end := start + w
			if end > hi {
				end = hi
			}
			win := doc[start:end]
			if len(win) == 0 {
				continue
			}
			maxLen := len(win)
			if L > maxLen {
				maxLen = L
			}
			band := maxLen / 4 // threshold 0.75 ⇔ d ≤ maxLen/4
			if adapt := int(float64(maxLen) * (1 - bestScore)); adapt < band {
				band = adapt
			}
			if band < 0 {
				continue
			}
			d := bandedLev(win, exact, band, prev, cur)
			if d > band {
				continue
			}
			if score := 1 - float64(d)/float64(maxLen); score > bestScore {
				bestScore = score
				bestStart, bestEnd = start, end
			}
		}
	}

	for start := lo; start < hi; start += step {
		if start+L <= hi {
			// Maintain the rolling length-L window up to this start.
			if winLo == -1 {
				for i := start; i < start+L; i++ {
					add(doc[i])
				}
				winLo = start
			} else {
				for winLo < start {
					remove(doc[winLo])
					add(doc[winLo+L])
					winLo++
				}
			}
			// Prefilter: bag distance (= D/2 for equal lengths) lower-bounds the
			// edit distance. Skip if it cannot beat the working band.
			filterBand := L / 4
			if adapt := int(float64(L) * (1 - bestScore)); adapt < filterBand {
				filterBand = adapt
			}
			if D/2 > filterBand {
				continue
			}
		}
		runDP(start)
	}

	if bestStart >= 0 && bestScore >= fuzzyThreshold {
		return Placement{Start: bestStart, End: bestEnd, Confidence: bestScore, Method: MethodFuzzy}, true
	}
	return Placement{}, false
}

func pathEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

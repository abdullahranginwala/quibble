package comment

// bandedLev computes the Levenshtein distance between a and b, but only within
// a diagonal band of half-width maxDist. If the true distance exceeds maxDist
// it returns maxDist+1 (a sentinel) as early as possible. prev and cur are
// caller-owned scratch buffers of length >= len(b)+2, reused across calls so
// the hot fuzzy loop never allocates.
func bandedLev(a, b []rune, maxDist int, prev, cur []int) int {
	la, lb := len(a), len(b)
	inf := maxDist + 1
	if la-lb > maxDist || lb-la > maxDist {
		return inf
	}
	if lb == 0 {
		if la > maxDist {
			return inf
		}
		return la
	}

	// Row 0: distance from empty a to b[:j] is j.
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		lo := i - maxDist
		if lo < 1 {
			lo = 1
		}
		hi := i + maxDist
		if hi > lb {
			hi = lb
		}
		cur[0] = i
		// Left boundary sentinel: the cell just left of the band is unreachable.
		if lo-1 >= 1 {
			cur[lo-1] = inf
		}
		ai := a[i-1]
		rowMin := inf
		for j := lo; j <= hi; j++ {
			cost := 1
			if ai == b[j-1] {
				cost = 0
			}
			v := prev[j-1] + cost // substitution / match
			if d := prev[j] + 1; d < v {
				v = d // deletion
			}
			if in := cur[j-1] + 1; in < v {
				v = in // insertion
			}
			cur[j] = v
			if v < rowMin {
				rowMin = v
			}
		}
		// Right boundary sentinel so the next row reads inf, not stale data.
		if hi+1 <= lb {
			cur[hi+1] = inf
		}
		if rowMin > maxDist {
			return inf
		}
		prev, cur = cur, prev
	}
	d := prev[lb]
	if d > maxDist {
		return inf
	}
	return d
}

// levenshtein is an unbanded reference implementation used only in tests.
func levenshtein(a, b []rune) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := prev[j-1] + cost
			if d := prev[j] + 1; d < v {
				v = d
			}
			if in := cur[j-1] + 1; in < v {
				v = in
			}
			cur[j] = v
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

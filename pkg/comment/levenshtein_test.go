package comment

import (
	"math/rand"
	"testing"
)

// TestBandedLevMatchesReference fuzzes the banded implementation against the
// plain reference: whenever the true distance is within the band, banded must
// return it exactly; otherwise it must return the sentinel (> maxDist).
func TestBandedLevMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []rune("abcde")
	randStr := func(n int) []rune {
		s := make([]rune, n)
		for i := range s {
			s[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return s
	}
	for iter := 0; iter < 5000; iter++ {
		a := randStr(rng.Intn(12))
		b := randStr(rng.Intn(12))
		maxDist := rng.Intn(6)
		prev := make([]int, len(b)+2)
		cur := make([]int, len(b)+2)
		want := levenshtein(a, b)
		got := bandedLev(a, b, maxDist, prev, cur)
		if want <= maxDist {
			if got != want {
				t.Fatalf("a=%q b=%q maxDist=%d: banded=%d want=%d", string(a), string(b), maxDist, got, want)
			}
		} else {
			if got <= maxDist {
				t.Fatalf("a=%q b=%q maxDist=%d: banded=%d should exceed maxDist (true=%d)", string(a), string(b), maxDist, got, want)
			}
		}
	}
}

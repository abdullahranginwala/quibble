package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestFindQuote(t *testing.T) {
	const doc = "Alpha beta gamma one\nAlpha beta gamma two\nThe quick brown fox"
	tests := []struct {
		name      string
		quote     string
		wantCount int
		wantStart int
	}{
		{"unique", "quick brown fox", 1, 46},
		{"absent", "nonexistent phrase", 0, 0},
		{"ambiguous", "Alpha beta gamma", 2, 0},
		{"whole doc unique tail", "brown fox", 1, 52},
		{"too long", doc + " extra", 0, 0},
		{"empty", "", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findQuote(doc, tc.quote)
			if got.count != tc.wantCount {
				t.Fatalf("count = %d, want %d", got.count, tc.wantCount)
			}
			if tc.wantCount > 0 {
				if got.start != tc.wantStart {
					t.Fatalf("start = %d, want %d", got.start, tc.wantStart)
				}
				if got.end != tc.wantStart+len([]rune(tc.quote)) {
					t.Fatalf("end = %d, want %d", got.end, tc.wantStart+len([]rune(tc.quote)))
				}
			}
		})
	}
}

func TestFindQuoteRuneOffsets(t *testing.T) {
	// Multibyte runes must be counted as runes, not bytes, so anchors line up
	// with pkg/comment's rune-based selectors.
	const doc = "café brûlée dessert menu"
	got := findQuote(doc, "dessert")
	if got.count != 1 {
		t.Fatalf("count = %d, want 1", got.count)
	}
	if want := len([]rune("café brûlée ")); got.start != want {
		t.Fatalf("start = %d, want %d (rune offset)", got.start, want)
	}
}

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantOK   bool
	}{
		{"nil", nil, 0, false},
		{"plain error has no code", errors.New("boom"), 0, false},
		{"coded 1", withExitCode(1, errors.New("usage")), 1, true},
		{"coded 2", withExitCode(2, errors.New("fuzzy")), 2, true},
		{"coded 3", withExitCode(3, errors.New("policy")), 3, true},
		{"wrapped coded survives %w", fmt.Errorf("ctx: %w", withExitCode(3, errors.New("policy"))), 3, true},
		{"withExitCode(nil) is nil", withExitCode(1, nil), 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, ok := exitCodeOf(tc.err)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && code != tc.wantCode {
				t.Fatalf("code = %d, want %d", code, tc.wantCode)
			}
		})
	}
}

func TestResolveAuthorPrecedence(t *testing.T) {
	t.Setenv("QUIBBLE_AUTHOR", "envperson")
	if got := resolveAuthor("flagperson", "cfgdefault"); got != "flagperson" {
		t.Fatalf("flag should win: got %q", got)
	}
	if got := resolveAuthor("", "cfgdefault"); got != "envperson" {
		t.Fatalf("env should win over config: got %q", got)
	}
	t.Setenv("QUIBBLE_AUTHOR", "")
	if got := resolveAuthor("", "cfgdefault"); got != "cfgdefault" {
		t.Fatalf("config default should win when flag+env empty: got %q", got)
	}
}

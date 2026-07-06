package comment

import (
	"strings"
	"testing"
)

// containsValidMarkerLine reports whether the body or any reply body has a line
// that ParseThread would read as a reply marker — the documented round-trip
// limitation from row 3.
func containsValidMarkerLine(t *Thread) bool {
	has := func(s string) bool {
		for _, line := range strings.Split(s, "\n") {
			if replyMarker.MatchString(line) {
				return true
			}
		}
		return false
	}
	if has(t.Body) {
		return true
	}
	for _, r := range t.Replies {
		if has(r.Body) {
			return true
		}
	}
	return false
}

// FuzzParseThread asserts that ParseThread never panics on arbitrary input and
// that any thread it accepts survives a Marshal round-trip without error. The
// seed corpus below (plus files under testdata/fuzz/FuzzParseThread) is
// committed so `go test -fuzz=FuzzParseThread` starts from realistic shapes.
func FuzzParseThread(f *testing.F) {
	seeds := []string{
		"",
		"---",
		"---\n---\n",
		"not a thread at all",
		"---\nid: qbl-abc234\ndoc: docs/plan.md\nstatus: open\ncreated: 2026-07-06T14:31:00Z\nauthor: human\nanchor:\n  exact: \"x\"\n  prefix: \"\"\n  suffix: \"\"\n  position: 0\n---\n\nBody text.\n",
		"---\nid: qbl-7f3k2a\ndoc: d.md\nstatus: resolved\ncreated: 2026-07-06T14:31:00+05:30\nauthor: a\nanchor:\n  exact: \"e\"\n  prefix: \"p\"\n  suffix: \"s\"\n  heading: [\"H1\", \"H2\"]\n  position: 42\nresolved_by: a\nresolved_at: 2026-07-06T18:00:00+05:30\n---\n\nB\n\n<!-- reply author=claude time=2026-07-06T15:02:00Z -->\n\nreply body\n",
		"---\n\tbad: \"yaml\n---\n\nbody",
		"---\nid: qbl-uni234\ndoc: h.md\nstatus: open\ncreated: 2026-07-06T14:31:00Z\nauthor: h\nanchor:\n  exact: \"बुकिंग 🔒\"\n  prefix: \"\"\n  suffix: \"\"\n  position: 0\n---\n\nहिन्दी 🤔\n",
		"---\nstatus: open\n---\n<!-- reply author=x time=not-a-time -->\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		th, err := ParseThread(data)
		if err != nil {
			return // rejecting malformed input is fine; the contract is "no panic"
		}
		if th == nil {
			t.Fatal("ParseThread returned nil thread with nil error")
		}
		// Anything accepted must marshal without panicking.
		b, merr := th.Marshal()
		if merr != nil {
			t.Fatalf("Marshal of parsed thread failed: %v", merr)
		}
		// Marshal output re-parses to an identical thread — EXCEPT when a body
		// or reply body itself contains a line that is a fully valid reply
		// marker, which Marshal emits verbatim and re-parse would then read as a
		// new reply. That marker-lookalike-at-line-start case is the documented
		// known limitation (row 3; see plan/DECISIONS.md), so it is excluded.
		if containsValidMarkerLine(th) {
			return
		}
		th2, perr := ParseThread(b)
		if perr != nil {
			t.Fatalf("re-parse of marshaled thread failed: %v\n---\n%s", perr, b)
		}
		if !threadEqual(th, th2) {
			t.Fatalf("round-trip mismatch:\nwant %+v\ngot  %+v\n---\n%s", th, th2, b)
		}
	})
}

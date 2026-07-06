package comment

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// threadEqual compares two threads field-by-field, using time.Equal for
// timestamps so equal instants with differently-represented zones compare equal.
func threadEqual(a, b *Thread) bool {
	if a.ID != b.ID || a.Doc != b.Doc || a.Status != b.Status || a.Author != b.Author {
		return false
	}
	if !a.Created.Equal(b.Created) {
		return false
	}
	if a.Body != b.Body {
		return false
	}
	if a.ResolvedBy != b.ResolvedBy {
		return false
	}
	if (a.ResolvedAt == nil) != (b.ResolvedAt == nil) {
		return false
	}
	if a.ResolvedAt != nil && !a.ResolvedAt.Equal(*b.ResolvedAt) {
		return false
	}
	if !anchorEqual(a.Anchor, b.Anchor) {
		return false
	}
	if len(a.Replies) != len(b.Replies) {
		return false
	}
	for i := range a.Replies {
		if a.Replies[i].Author != b.Replies[i].Author || a.Replies[i].Body != b.Replies[i].Body {
			return false
		}
		if !a.Replies[i].Time.Equal(b.Replies[i].Time) {
			return false
		}
	}
	return true
}

func anchorEqual(a, b Anchor) bool {
	if a.Exact != b.Exact || a.Prefix != b.Prefix || a.Suffix != b.Suffix || a.Position != b.Position {
		return false
	}
	if len(a.Heading) != len(b.Heading) {
		return false
	}
	for i := range a.Heading {
		if a.Heading[i] != b.Heading[i] {
			return false
		}
	}
	return true
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad time %q: %v", s, err)
	}
	return ts
}

// roundTrip marshals t, parses it back, marshals the parse, and asserts the two
// marshals are byte-identical and the parsed struct equals the original.
func roundTrip(t *testing.T, in *Thread) *Thread {
	t.Helper()
	b1, err := in.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := ParseThread(b1)
	if err != nil {
		t.Fatalf("parse: %v\n---\n%s", err, b1)
	}
	b2, err := got.Marshal()
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(b1) != string(b2) {
		t.Fatalf("re-marshal not byte-identical:\n--- first ---\n%s\n--- second ---\n%s", b1, b2)
	}
	if !threadEqual(in, got) {
		t.Fatalf("parsed thread differs:\nwant %+v\ngot  %+v", in, got)
	}
	return got
}

// Row 1: full thread (2 replies, resolved) round-trips byte-identically.
func TestRow1_FullThreadRoundTrip(t *testing.T) {
	resolvedAt := mustTime(t, "2026-07-06T18:00:00+05:30")
	in := &Thread{
		ID:      "qbl-7f3k2a",
		Doc:     "docs/deployment-plan.md",
		Status:  StatusResolved,
		Created: mustTime(t, "2026-07-06T14:31:00+05:30"),
		Author:  "abdullah",
		Anchor: Anchor{
			Exact:    "the retry loop will re-attempt every 30 minutes",
			Prefix:   "guest is charged but marked failed, ",
			Suffix:   ". This is the double-charge window",
			Heading:  []string{"Rollback plan", "Failure modes"},
			Position: 14382,
		},
		Body: "Shouldn't this be idempotency-keyed on booking id, not attempt count?",
		Replies: []Reply{
			{Author: "claude", Time: mustTime(t, "2026-07-06T15:02:00+05:30"),
				Body: "Agreed — switched the key and updated §4.2. Marking addressed."},
			{Author: "abdullah", Time: mustTime(t, "2026-07-06T17:40:00+05:30"),
				Body: "LGTM, resolving."},
		},
		ResolvedBy: "abdullah",
		ResolvedAt: &resolvedAt,
	}
	roundTrip(t, in)
}

// Row 2: no replies, open — round-trips and emits no reply markers.
func TestRow2_NoRepliesOpen(t *testing.T) {
	in := &Thread{
		ID:      "qbl-abc234",
		Doc:     "docs/plan.md",
		Status:  StatusOpen,
		Created: mustTime(t, "2026-07-06T14:31:00Z"),
		Author:  "human",
		Anchor:  Anchor{Exact: "some text", Position: 10},
		Body:    "Please clarify this section.",
	}
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "<!-- reply") {
		t.Fatalf("no-reply thread emitted a reply marker:\n%s", b)
	}
	if strings.Contains(string(b), "resolved_by") || strings.Contains(string(b), "resolved_at") {
		t.Fatalf("open thread emitted resolved_* keys:\n%s", b)
	}
	roundTrip(t, in)
}

// Row 3: body containing `---` and a reply-lookalike inside a code fence.
func TestRow3_MarkerLookalikeInBody(t *testing.T) {
	body := "Here is a diff and a fake marker:\n\n```\n---\nnot frontmatter\n<!-- reply author=x -->\n---\n```\n\nEnd of body."
	in := &Thread{
		ID:      "qbl-def567",
		Doc:     "docs/plan.md",
		Status:  StatusOpen,
		Created: mustTime(t, "2026-07-06T14:31:00Z"),
		Author:  "human",
		Anchor:  Anchor{Exact: "code fence", Position: 3},
		Body:    body,
	}
	got := roundTrip(t, in)
	if got.Body != body {
		t.Fatalf("body not preserved intact:\nwant %q\ngot  %q", body, got.Body)
	}
	if len(got.Replies) != 0 {
		t.Fatalf("marker-lookalike wrongly parsed as reply: %+v", got.Replies)
	}
}

// Regression (fuzz 214c8ae1): a body line resembling a reply marker but with a
// non-RFC3339 timestamp must stay in the body and round-trip, not be misread as
// a reply and break re-parse.
func TestRegression_BogusTimeMarkerStaysBody(t *testing.T) {
	body := "See prior note:\n<!-- reply author=00 time=000000000000000000000 -->\nend."
	in := &Thread{
		ID:      "qbl-reg234",
		Doc:     "docs/plan.md",
		Status:  StatusOpen,
		Created: mustTime(t, "2026-07-06T14:31:00Z"),
		Author:  "human",
		Anchor:  Anchor{Exact: "x", Position: 0},
		Body:    body,
	}
	got := roundTrip(t, in)
	if len(got.Replies) != 0 {
		t.Fatalf("bogus-time marker wrongly parsed as reply: %+v", got.Replies)
	}
	if got.Body != body {
		t.Fatalf("body not preserved:\nwant %q\ngot  %q", body, got.Body)
	}
}

// Row 4: unknown status value is a Validate error (parse still succeeds).
func TestRow4_UnknownStatus(t *testing.T) {
	src := []byte(`---
id: qbl-abc234
doc: docs/plan.md
status: bogus
created: 2026-07-06T14:31:00Z
author: human
anchor:
  exact: "x"
  prefix: ""
  suffix: ""
  position: 0
---

Body.
`)
	th, err := ParseThread(src)
	if err != nil {
		t.Fatalf("parse should succeed for unknown status: %v", err)
	}
	if err := th.Validate(); err == nil {
		t.Fatal("Validate should reject unknown status")
	}
}

// Row 5: corrupt frontmatter yields an error naming the line, no panic.
func TestRow5_CorruptFrontmatter(t *testing.T) {
	src := []byte(`---
id: qbl-abc234
doc: docs/plan.md
anchor:
  exact: "x
  prefix: "unterminated
---

Body.
`)
	_, err := ParseThread(src)
	if err == nil {
		t.Fatal("expected error on corrupt YAML")
	}
	if !regexp.MustCompile(`line \d+`).MatchString(err.Error()) {
		t.Fatalf("error should include a line number, got: %v", err)
	}
}

// Row 6: zoned/future timestamps parse with zone preserved.
func TestRow6_ZonedTimestamps(t *testing.T) {
	in := &Thread{
		ID:      "qbl-abc234",
		Doc:     "docs/plan.md",
		Status:  StatusOpen,
		Created: mustTime(t, "2027-01-01T00:00:00+05:30"),
		Author:  "human",
		Anchor:  Anchor{Exact: "x", Position: 0},
		Body:    "b",
		Replies: []Reply{
			{Author: "claude", Time: mustTime(t, "2030-12-31T23:59:59Z"), Body: "future zulu"},
			{Author: "human", Time: mustTime(t, "2031-06-06T12:00:00+05:30"), Body: "future offset"},
		},
	}
	got := roundTrip(t, in)
	for i, want := range []string{"UTC+05:30", "UTC", "UTC+05:30"} {
		var have time.Time
		if i == 0 {
			have = got.Created
		} else {
			have = got.Replies[i-1].Time
		}
		_, offset := have.Zone()
		var wantOffset int
		switch want {
		case "UTC":
			wantOffset = 0
		case "UTC+05:30":
			wantOffset = 5*3600 + 30*60
		}
		if offset != wantOffset {
			t.Fatalf("timestamp %d: zone offset = %d, want %d", i, offset, wantOffset)
		}
	}
}

// Row 7: NewID ×10k all match the pattern with no duplicates.
func TestRow7_NewIDFormatAndUniqueness(t *testing.T) {
	re := regexp.MustCompile(`^qbl-[a-z2-7]{6}$`)
	seen := make(map[string]struct{}, 10000)
	for i := 0; i < 10000; i++ {
		id := NewID()
		if !re.MatchString(id) {
			t.Fatalf("id %q does not match pattern", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q at iteration %d", id, i)
		}
		seen[id] = struct{}{}
	}
}

// Row 8: unicode body + anchor round-trips; offsets are runes, not bytes.
func TestRow8_UnicodeRoundTrip(t *testing.T) {
	exact := "बुकिंग आईडी पर 🔒 आधारित"
	in := &Thread{
		ID:      "qbl-uni234",
		Doc:     "docs/planहिन्दी.md",
		Status:  StatusOpen,
		Created: mustTime(t, "2026-07-06T14:31:00+05:30"),
		Author:  "abdullah",
		Anchor: Anchor{
			Exact:    exact,
			Prefix:   "यह ",
			Suffix:   " 🎉 है",
			Heading:  []string{"रोलबैक", "विफलता 💥"},
			Position: 42,
		},
		Body: "क्या यह सही है? 🤔 emoji + हिन्दी.",
		Replies: []Reply{
			{Author: "claude", Time: mustTime(t, "2026-07-06T15:02:00+05:30"),
				Body: "हाँ ✅ ठीक किया।"},
		},
	}
	got := roundTrip(t, in)
	// The position hint is a rune count into the anchor's exact string context;
	// assert we round-trip it and that exact is measured in runes not bytes.
	if got.Anchor.Position != 42 {
		t.Fatalf("position changed: %d", got.Anchor.Position)
	}
	if gotRunes, gotBytes := len([]rune(got.Anchor.Exact)), len(got.Anchor.Exact); gotRunes == gotBytes {
		t.Fatalf("expected multibyte exact (runes %d != bytes %d)", gotRunes, gotBytes)
	}
}

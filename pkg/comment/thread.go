package comment

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Status is a thread's lifecycle state. The legal transitions are
// open → addressed → resolved, plus resolved/addressed → open (reopen).
// See DESIGN.md §4.
type Status string

// The three lifecycle states. "orphaned" is deliberately not a Status — it is
// a derived render-time condition (a Placement with MethodOrphan) so it never
// masks the real lifecycle state.
const (
	StatusOpen      Status = "open"
	StatusAddressed Status = "addressed"
	StatusResolved  Status = "resolved"
)

// Anchor is a W3C Web Annotation-style selector (TextQuote + TextPosition +
// heading path) that lets a comment re-locate its target after the document
// is edited. All lengths and offsets are in runes, never bytes.
type Anchor struct {
	Exact    string   `yaml:"exact"`
	Prefix   string   `yaml:"prefix"`            // ≤64 runes of context before
	Suffix   string   `yaml:"suffix"`            // ≤64 runes after
	Heading  []string `yaml:"heading,omitempty"` // heading path at creation, outermost first
	Position int      `yaml:"position"`          // rune offset hint into the doc's normalized text at creation
}

// Reply is a single appended message on a thread.
type Reply struct {
	Author string
	Time   time.Time
	Body   string
}

// Thread is one comment thread: frontmatter metadata, an opening body, and
// zero or more replies. One Thread == one file on disk.
type Thread struct {
	ID         string
	Doc        string // rel path of the document, e.g. "docs/plan.md"
	Status     Status
	Created    time.Time
	Author     string
	Anchor     Anchor
	Body       string
	Replies    []Reply
	ResolvedBy string
	ResolvedAt *time.Time
}

const idAlphabet = "abcdefghijklmnopqrstuvwxyz234567"

var idRe = regexp.MustCompile(`^qbl-[a-z2-7]{6}$`)

// NewID returns a fresh thread id: "qbl-" followed by 6 lowercase base32
// characters drawn from crypto/rand. 256 is divisible by 32, so masking the
// low 5 bits of each random byte is unbiased.
func NewID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read never returns an error on supported platforms;
		// a failure here is unrecoverable and only reachable at process init.
		panic(fmt.Sprintf("comment: crypto/rand failed: %v", err))
	}
	var sb strings.Builder
	sb.WriteString("qbl-")
	for _, x := range b {
		sb.WriteByte(idAlphabet[x&0x1f])
	}
	return sb.String()
}

// Validate checks the invariants a well-formed thread must satisfy: a legal id,
// a non-empty doc path, a known status, and a non-empty anchor exact string.
func (t *Thread) Validate() error {
	if !idRe.MatchString(t.ID) {
		return fmt.Errorf("comment: invalid id %q (want ^qbl-[a-z2-7]{6}$)", t.ID)
	}
	if strings.TrimSpace(t.Doc) == "" {
		return fmt.Errorf("comment: thread %s: doc is empty", t.ID)
	}
	switch t.Status {
	case StatusOpen, StatusAddressed, StatusResolved:
	default:
		return fmt.Errorf("comment: thread %s: unknown status %q", t.ID, t.Status)
	}
	if t.Anchor.Exact == "" {
		return fmt.Errorf("comment: thread %s: anchor.exact is empty", t.ID)
	}
	return nil
}

// replyMarker matches a reply delimiter at the start of a line. The author
// token is [a-z0-9_.-]+ and the time must be RFC 3339-shaped — requiring the
// timestamp shape (rather than any \S+) keeps body lines that merely resemble a
// marker but carry a bogus time in the body, where they round-trip cleanly.
var replyMarker = regexp.MustCompile(`^<!-- reply author=([a-z0-9_.-]+) time=(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})) -->\s*$`)

// fmYAML mirrors the frontmatter for tolerant decoding. resolved_by /
// resolved_at are pointers so their absence is distinguishable from a zero
// value, matching the "keys appear only when set" rule.
type fmYAML struct {
	ID         string     `yaml:"id"`
	Doc        string     `yaml:"doc"`
	Status     string     `yaml:"status"`
	Created    time.Time  `yaml:"created"`
	Author     string     `yaml:"author"`
	Anchor     Anchor     `yaml:"anchor"`
	ResolvedBy *string    `yaml:"resolved_by"`
	ResolvedAt *time.Time `yaml:"resolved_at"`
}

// ParseThread decodes a thread file. It is tolerant of whitespace variance in
// the frontmatter and never panics on arbitrary input; every error names the
// offending line so a directory scan can report a corrupt file and continue.
func ParseThread(src []byte) (*Thread, error) {
	lines := strings.Split(string(src), "\n")

	// Locate the opening frontmatter fence, tolerating leading blank lines.
	open := 0
	for open < len(lines) && strings.TrimSpace(lines[open]) == "" {
		open++
	}
	if open >= len(lines) || strings.TrimSpace(lines[open]) != "---" {
		return nil, fmt.Errorf("thread: line %d: missing opening frontmatter fence '---'", open+1)
	}

	// Locate the closing fence.
	close := open + 1
	for close < len(lines) && strings.TrimSpace(lines[close]) != "---" {
		close++
	}
	if close >= len(lines) {
		return nil, fmt.Errorf("thread: line %d: unterminated frontmatter (no closing '---')", open+1)
	}

	fmText := strings.Join(lines[open+1:close], "\n")
	var fm fmYAML
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return nil, fmt.Errorf("thread: %s", rebaseYAMLLine(err, open+1))
	}

	t := &Thread{
		ID:         fm.ID,
		Doc:        fm.Doc,
		Status:     Status(fm.Status),
		Created:    fm.Created,
		Author:     fm.Author,
		Anchor:     fm.Anchor,
		ResolvedAt: fm.ResolvedAt,
	}
	if fm.ResolvedBy != nil {
		t.ResolvedBy = *fm.ResolvedBy
	}

	// Body and replies. Line numbers are 1-based and absolute.
	bodyLines := lines[close+1:]
	base := close + 1 // 0-based index of bodyLines[0] within lines

	var (
		bodySeg   []string
		haveReply bool
		replies   []Reply
		cur       *Reply
		curLines  []string
	)
	flush := func() {
		if cur != nil {
			cur.Body = trimBlock(curLines)
			replies = append(replies, *cur)
		}
	}
	for i, line := range bodyLines {
		if m := replyMarker.FindStringSubmatch(line); m != nil {
			lineNo := base + i + 1
			ts, err := time.Parse(time.RFC3339, m[2])
			if err != nil {
				return nil, fmt.Errorf("thread: line %d: bad reply timestamp %q: %w", lineNo, m[2], err)
			}
			flush()
			haveReply = true
			cur = &Reply{Author: m[1], Time: ts}
			curLines = nil
			continue
		}
		if haveReply {
			curLines = append(curLines, line)
		} else {
			bodySeg = append(bodySeg, line)
		}
	}
	flush()

	t.Body = trimBlock(bodySeg)
	t.Replies = replies
	return t, nil
}

// Marshal renders a thread in the frozen file format. It is deterministic:
// Marshal(Parse(Marshal(t))) is byte-identical to Marshal(t).
func (t *Thread) Marshal() ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: ")
	b.WriteString(yamlScalar(t.ID))
	b.WriteByte('\n')
	b.WriteString("doc: ")
	b.WriteString(yamlScalar(t.Doc))
	b.WriteByte('\n')
	b.WriteString("status: ")
	b.WriteString(yamlScalar(string(t.Status)))
	b.WriteByte('\n')
	b.WriteString("created: ")
	b.WriteString(t.Created.Format(time.RFC3339))
	b.WriteByte('\n')
	b.WriteString("author: ")
	b.WriteString(yamlScalar(t.Author))
	b.WriteByte('\n')
	b.WriteString("anchor:\n")
	b.WriteString("  exact: ")
	b.WriteString(yamlDQ(t.Anchor.Exact))
	b.WriteByte('\n')
	b.WriteString("  prefix: ")
	b.WriteString(yamlDQ(t.Anchor.Prefix))
	b.WriteByte('\n')
	b.WriteString("  suffix: ")
	b.WriteString(yamlDQ(t.Anchor.Suffix))
	b.WriteByte('\n')
	if len(t.Anchor.Heading) > 0 {
		b.WriteString("  heading: [")
		for i, h := range t.Anchor.Heading {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(yamlDQ(h))
		}
		b.WriteString("]\n")
	}
	b.WriteString("  position: ")
	b.WriteString(strconv.Itoa(t.Anchor.Position))
	b.WriteByte('\n')
	if t.ResolvedBy != "" {
		b.WriteString("resolved_by: ")
		b.WriteString(yamlScalar(t.ResolvedBy))
		b.WriteByte('\n')
	}
	if t.ResolvedAt != nil {
		b.WriteString("resolved_at: ")
		b.WriteString(t.ResolvedAt.Format(time.RFC3339))
		b.WriteByte('\n')
	}
	b.WriteString("---\n\n")
	b.WriteString(t.Body)
	for _, r := range t.Replies {
		b.WriteString("\n\n<!-- reply author=")
		b.WriteString(r.Author)
		b.WriteString(" time=")
		b.WriteString(r.Time.Format(time.RFC3339))
		b.WriteString(" -->\n\n")
		b.WriteString(r.Body)
	}
	b.WriteByte('\n')
	return []byte(b.String()), nil
}

// trimBlock joins lines and trims surrounding whitespace.
func trimBlock(lines []string) string {
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// plainSafe matches scalars that can be emitted unquoted without changing
// meaning under YAML. Timestamps are emitted separately (they contain colons
// but are valid plain scalars); everything routed here is an id, path, status,
// or author token.
var plainSafe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./+@-]*$`)

// yamlScalar emits s as a plain scalar when unambiguous, else double-quoted.
func yamlScalar(s string) string {
	if plainSafe.MatchString(s) {
		return s
	}
	return yamlDQ(s)
}

// yamlDQ emits s as a YAML double-quoted scalar. Non-ASCII runes are written
// literally as UTF-8 (valid YAML); control characters are escaped.
func yamlDQ(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

var yamlLineRe = regexp.MustCompile(`line (\d+):`)

// rebaseYAMLLine rewrites the "line N" reference in a yaml.v3 error so it is
// absolute within the thread file rather than relative to the frontmatter
// snippet. offset is the 1-based file line of the first frontmatter line.
func rebaseYAMLLine(err error, offset int) string {
	msg := err.Error()
	loc := yamlLineRe.FindStringSubmatchIndex(msg)
	if loc == nil {
		return fmt.Sprintf("line %d: parsing frontmatter: %v", offset, err)
	}
	n, _ := strconv.Atoi(msg[loc[2]:loc[3]])
	abs := offset + n - 1
	return fmt.Sprintf("line %d: parsing frontmatter: %s", abs, strings.TrimPrefix(msg, "yaml: "))
}

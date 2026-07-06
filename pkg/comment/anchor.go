package comment

// Heading and Block are minimal local mirrors of render.Heading / render.Block.
// pkg/comment is kept self-contained so it can be built and tested without
// importing pkg/render (which is developed in parallel); the field sets match
// render's so the two reconcile trivially. See plan/DECISIONS.md.

// Heading is one entry in a document outline.
type Heading struct {
	Level  int
	Text   string
	Anchor string
}

// Block is a top-level normalized text block with a rune span into the doc's
// normalized text.
type Block struct {
	ID    string
	Text  string
	Start int
	End   int
}

// Section is a heading and the rune span of everything under it (including
// nested subsections), in the document's normalized text.
type Section struct {
	Path       []string // heading path, outermost first
	Start, End int      // rune span of the section in docText
}

// Sectionize derives heading-path spans from a document outline. A section runs
// from its heading to the next heading of the same or higher level; nested
// subsections are contained within their parents' spans. Headings are located
// by matching outline entries to blocks in document order.
func Sectionize(outline []Heading, docText string, blocks []Block) []Section {
	runeLen := len([]rune(docText))

	// Resolve each heading's start offset by walking blocks in order and
	// matching normalized text. A heading is emitted as its own block whose
	// text equals the heading text.
	type hpos struct {
		level int
		text  string
		start int
	}
	var heads []hpos
	bi := 0
	for _, h := range outline {
		start := -1
		for bi < len(blocks) {
			if blocks[bi].Text == h.Text {
				start = blocks[bi].Start
				bi++
				break
			}
			bi++
		}
		if start < 0 {
			// No matching block (outline/blocks disagree); skip this heading
			// rather than fabricate a span.
			continue
		}
		heads = append(heads, hpos{level: h.Level, text: h.Text, start: start})
	}

	sections := make([]Section, 0, len(heads))
	var stack []hpos
	for i, h := range heads {
		// Maintain the ancestor stack: pop siblings and deeper headings.
		for len(stack) > 0 && stack[len(stack)-1].level >= h.level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, h)

		// End = start of the next heading at same-or-higher level, else EOF.
		end := runeLen
		for j := i + 1; j < len(heads); j++ {
			if heads[j].level <= h.level {
				end = heads[j].start
				break
			}
		}

		path := make([]string, len(stack))
		for k, s := range stack {
			path[k] = s.text
		}
		sections = append(sections, Section{Path: path, Start: h.start, End: end})
	}
	return sections
}

// NewAnchor captures a selector for the rune range [start,end) of docText:
// the exact selected text, up to 64 runes of prefix/suffix context, the heading
// path of the innermost containing section, and the start offset as position.
func NewAnchor(docText string, sections []Section, start, end int) Anchor {
	runes := []rune(docText)
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if end < start {
		end = start
	}

	pStart := start - 64
	if pStart < 0 {
		pStart = 0
	}
	sEnd := end + 64
	if sEnd > len(runes) {
		sEnd = len(runes)
	}

	a := Anchor{
		Exact:    string(runes[start:end]),
		Prefix:   string(runes[pStart:start]),
		Suffix:   string(runes[end:sEnd]),
		Position: start,
	}

	// Innermost containing section = the one with the largest Start whose span
	// contains the range start.
	best := -1
	for i, s := range sections {
		if s.Start <= start && start < s.End {
			if best < 0 || s.Start > sections[best].Start {
				best = i
			}
		}
	}
	if best >= 0 {
		a.Heading = append([]string(nil), sections[best].Path...)
	}
	return a
}

package render

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Block is a top-level markdown block with a stable fingerprint. The comment
// layer uses these fingerprints as anchoring hints; they are never written
// back into the markdown source.
type Block struct {
	// ID is the fingerprint: the first 10 hex characters of
	// sha256(normalized text), a hyphen, and a zero-based occurrence index
	// that disambiguates blocks with identical text, e.g. "a3f9c210de-0".
	ID string
	// Text is the normalized plain text of the block.
	Text string
	// Start and End are rune offsets of the block's text within Doc.Text.
	Start int
	End   int
}

// blockUnit pairs an AST node with the normalized plain text of the block it
// represents. A list expands into one unit per list item, so the node may be a
// list item rather than a top-level document child.
type blockUnit struct {
	node ast.Node
	text string
}

// normMarkdown is the parser used by NormalizeBlocks. It must stay identical to
// the parser configured for rendering (see newMarkdown) so that offsets and
// fingerprints computed here match those emitted into HTML.
var normMarkdown = goldmark.New(goldmark.WithExtensions(baseExtensions()...))

// NormalizeBlocks parses src and returns the document's normalized plain text
// together with the fingerprint of every non-empty top-level block.
//
// Normalization is deliberately frozen — changing it would orphan existing
// comment anchors, so its behaviour is pinned by golden tests:
//
//   - Runs of whitespace within a block collapse to a single space and the
//     block text is trimmed.
//   - A list contributes one block per list item; every other top-level
//     construct is a single block.
//   - Code fences are normalized like any other block: their interior newlines
//     become single spaces.
//   - Blocks are joined with "\n" to form the document text; each block's
//     Start/End are rune (not byte) offsets into that text.
//
// pkg/comment relies on this exact function for its anchoring input.
func NormalizeBlocks(src []byte) (text string, blocks []Block) {
	node := normMarkdown.Parser().Parse(textReader(src))
	units := blockUnits(node, src)
	text, blocks, _ = buildBlocks(units)
	return text, blocks
}

func textReader(src []byte) text.Reader { return text.NewReader(src) }

// blockUnits enumerates the block units of a parsed document in document order.
// Each direct child of the document is one unit, except lists, which expand
// into one unit per list item.
func blockUnits(doc ast.Node, source []byte) []blockUnit {
	var units []blockUnit
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Kind() == ast.KindList {
			for li := c.FirstChild(); li != nil; li = li.NextSibling() {
				units = append(units, blockUnit{li, normalizeText(extractText(li, source))})
			}
			continue
		}
		units = append(units, blockUnit{c, normalizeText(extractText(c, source))})
	}
	return units
}

// buildBlocks assigns fingerprints and rune offsets to non-empty units and
// joins their text into the document text. It also returns a map from each
// contributing node to its block ID, used by the renderer to emit data-qbl.
func buildBlocks(units []blockUnit) (docText string, blocks []Block, ids map[ast.Node]string) {
	ids = make(map[ast.Node]string)
	counts := make(map[string]int)
	var texts []string
	pos := 0
	for _, u := range units {
		if u.text == "" {
			continue
		}
		hash := hash10(u.text)
		occ := counts[hash]
		counts[hash]++
		id := hash + "-" + strconv.Itoa(occ)

		start := pos
		end := start + utf8.RuneCountInString(u.text)
		blocks = append(blocks, Block{ID: id, Text: u.text, Start: start, End: end})
		ids[u.node] = id
		texts = append(texts, u.text)
		pos = end + 1 // account for the "\n" join separator
	}
	docText = strings.Join(texts, "\n")
	return docText, blocks, ids
}

// hash10 is the first 10 hex characters of sha256(text).
func hash10(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:10]
}

// extractText walks a block subtree and concatenates its visible text content.
// Code blocks contribute their raw line content; block boundaries and soft/hard
// line breaks contribute a separating space so adjacent text never fuses. All
// of this is later collapsed by normalizeText.
func extractText(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if node.Type() == ast.TypeBlock {
				b.WriteByte(' ')
			}
			return ast.WalkContinue, nil
		}
		switch node.Kind() {
		case ast.KindFencedCodeBlock, ast.KindCodeBlock:
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				b.Write(seg.Value(source))
			}
			return ast.WalkSkipChildren, nil
		case ast.KindRawHTML, ast.KindHTMLBlock:
			return ast.WalkSkipChildren, nil
		}
		switch t := node.(type) {
		case *ast.Text:
			seg := t.Segment
			b.Write(seg.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.AutoLink:
			b.Write(t.URL(source))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// normalizeText collapses every run of Unicode whitespace to a single space and
// trims the ends. This is the frozen normalization primitive.
func normalizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pendingSpace := false
	started := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			pendingSpace = true
			continue
		}
		if pendingSpace && started {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		started = true
		pendingSpace = false
	}
	return b.String()
}

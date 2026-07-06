// Package docbridge translates a rendered render.Doc into the pkg/comment
// types the review layer anchors against (Heading/Block + Section spans), in
// one place shared by the CLI and the serve command. It is the single
// render→comment field bridge (see plan/DECISIONS.md).
package docbridge

import (
	"github.com/abdullahranginwala/quibble/pkg/comment"
	"github.com/abdullahranginwala/quibble/pkg/render"
)

// Rendered is a document rendered once, carrying both the HTML page and the
// normalized text / block / section model needed to place comments. All
// offsets are rune offsets into Text.
type Rendered struct {
	Slug     string
	RelPath  string
	Title    string
	Page     []byte // full standalone HTML page (chrome + article)
	Text     string // normalized plain text of the whole document
	Blocks   []comment.Block
	Outline  []comment.Heading
	Sections []comment.Section
}

// Render renders src (the markdown of relPath) with r and bridges the render
// types into the pkg/comment types. Normalization is theme-independent, so any
// renderer yields the same Text/Blocks/Sections; Page reflects r's theme.
func Render(r *render.Renderer, src []byte, relPath string) (*Rendered, error) {
	doc, err := r.RenderDoc(src, relPath)
	if err != nil {
		return nil, err
	}
	return bridge(doc), nil
}

func bridge(doc *render.Doc) *Rendered {
	headings := make([]comment.Heading, len(doc.Outline))
	for i, h := range doc.Outline {
		headings[i] = comment.Heading{Level: h.Level, Text: h.Text, Anchor: h.Anchor}
	}
	blocks := make([]comment.Block, len(doc.Blocks))
	for i, b := range doc.Blocks {
		blocks[i] = comment.Block{ID: b.ID, Text: b.Text, Start: b.Start, End: b.End}
	}
	return &Rendered{
		Slug:     doc.Slug,
		RelPath:  doc.RelPath,
		Title:    doc.Title,
		Page:     doc.Page,
		Text:     doc.Text,
		Blocks:   blocks,
		Outline:  headings,
		Sections: comment.Sectionize(headings, doc.Text, blocks),
	}
}

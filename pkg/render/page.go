package render

import (
	"bytes"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"unicode"
)

// pageTemplate wraps the parsed layout template.
type pageTemplate struct {
	tmpl *template.Template
}

func newPageTemplate() (*pageTemplate, error) {
	t, err := template.New("layout.html").ParseFS(templatesFS, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("render: parsing layout template: %w", err)
	}
	return &pageTemplate{tmpl: t}, nil
}

// pageData is the layout template's input.
type pageData struct {
	SiteTitle   string
	DocTitle    string
	AssetPrefix string
	Overrides   template.HTML
	TOC         bool
	Outline     []Heading
	Body        template.HTML
}

func (p *pageTemplate) render(data pageData) ([]byte, error) {
	var buf bytes.Buffer
	if err := p.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render: executing layout: %w", err)
	}
	return buf.Bytes(), nil
}

func (p *pageTemplate) renderDoc(r *Renderer, doc *Doc) ([]byte, error) {
	return p.render(pageData{
		SiteTitle:   r.opts.Title,
		DocTitle:    doc.Title,
		AssetPrefix: assetPrefix,
		Overrides:   overrideStyle(r.overrideCSS),
		TOC:         r.opts.TOC,
		Outline:     doc.Outline,
		Body:        template.HTML(doc.HTML),
	})
}

func (p *pageTemplate) renderIndex(r *Renderer, docs []*Doc) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, `<h1 class="qbl-index-title">%s</h1>`, template.HTMLEscapeString(r.opts.Title))
	b.WriteString(`<ul class="qbl-index">`)
	for _, d := range docs {
		fmt.Fprintf(&b,
			`<li><a href="%s.html">%s</a> <span class="qbl-index-path">%s</span></li>`,
			template.HTMLEscapeString(d.Slug),
			template.HTMLEscapeString(d.Title),
			template.HTMLEscapeString(d.RelPath),
		)
	}
	b.WriteString(`</ul>`)
	return p.render(pageData{
		SiteTitle:   r.opts.Title,
		DocTitle:    "",
		AssetPrefix: assetPrefix,
		Overrides:   overrideStyle(r.overrideCSS),
		TOC:         false,
		Body:        template.HTML(b.String()),
	})
}

// overrideStyle wraps the validated override body in a <style> element, or
// returns empty when there are no overrides. The body is trusted: keys are
// contract tokens and values come from the caller's own configuration.
func overrideStyle(css string) template.HTML {
	if css == "" {
		return ""
	}
	return template.HTML("<style>" + css + "</style>")
}

// slugify converts heading text to a GitHub-style anchor: lowercased, with only
// letters, digits and hyphens kept and spaces turned into hyphens. used tracks
// prior slugs so collisions get -1, -2, … suffixes.
func slugify(s string, used map[string]int) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	base := b.String()
	if base == "" {
		base = "section"
	}
	n := used[base]
	used[base]++
	if n > 0 {
		return base + "-" + strconv.Itoa(n)
	}
	return base
}

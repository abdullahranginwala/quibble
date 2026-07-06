package render

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Options configures a Renderer.
type Options struct {
	// Theme is required; use Paper() for the built-in default.
	Theme Theme
	// Overrides sets --qbl-* design tokens per render, layered after the
	// theme's tokens.css. Keys must be tokens in the contract, or New fails.
	Overrides map[string]string
	// TOC enables the sticky table-of-contents sidebar.
	TOC bool
	// Title is the site title shown in the header; it defaults to "Docs".
	Title string
}

// Renderer turns markdown into quibble's HTML. It is safe to reuse across many
// documents and holds the theme's precomputed assets.
type Renderer struct {
	opts        Options
	theme       Theme
	md          goldmark.Markdown
	hl          *highlighter
	page        *pageTemplate
	overrideCSS string            // inline <style> body, or "" when no overrides
	assets      map[string][]byte // shared site assets, keyed by site-relative path
}

// baseExtensions is the goldmark extension set shared by the renderer and by
// NormalizeBlocks, so both parse documents identically.
func baseExtensions() []goldmark.Extender {
	return []goldmark.Extender{
		extension.GFM, // tables, strikethrough, linkify, task lists
		extension.Footnote,
	}
}

// New builds a Renderer for the given options. It validates the theme against
// the token contract and rejects overrides that name unknown tokens.
func New(opts Options) (*Renderer, error) {
	if opts.Theme == nil {
		return nil, fmt.Errorf("render: Options.Theme is required")
	}
	if opts.Title == "" {
		opts.Title = "Docs"
	}

	if err := validateThemeFS(opts.Theme.FS()); err != nil {
		return nil, fmt.Errorf("render: theme %q: %w", opts.Theme.Name(), err)
	}

	overrideCSS, err := buildOverrideCSS(opts.Overrides)
	if err != nil {
		return nil, err
	}

	hl := newHighlighter()
	md := goldmark.New(
		goldmark.WithExtensions(baseExtensions()...),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(&nodeRenderer{hl: hl}, 100)),
			ghtml.WithUnsafe(), // author HTML in trusted local docs passes through
		),
	)

	pt, err := newPageTemplate()
	if err != nil {
		return nil, err
	}

	r := &Renderer{
		opts:        opts,
		theme:       opts.Theme,
		md:          md,
		hl:          hl,
		page:        pt,
		overrideCSS: overrideCSS,
	}
	if err := r.buildAssets(); err != nil {
		return nil, err
	}
	return r, nil
}

// buildOverrideCSS validates override keys and renders them as a :root style
// body. It returns "" when there are no overrides.
func buildOverrideCSS(overrides map[string]string) (string, error) {
	if len(overrides) == 0 {
		return "", nil
	}
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		if !tokenSet[k] {
			return "", fmt.Errorf("render: unknown override token %q (not part of the token contract)", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(":root {")
	for _, k := range keys {
		fmt.Fprintf(&b, " %s: %s;", k, overrides[k])
	}
	b.WriteString(" }")
	return b.String(), nil
}

// buildAssets precomputes the site-wide assets (theme + highlight + structural
// CSS) shared by every rendered page.
func (r *Renderer) buildAssets() error {
	tokens, err := fs.ReadFile(r.theme.FS(), "tokens.css")
	if err != nil {
		return fmt.Errorf("render: reading tokens.css: %w", err)
	}
	themeCSS, err := fs.ReadFile(r.theme.FS(), "theme.css")
	if err != nil {
		return fmt.Errorf("render: reading theme.css: %w", err)
	}
	structural, err := assetsFS.ReadFile("assets/quibble.css")
	if err != nil {
		return fmt.Errorf("render: reading structural css: %w", err)
	}
	r.assets = map[string][]byte{
		assetPrefix + "tokens.css":  tokens,
		assetPrefix + "theme.css":   themeCSS,
		assetPrefix + "chroma.css":  []byte(r.hl.css()),
		assetPrefix + "quibble.css": structural,
	}
	return nil
}

// RenderDoc renders one document. relPath supplies the slug and the title
// fallback.
func (r *Renderer) RenderDoc(src []byte, relPath string) (*Doc, error) {
	node := r.md.Parser().Parse(textReader(src))

	units := blockUnits(node, src)
	docText, blocks, ids := buildBlocks(units)
	outline, slugByNode, h1 := r.processHeadings(node, src)

	// Stamp anchoring and heading identity onto the AST for rendering. This
	// mutates the parsed tree only, never the source (DESIGN.md §5, §13).
	for n, id := range ids {
		n.SetAttributeString("data-qbl", []byte(id))
	}
	for n, slug := range slugByNode {
		n.SetAttributeString("id", []byte(slug))
	}

	var body bytes.Buffer
	if err := r.md.Renderer().Render(&body, src, node); err != nil {
		return nil, fmt.Errorf("render: rendering %s: %w", relPath, err)
	}

	title := h1
	if title == "" {
		title = fileTitle(relPath)
	}

	doc := &Doc{
		Slug:    slugFor(relPath),
		RelPath: relPath,
		Title:   title,
		HTML:    body.Bytes(),
		Blocks:  blocks,
		Outline: outline,
		Text:    docText,
	}
	page, err := r.page.renderDoc(r, doc)
	if err != nil {
		return nil, err
	}
	doc.Page = page
	return doc, nil
}

// RenderDir renders every .md file in fsys. Paths become slugs per the naming
// convention. The returned Site carries the shared assets and an index page.
func (r *Renderer) RenderDir(fsys fs.FS) (*Site, error) {
	var docs []*Doc
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		doc, err := r.RenderDoc(data, p)
		if err != nil {
			return err
		}
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("render: walking docs: %w", err)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Slug < docs[j].Slug })

	assets := make(map[string][]byte, len(r.assets))
	for k, v := range r.assets {
		assets[k] = v
	}
	site := &Site{Docs: docs, Assets: assets}
	index, err := r.page.renderIndex(r, docs)
	if err != nil {
		return nil, err
	}
	site.index = index
	return site, nil
}

// processHeadings computes GitHub-style heading slugs (deduped with -1, -2, …),
// the document outline, and the first h1's text (the title, when present).
func (r *Renderer) processHeadings(doc ast.Node, source []byte) (outline []Heading, slugByNode map[ast.Node]string, h1 string) {
	used := make(map[string]int)
	slugByNode = make(map[ast.Node]string)
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		h, ok := c.(*ast.Heading)
		if !ok {
			continue
		}
		text := normalizeText(extractText(h, source))
		slug := slugify(text, used)
		slugByNode[h] = slug
		outline = append(outline, Heading{Level: h.Level, Text: text, Anchor: slug})
		if h.Level == 1 && h1 == "" {
			h1 = text
		}
	}
	return outline, slugByNode, h1
}

// nodeRenderer overrides heading and code-block rendering. Headings get an id
// and a hover anchor link; code blocks are highlighted via chroma. All other
// nodes use goldmark's defaults, which already emit our data-qbl attribute.
type nodeRenderer struct {
	hl *highlighter
}

func (n *nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, n.renderHeading)
	reg.Register(ast.KindFencedCodeBlock, n.renderCode)
	reg.Register(ast.KindCodeBlock, n.renderCode)
}

func (n *nodeRenderer) renderHeading(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	h := node.(*ast.Heading)
	if entering {
		fmt.Fprintf(w, "<h%d", h.Level)
		ghtml.RenderAttributes(w, node, ghtml.HeadingAttributeFilter)
		_ = w.WriteByte('>')
		return ast.WalkContinue, nil
	}
	if id, ok := node.AttributeString("id"); ok {
		fmt.Fprintf(w, ` <a class="qbl-anchor" href="#%s" aria-hidden="true" tabindex="-1">#</a>`, id.([]byte))
	}
	fmt.Fprintf(w, "</h%d>\n", h.Level)
	return ast.WalkContinue, nil
}

func (n *nodeRenderer) renderCode(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var lang string
	if fcb, ok := node.(*ast.FencedCodeBlock); ok {
		lang = string(fcb.Language(source))
	}
	var code strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		code.Write(seg.Value(source))
	}
	dataQBL := ""
	if v, ok := node.AttributeString("data-qbl"); ok {
		if b, ok := v.([]byte); ok {
			dataQBL = string(b)
		}
	}
	n.hl.render(w, code.String(), lang, dataQBL)
	return ast.WalkSkipChildren, nil
}

// slugFor derives a doc slug from its path: extension dropped, path separators
// replaced with "--" (conventions §Naming).
func slugFor(rel string) string {
	rel = strings.TrimSuffix(rel, path.Ext(rel))
	rel = strings.ReplaceAll(rel, "/", "--")
	return rel
}

// fileTitle is the base filename without extension, used when a doc has no h1.
func fileTitle(rel string) string {
	return strings.TrimSuffix(path.Base(rel), path.Ext(rel))
}

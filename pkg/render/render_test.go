package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func newPaperRenderer(t *testing.T, opts Options) *Renderer {
	t.Helper()
	if opts.Theme == nil {
		opts.Theme = Paper()
	}
	r, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// blocksDump is the golden shape for Doc.Text + block fingerprints.
type blocksDump struct {
	Text   string  `json:"text"`
	Blocks []Block `json:"blocks"`
}

// Row 1: kitchen-sink body HTML matches golden.
func TestRow01_KitchenSinkBodyGolden(t *testing.T) {
	r := newPaperRenderer(t, Options{TOC: true, Title: "Docs"})
	doc, err := r.RenderDoc(readFixture(t, "kitchen-sink.md"), "kitchen-sink.md")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	goldenBytes(t, "kitchen-sink.body.html", doc.HTML)
}

// Row 2: normalized text + blocks (IDs, offsets) match golden JSON.
func TestRow02_KitchenSinkBlocksGolden(t *testing.T) {
	r := newPaperRenderer(t, Options{})
	doc, err := r.RenderDoc(readFixture(t, "kitchen-sink.md"), "kitchen-sink.md")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	out, err := json.MarshalIndent(blocksDump{Text: doc.Text, Blocks: doc.Blocks}, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	goldenBytes(t, "kitchen-sink.blocks.json", append(out, '\n'))

	// Every non-empty block ID must also appear as a data-qbl in the HTML.
	html := string(doc.Page)
	for _, b := range doc.Blocks {
		if !strings.Contains(html, `data-qbl="`+b.ID+`"`) {
			t.Errorf("block %q (%q) has no data-qbl in HTML", b.ID, b.Text)
		}
	}
}

// Row 3: rendering the same doc twice yields identical block IDs.
func TestRow03_FingerprintStability(t *testing.T) {
	r := newPaperRenderer(t, Options{})
	src := readFixture(t, "kitchen-sink.md")
	a, _ := r.RenderDoc(src, "k.md")
	b, _ := r.RenderDoc(src, "k.md")
	if len(a.Blocks) != len(b.Blocks) {
		t.Fatalf("block count differs: %d vs %d", len(a.Blocks), len(b.Blocks))
	}
	for i := range a.Blocks {
		if a.Blocks[i].ID != b.Blocks[i].ID {
			t.Errorf("block %d ID unstable: %q vs %q", i, a.Blocks[i].ID, b.Blocks[i].ID)
		}
	}
}

// Row 4: three identical paragraphs share a hash and index -0 -1 -2 in order.
func TestRow04_DuplicateBlocks(t *testing.T) {
	src := []byte("same para\n\nsame para\n\nsame para\n")
	_, blocks := NormalizeBlocks(src)
	if len(blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d", len(blocks))
	}
	hash := strings.SplitN(blocks[0].ID, "-", 2)[0]
	for i, b := range blocks {
		wantID := hash + "-" + itoa(i)
		if b.ID != wantID {
			t.Errorf("block %d ID = %q, want %q", i, b.ID, wantID)
		}
	}
}

// Row 5: editing block N changes only block N's ID; other hashes' occurrence
// indexes are unaffected.
func TestRow05_EditOneBlock(t *testing.T) {
	before := []byte("alpha\n\nbeta\n\ngamma\n")
	after := []byte("alpha\n\nBETA EDITED\n\ngamma\n")
	_, b1 := NormalizeBlocks(before)
	_, b2 := NormalizeBlocks(after)
	if len(b1) != 3 || len(b2) != 3 {
		t.Fatalf("unexpected block counts %d %d", len(b1), len(b2))
	}
	if b1[0].ID != b2[0].ID {
		t.Errorf("block 0 (alpha) changed: %q -> %q", b1[0].ID, b2[0].ID)
	}
	if b1[2].ID != b2[2].ID {
		t.Errorf("block 2 (gamma) changed: %q -> %q", b1[2].ID, b2[2].ID)
	}
	if b1[1].ID == b2[1].ID {
		t.Errorf("edited block 1 kept its ID %q", b1[1].ID)
	}
}

// Row 6: two "## Setup" headings dedupe to setup, setup-1.
func TestRow06_HeadingSlugDedupe(t *testing.T) {
	r := newPaperRenderer(t, Options{})
	doc, err := r.RenderDoc([]byte("## Setup\n\ntext\n\n## Setup\n\nmore\n"), "d.md")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	var anchors []string
	for _, h := range doc.Outline {
		anchors = append(anchors, h.Anchor)
	}
	want := []string{"setup", "setup-1"}
	if strings.Join(anchors, ",") != strings.Join(want, ",") {
		t.Errorf("anchors = %v, want %v", anchors, want)
	}
	if !strings.Contains(string(doc.HTML), `id="setup"`) || !strings.Contains(string(doc.HTML), `id="setup-1"`) {
		t.Errorf("heading ids missing from HTML")
	}
}

// Row 7: first h1 is the title; without an h1 the filename (no ext) is used.
func TestRow07_TitleExtraction(t *testing.T) {
	r := newPaperRenderer(t, Options{})
	withH1, _ := r.RenderDoc([]byte("# Real Title\n\n## Later\n"), "docs/ignored.md")
	if withH1.Title != "Real Title" {
		t.Errorf("title with h1 = %q, want %q", withH1.Title, "Real Title")
	}
	noH1, _ := r.RenderDoc([]byte("## Only h2\n\nbody\n"), "docs/my-file.md")
	if noH1.Title != "my-file" {
		t.Errorf("title without h1 = %q, want %q", noH1.Title, "my-file")
	}
}

// Row 8: RenderDir produces convention slugs and an index listing all docs.
func TestRow08_RenderDirSlugs(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/payments/plan.md": {Data: []byte("# Plan\n\nbody\n")},
		"README.md":             {Data: []byte("# Readme\n\nhi\n")},
		"notes.txt":             {Data: []byte("ignored")},
	}
	r := newPaperRenderer(t, Options{Title: "My Docs"})
	site, err := r.RenderDir(fsys)
	if err != nil {
		t.Fatalf("RenderDir: %v", err)
	}
	if len(site.Docs) != 2 {
		t.Fatalf("want 2 docs, got %d", len(site.Docs))
	}
	slugs := map[string]bool{}
	for _, d := range site.Docs {
		slugs[d.Slug] = true
	}
	if !slugs["docs--payments--plan"] {
		t.Errorf("missing slug docs--payments--plan; got %v", slugs)
	}
	if !slugs["README"] {
		t.Errorf("missing slug README; got %v", slugs)
	}
	index := string(site.index)
	if !strings.Contains(index, `href="docs--payments--plan.html"`) ||
		!strings.Contains(index, `href="README.html"`) {
		t.Errorf("index.html does not list all docs:\n%s", index)
	}
}

// Row 9: WriteTo output is self-contained — relative refs only, no network.
func TestRow09_WriteToSelfContained(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md": {Data: []byte("# Doc A\n\nSee [B](./b.md).\n\n```go\nvar x = 1\n```\n")},
		"b.md": {Data: []byte("# Doc B\n\nBack to [A](./a.md).\n")},
	}
	r := newPaperRenderer(t, Options{TOC: true, Title: "Site"})
	site, err := r.RenderDir(fsys)
	if err != nil {
		t.Fatalf("RenderDir: %v", err)
	}
	dir := t.TempDir()
	if err := site.WriteTo(dir); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	// index.html + 2 docs must exist.
	for _, f := range []string{"index.html", "a.html", "b.html", "qbl/tokens.css", "qbl/chroma.css"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected output file %s: %v", f, err)
		}
	}
	// No absolute or network references anywhere in the output.
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, _ := os.ReadFile(p)
		s := string(data)
		if strings.Contains(s, "http://") || strings.Contains(s, "https://") {
			t.Errorf("%s contains an absolute http(s) reference", p)
		}
		if strings.Contains(s, `href="/`) || strings.Contains(s, `src="/`) {
			t.Errorf("%s contains an absolute path reference", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// Row 10: a custom theme missing a token fails ThemeFromFS, naming the token.
func TestRow10_MissingTokenInCustomTheme(t *testing.T) {
	// tokens.css omits --qbl-accent from the dark block.
	tokens := `:root {
  --qbl-bg: #fff; --qbl-bg-raised:#eee; --qbl-fg:#000; --qbl-fg-muted:#555;
  --qbl-accent:#00f; --qbl-border:#ccc; --qbl-prose-max:68ch;
  --qbl-font-body:serif; --qbl-font-heading:serif; --qbl-font-mono:monospace;
  --qbl-radius:6px; --qbl-mark-bg:#ff0; --qbl-mark-bg-active:#fa0;
  --qbl-comment-bg:#fff; --qbl-comment-border:#ccc;
}
[data-qbl-scheme="dark"] {
  --qbl-bg:#000; --qbl-bg-raised:#111; --qbl-fg:#fff; --qbl-fg-muted:#aaa;
  --qbl-border:#333; --qbl-prose-max:68ch;
  --qbl-font-body:serif; --qbl-font-heading:serif; --qbl-font-mono:monospace;
  --qbl-radius:6px; --qbl-mark-bg:#ff0; --qbl-mark-bg-active:#fa0;
  --qbl-comment-bg:#000; --qbl-comment-border:#333;
}`
	fsys := fstest.MapFS{
		"theme.yml":  {Data: []byte("name: broken\nschemes: [light, dark]\n")},
		"tokens.css": {Data: []byte(tokens)},
		"theme.css":  {Data: []byte("body{}")},
	}
	_, err := ThemeFromFS(fsys)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !strings.Contains(err.Error(), "--qbl-accent") {
		t.Errorf("error should name the missing token --qbl-accent, got: %v", err)
	}
}

// Row 11: overriding an unknown token makes New fail, naming it.
func TestRow11_OverrideUnknownToken(t *testing.T) {
	_, err := New(Options{Theme: Paper(), Overrides: map[string]string{"--qbl-nope": "#123456"}})
	if err == nil {
		t.Fatal("expected error for unknown override token")
	}
	if !strings.Contains(err.Error(), "--qbl-nope") {
		t.Errorf("error should name --qbl-nope, got: %v", err)
	}
}

// Row 12: an --qbl-accent override appears exactly once, after the theme CSS.
func TestRow12_OverrideAccentPlacement(t *testing.T) {
	const val = "#7c3aed"
	r := newPaperRenderer(t, Options{Overrides: map[string]string{"--qbl-accent": val}})
	doc, err := r.RenderDoc([]byte("# Hi\n\nbody\n"), "d.md")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	page := string(doc.Page)
	if n := strings.Count(page, val); n != 1 {
		t.Fatalf("override value appears %d times, want 1", n)
	}
	themeIdx := strings.Index(page, "theme.css")
	valIdx := strings.Index(page, val)
	if themeIdx < 0 || valIdx < 0 || valIdx < themeIdx {
		t.Errorf("override (%d) must come after theme.css link (%d)", valIdx, themeIdx)
	}
}

// Row 13: an unknown code-fence language renders plainly, without error.
func TestRow13_UnknownCodeFence(t *testing.T) {
	r := newPaperRenderer(t, Options{})
	doc, err := r.RenderDoc([]byte("```wibble\nnot a language\n```\n"), "d.md")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	html := string(doc.HTML)
	if !strings.Contains(html, "not a language") {
		t.Errorf("code content missing: %s", html)
	}
	if !strings.Contains(html, "<pre") {
		t.Errorf("expected a <pre> for unknown language: %s", html)
	}
}

// Row 14: an empty markdown file yields a valid page with an empty article.
func TestRow14_EmptyDocument(t *testing.T) {
	r := newPaperRenderer(t, Options{TOC: true})
	doc, err := r.RenderDoc([]byte(""), "empty.md")
	if err != nil {
		t.Fatalf("RenderDoc: %v", err)
	}
	if len(doc.Blocks) != 0 {
		t.Errorf("empty doc should have no blocks, got %d", len(doc.Blocks))
	}
	if doc.Title != "empty" {
		t.Errorf("title = %q, want filename fallback %q", doc.Title, "empty")
	}
	page := string(doc.Page)
	if !strings.Contains(page, "<article") || !strings.Contains(page, "</html>") {
		t.Errorf("page missing chrome: %s", page)
	}
}

// itoa is a tiny helper to avoid importing strconv in table assertions.
func itoa(i int) string {
	return string(rune('0' + i))
}

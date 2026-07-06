package render

import (
	"fmt"
	"os"
	"path/filepath"
)

// Heading is one entry in a document's outline, used to build the TOC.
type Heading struct {
	Level  int
	Text   string
	Anchor string
}

// Doc is a single rendered document.
type Doc struct {
	Slug    string    // path-derived slug (conventions §Naming)
	RelPath string    // path relative to the docs root
	Title   string    // first h1, else the filename without extension
	HTML    []byte    // article body only, no page chrome
	Page    []byte    // full standalone page: chrome + optional TOC + article
	Blocks  []Block   // top-level block fingerprints, in document order
	Outline []Heading // headings, in document order
	Text    string    // normalized plain text of the whole document (anchoring input)
}

// Site is a rendered collection of documents plus the assets they share.
type Site struct {
	Docs   []*Doc
	Assets map[string][]byte // css/js served alongside; keys are site-relative paths

	index []byte // rendered index.html; written by WriteTo
}

// WriteTo writes index.html, one <slug>.html per document, and every asset into
// dir, creating subdirectories as needed. The result is a self-contained static
// site with only relative references.
func (s *Site) WriteTo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", dir, err)
	}
	if s.index != nil {
		if err := writeFile(filepath.Join(dir, "index.html"), s.index); err != nil {
			return err
		}
	}
	for _, d := range s.Docs {
		if err := writeFile(filepath.Join(dir, d.Slug+".html"), d.Page); err != nil {
			return err
		}
	}
	for name, data := range s.Assets {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("creating asset dir for %s: %w", name, err)
		}
		if err := writeFile(p, data); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

package render

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

// requiredTokens is the design-token contract every theme must satisfy. Each
// token must be defined for both the light (:root) and dark
// ([data-qbl-scheme="dark"]) schemes. The comment UI and structural chrome
// style themselves exclusively from these tokens, so any theme automatically
// matches quibble's overlays.
var requiredTokens = []string{
	"--qbl-bg",
	"--qbl-bg-raised",
	"--qbl-fg",
	"--qbl-fg-muted",
	"--qbl-accent",
	"--qbl-border",
	"--qbl-prose-max",
	"--qbl-font-body",
	"--qbl-font-heading",
	"--qbl-font-mono",
	"--qbl-radius",
	"--qbl-mark-bg",
	"--qbl-mark-bg-active",
	"--qbl-comment-bg",
	"--qbl-comment-border",
}

// tokenSet is requiredTokens as a set, for override-key validation.
var tokenSet = func() map[string]bool {
	m := make(map[string]bool, len(requiredTokens))
	for _, t := range requiredTokens {
		m[t] = true
	}
	return m
}()

// Theme is a rendering theme: a name plus a filesystem holding the token
// contract and styling. A conforming FS contains theme.yml, tokens.css and
// theme.css; layout.html and assets/ are optional (full custom layouts land in
// v0.2, but the loader already accepts them).
type Theme interface {
	Name() string
	FS() fs.FS // must contain theme.yml, tokens.css, theme.css
}

//go:embed themes
var themesFS embed.FS

// Paper returns the built-in "paper" theme: a quiet, editorial, serif design
// tuned for long-form reading. It is the default and v0.1's showcase theme.
func Paper() Theme {
	sub, err := fs.Sub(themesFS, "themes/paper")
	if err != nil {
		// The embedded tree is compiled in; this cannot fail in a built binary.
		panic(fmt.Sprintf("render: embedded paper theme unavailable: %v", err))
	}
	return &fsTheme{name: "paper", fsys: sub}
}

// ThemeFromFS validates that fsys implements the theme contract and returns a
// Theme backed by it. It fails loudly, naming the first missing file or token.
func ThemeFromFS(fsys fs.FS) (Theme, error) {
	meta, err := readThemeMeta(fsys)
	if err != nil {
		return nil, err
	}
	if err := validateThemeFS(fsys); err != nil {
		return nil, err
	}
	name := meta.Name
	if name == "" {
		name = "custom"
	}
	return &fsTheme{name: name, fsys: fsys}, nil
}

type fsTheme struct {
	name string
	fsys fs.FS
}

func (t *fsTheme) Name() string { return t.name }
func (t *fsTheme) FS() fs.FS    { return t.fsys }

// themeMeta mirrors theme.yml: {name, author, schemes: [light, dark]}.
type themeMeta struct {
	Name    string   `yaml:"name"`
	Author  string   `yaml:"author"`
	Schemes []string `yaml:"schemes"`
}

func readThemeMeta(fsys fs.FS) (themeMeta, error) {
	var m themeMeta
	data, err := fs.ReadFile(fsys, "theme.yml")
	if err != nil {
		return m, fmt.Errorf("reading theme.yml: %w", err)
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parsing theme.yml: %w", err)
	}
	return m, nil
}

// validateThemeFS checks that theme.css exists and that tokens.css defines every
// required token under both the light and dark scheme selectors.
func validateThemeFS(fsys fs.FS) error {
	if _, err := fs.Stat(fsys, "theme.css"); err != nil {
		return fmt.Errorf("theme is missing theme.css: %w", err)
	}
	css, err := fs.ReadFile(fsys, "tokens.css")
	if err != nil {
		return fmt.Errorf("theme is missing tokens.css: %w", err)
	}
	return validateTokens(string(css))
}

// validateTokens enforces the token contract on a tokens.css body.
func validateTokens(css string) error {
	light, ok := cssBlock(css, ":root")
	if !ok {
		return fmt.Errorf("tokens.css: no :root block for the light scheme")
	}
	dark, ok := cssBlock(css, `[data-qbl-scheme="dark"]`)
	if !ok {
		return fmt.Errorf(`tokens.css: no [data-qbl-scheme="dark"] block for the dark scheme`)
	}
	for _, tok := range requiredTokens {
		if !strings.Contains(light, tok+":") {
			return fmt.Errorf("tokens.css: missing required token %s under :root", tok)
		}
		if !strings.Contains(dark, tok+":") {
			return fmt.Errorf(`tokens.css: missing required token %s under [data-qbl-scheme="dark"]`, tok)
		}
	}
	return nil
}

// cssBlock returns the body between the braces of the first rule whose selector
// begins with sel.
func cssBlock(css, sel string) (string, bool) {
	idx := strings.Index(css, sel)
	if idx < 0 {
		return "", false
	}
	rest := css[idx+len(sel):]
	open := strings.IndexByte(rest, '{')
	if open < 0 {
		return "", false
	}
	depth := 0
	for i := open; i < len(rest); i++ {
		switch rest[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[open+1 : i], true
			}
		}
	}
	return "", false
}

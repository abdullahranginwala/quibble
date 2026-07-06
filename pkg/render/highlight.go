package render

import (
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// highlighter renders fenced code with chroma, server-side, using CSS classes
// (never inline styles) so a single pair of stylesheets covers every block. It
// keys a light and a dark style off [data-qbl-scheme].
type highlighter struct {
	formatter  *chromahtml.Formatter
	lightStyle *chroma.Style
	darkStyle  *chroma.Style
}

// noPreWrapper makes the chroma formatter emit only the token spans; the
// renderer supplies its own <pre> so it can attach data-qbl and a copy button.
type noPreWrapper struct{}

func (noPreWrapper) Start(bool, string) string { return "" }
func (noPreWrapper) End(bool) string           { return "" }

func newHighlighter() *highlighter {
	light := styles.Get("github")
	dark := styles.Get("github-dark")
	if light == nil {
		light = styles.Fallback
	}
	if dark == nil {
		dark = styles.Fallback
	}
	return &highlighter{
		formatter:  chromahtml.New(chromahtml.WithClasses(true), chromahtml.WithPreWrapper(noPreWrapper{})),
		lightStyle: light,
		darkStyle:  dark,
	}
}

// render writes a highlighted code block. dataQBL, when non-empty, is emitted as
// the block's data-qbl fingerprint. Unknown languages fall back to a plain,
// escaped <pre> without error.
func (h *highlighter) render(w io.Writer, code, lang, dataQBL string) {
	dq := ""
	if dataQBL != "" {
		dq = fmt.Sprintf(` data-qbl="%s"`, dataQBL)
	}
	langClass := ""
	if lang != "" {
		langClass = fmt.Sprintf(` class="language-%s"`, template.HTMLEscapeString(lang))
	}

	fmt.Fprint(w, `<div class="qbl-code"><button class="qbl-copy" type="button" aria-label="Copy code">Copy</button>`)

	lexer := lexers.Get(lang)
	if lexer == nil {
		fmt.Fprintf(w, `<pre class="chroma"%s tabindex="0"><code%s>%s</code></pre></div>`,
			dq, langClass, template.HTMLEscapeString(code))
		return
	}
	lexer = chroma.Coalesce(lexer)
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		fmt.Fprintf(w, `<pre class="chroma"%s tabindex="0"><code%s>%s</code></pre></div>`,
			dq, langClass, template.HTMLEscapeString(code))
		return
	}
	var inner strings.Builder
	if err := h.formatter.Format(&inner, h.lightStyle, iterator); err != nil {
		fmt.Fprintf(w, `<pre class="chroma"%s tabindex="0"><code%s>%s</code></pre></div>`,
			dq, langClass, template.HTMLEscapeString(code))
		return
	}
	fmt.Fprintf(w, `<pre class="chroma"%s tabindex="0"><code%s>%s</code></pre></div>`,
		dq, langClass, inner.String())
}

// css builds the combined light/dark highlighting stylesheet. Light rules apply
// by default; dark rules apply under an explicit [data-qbl-scheme="dark"] and,
// when no manual scheme is set, under a prefers-color-scheme: dark media query.
func (h *highlighter) css() string {
	var lb, db strings.Builder
	_ = h.formatter.WriteCSS(&lb, h.lightStyle)
	_ = h.formatter.WriteCSS(&db, h.darkStyle)
	dark := db.String()

	var out strings.Builder
	out.WriteString("/* light */\n")
	out.WriteString(lb.String())
	out.WriteString("\n/* dark (manual toggle) */\n")
	out.WriteString(scopeCSS(dark, `[data-qbl-scheme="dark"]`))
	out.WriteString("\n/* dark (system preference) */\n@media (prefers-color-scheme: dark) {\n")
	out.WriteString(scopeCSS(dark, `:root:not([data-qbl-scheme="light"])`))
	out.WriteString("}\n")
	return out.String()
}

// scopeCSS prefixes every rule's selector list with prefix, turning a flat
// chroma stylesheet into one scoped under an ancestor selector. chroma emits
// only flat, single-level rules, so a brace scan is sufficient.
func scopeCSS(css, prefix string) string {
	var out strings.Builder
	for {
		open := strings.IndexByte(css, '{')
		if open < 0 {
			break
		}
		sel := stripCSSComments(css[:open])
		shut := strings.IndexByte(css[open:], '}')
		if shut < 0 {
			break
		}
		shut += open
		body := css[open : shut+1]

		var scoped []string
		for _, part := range strings.Split(sel, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			scoped = append(scoped, prefix+" "+part)
		}
		if len(scoped) > 0 {
			out.WriteString(strings.Join(scoped, ", "))
			out.WriteByte(' ')
			out.WriteString(body)
			out.WriteByte('\n')
		}
		css = css[shut+1:]
	}
	return out.String()
}

// stripCSSComments removes /* ... */ comments from a selector fragment.
func stripCSSComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "*/")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + s[i+j+2:]
	}
}

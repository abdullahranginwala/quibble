// Package render converts markdown documents into clean, readable static
// HTML. It is the public, independently importable half of quibble: goldmark
// parsing, chroma highlighting, a single well-executed theme, and stable
// block fingerprints used as anchoring hints by the review layer.
//
// The package never mutates markdown sources. See DESIGN.md §5.
package render

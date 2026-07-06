// Package web holds the embedded, zero-dependency frontend for `quibble serve`:
// vanilla ES modules and a token-only stylesheet, shipped exactly as authored
// (no build step). internal/server serves these under /qbl/*.
package web

import "embed"

// Assets holds the comment-layer frontend files (comments.js, anchor-render.js,
// ui.css). They are embedded here because go:embed cannot cross package
// boundaries, and internal/server must not reach up the tree with "..".
//
//go:embed comments.js anchor-render.js ui.css
var Assets embed.FS

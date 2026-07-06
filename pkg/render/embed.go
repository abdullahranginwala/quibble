package render

import "embed"

// assetPrefix is the site-relative directory for all emitted assets. The
// comment layer's module script is served from this path too (see the layout),
// and 404s harmlessly under plain `build` output.
const assetPrefix = "qbl/"

//go:embed assets/quibble.css
var assetsFS embed.FS

//go:embed templates/layout.html
var templatesFS embed.FS

// Package assets embeds the static frontend assets — the Tokyo Night theme,
// the v1 report stylesheet, and the vim navigation script — so the renderer
// and server can ship them without a separate file dependency at runtime.
package assets

import _ "embed"

//go:embed tokyo-night.css
var TokyoNightCSS string

//go:embed v1.css
var V1CSS string

//go:embed deck.css
var DeckCSS string

//go:embed vim-nav.js
var VimNavJS string

// ReportCSS is the full stylesheet bundle for a rendered report page.
var ReportCSS = TokyoNightCSS + "\n" + V1CSS + "\n" + DeckCSS

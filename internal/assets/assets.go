// Package assets embeds the static frontend assets — the Tokyo Night theme,
// the v1 report stylesheet, and the vim navigation script — so the renderer
// and server can ship them without a separate file dependency at runtime.
package assets

import (
	_ "embed"
	"encoding/base64"
	"strings"
)

//go:embed tokyo-night.css
var TokyoNightCSS string

//go:embed v1.css
var V1CSS string

//go:embed deck.css
var DeckCSS string

//go:embed aggregator.css
var AggregatorCSS string

//go:embed vim-nav.js
var VimNavJS string

//go:embed aggregator.js
var AggregatorJS string

//go:embed respond.js
var RespondJS string

//go:embed hd.svg
var FaviconSVG string

// FaviconDataURI is the hd monogram as a data: URI, for an inline
// <link rel="icon"> — so rendered report pages stay self-contained.
var FaviconDataURI = "data:image/svg+xml;base64," +
	base64.StdEncoding.EncodeToString([]byte(FaviconSVG))

// ReportCSS is the full stylesheet bundle for a rendered report page.
var ReportCSS = TokyoNightCSS + "\n" + V1CSS + "\n" + DeckCSS

// DeckUICSS is the stylesheet bundle for the aggregator shell.
var DeckUICSS = ReportCSS + "\n" + AggregatorCSS

// VimNavJSInline is vim-nav.js made safe to embed in an inline <script>
// element. The HTML parser ends a script at the literal "</script" regardless
// of JavaScript context, and vim-nav.js mentions it in a header comment; the
// backslash is inert inside JS strings and comments.
var VimNavJSInline = strings.ReplaceAll(VimNavJS, "</script", `<\/script`)

// ReportJS is the script bundle inlined into a rendered report page: vim
// navigation plus the interactive-block response handler.
var ReportJS = VimNavJSInline + "\n" + RespondJS

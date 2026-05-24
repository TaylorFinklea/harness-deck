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

//go:embed mobile.css
var MobileCSS string

//go:embed vim-nav.js
var VimNavJS string

//go:embed aggregator.js
var AggregatorJS string

//go:embed mobile.js
var MobileJS string

//go:embed tabs.js
var TabsJS string

//go:embed triage.js
var TriageJS string

//go:embed respond.js
var RespondJS string

//go:embed hd.svg
var FaviconSVG string

//go:embed manifest.webmanifest
var ManifestJSON string

//go:embed service-worker.js
var ServiceWorkerJS string

// FaviconDataURI is the hd monogram as a data: URI, for an inline
// <link rel="icon"> — so rendered report pages stay self-contained.
var FaviconDataURI = "data:image/svg+xml;base64," +
	base64.StdEncoding.EncodeToString([]byte(FaviconSVG))

// ReportCSS is the full stylesheet bundle for a rendered report page —
// includes the mobile overrides so a phone-opened report scales too.
var ReportCSS = TokyoNightCSS + "\n" + V1CSS + "\n" + DeckCSS + "\n" + MobileCSS

// DeckUICSS is the stylesheet bundle for the aggregator shell.
var DeckUICSS = ReportCSS + "\n" + AggregatorCSS

// VimNavJSInline is vim-nav.js made safe to embed in an inline <script>
// element. The HTML parser ends a script at the literal "</script" regardless
// of JavaScript context, and vim-nav.js mentions it in a header comment; the
// backslash is inert inside JS strings and comments.
var VimNavJSInline = strings.ReplaceAll(VimNavJS, "</script", `<\/script`)

// MobileJSInline is mobile.js with the same </script-escape treatment so
// it is safe to inline.
var MobileJSInline = strings.ReplaceAll(MobileJS, "</script", `<\/script`)

// TabsJSInline is tabs.js with </script escaped for safe inlining.
var TabsJSInline = strings.ReplaceAll(TabsJS, "</script", `<\/script`)

// TriageJSInline is triage.js with </script escaped for safe inlining.
var TriageJSInline = strings.ReplaceAll(TriageJS, "</script", `<\/script`)

// ReportJS is the script bundle inlined into a rendered report page: vim
// navigation, the response handler, the mobile drawer + service worker
// registration, the in-app tab strip, and the keyboard triage helper.
var ReportJS = VimNavJSInline + "\n" + RespondJS + "\n" + MobileJSInline + "\n" + TabsJSInline + "\n" + TriageJSInline

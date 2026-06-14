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

//go:embed live.js
var LiveJS string

//go:embed live-banner.js
var LiveBannerJS string

//go:embed search.js
var SearchJS string

//go:embed usage.js
var UsageJS string

//go:embed html-block.js
var HTMLBlockJS string

//go:embed respond.js
var RespondJS string

//go:embed hd.svg
var FaviconSVG string

// Prerendered PNG icons for PWA installs. iOS rasterizes apple-touch-icon SVGs
// inconsistently across versions; Android's home-screen launcher likewise
// prefers a real bitmap. We bake 180/192/512/1024 — 180 covers
// apple-touch-icon, 192/512 are the Web App Manifest standard pair, 1024 is
// future-proofing for larger maskable surfaces.
//
//go:embed hd-180.png
var IconPNG180 []byte

//go:embed hd-192.png
var IconPNG192 []byte

//go:embed hd-512.png
var IconPNG512 []byte

//go:embed hd-1024.png
var IconPNG1024 []byte

//go:embed manifest.webmanifest
var ManifestJSON string

//go:embed service-worker.js
var ServiceWorkerJS string

// FaviconDataURI is the hd monogram as a data: URI, for an inline
// <link rel="icon"> — so rendered report pages stay self-contained.
var FaviconDataURI = "data:image/svg+xml;base64," +
	base64.StdEncoding.EncodeToString([]byte(FaviconSVG))

// baseCSS is the desktop stylesheet stack without mobile overrides.
// MobileCSS must come last in every bundle so its @media (max-width)
// rules trump every desktop selector with equal specificity.
var baseCSS = TokyoNightCSS + "\n" + V1CSS + "\n" + DeckCSS

// ReportCSS is the stylesheet bundle for a rendered report page.
var ReportCSS = baseCSS + "\n" + MobileCSS

// DeckUICSS is the stylesheet bundle for the aggregator shell.
var DeckUICSS = baseCSS + "\n" + AggregatorCSS + "\n" + MobileCSS

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

// LiveJSInline is live.js with </script escaped for safe inlining.
var LiveJSInline = strings.ReplaceAll(LiveJS, "</script", `<\/script`)

// LiveBannerJSInline is live-banner.js with </script escaped for inline.
var LiveBannerJSInline = strings.ReplaceAll(LiveBannerJS, "</script", `<\/script`)

// SearchJSInline is search.js with </script escaped for safe inlining.
var SearchJSInline = strings.ReplaceAll(SearchJS, "</script", `<\/script`)

// UsageJSInline is usage.js with </script escaped for safe inlining.
var UsageJSInline = strings.ReplaceAll(UsageJS, "</script", `<\/script`)

// HTMLBlockJSInline is html-block.js with </script escaped for safe inlining.
var HTMLBlockJSInline = strings.ReplaceAll(HTMLBlockJS, "</script", `<\/script`)

// RespondJSInline is respond.js with </script escaped for safe inlining.
// respond.js does not currently contain the sequence, but applying the guard
// here ensures any future edit that introduces it cannot break the inline
// <script> context. All bundle members must go through this escape.
var RespondJSInline = strings.ReplaceAll(RespondJS, "</script", `<\/script`)

// ReportJS is the script bundle inlined into a rendered report page: vim
// navigation, the response handler, the mobile drawer + service worker
// registration, the in-app tab strip, the keyboard triage helper, the
// SSE-driven live-reload watcher, the live in-flight telemetry banner,
// the Cmd+K search palette, and the html-block shadow-DOM isolator.
//
// ORDER IS KEYBOARD PRECEDENCE. Every file registers its document-level
// keydown listener at load, and bubble-phase listeners fire in
// registration order — so concatenation order here (and the <script>
// order in server/shell.html.tmpl) decides who sees a key first:
// vim-nav → respond → mobile → tabs (the single g-chord owner, see
// window.HDKeys) → triage. Reordering this line silently reshuffles
// keyboard semantics; don't.
var ReportJS = VimNavJSInline + "\n" + RespondJSInline + "\n" + MobileJSInline + "\n" + TabsJSInline + "\n" + TriageJSInline + "\n" + LiveJSInline + "\n" + LiveBannerJSInline + "\n" + SearchJSInline + "\n" + HTMLBlockJSInline

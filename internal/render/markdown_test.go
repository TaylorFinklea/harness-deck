package render

import (
	"strings"
	"testing"
)

func TestMarkdownLinksRejectScriptableSchemes(t *testing.T) {
	// The [text](url) path is the SAFE typed-rendering pipeline (prose,
	// callouts, ask prompts, the roadmap view) — a javascript:/data: href
	// must never become a live link there. The html block is the only
	// sanctioned escape hatch.
	for _, src := range []string{
		"[click](javascript:alert(1))",
		"[click](JavaScript:alert(document.domain))",
		"[click](data:text/html;base64,PHNjcmlwdD4=)",
		"[click](vbscript:Msg)",
	} {
		out := string(Markdown(src))
		if strings.Contains(out, "<a ") {
			t.Errorf("scriptable scheme became a live link: %q -> %s", src, out)
		}
	}
}

func TestMarkdownLinksAllowSafeSchemes(t *testing.T) {
	for _, src := range []string{
		"[site](https://example.com/path)",
		"[site](http://example.com)",
		"[mail](mailto:a@example.com)",
		"[rel](/reports/x)",
		"[anchor](#section)",
	} {
		out := string(Markdown(src))
		if !strings.Contains(out, "<a href=") {
			t.Errorf("safe link not rendered: %q -> %s", src, out)
		}
	}
}

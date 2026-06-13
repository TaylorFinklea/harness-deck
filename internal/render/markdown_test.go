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

// TestInlineEmphasisDoesNotCorruptSpans guards the placeholder-tokenizing
// inline pass: emphasis must not run over a link's href, a code span's body,
// or spaced-out asterisks, while ordinary *italic*/**bold** keep working.
func TestInlineEmphasisDoesNotCorruptSpans(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "asterisks in href stay intact",
			src:  "[a](https://e.com/a*b*c)",
			want: `<a href="https://e.com/a*b*c" rel="noopener">a</a>`,
		},
		{
			name: "underscores in href stay intact",
			src:  "[a](https://e.com/a_b_c)",
			want: `<a href="https://e.com/a_b_c" rel="noopener">a</a>`,
		},
		{
			name: "autolink href with asterisk stays intact",
			src:  "<https://e.com/a*b*c>",
			want: `<a href="https://e.com/a*b*c" rel="noopener">https://e.com/a*b*c</a>`,
		},
		{
			name: "asterisks inside a code span are literal",
			src:  "`a * b * c`",
			want: "<code>a * b * c</code>",
		},
		{
			name: "spaced asterisks are not emphasis",
			src:  "a * b * c",
			want: "a * b * c",
		},
		{
			name: "plain italic still works",
			src:  "*italic*",
			want: "<i>italic</i>",
		},
		{
			name: "plain bold still works",
			src:  "**bold**",
			want: "<b>bold</b>",
		},
		{
			name: "emphasis inside link text still renders",
			src:  "[**bold** text](https://e.com)",
			want: `<a href="https://e.com" rel="noopener"><b>bold</b> text</a>`,
		},
		{
			name: "code span and italic coexist on one line",
			src:  "text with `code *x*` and *real*",
			want: "text with <code>code *x*</code> and <i>real</i>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inlineMarkdown(tc.src); got != tc.want {
				t.Errorf("inlineMarkdown(%q)\n got: %s\nwant: %s", tc.src, got, tc.want)
			}
		})
	}
}

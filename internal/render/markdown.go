package render

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

// A deliberately small Markdown renderer — paragraphs, unordered lists, and the
// inline marks the report format actually uses (**bold**, *italic*, `code`).
// It is dependency-free so the binary builds with no module downloads. If
// reports ever need richer Markdown (headings, tables, links), swap this for
// goldmark behind the same two functions.

var (
	reCode   = regexp.MustCompile("`([^`]+)`")
	reBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItalic = regexp.MustCompile(`\*([^*]+)\*`)
	reBullet = regexp.MustCompile(`^[-*] +`)
)

// inlineMarkdown escapes HTML then applies inline marks. The replacement tags
// are inserted after escaping, so user text can never inject markup.
func inlineMarkdown(s string) string {
	s = html.EscapeString(s)
	s = reCode.ReplaceAllString(s, "<code>$1</code>")
	s = reBold.ReplaceAllString(s, "<b>$1</b>")
	s = reItalic.ReplaceAllString(s, "<i>$1</i>")
	return s
}

// renderMarkdownInline renders a single line/run of Markdown with no block
// wrapping — used for timeline messages, recommendation bodies, table cells.
func renderMarkdownInline(s string) template.HTML {
	return template.HTML(inlineMarkdown(strings.TrimSpace(s)))
}

// renderMarkdown renders block-level Markdown: blank-line-separated paragraphs
// and "- "/"* " bullet lists.
func renderMarkdown(s string) template.HTML {
	var out strings.Builder
	for _, chunk := range regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(s), -1) {
		lines := strings.Split(strings.TrimSpace(chunk), "\n")
		if isBulletList(lines) {
			out.WriteString("<ul>")
			for _, ln := range lines {
				item := reBullet.ReplaceAllString(strings.TrimSpace(ln), "")
				out.WriteString("<li>" + inlineMarkdown(item) + "</li>")
			}
			out.WriteString("</ul>")
			continue
		}
		out.WriteString("<p>" + inlineMarkdown(strings.Join(lines, " ")) + "</p>")
	}
	return template.HTML(out.String())
}

func isBulletList(lines []string) bool {
	for _, ln := range lines {
		if !reBullet.MatchString(strings.TrimSpace(ln)) {
			return false
		}
	}
	return len(lines) > 0
}

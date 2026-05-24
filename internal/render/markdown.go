package render

import (
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
)

// A deliberately small Markdown renderer — headings, paragraphs, unordered
// lists, and the inline marks the report format uses (**bold**, *italic*,
// `code`). It is dependency-free so the binary builds with no module
// downloads. If reports ever need richer Markdown (tables, links), swap this
// for goldmark behind the same functions.

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

// Markdown renders block-level Markdown to HTML: ATX headings (# … ######),
// blank-line-separated paragraphs, and "- "/"* " bullet lists. Exported for
// the roadmap view, which renders each project's .docs/ai/roadmap.md.
func Markdown(s string) template.HTML { return renderMarkdown(s) }

// fencedCodeHTML returns the HTML for a fenced code block — a copy-button
// wrapper around <pre><code>. The body is HTML-escaped but otherwise
// untouched: inline marks like ` * _ inside code are literal.
func fencedCodeHTML(lang, body string) string {
	classAttr := ""
	if lang != "" {
		classAttr = ` class="lang-` + html.EscapeString(lang) + `"`
	}
	langChip := ""
	if lang != "" {
		langChip = `<span class="lang">` + html.EscapeString(lang) + `</span>`
	}
	return `<div class="code-block">` + langChip +
		`<button class="copy-btn" type="button" aria-label="Copy">copy</button>` +
		`<pre><code` + classAttr + `>` + html.EscapeString(body) + `</code></pre></div>`
}

// isFence reports whether line opens or closes a fenced code block, and
// returns the language tag if present. Only triple-backtick fences are
// recognised; tildes are uncommon in agent-authored reports.
func isFence(line string) (bool, string) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "```") {
		return false, ""
	}
	return true, strings.TrimSpace(strings.TrimPrefix(t, "```"))
}

func renderMarkdown(s string) template.HTML {
	var out strings.Builder
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "":
			i++
		case func() bool { ok, _ := isFence(lines[i]); return ok }():
			// Fenced code block: gather literal lines until the closing fence
			// (or EOF) and emit a single copyable <pre> block.
			_, lang := isFence(lines[i])
			i++
			var code []string
			for i < len(lines) {
				if ok, _ := isFence(lines[i]); ok {
					i++ // consume the closing fence
					break
				}
				code = append(code, lines[i])
				i++
			}
			out.WriteString(fencedCodeHTML(lang, strings.Join(code, "\n")))
		case headingLevel(line) > 0:
			lvl := headingLevel(line)
			text := strings.TrimSpace(strings.TrimLeft(line, "#"))
			fmt.Fprintf(&out, "<h%d>%s</h%d>", lvl, inlineMarkdown(text), lvl)
			i++
		case reBullet.MatchString(line):
			out.WriteString("<ul>")
			for i < len(lines) {
				cur := strings.TrimSpace(lines[i])
				if !reBullet.MatchString(cur) {
					break
				}
				out.WriteString("<li>" + inlineMarkdown(reBullet.ReplaceAllString(cur, "")) + "</li>")
				i++
			}
			out.WriteString("</ul>")
		default:
			var para []string
			for i < len(lines) {
				cur := strings.TrimSpace(lines[i])
				if cur == "" || headingLevel(cur) > 0 || reBullet.MatchString(cur) {
					break
				}
				if ok, _ := isFence(lines[i]); ok {
					break
				}
				para = append(para, cur)
				i++
			}
			out.WriteString("<p>" + inlineMarkdown(strings.Join(para, " ")) + "</p>")
		}
	}
	return template.HTML(out.String())
}

// headingLevel returns the ATX heading level (1–6) of a line, or 0 if it is
// not a heading. A heading is 1–6 leading '#' followed by a space.
func headingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

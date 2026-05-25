package render

import (
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
)

// A deliberately small Markdown renderer covering what report manifests
// actually use: headings, paragraphs, unordered lists, fenced code blocks,
// GitHub-style tables, blockquotes, inline marks (**bold**, *italic*,
// `code`), and links (both `[text](url)` and `<https://…>` autolinks).
// Dependency-free so the binary builds with no module downloads.

var (
	reCode   = regexp.MustCompile("`([^`]+)`")
	reBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItalic = regexp.MustCompile(`\*([^*]+)\*`)
	reBullet = regexp.MustCompile(`^[-*] +`)
	reQuote  = regexp.MustCompile(`^> ?`)
	// reLink matches `[text](url)` after HTML escape, so brackets in text are
	// preserved literally. Text is non-greedy, URL stops at `)`.
	reLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	// reAutolink matches `<https://…>` once it has been HTML-escaped to
	// `&lt;https://…&gt;` by inlineMarkdown. We use the escaped form so the
	// regex runs after the escape pass (otherwise `<` would have a meaning).
	reAutolink = regexp.MustCompile(`&lt;(https?://[^&\s>]+)&gt;`)
	// reTableSep matches the dashes-and-pipes row that separates a table
	// header from its body, e.g. `| --- | :--: | --: |`.
	reTableSep = regexp.MustCompile(`^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)+\|?\s*$`)
)

// inlineMarkdown escapes HTML then applies inline marks. The replacement tags
// are inserted after escaping, so user text can never inject markup. Order
// matters: links/autolinks come before bold/italic so an asterisk inside a
// link's text or URL doesn't get mistakenly styled.
func inlineMarkdown(s string) string {
	s = html.EscapeString(s)
	s = reAutolink.ReplaceAllString(s, `<a href="$1" rel="noopener">$1</a>`)
	s = reLink.ReplaceAllString(s, `<a href="$2" rel="noopener">$1</a>`)
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
		case reQuote.MatchString(line):
			// Blockquote: collect contiguous `> ` lines, join with spaces so
			// soft-wrapped quote lines render as one paragraph, then emit a
			// single <blockquote><p>…</p></blockquote>.
			var quoted []string
			for i < len(lines) {
				cur := strings.TrimSpace(lines[i])
				if !reQuote.MatchString(cur) {
					break
				}
				quoted = append(quoted, reQuote.ReplaceAllString(cur, ""))
				i++
			}
			out.WriteString("<blockquote><p>" + inlineMarkdown(strings.Join(quoted, " ")) + "</p></blockquote>")
		case isTableHeader(lines, i):
			// Table: `| h1 | h2 |` header, `| --- | --- |` separator, then
			// zero or more `| c1 | c2 |` data rows until a non-pipe line.
			header := splitTableRow(lines[i])
			i += 2 // skip header + separator
			out.WriteString(`<table class="md-table"><thead><tr>`)
			for _, h := range header {
				out.WriteString("<th>" + inlineMarkdown(h) + "</th>")
			}
			out.WriteString(`</tr></thead><tbody>`)
			for i < len(lines) {
				cur := strings.TrimSpace(lines[i])
				if cur == "" || !strings.Contains(cur, "|") {
					break
				}
				cells := splitTableRow(cur)
				out.WriteString("<tr>")
				for _, c := range cells {
					out.WriteString("<td>" + inlineMarkdown(c) + "</td>")
				}
				out.WriteString("</tr>")
				i++
			}
			out.WriteString(`</tbody></table>`)
		default:
			var para []string
			for i < len(lines) {
				cur := strings.TrimSpace(lines[i])
				if cur == "" || headingLevel(cur) > 0 || reBullet.MatchString(cur) || reQuote.MatchString(cur) {
					break
				}
				if ok, _ := isFence(lines[i]); ok {
					break
				}
				if isTableHeader(lines, i) {
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

// isTableHeader returns true if lines[i] is a header row and lines[i+1]
// is the matching `| --- |` separator — the GitHub Flavored Markdown
// table shape. Both rows must contain at least one `|`; the separator
// must consist of pipes, optional colons, and dashes.
func isTableHeader(lines []string, i int) bool {
	if i+1 >= len(lines) {
		return false
	}
	head := strings.TrimSpace(lines[i])
	sep := strings.TrimSpace(lines[i+1])
	if !strings.Contains(head, "|") {
		return false
	}
	return reTableSep.MatchString(sep)
}

// splitTableRow splits `| a | b | c |` into ["a", "b", "c"]. Leading and
// trailing pipes are optional; cells are trimmed.
func splitTableRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	for j, p := range parts {
		parts[j] = strings.TrimSpace(p)
	}
	return parts
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

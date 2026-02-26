package exporter

import (
	"html"
	"regexp"
	"strings"
)

var (
	mdLinkRE   = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	inlineCode = regexp.MustCompile("`([^`]+)`")
	strikeRE   = regexp.MustCompile(`~~([^~]+)~~`)
)

// renderMarkdown implements a small Markdown subset that works for typical notes
// while keeping this project stdlib-only.
func renderMarkdown(input string) string {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	var out strings.Builder
	var paragraph []string
	inCode := false
	inList := false

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		joined := strings.Join(paragraph, " ")
		out.WriteString("<p>")
		out.WriteString(renderInline(joined))
		out.WriteString("</p>\n")
		paragraph = paragraph[:0]
	}

	closeList := func() {
		if inList {
			out.WriteString("</ul>\n")
			inList = false
		}
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trim := strings.TrimSpace(line)

		if strings.HasPrefix(trim, "```") {
			flushParagraph()
			closeList()
			if inCode {
				out.WriteString("</code></pre>\n")
				inCode = false
			} else {
				out.WriteString("<pre><code>")
				inCode = true
			}
			continue
		}
		if inCode {
			out.WriteString(html.EscapeString(line))
			out.WriteByte('\n')
			continue
		}

		if trim == "" {
			flushParagraph()
			closeList()
			continue
		}

		if strings.HasPrefix(trim, "<") && strings.HasSuffix(trim, ">") {
			flushParagraph()
			closeList()
			out.WriteString(trim)
			out.WriteByte('\n')
			continue
		}

		headingLevel := 0
		for i := 0; i < len(trim) && i < 6 && trim[i] == '#'; i++ {
			headingLevel++
		}
		if headingLevel > 0 && len(trim) > headingLevel && trim[headingLevel] == ' ' {
			flushParagraph()
			closeList()
			text := strings.TrimSpace(trim[headingLevel:])
			out.WriteString("<h")
			out.WriteByte(byte('0' + headingLevel))
			out.WriteString(">")
			out.WriteString(renderInline(text))
			out.WriteString("</h")
			out.WriteByte(byte('0' + headingLevel))
			out.WriteString(">\n")
			continue
		}

		if strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "* ") || strings.HasPrefix(trim, "+ ") {
			flushParagraph()
			if !inList {
				out.WriteString("<ul>\n")
				inList = true
			}
			item := strings.TrimSpace(trim[2:])
			out.WriteString("<li>")
			out.WriteString(renderInline(item))
			out.WriteString("</li>\n")
			continue
		}

		paragraph = append(paragraph, trim)
	}

	flushParagraph()
	closeList()
	if inCode {
		out.WriteString("</code></pre>\n")
	}

	return out.String()
}

func renderInline(text string) string {
	esc := html.EscapeString(text)
	esc = mdLinkRE.ReplaceAllString(esc, `<a href="$2">$1</a>`)
	esc = inlineCode.ReplaceAllString(esc, `<code>$1</code>`)
	esc = strikeRE.ReplaceAllString(esc, `<del>$1</del>`)
	return esc
}

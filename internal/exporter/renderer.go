package exporter

import (
	"bytes"
	stdhtml "html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	mdhtml "github.com/yuin/goldmark/renderer/html"
)

var markdownEngine = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Footnote,
		extension.DefinitionList,
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		mdhtml.WithUnsafe(),
	),
)

// renderMarkdown renders CommonMark + GFM so exported pages preserve typical
// Obsidian markdown semantics (tables, emphasis, task lists, blockquotes, etc).
func renderMarkdown(input string) string {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	var buf bytes.Buffer
	if err := markdownEngine.Convert([]byte(normalized), &buf); err != nil {
		return "<p>" + stdhtml.EscapeString(normalized) + "</p>\n"
	}
	return buf.String()
}

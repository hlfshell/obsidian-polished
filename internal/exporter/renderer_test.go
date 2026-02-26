package exporter

import (
	"strings"
	"testing"
)

func TestRenderMarkdownCommonFeatures(t *testing.T) {
	in := strings.Join([]string{
		"# Heading One",
		"",
		"This has *italic* and **bold** and ~~strike~~.",
		"",
		"> quoted line",
		"",
		"1. first",
		"2. second",
		"",
		"- [x] done item",
		"- [ ] open item",
		"",
		"| A | B |",
		"|---|---|",
		"| 1 | 2 |",
		"",
		"---",
	}, "\n")

	out := renderMarkdown(in)

	checks := []string{
		`<h1 id="heading-one">Heading One</h1>`,
		`<em>italic</em>`,
		`<strong>bold</strong>`,
		`<del>strike</del>`,
		`<blockquote>`,
		`<ol>`,
		`<input checked="" disabled="" type="checkbox">`,
		`<table>`,
		`<hr>`,
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered markdown to include %q, got:\n%s", want, out)
		}
	}
}

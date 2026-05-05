package jira

import (
	"encoding/json"
	"testing"
)

func parseADF(t *testing.T, s string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("invalid ADF JSON in test: %v", err)
	}
	return v
}

func TestRenderADF(t *testing.T) {
	cases := []struct {
		name string
		adf  string
		want string
	}{
		{
			name: "nil returns empty",
			adf:  `null`,
			want: "",
		},
		{
			name: "single paragraph",
			adf: `{"type":"doc","version":1,"content":[
				{"type":"paragraph","content":[{"type":"text","text":"hello world"}]}
			]}`,
			want: "hello world",
		},
		{
			name: "headings at multiple levels",
			adf: `{"type":"doc","content":[
				{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Section"}]},
				{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Sub"}]}
			]}`,
			want: "## Section\n\n### Sub",
		},
		{
			name: "marks: bold, italic, code, link",
			adf: `{"type":"doc","content":[{"type":"paragraph","content":[
				{"type":"text","text":"a "},
				{"type":"text","text":"bold","marks":[{"type":"strong"}]},
				{"type":"text","text":" and "},
				{"type":"text","text":"italic","marks":[{"type":"em"}]},
				{"type":"text","text":" and "},
				{"type":"text","text":"code","marks":[{"type":"code"}]},
				{"type":"text","text":" and "},
				{"type":"text","text":"link","marks":[{"type":"link","attrs":{"href":"https://example.com"}}]}
			]}]}`,
			want: "a **bold** and *italic* and `code` and [link](https://example.com)",
		},
		{
			name: "bullet list with nested list",
			adf: `{"type":"doc","content":[{"type":"bulletList","content":[
				{"type":"listItem","content":[
					{"type":"paragraph","content":[{"type":"text","text":"top"}]},
					{"type":"bulletList","content":[
						{"type":"listItem","content":[
							{"type":"paragraph","content":[{"type":"text","text":"nested"}]}
						]}
					]}
				]},
				{"type":"listItem","content":[
					{"type":"paragraph","content":[{"type":"text","text":"second"}]}
				]}
			]}]}`,
			want: "- top\n  - nested\n- second",
		},
		{
			name: "ordered list",
			adf: `{"type":"doc","content":[{"type":"orderedList","content":[
				{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]},
				{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}
			]}]}`,
			want: "1. one\n2. two",
		},
		{
			name: "code block with language",
			adf: `{"type":"doc","content":[{"type":"codeBlock","attrs":{"language":"go"},"content":[
				{"type":"text","text":"fmt.Println(\"hi\")"}
			]}]}`,
			want: "```go\nfmt.Println(\"hi\")\n```",
		},
		{
			name: "blockquote",
			adf: `{"type":"doc","content":[{"type":"blockquote","content":[
				{"type":"paragraph","content":[{"type":"text","text":"quoted line one"}]},
				{"type":"paragraph","content":[{"type":"text","text":"quoted line two"}]}
			]}]}`,
			want: "> quoted line one\n> \n> quoted line two",
		},
		{
			name: "rule",
			adf: `{"type":"doc","content":[
				{"type":"paragraph","content":[{"type":"text","text":"a"}]},
				{"type":"rule"},
				{"type":"paragraph","content":[{"type":"text","text":"b"}]}
			]}`,
			want: "a\n\n---\n\nb",
		},
		{
			name: "mention renders display text",
			adf: `{"type":"doc","content":[{"type":"paragraph","content":[
				{"type":"text","text":"cc "},
				{"type":"mention","attrs":{"id":"abc","text":"@Andrew"}}
			]}]}`,
			want: "cc @Andrew",
		},
		{
			name: "info panel",
			adf: `{"type":"doc","content":[{"type":"panel","attrs":{"panelType":"info"},"content":[
				{"type":"paragraph","content":[{"type":"text","text":"heads up"}]}
			]}]}`,
			want: "> **INFO:** heads up",
		},
		{
			name: "hard break inside paragraph",
			adf: `{"type":"doc","content":[{"type":"paragraph","content":[
				{"type":"text","text":"line one"},
				{"type":"hardBreak"},
				{"type":"text","text":"line two"}
			]}]}`,
			want: "line one  \nline two",
		},
		{
			name: "unknown node degrades to inline content",
			adf: `{"type":"doc","content":[{"type":"weirdBlock","content":[
				{"type":"text","text":"still readable"}
			]}]}`,
			want: "still readable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderADF(parseADF(t, tc.adf))
			if got != tc.want {
				t.Errorf("RenderADF mismatch\nwant:\n%q\ngot:\n%q", tc.want, got)
			}
		})
	}
}

func TestRenderADFNilInput(t *testing.T) {
	if got := RenderADF(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

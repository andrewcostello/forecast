package jira

import (
	"encoding/json"
	"testing"
)

// roundTrip parses markdown to ADF and then renders it back to markdown via
// RenderADF. This is a coarse-grained check that the doc structure is sane and
// that the same renderer used to display tickets can faithfully read what
// MarkdownToADF emits.
func roundTrip(t *testing.T, md string) string {
	t.Helper()
	adf := MarkdownToADF(md)
	if adf["type"] != "doc" {
		t.Fatalf("expected doc root, got %v", adf["type"])
	}
	if _, ok := adf["content"].([]map[string]interface{}); !ok {
		t.Fatalf("expected []map content, got %T", adf["content"])
	}
	// Re-encode through interface{} so RenderADF (which expects interface{})
	// sees the same shape it would parse from JSON.
	buf, err := json.Marshal(adf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var v interface{}
	if err := json.Unmarshal(buf, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return RenderADF(v)
}

func TestMarkdownToADF_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single paragraph",
			in:   "hello world",
			want: "hello world",
		},
		{
			name: "two paragraphs separated by blank line",
			in:   "first paragraph\n\nsecond paragraph",
			want: "first paragraph\n\nsecond paragraph",
		},
		{
			name: "soft break joins lines with a space",
			in:   "line one\nline two",
			want: "line one line two",
		},
		{
			name: "headings",
			in:   "## Section\n\n### Sub",
			want: "## Section\n\n### Sub",
		},
		{
			name: "bold and italic",
			in:   "a **bold** and *italic* word",
			want: "a **bold** and *italic* word",
		},
		{
			name: "inline code",
			in:   "use `auto_mode` flag",
			want: "use `auto_mode` flag",
		},
		{
			name: "link",
			in:   "see [docs](https://example.com)",
			want: "see [docs](https://example.com)",
		},
		{
			name: "bullet list",
			in:   "- one\n- two\n- three",
			want: "- one\n- two\n- three",
		},
		{
			name: "ordered list",
			in:   "1. one\n2. two",
			want: "1. one\n2. two",
		},
		{
			name: "thematic break",
			in:   "a\n\n---\n\nb",
			want: "a\n\n---\n\nb",
		},
		{
			name: "blockquote",
			in:   "> quoted line",
			want: "> quoted line",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := roundTrip(t, tc.in)
			if got != tc.want {
				t.Errorf("round-trip mismatch\nin:   %q\nwant: %q\ngot:  %q", tc.in, tc.want, got)
			}
		})
	}
}

func TestMarkdownToADF_FencedCodeBlock(t *testing.T) {
	// The critical case: a fenced code block must produce a single ADF codeBlock
	// node with the language preserved, not one paragraph per line. This is the
	// bug that prevented Mermaid diagrams from rendering in Jira.
	adf := MarkdownToADF("```mermaid\nstateDiagram-v2\n[*] --> Idle\nIdle --> Solo\n```")
	content := adf["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("expected exactly one block, got %d: %+v", len(content), content)
	}
	block := content[0]
	if block["type"] != "codeBlock" {
		t.Fatalf("expected codeBlock, got %v", block["type"])
	}
	attrs := block["attrs"].(map[string]interface{})
	if attrs["language"] != "mermaid" {
		t.Errorf("expected language=mermaid, got %v", attrs["language"])
	}
	body := block["content"].([]map[string]interface{})[0]
	wantBody := "stateDiagram-v2\n[*] --> Idle\nIdle --> Solo"
	if body["text"] != wantBody {
		t.Errorf("code block body mismatch\nwant: %q\ngot:  %q", wantBody, body["text"])
	}
}

func TestMarkdownToADF_FencedCodeBlockNoLanguage(t *testing.T) {
	adf := MarkdownToADF("```\nplain code\n```")
	block := adf["content"].([]map[string]interface{})[0]
	if block["type"] != "codeBlock" {
		t.Fatalf("expected codeBlock, got %v", block["type"])
	}
	if _, hasAttrs := block["attrs"]; hasAttrs {
		t.Errorf("expected no attrs when language is empty, got %v", block["attrs"])
	}
}

func TestMarkdownToADF_Table(t *testing.T) {
	// The other critical case: a pipe table must produce a table node with
	// tableHeader and tableRow cells, not one paragraph per row.
	src := "| Input | Source | Authority |\n" +
		"|---|---|---|\n" +
		"| `auto_mode` | Simulator | opt-in |\n" +
		"| `user_count` | Server | derived |\n"

	adf := MarkdownToADF(src)
	content := adf["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("expected one table block, got %d: %+v", len(content), content)
	}
	table := content[0]
	if table["type"] != "table" {
		t.Fatalf("expected table, got %v", table["type"])
	}
	rows := table["content"].([]map[string]interface{})
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (1 header + 2 body), got %d", len(rows))
	}

	// First row must be header cells.
	headerCells := rows[0]["content"].([]map[string]interface{})
	if len(headerCells) != 3 {
		t.Fatalf("expected 3 header cells, got %d", len(headerCells))
	}
	if headerCells[0]["type"] != "tableHeader" {
		t.Errorf("expected tableHeader for first cell, got %v", headerCells[0]["type"])
	}

	// Subsequent rows must be body cells.
	bodyCells := rows[1]["content"].([]map[string]interface{})
	if bodyCells[0]["type"] != "tableCell" {
		t.Errorf("expected tableCell for body, got %v", bodyCells[0]["type"])
	}

	// Inline code in a body cell should carry a code mark.
	cellPara := bodyCells[0]["content"].([]map[string]interface{})[0]
	if cellPara["type"] != "paragraph" {
		t.Fatalf("expected paragraph inside tableCell, got %v", cellPara["type"])
	}
	textNode := cellPara["content"].([]map[string]interface{})[0]
	marks, ok := textNode["marks"].([]map[string]interface{})
	if !ok || len(marks) == 0 || marks[0]["type"] != "code" {
		t.Errorf("expected code mark on `auto_mode`, got marks=%v", textNode["marks"])
	}
}

func TestMarkdownToADF_NestedBulletList(t *testing.T) {
	src := "- top\n  - nested\n- second"
	got := roundTrip(t, src)
	want := "- top\n  - nested\n- second"
	if got != want {
		t.Errorf("nested list round-trip\nwant: %q\ngot:  %q", want, got)
	}
}

func TestMarkdownToADF_OrderedListNonOneStart(t *testing.T) {
	adf := MarkdownToADF("3. three\n4. four")
	list := adf["content"].([]map[string]interface{})[0]
	if list["type"] != "orderedList" {
		t.Fatalf("expected orderedList, got %v", list["type"])
	}
	attrs, ok := list["attrs"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected attrs with start, got none")
	}
	if attrs["order"] != 3 {
		t.Errorf("expected order=3, got %v", attrs["order"])
	}
}

func TestMarkdownToADF_EmptyInput(t *testing.T) {
	adf := MarkdownToADF("")
	if adf["type"] != "doc" {
		t.Fatalf("expected doc root, got %v", adf["type"])
	}
	content, ok := adf["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected []map content even when empty, got %T", adf["content"])
	}
	if len(content) != 0 {
		t.Errorf("expected empty content, got %+v", content)
	}
}

func TestMarkdownToADF_HardBreak(t *testing.T) {
	// Two trailing spaces before the newline = CommonMark hard break. Goldmark
	// may split the surrounding text across multiple Text segments; what
	// matters is that exactly one hardBreak node lands between the two lines,
	// and that text before/after concatenates to the expected content.
	adf := MarkdownToADF("line one  \nline two")
	para := adf["content"].([]map[string]interface{})[0]
	inline := para["content"].([]map[string]interface{})

	hardBreakIdx := -1
	for i, n := range inline {
		if n["type"] == "hardBreak" {
			if hardBreakIdx != -1 {
				t.Fatalf("expected exactly one hardBreak, got multiple: %+v", inline)
			}
			hardBreakIdx = i
		}
	}
	if hardBreakIdx == -1 {
		t.Fatalf("expected a hardBreak node, got %+v", inline)
	}

	concat := func(nodes []map[string]interface{}) string {
		var s string
		for _, n := range nodes {
			if n["type"] == "text" {
				s += n["text"].(string)
			}
		}
		return s
	}
	if got := concat(inline[:hardBreakIdx]); got != "line one" {
		t.Errorf("text before hardBreak: want %q, got %q", "line one", got)
	}
	if got := concat(inline[hardBreakIdx+1:]); got != "line two" {
		t.Errorf("text after hardBreak: want %q, got %q", "line two", got)
	}
}

func TestMarkdownToADF_Strikethrough(t *testing.T) {
	adf := MarkdownToADF("~~struck~~ text")
	para := adf["content"].([]map[string]interface{})[0]
	inline := para["content"].([]map[string]interface{})
	if len(inline) == 0 {
		t.Fatalf("no inline content")
	}
	marks, ok := inline[0]["marks"].([]map[string]interface{})
	if !ok || len(marks) == 0 || marks[0]["type"] != "strike" {
		t.Errorf("expected strike mark on first segment, got marks=%v", inline[0]["marks"])
	}
}

func TestMarkdownToADF_FullTicketShape(t *testing.T) {
	// Smoke test against a description that mirrors the structure of SMG-2775:
	// headings, paragraph with inline code, a fenced Mermaid block, and a
	// pipe table. We assert on block ordering and types, not on full content.
	src := "## Why\n\n" +
		"Today the session's play mode is fixed at configure time — see PR #464.\n\n" +
		"## State machine\n\n" +
		"```mermaid\n" +
		"stateDiagram-v2\n" +
		"[*] --> Idle\n" +
		"Idle --> Solo\n" +
		"```\n\n" +
		"## Inputs\n\n" +
		"| Input | Source |\n" +
		"|---|---|\n" +
		"| `auto_mode` | Simulator |\n"

	adf := MarkdownToADF(src)
	content := adf["content"].([]map[string]interface{})

	wantTypes := []string{"heading", "paragraph", "heading", "codeBlock", "heading", "table"}
	if len(content) != len(wantTypes) {
		t.Fatalf("expected %d blocks, got %d: %+v", len(wantTypes), len(content), content)
	}
	for i, want := range wantTypes {
		got := content[i]["type"]
		if got != want {
			t.Errorf("block %d: expected %q, got %q", i, want, got)
		}
	}

	// Verify the Mermaid code block has the right language.
	mermaid := content[3]
	attrs := mermaid["attrs"].(map[string]interface{})
	if attrs["language"] != "mermaid" {
		t.Errorf("expected mermaid language, got %v", attrs["language"])
	}
}

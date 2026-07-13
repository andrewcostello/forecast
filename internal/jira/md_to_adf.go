package jira

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// MarkdownToADF converts a CommonMark + GFM Markdown string to an Atlassian
// Document Format root node (a `doc` map with `type`, `version`, and `content`).
//
// Supported constructs:
//   - Headings (# ... ######)
//   - Paragraphs with soft and hard line breaks
//   - Bullet and ordered lists (including nesting and non-1 start values)
//   - Fenced code blocks (language hint preserved — e.g. ```mermaid)
//   - Indented code blocks
//   - Blockquotes
//   - Thematic breaks (---)
//   - GFM tables (first row → tableHeader cells)
//   - Inline: **strong**, *em*, `code`, ~~strike~~, [text](url), <url>
//
// Inline emphasis follows CommonMark: single asterisk/underscore is italic,
// double is bold. (The legacy parser this replaced treated `*x*` as bold.)
// Unrecognized constructs degrade to plain text rather than failing.
func MarkdownToADF(src string) map[string]interface{} {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	source := []byte(src)
	doc := md.Parser().Parse(text.NewReader(source))

	w := &mdWalker{source: source}
	content := w.walkBlocks(doc)
	if content == nil {
		content = []map[string]interface{}{}
	}

	return map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}

type mdWalker struct {
	source []byte
}

func (w *mdWalker) walkBlocks(parent ast.Node) []map[string]interface{} {
	var out []map[string]interface{}
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if n := w.convertBlock(c); n != nil {
			out = append(out, n)
		}
	}
	return out
}

func (w *mdWalker) convertBlock(node ast.Node) map[string]interface{} {
	switch n := node.(type) {
	case *ast.Heading:
		level := n.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		return map[string]interface{}{
			"type":    "heading",
			"attrs":   map[string]interface{}{"level": level},
			"content": w.walkInline(n, nil),
		}

	case *ast.Paragraph:
		return map[string]interface{}{
			"type":    "paragraph",
			"content": w.walkInline(n, nil),
		}

	case *ast.TextBlock:
		return map[string]interface{}{
			"type":    "paragraph",
			"content": w.walkInline(n, nil),
		}

	case *ast.FencedCodeBlock:
		return makeCodeBlock(string(n.Language(w.source)), readCodeLines(n, w.source))

	case *ast.CodeBlock:
		return makeCodeBlock("", readCodeLines(n, w.source))

	case *ast.Blockquote:
		return map[string]interface{}{
			"type":    "blockquote",
			"content": w.walkBlocks(n),
		}

	case *ast.List:
		return w.convertList(n)

	case *ast.ThematicBreak:
		return map[string]interface{}{"type": "rule"}

	case *ast.HTMLBlock:
		raw := readRawLines(n.BaseBlock.Lines(), w.source)
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		return map[string]interface{}{
			"type": "paragraph",
			"content": []map[string]interface{}{
				{"type": "text", "text": raw},
			},
		}

	case *east.Table:
		return w.convertTable(n)
	}
	return nil
}

func (w *mdWalker) convertList(n *ast.List) map[string]interface{} {
	listType := "bulletList"
	if n.IsOrdered() {
		listType = "orderedList"
	}

	var items []map[string]interface{}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*ast.ListItem); !ok {
			continue
		}
		items = append(items, map[string]interface{}{
			"type":    "listItem",
			"content": w.walkBlocks(c),
		})
	}

	node := map[string]interface{}{
		"type":    listType,
		"content": items,
	}
	if n.IsOrdered() && n.Start > 1 {
		node["attrs"] = map[string]interface{}{"order": n.Start}
	}
	return node
}

func (w *mdWalker) convertTable(t *east.Table) map[string]interface{} {
	var rows []map[string]interface{}
	for c := t.FirstChild(); c != nil; c = c.NextSibling() {
		switch row := c.(type) {
		case *east.TableHeader:
			rows = append(rows, w.convertTableRow(row, true))
		case *east.TableRow:
			rows = append(rows, w.convertTableRow(row, false))
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return map[string]interface{}{
		"type": "table",
		"attrs": map[string]interface{}{
			"isNumberColumnEnabled": false,
			"layout":                "default",
		},
		"content": rows,
	}
}

func (w *mdWalker) convertTableRow(row ast.Node, header bool) map[string]interface{} {
	cellType := "tableCell"
	if header {
		cellType = "tableHeader"
	}
	var cells []map[string]interface{}
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		cells = append(cells, map[string]interface{}{
			"type": cellType,
			"content": []map[string]interface{}{
				{
					"type":    "paragraph",
					"content": w.walkInline(c, nil),
				},
			},
		})
	}
	return map[string]interface{}{
		"type":    "tableRow",
		"content": cells,
	}
}

type adfMark struct {
	typ  string
	href string
}

func (w *mdWalker) walkInline(parent ast.Node, marks []adfMark) []map[string]interface{} {
	var out []map[string]interface{}
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, w.convertInline(c, marks)...)
	}
	if out == nil {
		return []map[string]interface{}{}
	}
	return out
}

func (w *mdWalker) convertInline(node ast.Node, marks []adfMark) []map[string]interface{} {
	switch n := node.(type) {
	case *ast.Text:
		txt := string(n.Segment.Value(w.source))
		hard := n.HardLineBreak()
		soft := n.SoftLineBreak() && !hard
		if soft {
			txt += " "
		}
		var out []map[string]interface{}
		if txt != "" {
			out = append(out, makeTextNode(txt, marks))
		}
		if hard {
			out = append(out, map[string]interface{}{"type": "hardBreak"})
		}
		return out

	case *ast.String:
		s := string(n.Value)
		if s == "" {
			return nil
		}
		return []map[string]interface{}{makeTextNode(s, marks)}

	case *ast.Emphasis:
		markType := "em"
		if n.Level >= 2 {
			markType = "strong"
		}
		return w.walkInline(n, appendMark(marks, adfMark{typ: markType}))

	case *ast.CodeSpan:
		var sb strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				sb.Write(t.Segment.Value(w.source))
			}
		}
		s := sb.String()
		if s == "" {
			return nil
		}
		return []map[string]interface{}{makeTextNode(s, appendMark(marks, adfMark{typ: "code"}))}

	case *ast.Link:
		return w.walkInline(n, appendMark(marks, adfMark{typ: "link", href: string(n.Destination)}))

	case *ast.AutoLink:
		url := string(n.URL(w.source))
		if url == "" {
			return nil
		}
		return []map[string]interface{}{makeTextNode(url, appendMark(marks, adfMark{typ: "link", href: url}))}

	case *east.Strikethrough:
		return w.walkInline(n, appendMark(marks, adfMark{typ: "strike"}))

	case *ast.RawHTML:
		var sb strings.Builder
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			sb.Write(seg.Value(w.source))
		}
		s := sb.String()
		if s == "" {
			return nil
		}
		return []map[string]interface{}{makeTextNode(s, marks)}
	}

	if node.HasChildren() {
		return w.walkInline(node, marks)
	}
	return nil
}

func appendMark(marks []adfMark, m adfMark) []adfMark {
	out := make([]adfMark, len(marks), len(marks)+1)
	copy(out, marks)
	return append(out, m)
}

func makeTextNode(text string, marks []adfMark) map[string]interface{} {
	node := map[string]interface{}{
		"type": "text",
		"text": text,
	}
	if len(marks) > 0 {
		mks := make([]map[string]interface{}, 0, len(marks))
		for _, m := range marks {
			mk := map[string]interface{}{"type": m.typ}
			if m.typ == "link" && m.href != "" {
				mk["attrs"] = map[string]interface{}{"href": m.href}
			}
			mks = append(mks, mk)
		}
		node["marks"] = mks
	}
	return node
}

func makeCodeBlock(lang, body string) map[string]interface{} {
	block := map[string]interface{}{"type": "codeBlock"}
	if lang != "" {
		block["attrs"] = map[string]interface{}{"language": lang}
	}
	if body != "" {
		block["content"] = []map[string]interface{}{
			{"type": "text", "text": body},
		}
	}
	return block
}

func readCodeLines(n ast.Node, source []byte) string {
	return readRawLines(n.Lines(), source)
}

func readRawLines(lines *text.Segments, source []byte) string {
	if lines == nil {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		sb.Write(seg.Value(source))
	}
	return strings.TrimRight(sb.String(), "\n")
}

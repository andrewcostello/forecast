package jira

import (
	"fmt"
	"strings"
)

// RenderADF converts an Atlassian Document Format value to Markdown.
// The input is whatever json.Unmarshal produced — typically a map[string]interface{}
// for the root doc node, or nil if the field was absent.
func RenderADF(adf interface{}) string {
	if adf == nil {
		return ""
	}
	r := &adfRenderer{}
	r.renderNode(adf, 0)
	return strings.TrimRight(r.b.String(), "\n")
}

type adfRenderer struct {
	b strings.Builder
}

func (r *adfRenderer) renderNode(node interface{}, listDepth int) {
	n, ok := node.(map[string]interface{})
	if !ok {
		return
	}

	nodeType, _ := n["type"].(string)
	attrs, _ := n["attrs"].(map[string]interface{})
	content := childNodes(n)

	switch nodeType {
	case "doc":
		for i, c := range content {
			r.renderNode(c, listDepth)
			if i < len(content)-1 {
				r.b.WriteString("\n\n")
			}
		}
	case "paragraph":
		r.renderInline(content)
	case "heading":
		level := 1
		if attrs != nil {
			if l, ok := attrs["level"].(float64); ok {
				level = int(l)
			}
		}
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		r.b.WriteString(strings.Repeat("#", level))
		r.b.WriteString(" ")
		r.renderInline(content)
	case "bulletList":
		r.renderList(content, listDepth, false)
	case "orderedList":
		r.renderList(content, listDepth, true)
	case "listItem":
		// listItem content is rendered by renderList; here just join its blocks.
		for i, c := range content {
			r.renderNode(c, listDepth)
			if i < len(content)-1 {
				r.b.WriteString("\n")
			}
		}
	case "codeBlock":
		lang := ""
		if attrs != nil {
			if l, ok := attrs["language"].(string); ok {
				lang = l
			}
		}
		r.b.WriteString("```")
		r.b.WriteString(lang)
		r.b.WriteString("\n")
		for _, c := range content {
			if cm, ok := c.(map[string]interface{}); ok {
				if t, ok := cm["text"].(string); ok {
					r.b.WriteString(t)
				}
			}
		}
		r.b.WriteString("\n```")
	case "blockquote":
		var sub adfRenderer
		for i, c := range content {
			sub.renderNode(c, listDepth)
			if i < len(content)-1 {
				sub.b.WriteString("\n\n")
			}
		}
		for i, line := range strings.Split(strings.TrimRight(sub.b.String(), "\n"), "\n") {
			if i > 0 {
				r.b.WriteString("\n")
			}
			r.b.WriteString("> ")
			r.b.WriteString(line)
		}
	case "rule":
		r.b.WriteString("---")
	case "panel":
		panelType := ""
		if attrs != nil {
			panelType, _ = attrs["panelType"].(string)
		}
		var sub adfRenderer
		for i, c := range content {
			sub.renderNode(c, listDepth)
			if i < len(content)-1 {
				sub.b.WriteString("\n\n")
			}
		}
		label := strings.ToUpper(panelType)
		if label == "" {
			label = "PANEL"
		}
		r.b.WriteString("> **")
		r.b.WriteString(label)
		r.b.WriteString(":** ")
		text := strings.TrimRight(sub.b.String(), "\n")
		for i, line := range strings.Split(text, "\n") {
			if i > 0 {
				r.b.WriteString("\n> ")
			}
			r.b.WriteString(line)
		}
	case "table":
		r.renderTable(content)
	default:
		// Unknown block — best-effort render of its content.
		r.renderInline(content)
	}
}

func (r *adfRenderer) renderInline(nodes []interface{}) {
	for _, raw := range nodes {
		n, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		switch n["type"] {
		case "text":
			text, _ := n["text"].(string)
			r.b.WriteString(applyMarks(text, n["marks"]))
		case "hardBreak":
			r.b.WriteString("  \n")
		case "mention":
			if attrs, ok := n["attrs"].(map[string]interface{}); ok {
				if t, ok := attrs["text"].(string); ok && t != "" {
					r.b.WriteString(t)
					continue
				}
			}
			r.b.WriteString("@user")
		case "emoji":
			if attrs, ok := n["attrs"].(map[string]interface{}); ok {
				if t, ok := attrs["text"].(string); ok && t != "" {
					r.b.WriteString(t)
					continue
				}
				if s, ok := attrs["shortName"].(string); ok && s != "" {
					r.b.WriteString(s)
					continue
				}
			}
		case "inlineCard":
			if attrs, ok := n["attrs"].(map[string]interface{}); ok {
				if u, ok := attrs["url"].(string); ok {
					r.b.WriteString(u)
				}
			}
		case "status":
			if attrs, ok := n["attrs"].(map[string]interface{}); ok {
				if t, ok := attrs["text"].(string); ok {
					r.b.WriteString("[")
					r.b.WriteString(t)
					r.b.WriteString("]")
				}
			}
		default:
			// Nested block inside inline context — fall back to recursing.
			r.renderNode(raw, 0)
		}
	}
}

func (r *adfRenderer) renderList(items []interface{}, depth int, ordered bool) {
	indent := strings.Repeat("  ", depth)
	for i, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if i > 0 {
			r.b.WriteString("\n")
		}
		r.b.WriteString(indent)
		if ordered {
			r.b.WriteString(fmt.Sprintf("%d. ", i+1))
		} else {
			r.b.WriteString("- ")
		}
		// Render item content. First block goes inline; subsequent blocks (nested
		// lists, paragraphs) get newline + indent.
		blocks := childNodes(item)
		for j, block := range blocks {
			if j > 0 {
				r.b.WriteString("\n")
				// nested lists handle their own indent via depth+1
				if !isListBlock(block) {
					r.b.WriteString(indent + "  ")
				}
			}
			if isListBlock(block) {
				r.renderNode(block, depth+1)
			} else {
				r.renderNode(block, depth)
			}
		}
	}
}

func (r *adfRenderer) renderTable(rows []interface{}) {
	if len(rows) == 0 {
		return
	}
	rendered := make([][]string, 0, len(rows))
	colCount := 0
	for _, raw := range rows {
		row, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cells := childNodes(row)
		cellsText := make([]string, 0, len(cells))
		for _, c := range cells {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			var sub adfRenderer
			for _, blk := range childNodes(cm) {
				sub.renderNode(blk, 0)
			}
			cellsText = append(cellsText, strings.ReplaceAll(strings.TrimSpace(sub.b.String()), "\n", " "))
		}
		if len(cellsText) > colCount {
			colCount = len(cellsText)
		}
		rendered = append(rendered, cellsText)
	}
	if colCount == 0 {
		return
	}
	for i, row := range rendered {
		// pad to colCount
		for len(row) < colCount {
			row = append(row, "")
		}
		r.b.WriteString("| ")
		r.b.WriteString(strings.Join(row, " | "))
		r.b.WriteString(" |")
		if i == 0 {
			r.b.WriteString("\n|")
			r.b.WriteString(strings.Repeat(" --- |", colCount))
		}
		if i < len(rendered)-1 {
			r.b.WriteString("\n")
		}
	}
}

func applyMarks(text string, marks interface{}) string {
	if text == "" {
		return ""
	}
	ms, ok := marks.([]interface{})
	if !ok || len(ms) == 0 {
		return text
	}
	// Apply in a stable order so output is deterministic regardless of mark
	// ordering in the source: code, link, strong, em, strike.
	hasCode := false
	hasStrong := false
	hasEm := false
	hasStrike := false
	var linkHref string
	for _, raw := range ms {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		switch m["type"] {
		case "code":
			hasCode = true
		case "strong":
			hasStrong = true
		case "em":
			hasEm = true
		case "strike":
			hasStrike = true
		case "link":
			if attrs, ok := m["attrs"].(map[string]interface{}); ok {
				linkHref, _ = attrs["href"].(string)
			}
		}
	}
	out := text
	if hasCode {
		out = "`" + out + "`"
	}
	if hasStrike {
		out = "~~" + out + "~~"
	}
	if hasStrong {
		out = "**" + out + "**"
	}
	if hasEm {
		out = "*" + out + "*"
	}
	if linkHref != "" {
		out = "[" + out + "](" + linkHref + ")"
	}
	return out
}

func childNodes(n map[string]interface{}) []interface{} {
	c, ok := n["content"].([]interface{})
	if !ok {
		return nil
	}
	return c
}

func isListBlock(node interface{}) bool {
	n, ok := node.(map[string]interface{})
	if !ok {
		return false
	}
	t, _ := n["type"].(string)
	return t == "bulletList" || t == "orderedList"
}

package diagram

import (
	"fmt"
	"strings"
)

// generateFlow creates a D2 flow diagram showing status distribution
func (g *Generator) generateFlow() string {
	stats := g.GetFlowStats()

	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Flow Diagram: %s\n\n", g.ProjectName))

	// Title
	sb.WriteString(fmt.Sprintf("title: \"%s Status Flow\" {\n", g.ProjectName))
	sb.WriteString("  shape: text\n")
	sb.WriteString("  style.font-size: 24\n")
	sb.WriteString("}\n\n")

	// Status nodes
	sb.WriteString("to_do: {\n")
	sb.WriteString(fmt.Sprintf("  label: \"To Do\\n%d items\"\n", stats.ToDo))
	sb.WriteString("  style: {\n")
	sb.WriteString("    fill: \"#3b82f6\"\n")
	sb.WriteString("    font-color: \"#ffffff\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n\n")

	sb.WriteString("in_progress: {\n")
	sb.WriteString(fmt.Sprintf("  label: \"In Progress\\n%d items\"\n", stats.InProgress))
	sb.WriteString("  style: {\n")
	sb.WriteString("    fill: \"#f59e0b\"\n")
	sb.WriteString("    font-color: \"#ffffff\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n\n")

	sb.WriteString("done: {\n")
	sb.WriteString(fmt.Sprintf("  label: \"Done\\n%d items\"\n", stats.Done))
	sb.WriteString("  style: {\n")
	sb.WriteString("    fill: \"#10b981\"\n")
	sb.WriteString("    font-color: \"#ffffff\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n\n")

	// Flow arrows
	sb.WriteString("to_do -> in_progress: \"Start\" {\n")
	sb.WriteString("  style.stroke: \"#a0a0a0\"\n")
	sb.WriteString("}\n\n")

	sb.WriteString("in_progress -> done: \"Complete\" {\n")
	sb.WriteString("  style.stroke: \"#a0a0a0\"\n")
	sb.WriteString("}\n")

	// Add canceled if any
	if stats.Canceled > 0 {
		sb.WriteString("\n")
		sb.WriteString("canceled: {\n")
		sb.WriteString(fmt.Sprintf("  label: \"Canceled\\n%d items\"\n", stats.Canceled))
		sb.WriteString("  style: {\n")
		sb.WriteString("    fill: \"#6b7280\"\n")
		sb.WriteString("    font-color: \"#ffffff\"\n")
		sb.WriteString("  }\n")
		sb.WriteString("}\n\n")

		sb.WriteString("to_do -> canceled: \"Cancel\" {\n")
		sb.WriteString("  style.stroke: \"#6b7280\"\n")
		sb.WriteString("  style.stroke-dash: 3\n")
		sb.WriteString("}\n")

		sb.WriteString("in_progress -> canceled: \"Cancel\" {\n")
		sb.WriteString("  style.stroke: \"#6b7280\"\n")
		sb.WriteString("  style.stroke-dash: 3\n")
		sb.WriteString("}\n")
	}

	return sb.String()
}

// GenerateFlowSVG generates a simple SVG flow diagram
func (g *Generator) GenerateFlowSVG() string {
	stats := g.GetFlowStats()

	width := 600
	height := 200

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`, width, height))
	sb.WriteString("\n<style>")
	sb.WriteString(".box { stroke: #2d3748; stroke-width: 1; }")
	sb.WriteString(".arrow { stroke: #4a5568; stroke-width: 2; fill: none; marker-end: url(#arrowhead); }")
	sb.WriteString(".label { fill: #ffffff; font-size: 14px; font-family: sans-serif; text-anchor: middle; }")
	sb.WriteString(".count { fill: #ffffff; font-size: 20px; font-family: sans-serif; font-weight: bold; text-anchor: middle; }")
	sb.WriteString(".title { fill: #eaeaea; font-size: 14px; font-family: sans-serif; font-weight: bold; }")
	sb.WriteString("</style>\n")

	// Arrow marker
	sb.WriteString(`<defs><marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">`)
	sb.WriteString(`<polygon points="0 0, 10 3.5, 0 7" fill="#4a5568"/></marker></defs>`)
	sb.WriteString("\n")

	// Title
	sb.WriteString(fmt.Sprintf(`<text class="title" x="%d" y="25" text-anchor="middle">%s Status Flow</text>`, width/2, g.ProjectName))
	sb.WriteString("\n")

	// Box dimensions
	boxW := 120
	boxH := 80
	y := 80
	gap := 80

	// To Do
	x1 := 50
	sb.WriteString(fmt.Sprintf(`<rect class="box" x="%d" y="%d" width="%d" height="%d" rx="8" fill="#3b82f6"/>`, x1, y, boxW, boxH))
	sb.WriteString(fmt.Sprintf(`<text class="label" x="%d" y="%d">To Do</text>`, x1+boxW/2, y+30))
	sb.WriteString(fmt.Sprintf(`<text class="count" x="%d" y="%d">%d</text>`, x1+boxW/2, y+60, stats.ToDo))
	sb.WriteString("\n")

	// Arrow 1
	sb.WriteString(fmt.Sprintf(`<path class="arrow" d="M %d %d L %d %d"/>`, x1+boxW+5, y+boxH/2, x1+boxW+gap-5, y+boxH/2))
	sb.WriteString("\n")

	// In Progress
	x2 := x1 + boxW + gap
	sb.WriteString(fmt.Sprintf(`<rect class="box" x="%d" y="%d" width="%d" height="%d" rx="8" fill="#f59e0b"/>`, x2, y, boxW, boxH))
	sb.WriteString(fmt.Sprintf(`<text class="label" x="%d" y="%d">In Progress</text>`, x2+boxW/2, y+30))
	sb.WriteString(fmt.Sprintf(`<text class="count" x="%d" y="%d">%d</text>`, x2+boxW/2, y+60, stats.InProgress))
	sb.WriteString("\n")

	// Arrow 2
	sb.WriteString(fmt.Sprintf(`<path class="arrow" d="M %d %d L %d %d"/>`, x2+boxW+5, y+boxH/2, x2+boxW+gap-5, y+boxH/2))
	sb.WriteString("\n")

	// Done
	x3 := x2 + boxW + gap
	sb.WriteString(fmt.Sprintf(`<rect class="box" x="%d" y="%d" width="%d" height="%d" rx="8" fill="#10b981"/>`, x3, y, boxW, boxH))
	sb.WriteString(fmt.Sprintf(`<text class="label" x="%d" y="%d">Done</text>`, x3+boxW/2, y+30))
	sb.WriteString(fmt.Sprintf(`<text class="count" x="%d" y="%d">%d</text>`, x3+boxW/2, y+60, stats.Done))
	sb.WriteString("\n")

	sb.WriteString("</svg>")

	return sb.String()
}

package diagram

import (
	"fmt"
	"strings"
)

// generateBurndown creates a D2 burndown chart diagram
func (g *Generator) generateBurndown() string {
	points := g.GetBurndownData()

	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Burndown Chart: %s\n\n", g.ProjectName))

	// Create the chart container
	sb.WriteString("chart: {\n")
	sb.WriteString("  shape: rectangle\n")
	sb.WriteString(fmt.Sprintf("  label: \"%s Burndown\"\n", g.ProjectName))
	sb.WriteString("  style: {\n")
	sb.WriteString("    fill: \"#1a1a2e\"\n")
	sb.WriteString("  }\n\n")

	// Create data points as nodes
	for i, point := range points {
		nodeID := fmt.Sprintf("p%d", i)
		label := fmt.Sprintf("%s\\n%d remaining", point.Date.Format("Jan 2"), point.Remaining)
		sb.WriteString(fmt.Sprintf("  %s: \"%s\" {\n", nodeID, label))
		sb.WriteString("    style: {\n")
		if point.Remaining == 0 {
			sb.WriteString("      fill: \"#4ade80\"\n") // Green for complete
		} else {
			sb.WriteString("      fill: \"#e94560\"\n") // Accent color
		}
		sb.WriteString("    }\n")
		sb.WriteString("  }\n\n")
	}

	// Connect points with arrows
	for i := 0; i < len(points)-1; i++ {
		sb.WriteString(fmt.Sprintf("  p%d -> p%d\n", i, i+1))
	}

	sb.WriteString("}\n")

	// Legend
	sb.WriteString("\nlegend: {\n")
	total, done, _, remaining := g.Stats()
	sb.WriteString(fmt.Sprintf("  label: \"Total: %d | Done: %d | Remaining: %d\"\n", total, done, remaining))
	sb.WriteString("  style: {\n")
	sb.WriteString("    fill: \"#16213e\"\n")
	sb.WriteString("    font-color: \"#a0a0a0\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")

	return sb.String()
}

// GenerateBurndownSVG generates a simple SVG burndown chart
func (g *Generator) GenerateBurndownSVG() string {
	points := g.GetBurndownData()

	width := 600
	height := 300
	padding := 40

	if len(points) == 0 {
		return fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">
			<text x="%d" y="%d" fill="#a0a0a0" text-anchor="middle">No data available</text>
		</svg>`, width, height, width/2, height/2)
	}

	// Find max Y value
	maxY := 0
	for _, p := range points {
		if p.Remaining > maxY {
			maxY = p.Remaining
		}
	}
	if maxY == 0 {
		maxY = 1
	}

	xScale := float64(width-padding*2) / float64(len(points)-1)
	if len(points) == 1 {
		xScale = 1
	}
	yScale := float64(height-padding*2) / float64(maxY)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`, width, height))
	sb.WriteString("\n<style>")
	sb.WriteString(".axis { stroke: #4a5568; stroke-width: 1; }")
	sb.WriteString(".grid { stroke: #2d3748; stroke-width: 0.5; }")
	sb.WriteString(".line { fill: none; stroke: #e94560; stroke-width: 2; }")
	sb.WriteString(".point { fill: #e94560; }")
	sb.WriteString(".label { fill: #a0a0a0; font-size: 12px; font-family: sans-serif; }")
	sb.WriteString(".title { fill: #eaeaea; font-size: 14px; font-family: sans-serif; font-weight: bold; }")
	sb.WriteString("</style>\n")

	// Title
	sb.WriteString(fmt.Sprintf(`<text class="title" x="%d" y="20" text-anchor="middle">%s Burndown</text>`, width/2, g.ProjectName))
	sb.WriteString("\n")

	// Grid lines
	for i := 0; i <= 4; i++ {
		y := padding + i*(height-padding*2)/4
		sb.WriteString(fmt.Sprintf(`<line class="grid" x1="%d" y1="%d" x2="%d" y2="%d"/>`, padding, y, width-padding, y))
		sb.WriteString("\n")
	}

	// Axes
	sb.WriteString(fmt.Sprintf(`<line class="axis" x1="%d" y1="%d" x2="%d" y2="%d"/>`, padding, padding, padding, height-padding))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf(`<line class="axis" x1="%d" y1="%d" x2="%d" y2="%d"/>`, padding, height-padding, width-padding, height-padding))
	sb.WriteString("\n")

	// Build path data
	var pathParts []string
	for i, p := range points {
		x := float64(padding) + float64(i)*xScale
		y := float64(height-padding) - float64(p.Remaining)*yScale
		if i == 0 {
			pathParts = append(pathParts, fmt.Sprintf("M %.1f %.1f", x, y))
		} else {
			pathParts = append(pathParts, fmt.Sprintf("L %.1f %.1f", x, y))
		}
	}

	// Data line
	sb.WriteString(fmt.Sprintf(`<path class="line" d="%s"/>`, strings.Join(pathParts, " ")))
	sb.WriteString("\n")

	// Data points
	for i, p := range points {
		x := float64(padding) + float64(i)*xScale
		y := float64(height-padding) - float64(p.Remaining)*yScale
		sb.WriteString(fmt.Sprintf(`<circle class="point" cx="%.1f" cy="%.1f" r="4"/>`, x, y))
		sb.WriteString("\n")
	}

	// Axis labels
	sb.WriteString(fmt.Sprintf(`<text class="label" x="%d" y="%d" text-anchor="middle">Time</text>`, width/2, height-10))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf(`<text class="label" x="15" y="%d" transform="rotate(-90, 15, %d)" text-anchor="middle">Items</text>`, height/2, height/2))
	sb.WriteString("\n")

	sb.WriteString("</svg>")

	return sb.String()
}

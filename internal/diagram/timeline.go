package diagram

import (
	"fmt"
	"strings"
	"time"
)

// generateTimeline creates a D2 forecast timeline diagram
func (g *Generator) generateTimeline() string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Forecast Timeline: %s\n\n", g.ProjectName))

	// Title
	sb.WriteString(fmt.Sprintf("title: \"%s Forecast\" {\n", g.ProjectName))
	sb.WriteString("  shape: text\n")
	sb.WriteString("  style.font-size: 24\n")
	sb.WriteString("}\n\n")

	// Today marker
	sb.WriteString("today: {\n")
	sb.WriteString(fmt.Sprintf("  label: \"Today\\n%s\"\n", time.Now().Format("Jan 2")))
	sb.WriteString("  style: {\n")
	sb.WriteString("    fill: \"#3b82f6\"\n")
	sb.WriteString("    font-color: \"#ffffff\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n\n")

	// Percentile markers
	now := time.Now()
	percentileOrder := []int{50, 70, 85, 95}
	colors := map[int]string{
		50: "#22c55e", // Green
		70: "#84cc16", // Lime
		85: "#eab308", // Yellow
		95: "#f97316", // Orange
	}

	prevNode := "today"
	for _, pct := range percentileOrder {
		days, ok := g.Percentiles[pct]
		if !ok {
			continue
		}

		targetDate := now.AddDate(0, 0, int(days))
		nodeID := fmt.Sprintf("p%d", pct)

		sb.WriteString(fmt.Sprintf("%s: {\n", nodeID))
		sb.WriteString(fmt.Sprintf("  label: \"%d%% Confidence\\n%s\\n(+%.0f days)\"\n", pct, targetDate.Format("Jan 2, 2006"), days))
		sb.WriteString("  style: {\n")
		sb.WriteString(fmt.Sprintf("    fill: \"%s\"\n", colors[pct]))
		sb.WriteString("    font-color: \"#ffffff\"\n")
		sb.WriteString("  }\n")
		sb.WriteString("}\n\n")

		// Connect to previous
		sb.WriteString(fmt.Sprintf("%s -> %s\n\n", prevNode, nodeID))
		prevNode = nodeID
	}

	return sb.String()
}

// GenerateTimelineSVG generates a simple SVG timeline diagram
func (g *Generator) GenerateTimelineSVG() string {
	width := 700
	height := 250

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`, width, height))
	sb.WriteString("\n<style>")
	sb.WriteString(".line { stroke: #4a5568; stroke-width: 3; }")
	sb.WriteString(".marker { stroke: #2d3748; stroke-width: 2; }")
	sb.WriteString(".label { fill: #eaeaea; font-size: 12px; font-family: sans-serif; text-anchor: middle; }")
	sb.WriteString(".date { fill: #a0a0a0; font-size: 11px; font-family: sans-serif; text-anchor: middle; }")
	sb.WriteString(".days { fill: #a0a0a0; font-size: 10px; font-family: sans-serif; text-anchor: middle; }")
	sb.WriteString(".title { fill: #eaeaea; font-size: 14px; font-family: sans-serif; font-weight: bold; }")
	sb.WriteString("</style>\n")

	// Title
	sb.WriteString(fmt.Sprintf(`<text class="title" x="%d" y="25" text-anchor="middle">%s Forecast Timeline</text>`, width/2, g.ProjectName))
	sb.WriteString("\n")

	// Timeline line
	lineY := 120
	sb.WriteString(fmt.Sprintf(`<line class="line" x1="50" y1="%d" x2="%d" y2="%d"/>`, lineY, width-50, lineY))
	sb.WriteString("\n")

	// Calculate positions
	now := time.Now()
	percentileOrder := []int{50, 70, 85, 95}
	colors := []string{"#22c55e", "#84cc16", "#eab308", "#f97316"}

	// Find max days for scaling
	maxDays := float64(1)
	for _, pct := range percentileOrder {
		if days, ok := g.Percentiles[pct]; ok && days > maxDays {
			maxDays = days
		}
	}

	padding := 80
	lineWidth := width - padding*2

	// Today marker
	todayX := padding
	sb.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="10" fill="#3b82f6"/>`, todayX, lineY))
	sb.WriteString(fmt.Sprintf(`<text class="label" x="%d" y="%d">Today</text>`, todayX, lineY-20))
	sb.WriteString(fmt.Sprintf(`<text class="date" x="%d" y="%d">%s</text>`, todayX, lineY+30, now.Format("Jan 2")))
	sb.WriteString("\n")

	// Percentile markers
	for i, pct := range percentileOrder {
		days, ok := g.Percentiles[pct]
		if !ok {
			continue
		}

		x := padding + int(days/maxDays*float64(lineWidth))
		targetDate := now.AddDate(0, 0, int(days))

		sb.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="10" fill="%s"/>`, x, lineY, colors[i]))
		sb.WriteString(fmt.Sprintf(`<text class="label" x="%d" y="%d">%d%%</text>`, x, lineY-20, pct))
		sb.WriteString(fmt.Sprintf(`<text class="date" x="%d" y="%d">%s</text>`, x, lineY+30, targetDate.Format("Jan 2")))
		sb.WriteString(fmt.Sprintf(`<text class="days" x="%d" y="%d">+%.0f days</text>`, x, lineY+45, days))
		sb.WriteString("\n")
	}

	// Legend
	legendY := height - 30
	sb.WriteString(fmt.Sprintf(`<text class="date" x="50" y="%d">Confidence levels: 50%% (likely) → 95%% (safe)</text>`, legendY))
	sb.WriteString("\n")

	sb.WriteString("</svg>")

	return sb.String()
}

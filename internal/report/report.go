package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/andrewcostello/forecast/internal/eva"
	"github.com/andrewcostello/forecast/internal/montecarlo"
	"github.com/andrewcostello/forecast/pkg/forecast"
)

// Generator creates project reports
type Generator struct {
	Items        []forecast.Item
	TeamCapacity float64
	ProjectName  string
	StartDate    time.Time
}

// NewGenerator creates a new report generator
func NewGenerator(items []forecast.Item, teamCapacity float64, projectName string, startDate time.Time) *Generator {
	return &Generator{
		Items:        items,
		TeamCapacity: teamCapacity,
		ProjectName:  projectName,
		StartDate:    startDate,
	}
}

// GenerateEVA generates an Earned Value Analysis report
func (g *Generator) GenerateEVA(w io.Writer) error {
	stats := g.calculateStats()

	fmt.Fprintf(w, "# Earned Value Analysis Report\n\n")
	fmt.Fprintf(w, "**Project:** %s  \n", g.ProjectName)
	fmt.Fprintf(w, "**Generated:** %s\n\n", time.Now().Format("January 2, 2006 3:04 PM"))

	fmt.Fprintf(w, "## Project Status\n\n")
	fmt.Fprintf(w, "| Metric | Value |\n")
	fmt.Fprintf(w, "|--------|-------|\n")
	fmt.Fprintf(w, "| Total Items | %d |\n", stats.Total)
	fmt.Fprintf(w, "| Completed | %d (%.1f%%) |\n", stats.Completed, stats.PercentComplete)
	fmt.Fprintf(w, "| In Progress | %d |\n", stats.InProgress)
	fmt.Fprintf(w, "| To Do | %d |\n", stats.ToDo)

	if stats.AvgCycleTime > 0 {
		fmt.Fprintf(w, "| Avg Cycle Time | %.1f hours |\n", stats.AvgCycleTime)
	}

	// Calculate EVA metrics if we have a start date
	if !g.StartDate.IsZero() {
		// Estimate project duration in weeks based on total items and cycle time
		estimatedWeeks := 12 // Default to 12 weeks if we can't calculate
		if stats.AvgCycleTime > 0 && g.TeamCapacity > 0 {
			totalHours := float64(stats.Total) * stats.AvgCycleTime
			totalDays := totalHours / g.TeamCapacity
			estimatedWeeks = int(totalDays / 5) // 5 work days per week
			if estimatedWeeks < 1 {
				estimatedWeeks = 1
			}
		}

		calc := eva.NewCalculator(stats.Total, estimatedWeeks, g.StartDate)
		metrics := calc.Calculate(g.Items)

		fmt.Fprintf(w, "\n## EVA Metrics\n\n")
		fmt.Fprintf(w, "| Metric | Value | Interpretation |\n")
		fmt.Fprintf(w, "|--------|-------|----------------|\n")
		fmt.Fprintf(w, "| Week | %d | Current project week |\n", metrics.Week)
		fmt.Fprintf(w, "| Planned Value (PV) | %d items | Work planned to be done by now |\n", metrics.PV)
		fmt.Fprintf(w, "| Earned Value (EV) | %d items | Work actually completed |\n", metrics.EV)
		fmt.Fprintf(w, "| Schedule Variance (SV) | %d items | %s |\n", metrics.SV, interpretSV(metrics.SV))
		fmt.Fprintf(w, "| Schedule Performance Index (SPI) | %.2f | %s |\n", metrics.SPI, interpretSPI(metrics.SPI))

		if metrics.SPI > 0 {
			forecastWeeks := calc.ForecastCompletion(metrics, stats.Total)
			fmt.Fprintf(w, "| Forecast Completion | Week %d | Based on current SPI |\n", forecastWeeks)
		}
	}

	return nil
}

// GenerateMonteCarlo generates a Monte Carlo simulation report
func (g *Generator) GenerateMonteCarlo(w io.Writer, iterations int) error {
	stats := g.calculateStats()

	if stats.CycleTimeCount == 0 {
		return fmt.Errorf("no completed items with cycle time data - cannot run simulation")
	}

	simulator := montecarlo.NewSimulator(g.Items, g.TeamCapacity)
	result := simulator.Run(iterations)

	throughput := montecarlo.CalculateThroughput(g.Items, 14)

	fmt.Fprintf(w, "# Monte Carlo Forecast Report\n\n")
	fmt.Fprintf(w, "**Project:** %s  \n", g.ProjectName)
	fmt.Fprintf(w, "**Generated:** %s  \n", time.Now().Format("January 2, 2006 3:04 PM"))
	fmt.Fprintf(w, "**Iterations:** %d\n\n", iterations)

	fmt.Fprintf(w, "## Current Status\n\n")
	fmt.Fprintf(w, "| Metric | Value |\n")
	fmt.Fprintf(w, "|--------|-------|\n")
	fmt.Fprintf(w, "| Remaining Items | %d |\n", result.Remaining)
	fmt.Fprintf(w, "| Completed Items | %d |\n", stats.Completed)
	fmt.Fprintf(w, "| Avg Cycle Time | %.1f hours |\n", stats.AvgCycleTime)
	fmt.Fprintf(w, "| Throughput (14d) | %.2f items/day |\n", throughput)

	fmt.Fprintf(w, "\n## Probabilistic Forecast\n\n")
	fmt.Fprintf(w, "| Confidence | Days | Target Date |\n")
	fmt.Fprintf(w, "|------------|------|-------------|\n")

	now := time.Now()
	percentiles := []int{50, 70, 85, 95}
	for _, p := range percentiles {
		days := result.Percentiles[p]
		targetDate := now.AddDate(0, 0, int(days))
		fmt.Fprintf(w, "| %d%% | %.0f | %s |\n", p, days, targetDate.Format("Jan 2, 2006"))
	}

	fmt.Fprintf(w, "\n### Interpretation\n\n")
	fmt.Fprintf(w, "- **50%%**: Even odds of completing by this date\n")
	fmt.Fprintf(w, "- **70%%**: Reasonable confidence for planning\n")
	fmt.Fprintf(w, "- **85%%**: High confidence, recommended for commitments\n")
	fmt.Fprintf(w, "- **95%%**: Very high confidence, accounts for most risks\n")

	return nil
}

// GenerateFull generates a comprehensive report with all metrics
func (g *Generator) GenerateFull(w io.Writer, iterations int) error {
	stats := g.calculateStats()

	fmt.Fprintf(w, "# Project Forecast Report\n\n")
	fmt.Fprintf(w, "**Project:** %s  \n", g.ProjectName)
	fmt.Fprintf(w, "**Generated:** %s\n\n", time.Now().Format("January 2, 2006 3:04 PM"))

	fmt.Fprintf(w, "---\n\n")

	// Project overview
	fmt.Fprintf(w, "## Project Overview\n\n")
	fmt.Fprintf(w, "| Metric | Value |\n")
	fmt.Fprintf(w, "|--------|-------|\n")
	fmt.Fprintf(w, "| Total Items | %d |\n", stats.Total)
	fmt.Fprintf(w, "| Completed | %d (%.1f%%) |\n", stats.Completed, stats.PercentComplete)
	fmt.Fprintf(w, "| In Progress | %d |\n", stats.InProgress)
	fmt.Fprintf(w, "| Remaining | %d |\n", stats.ToDo+stats.InProgress)

	// Cycle time analysis
	if stats.AvgCycleTime > 0 {
		fmt.Fprintf(w, "\n## Cycle Time Analysis\n\n")
		fmt.Fprintf(w, "| Metric | Value |\n")
		fmt.Fprintf(w, "|--------|-------|\n")
		fmt.Fprintf(w, "| Average | %.1f hours |\n", stats.AvgCycleTime)
		fmt.Fprintf(w, "| Sample Size | %d items |\n", stats.CycleTimeCount)

		// Breakdown by size
		sizeStats := g.calculateSizeStats()
		if len(sizeStats) > 0 {
			fmt.Fprintf(w, "\n### By Size\n\n")
			fmt.Fprintf(w, "| Size | Count | Avg Hours |\n")
			fmt.Fprintf(w, "|------|-------|----------|\n")

			sizes := []string{"S", "M", "L", "XL"}
			for _, size := range sizes {
				if s, ok := sizeStats[size]; ok {
					fmt.Fprintf(w, "| %s | %d | %.1f |\n", size, s.Count, s.AvgHours)
				}
			}
		}
	}

	// Monte Carlo forecast
	if stats.CycleTimeCount > 0 {
		fmt.Fprintf(w, "\n## Monte Carlo Forecast\n\n")

		simulator := montecarlo.NewSimulator(g.Items, g.TeamCapacity)
		result := simulator.Run(iterations)

		fmt.Fprintf(w, "_Based on %d simulation iterations_\n\n", iterations)
		fmt.Fprintf(w, "| Confidence | Days | Target Date |\n")
		fmt.Fprintf(w, "|------------|------|-------------|\n")

		now := time.Now()
		percentiles := []int{50, 70, 85, 95}
		for _, p := range percentiles {
			days := result.Percentiles[p]
			targetDate := now.AddDate(0, 0, int(days))
			fmt.Fprintf(w, "| %d%% | %.0f | %s |\n", p, days, targetDate.Format("Jan 2, 2006"))
		}
	}

	// Item breakdown by type
	typeStats := g.calculateTypeStats()
	if len(typeStats) > 0 {
		fmt.Fprintf(w, "\n## Item Breakdown by Type\n\n")
		fmt.Fprintf(w, "| Type | Total | Done | Remaining |\n")
		fmt.Fprintf(w, "|------|-------|------|----------|\n")

		// Sort types for consistent output
		types := make([]string, 0, len(typeStats))
		for t := range typeStats {
			types = append(types, t)
		}
		sort.Strings(types)

		for _, t := range types {
			s := typeStats[t]
			fmt.Fprintf(w, "| %s | %d | %d | %d |\n", t, s.Total, s.Done, s.Remaining)
		}
	}

	return nil
}

// Stats holds calculated statistics
type Stats struct {
	Total           int
	Completed       int
	InProgress      int
	ToDo            int
	PercentComplete float64
	AvgCycleTime    float64
	CycleTimeCount  int
}

func (g *Generator) calculateStats() Stats {
	var stats Stats
	var totalCycleTime float64

	for _, item := range g.Items {
		switch item.Status {
		case "Done":
			stats.Total++
			stats.Completed++
			if item.CycleTime > 0 {
				totalCycleTime += item.CycleTime
				stats.CycleTimeCount++
			}
		case "Canceled":
			// Exclude canceled items from all counts
			continue
		case "In Progress":
			stats.Total++
			stats.InProgress++
		default:
			stats.Total++
			stats.ToDo++
		}
	}

	if stats.Total > 0 {
		stats.PercentComplete = float64(stats.Completed) / float64(stats.Total) * 100
	}

	if stats.CycleTimeCount > 0 {
		stats.AvgCycleTime = totalCycleTime / float64(stats.CycleTimeCount)
	}

	return stats
}

type SizeStats struct {
	Count    int
	AvgHours float64
}

func (g *Generator) calculateSizeStats() map[string]SizeStats {
	sizeData := make(map[string]struct {
		total float64
		count int
	})

	for _, item := range g.Items {
		if item.Status == "Done" && item.CycleTime > 0 && item.Size != "" {
			data := sizeData[item.Size]
			data.total += item.CycleTime
			data.count++
			sizeData[item.Size] = data
		}
	}

	result := make(map[string]SizeStats)
	for size, data := range sizeData {
		result[size] = SizeStats{
			Count:    data.count,
			AvgHours: data.total / float64(data.count),
		}
	}

	return result
}

type TypeStats struct {
	Total     int
	Done      int
	Remaining int
}

func (g *Generator) calculateTypeStats() map[string]TypeStats {
	result := make(map[string]TypeStats)

	for _, item := range g.Items {
		// Exclude canceled items
		if item.Status == "Canceled" {
			continue
		}

		itemType := item.Type
		if itemType == "" {
			itemType = "Untyped"
		}

		stats := result[itemType]
		stats.Total++
		if item.Status == "Done" {
			stats.Done++
		} else {
			stats.Remaining++
		}
		result[itemType] = stats
	}

	return result
}

func interpretSV(sv int) string {
	if sv > 0 {
		return "Ahead of schedule"
	} else if sv < 0 {
		return "Behind schedule"
	}
	return "On schedule"
}

func interpretSPI(spi float64) string {
	if spi >= 1.0 {
		return "On or ahead of schedule"
	} else if spi >= 0.9 {
		return "Slightly behind schedule"
	} else if spi >= 0.8 {
		return "Behind schedule - attention needed"
	}
	return "Significantly behind schedule"
}

// FormatTable formats data as an ASCII table (for terminal output)
func FormatTable(headers []string, rows [][]string) string {
	if len(headers) == 0 || len(rows) == 0 {
		return ""
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder

	// Header
	for i, h := range headers {
		fmt.Fprintf(&sb, "%-*s", widths[i]+2, h)
	}
	sb.WriteString("\n")

	// Separator
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
	}
	sb.WriteString("\n")

	// Rows
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				fmt.Fprintf(&sb, "%-*s", widths[i]+2, cell)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

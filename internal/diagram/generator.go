// Package diagram provides D2 diagram generation for project visualizations.
package diagram

import (
	"time"

	"github.com/andrewcostello/forecast/pkg/forecast"
)

// DiagramType represents the type of diagram to generate
type DiagramType string

const (
	TypeBurndown DiagramType = "burndown"
	TypeFlow     DiagramType = "flow"
	TypeTimeline DiagramType = "timeline"
)

// Format represents the output format for diagrams
type Format string

const (
	FormatD2  Format = "d2"
	FormatSVG Format = "svg"
	FormatPNG Format = "png"
)

// Generator generates D2 diagrams from forecast data
type Generator struct {
	Items       []forecast.Item
	ProjectName string
	ProjectKey  string
	Percentiles map[int]float64 // Confidence percentile -> days
}

// NewGenerator creates a new diagram generator
func NewGenerator(items []forecast.Item, projectName, projectKey string) *Generator {
	return &Generator{
		Items:       items,
		ProjectName: projectName,
		ProjectKey:  projectKey,
		Percentiles: make(map[int]float64),
	}
}

// SetPercentiles sets the forecast percentiles for timeline diagrams
func (g *Generator) SetPercentiles(percentiles map[int]float64) {
	g.Percentiles = percentiles
}

// Generate creates D2 diagram code for the specified type
func (g *Generator) Generate(diagramType DiagramType) string {
	switch diagramType {
	case TypeBurndown:
		return g.generateBurndown()
	case TypeFlow:
		return g.generateFlow()
	case TypeTimeline:
		return g.generateTimeline()
	default:
		return g.generateBurndown()
	}
}

// Stats calculates item statistics
func (g *Generator) Stats() (total, done, inProgress, remaining int) {
	for _, item := range g.Items {
		switch item.Status {
		case "Done":
			total++
			done++
		case "Canceled":
			continue
		case "In Progress", "In Development":
			total++
			inProgress++
		default:
			total++
			remaining++
		}
	}
	return
}

// BurndownPoint represents a point on the burndown chart
type BurndownPoint struct {
	Date      time.Time
	Remaining int
	Completed int
}

// GetBurndownData generates burndown chart data points
func (g *Generator) GetBurndownData() []BurndownPoint {
	// Sort items by completion date
	type completedItem struct {
		date time.Time
	}

	var completions []completedItem
	for _, item := range g.Items {
		if item.Status == "Done" && item.Done != nil && !item.Done.IsZero() {
			completions = append(completions, completedItem{date: *item.Done})
		}
	}

	// Calculate total (excluding canceled)
	total, _, _, _ := g.Stats()

	if len(completions) == 0 {
		// No completions yet, return starting point
		return []BurndownPoint{
			{Date: time.Now(), Remaining: total, Completed: 0},
		}
	}

	// Sort by date
	for i := 0; i < len(completions)-1; i++ {
		for j := i + 1; j < len(completions); j++ {
			if completions[j].date.Before(completions[i].date) {
				completions[i], completions[j] = completions[j], completions[i]
			}
		}
	}

	// Generate points
	points := make([]BurndownPoint, 0, len(completions)+1)

	// Starting point
	startDate := completions[0].date.AddDate(0, 0, -1)
	points = append(points, BurndownPoint{
		Date:      startDate,
		Remaining: total,
		Completed: 0,
	})

	// Add completion points
	completed := 0
	for _, c := range completions {
		completed++
		points = append(points, BurndownPoint{
			Date:      c.date,
			Remaining: total - completed,
			Completed: completed,
		})
	}

	return points
}

// FlowStats represents flow diagram statistics
type FlowStats struct {
	ToDo       int
	InProgress int
	Done       int
	Canceled   int
}

// GetFlowStats calculates flow statistics
func (g *Generator) GetFlowStats() FlowStats {
	var stats FlowStats
	for _, item := range g.Items {
		switch item.Status {
		case "Done":
			stats.Done++
		case "Canceled":
			stats.Canceled++
		case "In Progress", "In Development":
			stats.InProgress++
		default:
			stats.ToDo++
		}
	}
	return stats
}

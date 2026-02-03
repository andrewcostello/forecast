package web

import "time"

// HealthResponse represents the health check API response
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// ProjectSummary represents basic project info for listing
type ProjectSummary struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	Epic     string  `json:"epic"`
	Total    int     `json:"total"`
	Done     int     `json:"done"`
	Progress float64 `json:"progress"`
}

// ProjectsResponse is returned by GET /api/projects
type ProjectsResponse struct {
	Projects []ProjectSummary `json:"projects"`
}

// ProjectDetail represents detailed project information
type ProjectDetail struct {
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	Epic       string  `json:"epic"`
	Total      int     `json:"total"`
	Done       int     `json:"done"`
	InProgress int     `json:"inProgress"`
	Remaining  int     `json:"remaining"`
	Progress   float64 `json:"progress"`
	AvgCycle   float64 `json:"avgCycleHours,omitempty"`
	Capacity   float64 `json:"capacity,omitempty"`
}

// ForecastResult represents Monte Carlo forecast results
type ForecastResult struct {
	ProjectKey  string             `json:"projectKey"`
	ProjectName string             `json:"projectName"`
	Remaining   int                `json:"remaining"`
	Completed   int                `json:"completed"`
	AvgCycle    float64            `json:"avgCycleHours"`
	Throughput  float64            `json:"throughputPerDay"`
	Percentiles map[int]Percentile `json:"percentiles"`
	GeneratedAt time.Time          `json:"generatedAt"`
}

// Percentile represents a single confidence level forecast
type Percentile struct {
	Days       float64   `json:"days"`
	TargetDate time.Time `json:"targetDate"`
}

// ReportResponse represents the full report data
type ReportResponse struct {
	ProjectKey  string       `json:"projectKey"`
	ProjectName string       `json:"projectName"`
	EVA         *EVAMetrics  `json:"eva,omitempty"`
	Forecast    *ForecastResult `json:"forecast,omitempty"`
}

// EVAMetrics represents Earned Value Analysis metrics
type EVAMetrics struct {
	PlannedValue float64 `json:"plannedValue"`
	EarnedValue  float64 `json:"earnedValue"`
	ActualCost   float64 `json:"actualCost"`
	SPI          float64 `json:"spi"`
	CPI          float64 `json:"cpi"`
	EAC          float64 `json:"eac"`
	VAC          float64 `json:"vac"`
}

// DiagramRequest represents a diagram generation request
type DiagramRequest struct {
	Type   string `json:"type"` // burndown, flow, timeline
	Format string `json:"format"` // d2, svg, png
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code"`
}

package forecast

import "time"

// Item represents a single countable work item
type Item struct {
	ID          string
	Type        string // Component, Fix, Migration, etc.
	Description string
	Size        string // S, M, L, XL
	Status      string // To Do, In Progress, Done
	Created     time.Time
	InProgress  *time.Time
	Done        *time.Time
	CycleTime   float64 // Hours from InProgress → Done
	Assignee    string
	JiraKey     string
}

// Distribution represents a probability distribution
type Distribution struct {
	Mean   float64
	StdDev float64
}

// EVAMetrics represents Earned Value Analysis metrics
type EVAMetrics struct {
	Week int
	PV   int     // Planned Value (items)
	EV   int     // Earned Value (items)
	AC   float64 // Actual Cost (hours)
	SV   int     // Schedule Variance
	CV   float64 // Cost Variance
	SPI  float64 // Schedule Performance Index
	CPI  float64 // Cost Performance Index
}

// ForecastResult represents a Monte Carlo simulation result
type ForecastResult struct {
	Remaining        int
	Completed        int
	ThroughputPerDay float64
	AvgCycleTime     float64
	Percentiles      map[int]float64 // 50, 70, 85, 95
}

// ReferenceClass represents historical data for similar projects
type ReferenceClass struct {
	ID          int64
	Name        string
	Type        string // "React Refactoring", "API Migration", etc.
	CompletedAt time.Time
	TeamSize    int
	Items       []ReferenceClassItem
}

// ReferenceClassItem represents a historical work item
type ReferenceClassItem struct {
	ProjectID     int64
	ItemType      string
	Size          string
	CycleTimeHrs  float64
}

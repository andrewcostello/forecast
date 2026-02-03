package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/diagram"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/andrewcostello/forecast/internal/montecarlo"
	"github.com/andrewcostello/forecast/internal/storage"
)

// Handlers holds dependencies for HTTP handlers
type Handlers struct {
	config *config.Config
	store  *storage.JSONStorage
}

// NewHandlers creates a new Handlers instance
func NewHandlers(cfg *config.Config, store *storage.JSONStorage) *Handlers {
	return &Handlers{
		config: cfg,
		store:  store,
	}
}

// Health handles GET /api/health
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  "ok",
		Version: "1.0.0",
	}
	writeJSON(w, http.StatusOK, resp)
}

// ListProjects handles GET /api/projects
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects := h.config.GetAllProjects()
	if len(projects) == 0 {
		writeJSON(w, http.StatusOK, ProjectsResponse{Projects: []ProjectSummary{}})
		return
	}

	jiraClient := jira.NewClient(&h.config.JIRA)
	summaries := make([]ProjectSummary, 0, len(projects))

	for _, proj := range projects {
		stats, err := h.getProjectStats(jiraClient, &proj)
		if err != nil {
			continue
		}

		progress := float64(0)
		if stats.Total > 0 {
			progress = float64(stats.Done) / float64(stats.Total) * 100
		}

		summaries = append(summaries, ProjectSummary{
			Key:      proj.Key,
			Name:     proj.Name,
			Epic:     proj.Epic,
			Total:    stats.Total,
			Done:     stats.Done,
			Progress: progress,
		})
	}

	writeJSON(w, http.StatusOK, ProjectsResponse{Projects: summaries})
}

// GetProject handles GET /api/projects/{key}
func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	key := extractPathParam(r.URL.Path, "/api/projects/")
	if key == "" {
		writeError(w, http.StatusBadRequest, "project key required")
		return
	}

	// Handle sub-paths like /forecast, /report, /diagram
	if strings.Contains(key, "/") {
		parts := strings.SplitN(key, "/", 2)
		key = parts[0]
		subPath := parts[1]

		switch subPath {
		case "forecast":
			h.GetProjectForecast(w, r, key)
			return
		case "report":
			h.GetProjectReport(w, r, key)
			return
		case "diagram":
			h.GetProjectDiagram(w, r, key)
			return
		}
	}

	proj := h.config.GetProject(key)
	if proj == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("project '%s' not found", key))
		return
	}

	jiraClient := jira.NewClient(&h.config.JIRA)
	stats, err := h.getProjectStats(jiraClient, proj)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	progress := float64(0)
	if stats.Total > 0 {
		progress = float64(stats.Done) / float64(stats.Total) * 100
	}

	capacity := h.config.TeamCapacity
	if proj.Capacity > 0 {
		capacity = proj.Capacity
	}

	detail := ProjectDetail{
		Key:        proj.Key,
		Name:       proj.Name,
		Epic:       proj.Epic,
		Total:      stats.Total,
		Done:       stats.Done,
		InProgress: stats.InProgress,
		Remaining:  stats.Remaining,
		Progress:   progress,
		AvgCycle:   stats.AvgCycle,
		Capacity:   capacity,
	}

	writeJSON(w, http.StatusOK, detail)
}

// GetProjectForecast handles GET /api/projects/{key}/forecast
func (h *Handlers) GetProjectForecast(w http.ResponseWriter, r *http.Request, key string) {
	proj := h.config.GetProject(key)
	if proj == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("project '%s' not found", key))
		return
	}

	projectKey := proj.Key
	if projectKey == "" {
		projectKey = proj.Epic
	}

	items, err := h.store.LoadProject(projectKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(items) == 0 {
		writeError(w, http.StatusNotFound, "no items found - run 'forecast sync' first")
		return
	}

	// Count items
	var completed, remaining, cycleTimeCount int
	for _, item := range items {
		if item.Status == "Done" {
			completed++
			if item.CycleTime > 0 {
				cycleTimeCount++
			}
		} else {
			remaining++
		}
	}

	if cycleTimeCount == 0 {
		writeError(w, http.StatusBadRequest, "no completed items with cycle time data - cannot forecast")
		return
	}

	teamCapacity := h.config.TeamCapacity
	if proj.Capacity > 0 {
		teamCapacity = proj.Capacity
	}
	if teamCapacity <= 0 {
		teamCapacity = 8.0
	}

	simulator := montecarlo.NewSimulator(items, teamCapacity)
	result := simulator.Run(10000)

	avgCycleTime := montecarlo.CalculateAvgCycleTime(items)
	throughput := montecarlo.CalculateThroughput(items, 14)

	now := time.Now()
	percentiles := make(map[int]Percentile)
	for _, p := range []int{50, 70, 85, 95} {
		days := result.Percentiles[p]
		percentiles[p] = Percentile{
			Days:       days,
			TargetDate: now.AddDate(0, 0, int(days)),
		}
	}

	resp := ForecastResult{
		ProjectKey:  key,
		ProjectName: proj.Name,
		Remaining:   remaining,
		Completed:   completed,
		AvgCycle:    avgCycleTime,
		Throughput:  throughput,
		Percentiles: percentiles,
		GeneratedAt: now,
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetProjectReport handles GET /api/projects/{key}/report
func (h *Handlers) GetProjectReport(w http.ResponseWriter, r *http.Request, key string) {
	proj := h.config.GetProject(key)
	if proj == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("project '%s' not found", key))
		return
	}

	// Get forecast data
	forecastResp := ForecastResult{
		ProjectKey:  key,
		ProjectName: proj.Name,
	}

	projectKey := proj.Key
	if projectKey == "" {
		projectKey = proj.Epic
	}

	items, err := h.store.LoadProject(projectKey)
	if err == nil && len(items) > 0 {
		teamCapacity := h.config.TeamCapacity
		if proj.Capacity > 0 {
			teamCapacity = proj.Capacity
		}
		if teamCapacity <= 0 {
			teamCapacity = 8.0
		}

		simulator := montecarlo.NewSimulator(items, teamCapacity)
		result := simulator.Run(10000)

		var completed, remaining int
		for _, item := range items {
			if item.Status == "Done" {
				completed++
			} else {
				remaining++
			}
		}

		now := time.Now()
		percentiles := make(map[int]Percentile)
		for _, p := range []int{50, 70, 85, 95} {
			days := result.Percentiles[p]
			percentiles[p] = Percentile{
				Days:       days,
				TargetDate: now.AddDate(0, 0, int(days)),
			}
		}

		forecastResp.Remaining = remaining
		forecastResp.Completed = completed
		forecastResp.AvgCycle = montecarlo.CalculateAvgCycleTime(items)
		forecastResp.Throughput = montecarlo.CalculateThroughput(items, 14)
		forecastResp.Percentiles = percentiles
		forecastResp.GeneratedAt = now
	}

	resp := ReportResponse{
		ProjectKey:  key,
		ProjectName: proj.Name,
		Forecast:    &forecastResp,
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetProjectDiagram handles GET /api/projects/{key}/diagram
func (h *Handlers) GetProjectDiagram(w http.ResponseWriter, r *http.Request, key string) {
	proj := h.config.GetProject(key)
	if proj == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("project '%s' not found", key))
		return
	}

	diagramTypeStr := r.URL.Query().Get("type")
	if diagramTypeStr == "" {
		diagramTypeStr = "burndown"
	}

	projectKey := proj.Key
	if projectKey == "" {
		projectKey = proj.Epic
	}

	items, err := h.store.LoadProject(projectKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(items) == 0 {
		writeError(w, http.StatusNotFound, "no items found - run 'forecast sync' first")
		return
	}

	// Create generator
	gen := diagram.NewGenerator(items, proj.Name, proj.Key)

	// Determine diagram type
	var dt diagram.DiagramType
	switch strings.ToLower(diagramTypeStr) {
	case "burndown":
		dt = diagram.TypeBurndown
	case "flow":
		dt = diagram.TypeFlow
	case "timeline":
		dt = diagram.TypeTimeline
		// Add percentiles for timeline
		teamCapacity := h.config.TeamCapacity
		if proj.Capacity > 0 {
			teamCapacity = proj.Capacity
		}
		if teamCapacity <= 0 {
			teamCapacity = 8.0
		}

		// Check for cycle time data
		hasCycleTime := false
		for _, item := range items {
			if item.Status == "Done" && item.CycleTime > 0 {
				hasCycleTime = true
				break
			}
		}

		if hasCycleTime {
			simulator := montecarlo.NewSimulator(items, teamCapacity)
			result := simulator.Run(10000)
			gen.SetPercentiles(result.Percentiles)
		}
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown diagram type: %s", diagramTypeStr))
		return
	}

	// Render diagram
	renderer := diagram.NewRenderer()
	svg, contentType, err := gen.RenderWithFallback(dt, renderer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(svg)
}

// projectStats holds computed project statistics
type projectStats struct {
	Total      int
	Done       int
	InProgress int
	Remaining  int
	AvgCycle   float64
}

func (h *Handlers) getProjectStats(client *jira.Client, proj *config.ProjectConfig) (*projectStats, error) {
	jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s`, h.config.JIRA.ProjectKey, proj.Epic)

	issues, err := client.SearchJQL(jql)
	if err != nil {
		// Try parent field for next-gen projects
		jql = fmt.Sprintf(`project = %s AND parent = %s`, h.config.JIRA.ProjectKey, proj.Epic)
		issues, err = client.SearchJQL(jql)
		if err != nil {
			return nil, err
		}
	}

	stats := &projectStats{}
	var totalCycleTime float64
	var cycleTimeCount int

	items := client.ConvertIssuesToItems(issues, h.config)

	for _, item := range items {
		stats.Total++
		switch item.Status {
		case "Done":
			stats.Done++
			if item.CycleTime > 0 {
				totalCycleTime += item.CycleTime
				cycleTimeCount++
			}
		case "In Progress", "In Development":
			stats.InProgress++
		default:
			stats.Remaining++
		}
	}

	if cycleTimeCount > 0 {
		stats.AvgCycle = totalCycleTime / float64(cycleTimeCount)
	}

	return stats, nil
}

// Helper functions

func extractPathParam(path, prefix string) string {
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	resp := ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
		Code:    status,
	}
	writeJSON(w, status, resp)
}

// Handler wrappers for compatibility with http.HandlerFunc

func (h *Handlers) HealthHandler() http.HandlerFunc {
	return h.Health
}

func (h *Handlers) ListProjectsHandler() http.HandlerFunc {
	return h.ListProjects
}

func (h *Handlers) GetProjectHandler() http.HandlerFunc {
	return h.GetProject
}

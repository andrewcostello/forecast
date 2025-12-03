package referenceclass

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/andrewcostello/forecast/pkg/forecast"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    completed_at DATE NOT NULL,
    team_size INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    item_type TEXT NOT NULL,
    size TEXT NOT NULL,
    cycle_time_hours REAL NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE INDEX IF NOT EXISTS idx_items_project_type
    ON items(project_id, item_type, size);

CREATE INDEX IF NOT EXISTS idx_projects_type
    ON projects(type);
`

// Database manages the reference class database
type Database struct {
	db *sql.DB
}

// NewDatabase creates or opens the reference class database
func NewDatabase() (*Database, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dbDir := filepath.Join(homeDir, ".forecast")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dbDir, "reference_classes.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Create schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return &Database{db: db}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// AddProject adds a project to the reference database
func (d *Database) AddProject(rc *forecast.ReferenceClass) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert project
	result, err := tx.Exec(`
		INSERT INTO projects (name, type, completed_at, team_size)
		VALUES (?, ?, ?, ?)
	`, rc.Name, rc.Type, rc.CompletedAt, rc.TeamSize)
	if err != nil {
		return err
	}

	projectID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	// Insert items
	stmt, err := tx.Prepare(`
		INSERT INTO items (project_id, item_type, size, cycle_time_hours)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range rc.Items {
		if _, err := stmt.Exec(projectID, item.ItemType, item.Size, item.CycleTimeHrs); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetDistribution gets cycle time distribution for a reference class
func (d *Database) GetDistribution(projectType, itemType, size string) (*forecast.Distribution, error) {
	// Get all cycle times for this combination
	query := `
		SELECT cycle_time_hours
		FROM items
		WHERE project_id IN (
		    SELECT id FROM projects WHERE type = ?
		)
		AND item_type = ?
		AND size = ?
	`

	rows, err := d.db.Query(query, projectType, itemType, size)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cycleTimes []float64
	for rows.Next() {
		var ct float64
		if err := rows.Scan(&ct); err != nil {
			return nil, err
		}
		cycleTimes = append(cycleTimes, ct)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(cycleTimes) == 0 {
		return nil, fmt.Errorf("no reference data for %s/%s/%s", projectType, itemType, size)
	}

	// Calculate mean
	var sum float64
	for _, ct := range cycleTimes {
		sum += ct
	}
	mean := sum / float64(len(cycleTimes))

	// Calculate standard deviation
	var varianceSum float64
	for _, ct := range cycleTimes {
		diff := ct - mean
		varianceSum += diff * diff
	}
	stdDev := math.Sqrt(varianceSum / float64(len(cycleTimes)))

	return &forecast.Distribution{
		Mean:   mean,
		StdDev: stdDev,
	}, nil
}

// ListReferenceClasses lists all available reference classes
func (d *Database) ListReferenceClasses() ([]ReferenceClassSummary, error) {
	// First get the basic aggregates
	query := `
		SELECT
		    p.type,
		    COUNT(DISTINCT p.id) as project_count,
		    COUNT(i.id) as item_count,
		    COALESCE(AVG(i.cycle_time_hours), 0) as avg_hours
		FROM projects p
		LEFT JOIN items i ON i.project_id = p.id
		GROUP BY p.type
		ORDER BY project_count DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []ReferenceClassSummary
	for rows.Next() {
		var s ReferenceClassSummary

		if err := rows.Scan(&s.Type, &s.ProjectCount, &s.ItemCount, &s.AvgHours); err != nil {
			return nil, err
		}

		summaries = append(summaries, s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Calculate stddev for each type
	for i := range summaries {
		stdDev, err := d.calculateStdDevForType(summaries[i].Type, summaries[i].AvgHours)
		if err == nil {
			summaries[i].StdDev = stdDev
		}
	}

	return summaries, nil
}

// calculateStdDevForType calculates the standard deviation for all items of a project type
func (d *Database) calculateStdDevForType(projectType string, mean float64) (float64, error) {
	query := `
		SELECT i.cycle_time_hours
		FROM items i
		JOIN projects p ON i.project_id = p.id
		WHERE p.type = ?
	`

	rows, err := d.db.Query(query, projectType)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var cycleTimes []float64
	for rows.Next() {
		var ct float64
		if err := rows.Scan(&ct); err != nil {
			return 0, err
		}
		cycleTimes = append(cycleTimes, ct)
	}

	if len(cycleTimes) == 0 {
		return 0, nil
	}

	var varianceSum float64
	for _, ct := range cycleTimes {
		diff := ct - mean
		varianceSum += diff * diff
	}

	return math.Sqrt(varianceSum / float64(len(cycleTimes))), nil
}

// ReferenceClassSummary summarizes a reference class
type ReferenceClassSummary struct {
	Type         string
	ProjectCount int
	ItemCount    int
	AvgHours     float64
	StdDev       float64
}

// CreateFromItems creates a reference class from completed items
func CreateFromItems(name, projectType string, teamSize int, items []forecast.Item) *forecast.ReferenceClass {
	rcItems := make([]forecast.ReferenceClassItem, 0)

	for _, item := range items {
		if item.Status == "Done" && item.CycleTime > 0 {
			rcItems = append(rcItems, forecast.ReferenceClassItem{
				ItemType:     item.Type,
				Size:         item.Size,
				CycleTimeHrs: item.CycleTime,
			})
		}
	}

	return &forecast.ReferenceClass{
		Name:        name,
		Type:        projectType,
		CompletedAt: time.Now(),
		TeamSize:    teamSize,
		Items:       rcItems,
	}
}

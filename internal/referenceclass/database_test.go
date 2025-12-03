package referenceclass

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andrewcostello/forecast/pkg/forecast"
)

func TestNewDatabase(t *testing.T) {
	// Use temp directory for test database
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Verify database file was created
	dbPath := filepath.Join(tempDir, ".forecast", "reference_classes.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestAddAndListProjects(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Add a project
	rc := &forecast.ReferenceClass{
		Name:        "Test Project",
		Type:        "Web Application",
		CompletedAt: time.Now(),
		TeamSize:    3,
		Items: []forecast.ReferenceClassItem{
			{ItemType: "Component", Size: "M", CycleTimeHrs: 4.0},
			{ItemType: "Component", Size: "L", CycleTimeHrs: 8.0},
			{ItemType: "Fix", Size: "S", CycleTimeHrs: 2.0},
		},
	}

	err = db.AddProject(rc)
	if err != nil {
		t.Fatalf("failed to add project: %v", err)
	}

	// List reference classes
	summaries, err := db.ListReferenceClasses()
	if err != nil {
		t.Fatalf("failed to list reference classes: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("expected 1 reference class, got %d", len(summaries))
	}

	if summaries[0].Type != "Web Application" {
		t.Errorf("expected type 'Web Application', got %s", summaries[0].Type)
	}
	if summaries[0].ProjectCount != 1 {
		t.Errorf("expected 1 project, got %d", summaries[0].ProjectCount)
	}
	if summaries[0].ItemCount != 3 {
		t.Errorf("expected 3 items, got %d", summaries[0].ItemCount)
	}
}

func TestAddMultipleProjectsSameType(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Add two projects of the same type
	rc1 := &forecast.ReferenceClass{
		Name:        "Project 1",
		Type:        "API Migration",
		CompletedAt: time.Now(),
		TeamSize:    2,
		Items: []forecast.ReferenceClassItem{
			{ItemType: "Migration", Size: "M", CycleTimeHrs: 5.0},
		},
	}

	rc2 := &forecast.ReferenceClass{
		Name:        "Project 2",
		Type:        "API Migration",
		CompletedAt: time.Now(),
		TeamSize:    3,
		Items: []forecast.ReferenceClassItem{
			{ItemType: "Migration", Size: "M", CycleTimeHrs: 6.0},
			{ItemType: "Migration", Size: "L", CycleTimeHrs: 10.0},
		},
	}

	if err := db.AddProject(rc1); err != nil {
		t.Fatalf("failed to add project 1: %v", err)
	}
	if err := db.AddProject(rc2); err != nil {
		t.Fatalf("failed to add project 2: %v", err)
	}

	summaries, err := db.ListReferenceClasses()
	if err != nil {
		t.Fatalf("failed to list reference classes: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("expected 1 reference class type, got %d", len(summaries))
	}

	if summaries[0].ProjectCount != 2 {
		t.Errorf("expected 2 projects, got %d", summaries[0].ProjectCount)
	}
	if summaries[0].ItemCount != 3 {
		t.Errorf("expected 3 items, got %d", summaries[0].ItemCount)
	}
}

func TestGetDistribution(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Add project with known cycle times
	rc := &forecast.ReferenceClass{
		Name:        "Test Project",
		Type:        "Web Application",
		CompletedAt: time.Now(),
		TeamSize:    3,
		Items: []forecast.ReferenceClassItem{
			{ItemType: "Component", Size: "M", CycleTimeHrs: 4.0},
			{ItemType: "Component", Size: "M", CycleTimeHrs: 6.0},
			{ItemType: "Component", Size: "M", CycleTimeHrs: 5.0},
		},
	}

	if err := db.AddProject(rc); err != nil {
		t.Fatalf("failed to add project: %v", err)
	}

	// Get distribution
	dist, err := db.GetDistribution("Web Application", "Component", "M")
	if err != nil {
		t.Fatalf("failed to get distribution: %v", err)
	}

	// Mean should be (4+6+5)/3 = 5.0
	if dist.Mean != 5.0 {
		t.Errorf("expected mean 5.0, got %f", dist.Mean)
	}

	// StdDev should be calculated correctly
	if dist.StdDev < 0 {
		t.Errorf("expected non-negative stddev, got %f", dist.StdDev)
	}
}

func TestGetDistributionNotFound(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Try to get distribution for non-existent data
	_, err = db.GetDistribution("NonExistent", "Component", "M")
	if err == nil {
		t.Error("expected error for non-existent distribution")
	}
}

func TestCreateFromItems(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Type: "Component", Size: "M", Status: "Done", CycleTime: 4.0},
		{ID: "2", Type: "Component", Size: "L", Status: "Done", CycleTime: 8.0},
		{ID: "3", Type: "Fix", Size: "S", Status: "Done", CycleTime: 2.0},
		{ID: "4", Type: "Component", Size: "M", Status: "To Do", CycleTime: 0}, // Not completed
	}

	rc := CreateFromItems("Test Project", "Web App", 3, items)

	if rc.Name != "Test Project" {
		t.Errorf("expected name 'Test Project', got %s", rc.Name)
	}
	if rc.Type != "Web App" {
		t.Errorf("expected type 'Web App', got %s", rc.Type)
	}
	if rc.TeamSize != 3 {
		t.Errorf("expected team size 3, got %d", rc.TeamSize)
	}
	if len(rc.Items) != 3 {
		t.Errorf("expected 3 items (completed only), got %d", len(rc.Items))
	}
}

func TestCreateFromItemsNoCompleted(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Type: "Component", Size: "M", Status: "To Do", CycleTime: 0},
		{ID: "2", Type: "Component", Size: "L", Status: "In Progress", CycleTime: 0},
	}

	rc := CreateFromItems("Test Project", "Web App", 2, items)

	if len(rc.Items) != 0 {
		t.Errorf("expected 0 items (no completed), got %d", len(rc.Items))
	}
}

func TestDatabaseClose(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	err = db.Close()
	if err != nil {
		t.Errorf("failed to close database: %v", err)
	}
}

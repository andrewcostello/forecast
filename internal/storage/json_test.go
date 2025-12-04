package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bitbucket.org/supermoneygames/forecast/pkg/forecast"
)

func TestNew(t *testing.T) {
	store := New(".forecast")
	if store == nil {
		t.Fatal("expected store to be created")
	}
	if store.dataDir != ".forecast" {
		t.Errorf("expected dataDir '.forecast', got %s", store.dataDir)
	}
}

func TestNewWithEmptyDir(t *testing.T) {
	store := New("")
	if store.dataDir != ".forecast" {
		t.Errorf("expected default dataDir '.forecast', got %s", store.dataDir)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	store := New(tempDir)

	now := time.Now()
	inProgress := now.Add(24 * time.Hour)
	done := now.Add(48 * time.Hour)

	items := []forecast.Item{
		{
			ID:          "TEST-1",
			Type:        "Component",
			Description: "Test component",
			Size:        "M",
			Status:      "Done",
			Created:     now,
			InProgress:  &inProgress,
			Done:        &done,
			CycleTime:   24.0,
			Assignee:    "developer@example.com",
			JiraKey:     "TEST-1",
		},
		{
			ID:          "TEST-2",
			Type:        "Fix",
			Description: "Bug fix",
			Size:        "S",
			Status:      "To Do",
			Created:     now,
			JiraKey:     "TEST-2",
		},
	}

	// Save items
	err := store.Save(items)
	if err != nil {
		t.Fatalf("failed to save items: %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tempDir, "data.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("data.json file was not created")
	}

	// Load items
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load items: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 items, got %d", len(loaded))
	}

	// Verify first item
	if loaded[0].ID != "TEST-1" {
		t.Errorf("expected ID 'TEST-1', got %s", loaded[0].ID)
	}
	if loaded[0].Type != "Component" {
		t.Errorf("expected Type 'Component', got %s", loaded[0].Type)
	}
	if loaded[0].Size != "M" {
		t.Errorf("expected Size 'M', got %s", loaded[0].Size)
	}
	if loaded[0].Status != "Done" {
		t.Errorf("expected Status 'Done', got %s", loaded[0].Status)
	}
	if loaded[0].CycleTime != 24.0 {
		t.Errorf("expected CycleTime 24.0, got %f", loaded[0].CycleTime)
	}

	// Verify second item
	if loaded[1].ID != "TEST-2" {
		t.Errorf("expected ID 'TEST-2', got %s", loaded[1].ID)
	}
	if loaded[1].Status != "To Do" {
		t.Errorf("expected Status 'To Do', got %s", loaded[1].Status)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	tempDir := t.TempDir()
	store := New(tempDir)

	items, err := store.Load()
	if err != nil {
		t.Fatalf("expected no error for non-existent file, got: %v", err)
	}

	if len(items) != 0 {
		t.Errorf("expected empty slice for non-existent file, got %d items", len(items))
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "subdir", ".forecast")
	store := New(dataDir)

	items := []forecast.Item{
		{ID: "TEST-1", Status: "To Do"},
	}

	err := store.Save(items)
	if err != nil {
		t.Fatalf("failed to save items: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("data directory was not created")
	}
}

func TestSaveEmptySlice(t *testing.T) {
	tempDir := t.TempDir()
	store := New(tempDir)

	items := []forecast.Item{}

	err := store.Save(items)
	if err != nil {
		t.Fatalf("failed to save empty items: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load items: %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("expected 0 items, got %d", len(loaded))
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	store := New(tempDir)

	// Write invalid JSON
	filePath := filepath.Join(tempDir, "data.json")
	if err := os.WriteFile(filePath, []byte("invalid json{"), 0644); err != nil {
		t.Fatalf("failed to write invalid JSON: %v", err)
	}

	_, err := store.Load()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSaveOverwritesExisting(t *testing.T) {
	tempDir := t.TempDir()
	store := New(tempDir)

	// Save first set of items
	items1 := []forecast.Item{
		{ID: "TEST-1", Status: "To Do"},
		{ID: "TEST-2", Status: "To Do"},
	}
	if err := store.Save(items1); err != nil {
		t.Fatalf("failed to save first items: %v", err)
	}

	// Save second set of items (overwrites)
	items2 := []forecast.Item{
		{ID: "TEST-3", Status: "Done"},
	}
	if err := store.Save(items2); err != nil {
		t.Fatalf("failed to save second items: %v", err)
	}

	// Load and verify only second set exists
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load items: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("expected 1 item after overwrite, got %d", len(loaded))
	}
	if loaded[0].ID != "TEST-3" {
		t.Errorf("expected ID 'TEST-3', got %s", loaded[0].ID)
	}
}

func TestSaveWithAllFields(t *testing.T) {
	tempDir := t.TempDir()
	store := New(tempDir)

	now := time.Now()
	inProgress := now.Add(1 * time.Hour)
	done := now.Add(5 * time.Hour)

	items := []forecast.Item{
		{
			ID:          "FULL-1",
			Type:        "Component",
			Description: "Full test item with all fields",
			Size:        "L",
			Status:      "Done",
			Created:     now,
			InProgress:  &inProgress,
			Done:        &done,
			CycleTime:   4.0,
			Assignee:    "dev@example.com",
			JiraKey:     "FULL-1",
		},
	}

	if err := store.Save(items); err != nil {
		t.Fatalf("failed to save items: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load items: %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("expected 1 item, got %d", len(loaded))
	}

	item := loaded[0]
	if item.Description != "Full test item with all fields" {
		t.Errorf("expected description to match")
	}
	if item.Assignee != "dev@example.com" {
		t.Errorf("expected assignee 'dev@example.com', got %s", item.Assignee)
	}
	if item.InProgress == nil {
		t.Error("expected InProgress to be set")
	}
	if item.Done == nil {
		t.Error("expected Done to be set")
	}
}

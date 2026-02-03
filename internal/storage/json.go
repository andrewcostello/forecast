package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrewcostello/forecast/pkg/forecast"
)

// JSONStorage stores items as JSON in .forecast/data.json or .forecast/data-{project}.json
type JSONStorage struct {
	dataDir string
}

// New creates a new JSON storage instance
func New(dataDir string) *JSONStorage {
	if dataDir == "" {
		dataDir = ".forecast"
	}
	return &JSONStorage{dataDir: dataDir}
}

// Save writes items to JSON file (legacy - uses data.json)
func (s *JSONStorage) Save(items []forecast.Item) error {
	return s.SaveProject(items, "")
}

// SaveProject writes items to a project-specific JSON file
func (s *JSONStorage) SaveProject(items []forecast.Item, projectKey string) error {
	// Ensure directory exists
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	filePath := s.getDataFilePath(projectKey)

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write data file: %w", err)
	}

	return nil
}

// Load reads items from JSON file (legacy - uses data.json)
func (s *JSONStorage) Load() ([]forecast.Item, error) {
	return s.LoadProject("")
}

// LoadProject reads items from a project-specific JSON file
func (s *JSONStorage) LoadProject(projectKey string) ([]forecast.Item, error) {
	filePath := s.getDataFilePath(projectKey)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []forecast.Item{}, nil // Return empty slice if file doesn't exist
		}
		return nil, fmt.Errorf("failed to read data file: %w", err)
	}

	var items []forecast.Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal items: %w", err)
	}

	return items, nil
}

// getDataFilePath returns the appropriate data file path for a project
func (s *JSONStorage) getDataFilePath(projectKey string) string {
	if projectKey == "" {
		return filepath.Join(s.dataDir, "data.json")
	}
	return filepath.Join(s.dataDir, fmt.Sprintf("data-%s.json", projectKey))
}

// ListProjects returns a list of project keys that have data files
func (s *JSONStorage) ListProjects() ([]string, error) {
	files, err := os.ReadDir(s.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var projects []string
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, "data-") && strings.HasSuffix(name, ".json") {
			// Extract project key from "data-{key}.json"
			key := strings.TrimSuffix(strings.TrimPrefix(name, "data-"), ".json")
			projects = append(projects, key)
		}
	}
	return projects, nil
}

package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"bitbucket.org/supermoneygames/forecast/pkg/forecast"
)

// JSONStorage stores items as JSON in .forecast/data.json
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

// Save writes items to JSON file
func (s *JSONStorage) Save(items []forecast.Item) error {
	// Ensure directory exists
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	filePath := filepath.Join(s.dataDir, "data.json")

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write data file: %w", err)
	}

	return nil
}

// Load reads items from JSON file
func (s *JSONStorage) Load() ([]forecast.Item, error) {
	filePath := filepath.Join(s.dataDir, "data.json")

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

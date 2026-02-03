// Package history provides forecast history tracking and storage.
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Entry represents a single forecast history entry
type Entry struct {
	Timestamp   time.Time          `json:"timestamp"`
	ProjectKey  string             `json:"projectKey"`
	Total       int                `json:"total"`
	Completed   int                `json:"completed"`
	Remaining   int                `json:"remaining"`
	AvgCycle    float64            `json:"avgCycleHours"`
	Throughput  float64            `json:"throughputPerDay"`
	Percentiles map[int]float64    `json:"percentiles"` // confidence level -> days
}

// Store manages forecast history storage
type Store struct {
	baseDir string
}

// New creates a new history store
func New(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// Save saves a forecast entry to history
func (s *Store) Save(entry Entry) error {
	// Load existing history
	entries, err := s.LoadProject(entry.ProjectKey)
	if err != nil {
		entries = []Entry{}
	}

	// Add new entry
	entries = append(entries, entry)

	// Keep last 100 entries max
	if len(entries) > 100 {
		entries = entries[len(entries)-100:]
	}

	// Write back
	return s.saveEntries(entry.ProjectKey, entries)
}

// LoadProject loads forecast history for a project
func (s *Store) LoadProject(projectKey string) ([]Entry, error) {
	path := s.historyPath(projectKey)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	// Sort by timestamp ascending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries, nil
}

// GetLatest returns the most recent forecast entry for a project
func (s *Store) GetLatest(projectKey string) (*Entry, error) {
	entries, err := s.LoadProject(projectKey)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[len(entries)-1], nil
}

// GetTrend returns forecast entries for the last N days
func (s *Store) GetTrend(projectKey string, days int) ([]Entry, error) {
	entries, err := s.LoadProject(projectKey)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var trend []Entry
	for _, e := range entries {
		if e.Timestamp.After(cutoff) {
			trend = append(trend, e)
		}
	}

	return trend, nil
}

// Clear removes all history for a project
func (s *Store) Clear(projectKey string) error {
	path := s.historyPath(projectKey)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) historyPath(projectKey string) string {
	if projectKey == "" {
		return filepath.Join(s.baseDir, "history.json")
	}
	return filepath.Join(s.baseDir, "history-"+projectKey+".json")
}

func (s *Store) saveEntries(projectKey string, entries []Entry) error {
	// Ensure directory exists
	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.historyPath(projectKey), data, 0644)
}

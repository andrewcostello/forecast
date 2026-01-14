// internal/yaml/parser.go
package yaml

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// TaskFile represents the structure of a YAML task file
type TaskFile struct {
	Project string `yaml:"project"`
	Epic    string `yaml:"epic"`
	Tasks   []Task `yaml:"tasks"`

	// Internal: preserve original for round-trip editing
	filePath string
	node     *yaml.Node
}

// Task represents a single task definition
type Task struct {
	Key         string   `yaml:"key"`
	JiraKey     string   `yaml:"jira_key,omitempty"`
	Summary     string   `yaml:"summary"`
	Description string   `yaml:"description,omitempty"`
	Type        string   `yaml:"type,omitempty"`
	Estimate    string   `yaml:"estimate,omitempty"`
	Labels      []string `yaml:"labels,omitempty"`
	Status      string   `yaml:"status,omitempty"`
}

// ParseTaskFile reads and parses a YAML task file
func ParseTaskFile(path string) (*TaskFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var tf TaskFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Also parse into node for round-trip preservation
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("failed to parse YAML node: %w", err)
	}

	tf.filePath = path
	tf.node = &node

	return &tf, nil
}

// internal/yaml/parser.go
package yaml

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// TaskFile represents the structure of a YAML task file
type TaskFile struct {
	JiraInstance string `yaml:"jira_instance,omitempty"` // Named JIRA instance from config
	Project      string `yaml:"project"`
	Epic         string `yaml:"epic"`
	Tasks        []Task `yaml:"tasks"`

	// Internal: preserve original for round-trip editing
	filePath string
	node     *yaml.Node
}

// Task represents a single task definition.
//
// NOTE: this struct intentionally models only the fields forecast reads. The
// task YAML is often owned by claude-dispatcher and carries many more fields
// (agent, effort, verify, panel, model, batch_id, ...). Save() therefore does a
// surgical edit on the preserved YAML node rather than re-marshaling this
// struct, so unmodeled fields and comments survive the round-trip.
type Task struct {
	Key         string   `yaml:"key"`
	JiraKey     string   `yaml:"jira_key,omitempty"`
	Summary     string   `yaml:"summary"`
	Description string   `yaml:"description,omitempty"`
	Type        string   `yaml:"type,omitempty"`
	Estimate    string   `yaml:"estimate,omitempty"`
	Labels      []string `yaml:"labels,omitempty"`
	Status      string   `yaml:"status,omitempty"`
	BlockedBy   []string `yaml:"blockedBy,omitempty"` // dispatcher task keys this task depends on
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

// Save writes the task file back to disk. It edits the preserved YAML node in
// place — setting only jira_key + status on each task — so all other fields
// (blockedBy, agent, effort, verify, panel, model, ...), the top-level keys
// (prd, base_branch, ...), and comments are preserved verbatim.
func (tf *TaskFile) Save() error {
	if tf.filePath == "" {
		return fmt.Errorf("no file path set")
	}

	// Fall back to struct marshal only if we somehow have no node (should not
	// happen via ParseTaskFile).
	if tf.node == nil {
		data, err := yaml.Marshal(tf)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		return os.WriteFile(tf.filePath, data, 0644)
	}

	root := tf.node
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}

	tasksNode := mapGet(root, "tasks")
	if tasksNode == nil || tasksNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("could not locate a 'tasks' sequence in %s", tf.filePath)
	}

	byKey := make(map[string]*Task, len(tf.Tasks))
	for i := range tf.Tasks {
		byKey[tf.Tasks[i].Key] = &tf.Tasks[i]
	}

	for _, tnode := range tasksNode.Content {
		if tnode.Kind != yaml.MappingNode {
			continue
		}
		keyNode := mapGet(tnode, "key")
		if keyNode == nil {
			continue
		}
		t := byKey[keyNode.Value]
		if t == nil {
			continue
		}
		if t.JiraKey != "" {
			mapSetAfter(tnode, "key", "jira_key", t.JiraKey)
		}
		if t.Status != "" {
			mapSet(tnode, "status", t.Status)
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(tf.node); err != nil {
		_ = enc.Close()
		return fmt.Errorf("failed to marshal YAML node: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("failed to finalize YAML: %w", err)
	}

	if err := os.WriteFile(tf.filePath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// --- yaml.Node mapping helpers (comment-preserving edits) -------------------

// mapGet returns the value node for key in a mapping node, or nil.
func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mapSet sets key=val on a mapping node (updating in place if present, else
// appending at the end).
func mapSet(m *yaml.Node, key, val string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			vn := m.Content[i+1]
			vn.Kind = yaml.ScalarNode
			vn.Tag = "!!str"
			vn.Value = val
			return
		}
	}
	m.Content = append(m.Content, scalar(key), scalar(val))
}

// mapSetAfter sets key=val, inserting the new pair immediately after afterKey
// when the key is not already present (keeps jira_key next to key). Updates in
// place if the key already exists.
func mapSetAfter(m *yaml.Node, afterKey, key, val string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			vn := m.Content[i+1]
			vn.Kind = yaml.ScalarNode
			vn.Tag = "!!str"
			vn.Value = val
			return
		}
	}
	pair := []*yaml.Node{scalar(key), scalar(val)}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == afterKey {
			idx := i + 2
			out := make([]*yaml.Node, 0, len(m.Content)+2)
			out = append(out, m.Content[:idx]...)
			out = append(out, pair...)
			out = append(out, m.Content[idx:]...)
			m.Content = out
			return
		}
	}
	m.Content = append(m.Content, pair...)
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

// GetTask returns a task by its key
func (tf *TaskFile) GetTask(key string) *Task {
	for i := range tf.Tasks {
		if tf.Tasks[i].Key == key {
			return &tf.Tasks[i]
		}
	}
	return nil
}

// FilePath returns the path to the task file
func (tf *TaskFile) FilePath() string {
	return tf.filePath
}

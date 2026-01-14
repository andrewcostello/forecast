// internal/yaml/parser_test.go
package yaml

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTaskFile(t *testing.T) {
	// Create temp file with test content
	content := `project: NEON
epic: NEON-2

tasks:
  - key: TEST-001
    summary: Test task one
    description: |
      Multi-line description here.
    type: Story
    estimate: 2h
    labels: [test, size:S]
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "tasks.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tf, err := ParseTaskFile(tmpFile)
	if err != nil {
		t.Fatalf("ParseTaskFile failed: %v", err)
	}

	if tf.Project != "NEON" {
		t.Errorf("expected project NEON, got %s", tf.Project)
	}
	if tf.Epic != "NEON-2" {
		t.Errorf("expected epic NEON-2, got %s", tf.Epic)
	}
	if len(tf.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tf.Tasks))
	}
	if tf.Tasks[0].Key != "TEST-001" {
		t.Errorf("expected key TEST-001, got %s", tf.Tasks[0].Key)
	}
	if tf.Tasks[0].Summary != "Test task one" {
		t.Errorf("expected summary 'Test task one', got %s", tf.Tasks[0].Summary)
	}
}

//go:build integration

package yaml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripPreservesComments(t *testing.T) {
	// Test that saving preserves the general structure
	content := `# Header comment
project: NEON
epic: NEON-2

tasks:
  - key: TEST-001
    summary: Test task
    type: Story
    labels: [test]
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

	tf.Tasks[0].JiraKey = "NEON-100"
	tf.Tasks[0].Status = "Done"

	if err := tf.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	result := string(data)

	// Verify key fields are present
	if !strings.Contains(result, "jira_key: NEON-100") {
		t.Errorf("expected jira_key in output, got:\n%s", result)
	}
	if !strings.Contains(result, "status: Done") {
		t.Errorf("expected status in output, got:\n%s", result)
	}
}

// internal/yaml/parser_test.go
package yaml

import (
	"os"
	"path/filepath"
	"strings"
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

func TestUpdateTaskJiraKey(t *testing.T) {
	content := `project: NEON
epic: NEON-2

tasks:
  - key: TEST-001
    summary: Test task one
    type: Story
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

	// Update the task
	tf.Tasks[0].JiraKey = "NEON-123"
	tf.Tasks[0].Status = "To Do"

	if err := tf.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Re-read and verify
	tf2, err := ParseTaskFile(tmpFile)
	if err != nil {
		t.Fatalf("ParseTaskFile failed: %v", err)
	}

	if tf2.Tasks[0].JiraKey != "NEON-123" {
		t.Errorf("expected jira_key NEON-123, got %s", tf2.Tasks[0].JiraKey)
	}
	if tf2.Tasks[0].Status != "To Do" {
		t.Errorf("expected status 'To Do', got %s", tf2.Tasks[0].Status)
	}
}

// TestSavePreservesDispatcherFields guards against Save() dropping fields
// forecast doesn't model (blockedBy, agent, verify, panel, ...), the top-level
// prd/base_branch, and comments. Save() must round-trip the node, touching only
// jira_key + status.
func TestSavePreservesDispatcherFields(t *testing.T) {
	content := `prd: features/rc1/PRD.md
project: FI
epic: rc1
base_branch: main
# a load-bearing comment that must survive
tasks:
  - key: FI-336
    summary: skeleton
    type: Task
    labels: [size:S, area:api]
    blockedBy: []
    agent: claude
    model: claude-fable-5
    verify: mechanical
    panel: single
  - key: FI-337
    summary: governor
    type: Task
    labels: [size:M, area:api]
    blockedBy: [FI-336]
    agent: grok
    verify: llm
    panel: full
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "tasks.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	tf, err := ParseTaskFile(tmpFile)
	if err != nil {
		t.Fatalf("ParseTaskFile: %v", err)
	}
	if got := tf.GetTask("FI-337"); got == nil || len(got.BlockedBy) != 1 || got.BlockedBy[0] != "FI-336" {
		t.Fatalf("blockedBy not parsed: %+v", got)
	}
	tf.GetTask("FI-336").JiraKey = "FI-14"
	tf.GetTask("FI-336").Status = "To Do"
	tf.GetTask("FI-337").JiraKey = "FI-24"
	if err := tf.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, must := range []string{
		"prd: features/rc1/PRD.md", "base_branch: main", "# a load-bearing comment",
		"agent: claude", "model: claude-fable-5", "verify: mechanical", "panel: full",
		"blockedBy:", "FI-336", "jira_key: FI-14", "jira_key: FI-24", "status: To Do",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("Save() dropped %q\n---\n%s", must, s)
		}
	}
	tf2, err := ParseTaskFile(tmpFile)
	if err != nil {
		t.Fatalf("re-parse after Save: %v", err)
	}
	if len(tf2.Tasks) != 2 || tf2.GetTask("FI-336").JiraKey != "FI-14" {
		t.Errorf("round-trip lost data: %d tasks", len(tf2.Tasks))
	}
}

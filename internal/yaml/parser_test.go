// internal/yaml/parser_test.go
package yaml

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestToItems_Authoritative(t *testing.T) {
	content := `project: protodocs
reference_class: "Go CLI / docs platform"

milestones:
  - name: v0.1
    target: 2026-08-01
    scope: "Local folder rendering"

tasks:
  - key: PROTO-1
    summary: "Wire buf generate"
    type: component
    size: s
    status: done
    created_at: 2026-05-14
    started_at: 2026-05-14
    completed_at: 2026-05-14
    assignee: andrew

  - key: PROTO-2
    summary: "Parser: proto graph"
    type: component
    size: M
    status: in_progress
    created_at: 2026-05-14
    started_at: 2026-05-15
    dependencies: [PROTO-1]

  - key: PROTO-3
    summary: "Renderer"
    type: component
    size: XL
    status: todo
    created_at: 2026-05-14

  - key: PROTO-4
    summary: "Blocked task"
    type: fix
    size: L
    status: blocked
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
	if tf.ReferenceClass != "Go CLI / docs platform" {
		t.Errorf("reference_class: %q", tf.ReferenceClass)
	}
	if len(tf.Milestones) != 1 || tf.Milestones[0].Name != "v0.1" {
		t.Errorf("milestones: %+v", tf.Milestones)
	}

	items, err := tf.ToItems()
	if err != nil {
		t.Fatalf("ToItems: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d items, want 4", len(items))
	}

	p1 := items[0]
	if p1.ID != "PROTO-1" || p1.Status != "Done" || p1.Size != "S" || p1.Assignee != "andrew" {
		t.Errorf("PROTO-1 unexpected: %+v", p1)
	}
	if p1.InProgress == nil || p1.Done == nil {
		t.Fatalf("PROTO-1 timestamps missing")
	}
	if p1.CycleTime != 0 {
		t.Errorf("PROTO-1 cycle time: got %v, want 0", p1.CycleTime)
	}

	p2 := items[1]
	if p2.Status != "In Progress" || p2.Size != "M" || p2.InProgress == nil || p2.Done != nil {
		t.Errorf("PROTO-2 unexpected: %+v", p2)
	}

	p3 := items[2]
	if p3.Status != "To Do" || p3.Size != "XL" || p3.InProgress != nil || p3.Done != nil {
		t.Errorf("PROTO-3 unexpected: %+v", p3)
	}

	if items[3].Status != "Blocked" {
		t.Errorf("PROTO-4 status: %q", items[3].Status)
	}
}

func TestToItems_CycleTime(t *testing.T) {
	content := `tasks:
  - key: X
    summary: t
    type: component
    size: M
    status: done
    started_at: 2026-05-01
    completed_at: 2026-05-04
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "tasks.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	tf, err := ParseTaskFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	items, err := tf.ToItems()
	if err != nil {
		t.Fatal(err)
	}
	want := (3 * 24 * time.Hour).Hours()
	if items[0].CycleTime != want {
		t.Errorf("cycle time: got %v, want %v", items[0].CycleTime, want)
	}
}

func TestToItems_MissingKey(t *testing.T) {
	content := `tasks:
  - summary: no key
    type: component
    size: M
    status: todo
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "tasks.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	tf, err := ParseTaskFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tf.ToItems(); err == nil {
		t.Error("expected error for missing key, got nil")
	}
}

func TestToItems_BadDate(t *testing.T) {
	content := `tasks:
  - key: X
    summary: t
    type: component
    size: M
    status: done
    started_at: not-a-date
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "tasks.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	tf, err := ParseTaskFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tf.ToItems(); err == nil {
		t.Error("expected error for bad date, got nil")
	}
}

func TestNormalizeStatus(t *testing.T) {
	cases := map[string]string{
		"todo":        "To Do",
		"To Do":       "To Do",
		"to_do":       "To Do",
		"in_progress": "In Progress",
		"In Progress": "In Progress",
		"done":        "Done",
		"completed":   "Done",
		"blocked":     "Blocked",
		"":            "To Do",
		"Custom":      "Custom",
	}
	for in, want := range cases {
		if got := NormalizeStatus(in); got != want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

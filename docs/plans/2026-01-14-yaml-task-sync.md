# YAML Task Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add bidirectional sync between YAML task files and JIRA, allowing tasks defined in YAML to be created in JIRA and their status synced back.

**Architecture:** New `internal/yaml` package parses task files. Extended `sync` command accepts `--file` flag. Sync creates missing JIRA tickets, fetches status for existing ones, and writes JIRA keys back to YAML.

**Tech Stack:** Go, gopkg.in/yaml.v3 (for round-trip preservation), existing JIRA client

---

## Task 1: Add YAML Parser Package

**Files:**
- Create: `internal/yaml/parser.go`
- Create: `internal/yaml/parser_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/yaml/... -v`
Expected: FAIL - package does not exist

**Step 3: Write minimal implementation**

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/yaml/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/yaml/parser.go internal/yaml/parser_test.go
git commit -m "feat(yaml): add task file parser"
```

---

## Task 2: Add YAML Write-Back Functionality

**Files:**
- Modify: `internal/yaml/parser.go`
- Modify: `internal/yaml/parser_test.go`

**Step 1: Write the failing test**

```go
// Add to internal/yaml/parser_test.go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/yaml/... -v -run TestUpdateTaskJiraKey`
Expected: FAIL - Save method does not exist

**Step 3: Write minimal implementation**

Add to `internal/yaml/parser.go`:

```go
// Save writes the task file back to disk, preserving formatting where possible
func (tf *TaskFile) Save() error {
	if tf.filePath == "" {
		return fmt.Errorf("no file path set")
	}

	data, err := yaml.Marshal(tf)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if err := os.WriteFile(tf.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/yaml/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/yaml/parser.go internal/yaml/parser_test.go
git commit -m "feat(yaml): add Save method for write-back"
```

---

## Task 3: Add --file Flag to Sync Command

**Files:**
- Modify: `cmd/forecast/main.go`

**Step 1: Add flag registration**

Find the `init()` function that sets up `syncCmd` flags (around line 138) and add the file flag:

```go
syncCmd.Flags().StringP("project", "p", "", "Sync specific project (by key or epic), or all if omitted")
syncCmd.Flags().StringP("file", "f", "", "Sync tasks from a YAML file (bidirectional)")
syncCmd.Flags().Bool("dry-run", false, "Preview what would be created without making changes")
```

**Step 2: Update syncCmd.RunE to read the flag**

Update the RunE function in syncCmd:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	project, _ := cmd.Flags().GetString("project")
	file, _ := cmd.Flags().GetString("file")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if file != "" {
		return runSyncFromFile(file, dryRun)
	}
	return runSync(project)
},
```

**Step 3: Add stub function**

Add after the existing `runSync` function:

```go
func runSyncFromFile(filePath string, dryRun bool) error {
	fmt.Printf("Syncing from YAML file: %s\n", filePath)
	if dryRun {
		fmt.Println("(dry-run mode - no changes will be made)")
	}
	// TODO: Implement in next task
	return fmt.Errorf("not yet implemented")
}
```

**Step 4: Verify it compiles**

Run: `go build ./cmd/forecast`
Expected: SUCCESS

**Step 5: Commit**

```bash
git add cmd/forecast/main.go
git commit -m "feat(cli): add --file and --dry-run flags to sync command"
```

---

## Task 4: Implement YAML-to-JIRA Sync Logic

**Files:**
- Modify: `cmd/forecast/main.go`

**Step 1: Add import for yaml package**

Add to imports at top of `cmd/forecast/main.go`:

```go
yamlparser "bitbucket.org/supermoneygames/forecast/internal/yaml"
```

**Step 2: Implement runSyncFromFile**

Replace the stub with full implementation:

```go
func runSyncFromFile(filePath string, dryRun bool) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config")
	}

	// Parse YAML file
	tf, err := yamlparser.ParseTaskFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to parse task file: %w", err)
	}

	fmt.Printf("Loaded %d tasks from %s\n", len(tf.Tasks), filePath)
	fmt.Printf("Project: %s, Epic: %s\n\n", tf.Project, tf.Epic)

	// Create JIRA client
	jiraClient := jira.NewClient(&cfg.JIRA)

	var created, updated, skipped int
	var modified bool

	for i := range tf.Tasks {
		task := &tf.Tasks[i]

		if task.JiraKey != "" {
			// Task already has JIRA key - fetch status
			fmt.Printf("  [%s] %s → fetching status from %s...\n", task.Key, truncate(task.Summary, 40), task.JiraKey)

			if !dryRun {
				issue, err := jiraClient.GetIssue(task.JiraKey)
				if err != nil {
					fmt.Printf("    Warning: failed to fetch %s: %v\n", task.JiraKey, err)
					skipped++
					continue
				}

				if task.Status != issue.Fields.Status.Name {
					task.Status = issue.Fields.Status.Name
					modified = true
					fmt.Printf("    Status: %s\n", task.Status)
				}
			}
			updated++
		} else {
			// No JIRA key - create new ticket
			fmt.Printf("  [%s] %s → creating in JIRA...\n", task.Key, truncate(task.Summary, 40))

			if !dryRun {
				req := jira.CreateIssueRequest{
					Summary:     fmt.Sprintf("%s: %s", task.Key, task.Summary),
					IssueType:   task.Type,
					Description: task.Description,
					Priority:    "Medium",
					Labels:      task.Labels,
					Epic:        tf.Epic,
					Project:     tf.Project,
				}

				// Default to Story if type not specified
				if req.IssueType == "" {
					req.IssueType = "Story"
				}

				result, err := jiraClient.CreateIssue(req)
				if err != nil {
					fmt.Printf("    Error: %v\n", err)
					skipped++
					continue
				}

				task.JiraKey = result.Key
				task.Status = "To Do"
				modified = true
				fmt.Printf("    Created: %s\n", result.Key)
			} else {
				fmt.Printf("    Would create: %s: %s\n", task.Key, task.Summary)
			}
			created++
		}
	}

	// Save updated YAML file
	if modified && !dryRun {
		if err := tf.Save(); err != nil {
			return fmt.Errorf("failed to save updated YAML: %w", err)
		}
		fmt.Printf("\n✓ Updated %s with JIRA keys\n", filePath)
	}

	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Summary: %d created, %d updated, %d skipped\n", created, updated, skipped)

	if dryRun {
		fmt.Println("\n(dry-run mode - no changes were made)")
	}

	return nil
}
```

**Step 3: Verify it compiles**

Run: `go build ./cmd/forecast`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/forecast/main.go
git commit -m "feat(sync): implement bidirectional YAML-to-JIRA sync"
```

---

## Task 5: Add Local Storage Update for Forecasting

**Files:**
- Modify: `cmd/forecast/main.go`

**Step 1: Extend runSyncFromFile to save to local storage**

Add after the YAML save block in `runSyncFromFile`:

```go
	// Also save to local storage for forecasting
	store := storage.New(".forecast")

	// Convert tasks to forecast items
	items := make([]forecast.Item, 0, len(tf.Tasks))
	for _, task := range tf.Tasks {
		item := forecast.Item{
			ID:          task.Key,
			JiraKey:     task.JiraKey,
			Description: task.Summary,
			Status:      task.Status,
			Type:        task.Type,
		}

		// Parse size from labels
		for _, label := range task.Labels {
			if strings.HasPrefix(label, "size:") {
				item.Size = strings.TrimPrefix(label, "size:")
				break
			}
		}

		items = append(items, item)
	}

	// Determine project key for storage
	projectKey := tf.Project
	if projectKey == "" {
		projectKey = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	if !dryRun {
		if err := store.SaveProject(items, projectKey); err != nil {
			fmt.Printf("Warning: failed to save to local storage: %v\n", err)
		} else {
			fmt.Printf("✓ Saved %d items to .forecast/data-%s.json\n", len(items), projectKey)
		}
	}
```

**Step 2: Add filepath import if not present**

Ensure imports include:
```go
"path/filepath"
```

**Step 3: Verify it compiles**

Run: `go build ./cmd/forecast`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/forecast/main.go
git commit -m "feat(sync): save YAML tasks to local storage for forecasting"
```

---

## Task 6: Add Integration Test

**Files:**
- Create: `internal/yaml/integration_test.go`

**Step 1: Write integration test**

```go
// internal/yaml/integration_test.go
//go:build integration

package yaml

import (
	"os"
	"path/filepath"
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
	if !contains(result, "jira_key: NEON-100") {
		t.Errorf("expected jira_key in output, got:\n%s", result)
	}
	if !contains(result, "status: Done") {
		t.Errorf("expected status in output, got:\n%s", result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

**Step 2: Run test**

Run: `go test ./internal/yaml/... -v -tags=integration`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/yaml/integration_test.go
git commit -m "test(yaml): add integration test for round-trip save"
```

---

## Task 7: Update Help Text and Documentation

**Files:**
- Modify: `cmd/forecast/main.go`

**Step 1: Update syncCmd Long description**

Find `syncCmd` and update the Long field:

```go
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync data from JIRA or YAML files",
	Long: `Pulls latest issue data from JIRA including:
  - Issue status
  - Cycle times (Created → Done)
  - Item types and sizes
  - Assignees

Use --project to sync a specific project, or sync all configured projects.

YAML File Sync (--file):
  Bidirectional sync between a YAML task file and JIRA:
  - Creates JIRA tickets for tasks without jira_key
  - Fetches status for tasks with jira_key
  - Writes jira_key back to YAML after creation
  - Saves items to local storage for forecasting

Example:
  forecast sync --file docs/tasks/WALLET_TASKS.yaml
  forecast sync --file tasks.yaml --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// ... existing code
	},
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/forecast`
Expected: SUCCESS

**Step 3: Test help output**

Run: `./forecast sync --help`
Expected: Shows updated help with YAML sync documentation

**Step 4: Commit**

```bash
git add cmd/forecast/main.go
git commit -m "docs(cli): update sync command help with YAML file usage"
```

---

## Task 8: Final Integration Test with Real YAML

**Files:** None (manual test)

**Step 1: Create a test YAML file**

Create `test-tasks.yaml` in forecast directory:

```yaml
project: NEON
epic: NEON-2

tasks:
  - key: TEST-YAML-001
    summary: Test YAML sync feature
    description: |
      This is a test task to verify YAML sync works.
    type: Story
    estimate: 1h
    labels: [test, size:S]
```

**Step 2: Run dry-run sync**

Run: `./forecast sync --file test-tasks.yaml --dry-run`
Expected: Shows what would be created without making changes

**Step 3: Run actual sync (if JIRA configured)**

Run: `./forecast sync --file test-tasks.yaml`
Expected:
- Creates ticket in JIRA
- Updates YAML with jira_key
- Saves to .forecast/data-NEON.json

**Step 4: Clean up test file**

Run: `rm test-tasks.yaml`

**Step 5: Final commit**

```bash
git add -A
git commit -m "feat(yaml): complete YAML task sync implementation"
```

---

## Summary

This implementation adds:
1. **New package**: `internal/yaml` for parsing task files
2. **Bidirectional sync**: Creates JIRA tickets, fetches status, writes back keys
3. **Local storage**: Saves tasks for Monte Carlo forecasting
4. **Dry-run mode**: Preview changes before making them
5. **Full test coverage**: Unit and integration tests

Usage:
```bash
# Preview what would happen
forecast sync --file ~/Project/neon/docs/specs/impl/WALLET_TASKS.yaml --dry-run

# Actually sync
forecast sync --file ~/Project/neon/docs/specs/impl/WALLET_TASKS.yaml

# Then run forecasts
forecast run --project NEON
```

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNonExistentConfig(t *testing.T) {
	// Reset current config
	current = nil

	// Try to load a non-existent config file
	err := Load("/non/existent/path/config.yaml")
	if err == nil {
		// viper doesn't error on missing config file
		// but current should be nil
	}
}

func TestLoadValidConfig(t *testing.T) {
	tempDir := t.TempDir()
	configContent := `
project_name: "Test Project"
project_type: "API Migration"
team_size: 5
team_capacity: 6.0

jira:
  url: https://test.atlassian.net
  email: test@example.com
  api_token: test-token
  project_key: TEST
  epic: TEST-1
  labels:
    - test-label

item_types:
  - name: Component
    jira_label: type:component
  - name: Fix
    jira_label: type:fix

reference_class:
  name: "Test Reference"
  type: "API Migration"

jira_mapping:
  item_type:
    field: labels
    mappings:
      - jira: "type:component"
        forecast: "Component"
  size:
    field: labels
    mappings:
      - jira: "size:M"
        forecast: "M"
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	current = nil
	err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cfg := Get()
	if cfg == nil {
		t.Fatal("expected config to be loaded")
	}

	if cfg.ProjectName != "Test Project" {
		t.Errorf("expected project name 'Test Project', got %s", cfg.ProjectName)
	}
	if cfg.ProjectType != "API Migration" {
		t.Errorf("expected project type 'API Migration', got %s", cfg.ProjectType)
	}
	if cfg.TeamSize != 5 {
		t.Errorf("expected team size 5, got %d", cfg.TeamSize)
	}
	if cfg.TeamCapacity != 6.0 {
		t.Errorf("expected team capacity 6.0, got %f", cfg.TeamCapacity)
	}
}

func TestLoadJIRAConfig(t *testing.T) {
	tempDir := t.TempDir()
	configContent := `
jira:
  url: https://company.atlassian.net
  email: user@company.com
  api_token: secret-token
  project_key: PROJ
  epic: PROJ-123
  labels:
    - phase1
    - refactor
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	current = nil
	if err := Load(configPath); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cfg := Get()
	if cfg.JIRA.URL != "https://company.atlassian.net" {
		t.Errorf("expected JIRA URL 'https://company.atlassian.net', got %s", cfg.JIRA.URL)
	}
	if cfg.JIRA.Email != "user@company.com" {
		t.Errorf("expected JIRA email 'user@company.com', got %s", cfg.JIRA.Email)
	}
	if cfg.JIRA.ProjectKey != "PROJ" {
		t.Errorf("expected JIRA project key 'PROJ', got %s", cfg.JIRA.ProjectKey)
	}
	if cfg.JIRA.Epic != "PROJ-123" {
		t.Errorf("expected JIRA epic 'PROJ-123', got %s", cfg.JIRA.Epic)
	}
	if len(cfg.JIRA.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(cfg.JIRA.Labels))
	}
}

func TestGetReturnsDefaultWhenNil(t *testing.T) {
	current = nil
	cfg := Get()

	if cfg == nil {
		t.Error("expected Get() to return non-nil config")
	}
}

func TestInitProject(t *testing.T) {
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(origDir)

	err := InitProject()
	if err != nil {
		t.Fatalf("failed to init project: %v", err)
	}

	// Verify .forecast directory was created
	forecastDir := filepath.Join(tempDir, ".forecast")
	if _, err := os.Stat(forecastDir); os.IsNotExist(err) {
		t.Error(".forecast directory was not created")
	}

	// Verify config file was created
	configPath := filepath.Join(forecastDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.yaml was not created")
	}

	// Read config file and verify content
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	// Check that it contains expected content
	contentStr := string(content)
	if len(contentStr) == 0 {
		t.Error("config file is empty")
	}
}

func TestInitProjectAlreadyExists(t *testing.T) {
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(origDir)

	// First init should succeed
	if err := InitProject(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Second init should fail
	err := InitProject()
	if err == nil {
		t.Error("expected error when config already exists")
	}
}

func TestItemTypesConfig(t *testing.T) {
	tempDir := t.TempDir()
	configContent := `
item_types:
  - name: Component
    jira_label: type:component
  - name: Fix
    jira_label: type:fix
  - name: Migration
    jira_label: type:migration
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	current = nil
	if err := Load(configPath); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cfg := Get()
	if len(cfg.ItemTypes) != 3 {
		t.Errorf("expected 3 item types, got %d", len(cfg.ItemTypes))
	}

	if cfg.ItemTypes[0].Name != "Component" {
		t.Errorf("expected first item type 'Component', got %s", cfg.ItemTypes[0].Name)
	}
	if cfg.ItemTypes[0].JIRALabel != "type:component" {
		t.Errorf("expected first jira label 'type:component', got %s", cfg.ItemTypes[0].JIRALabel)
	}
}

func TestJIRAMappingConfig(t *testing.T) {
	tempDir := t.TempDir()
	configContent := `
jira_mapping:
  item_type:
    field: labels
    mappings:
      - jira: "type:component"
        forecast: "Component"
      - jira: "type:fix"
        forecast: "Fix"
  size:
    field: labels
    mappings:
      - jira: "size:S"
        forecast: "S"
      - jira: "size:M"
        forecast: "M"
      - jira: "size:L"
        forecast: "L"
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	current = nil
	if err := Load(configPath); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cfg := Get()
	if cfg.JIRAMapping.ItemType.Field != "labels" {
		t.Errorf("expected item_type field 'labels', got %s", cfg.JIRAMapping.ItemType.Field)
	}
	if len(cfg.JIRAMapping.ItemType.Mappings) != 2 {
		t.Errorf("expected 2 item type mappings, got %d", len(cfg.JIRAMapping.ItemType.Mappings))
	}
	if len(cfg.JIRAMapping.Size.Mappings) != 3 {
		t.Errorf("expected 3 size mappings, got %d", len(cfg.JIRAMapping.Size.Mappings))
	}
}

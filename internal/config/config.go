package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	ProjectName    string                 `mapstructure:"project_name"`
	ProjectType    string                 `mapstructure:"project_type"`
	TeamSize       int                    `mapstructure:"team_size"`
	TeamCapacity   float64                `mapstructure:"team_capacity"` // Hours per day
	JIRA           JIRAConfig             `mapstructure:"jira"`
	JIRAInstances  map[string]JIRAConfig  `mapstructure:"jira_instances"` // Named JIRA instances for multi-JIRA support
	Projects       []ProjectConfig        `mapstructure:"projects"`       // Multiple project/epic tracking
	ItemTypes      []ItemTypeConfig       `mapstructure:"item_types"`
	ReferenceClass ReferenceClassConfig   `mapstructure:"reference_class"`
	JIRAMapping    JIRAMappingConfig      `mapstructure:"jira_mapping"`
}

// ProjectConfig defines a trackable project/initiative
type ProjectConfig struct {
	Name     string `mapstructure:"name"`      // Display name
	Epic     string `mapstructure:"epic"`      // Epic key (e.g., SMG-1688)
	Key      string `mapstructure:"key"`       // Short key for CLI (e.g., "monorepo")
	Capacity float64 `mapstructure:"capacity"` // Optional: team capacity for this project
}

type JIRAConfig struct {
	URL              string   `mapstructure:"url"`
	Email            string   `mapstructure:"email"`
	APIToken         string   `mapstructure:"api_token"`
	ProjectKey       string   `mapstructure:"project_key"`
	Epic             string   `mapstructure:"epic"`
	Labels           []string `mapstructure:"labels"`
	CycleTimeField   string   `mapstructure:"cycle_time_field"`   // Custom field ID for manual cycle time override (e.g., "customfield_10001")
	StoryPointsField string   `mapstructure:"story_points_field"` // Custom field ID for story points (e.g., "customfield_10004")
}

type ItemTypeConfig struct {
	Name      string `mapstructure:"name"`
	JIRALabel string `mapstructure:"jira_label"`
}

type ReferenceClassConfig struct {
	Name string `mapstructure:"name"`
	Type string `mapstructure:"type"`
}

type JIRAMappingConfig struct {
	ItemType MappingConfig `mapstructure:"item_type"`
	Size     MappingConfig `mapstructure:"size"`
}

type MappingConfig struct {
	Field    string            `mapstructure:"field"`
	Mappings []FieldMapping    `mapstructure:"mappings"`
}

type FieldMapping struct {
	JIRA     string `mapstructure:"jira"`
	Forecast string `mapstructure:"forecast"`
}

var current *Config

// ConfigNotFoundError indicates config file was not found
type ConfigNotFoundError struct{}

func (e ConfigNotFoundError) Error() string {
	return "config file not found"
}

// Load loads configuration from file
func Load(configFile string) error {
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.AddConfigPath("./.forecast")
		viper.AddConfigPath("$HOME/.forecast")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found
			return ConfigNotFoundError{}
		}
		return fmt.Errorf("failed to parse config: %w", err)
	}

	current = &Config{}
	if err := viper.Unmarshal(current); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	return nil
}

// IsLoaded returns true if a config has been loaded
func IsLoaded() bool {
	return current != nil && (current.JIRA.URL != "" || len(current.Projects) > 0)
}

// Get returns the current configuration
func Get() *Config {
	if current == nil {
		current = &Config{}
	}
	return current
}

// GetProject returns a project config by key or epic
func (c *Config) GetProject(keyOrEpic string) *ProjectConfig {
	for i := range c.Projects {
		if c.Projects[i].Key == keyOrEpic || c.Projects[i].Epic == keyOrEpic {
			return &c.Projects[i]
		}
	}
	return nil
}

// GetAllProjects returns all configured projects
func (c *Config) GetAllProjects() []ProjectConfig {
	return c.Projects
}

// GetJIRAInstance returns a JIRA instance config by name
// Falls back to default JIRA config if name is empty or not found
func (c *Config) GetJIRAInstance(name string) *JIRAConfig {
	if name == "" {
		return &c.JIRA
	}
	if c.JIRAInstances != nil {
		if instance, ok := c.JIRAInstances[name]; ok {
			return &instance
		}
	}
	// Fall back to default
	return &c.JIRA
}

// InitProject creates a new .forecast directory with default config
func InitProject() error {
	forecastDir := "./.forecast"

	if err := os.MkdirAll(forecastDir, 0755); err != nil {
		return fmt.Errorf("failed to create .forecast directory: %w", err)
	}

	configPath := filepath.Join(forecastDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config already exists at %s", configPath)
	}

	defaultConfig := `# Forecast Configuration
# See https://github.com/andrewcostello/forecast for documentation

# Project settings
project_name: "My Project"
project_type: "Web Application"
team_size: 3
team_capacity: 8  # Hours per day per team

jira:
  url: https://yourcompany.atlassian.net
  email: your.email@company.com
  api_token: ${JIRA_API_TOKEN}  # Set via environment variable
  project_key: PROJ
  epic: PROJ-123
  labels:
    - phase1-refactor
  # Optional: Custom field for manual cycle time override (in hours)
  # cycle_time_field: customfield_10001
  # Note: JIRA's built-in "Time Spent" field is also used as fallback

item_types:
  - name: Component
    jira_label: type:component
  - name: Fix
    jira_label: type:fix
  - name: Migration
    jira_label: type:migration
  - name: Integration
    jira_label: type:integration
  - name: Test
    jira_label: type:test
  - name: Extraction
    jira_label: type:extraction
  - name: Documentation
    jira_label: type:documentation

reference_class:
  name: "My Project Phase 1"
  type: "React Refactoring"

jira_mapping:
  item_type:
    field: labels
    mappings:
      - jira: "type:component"
        forecast: "Component"
      - jira: "type:fix"
        forecast: "Fix"
      - jira: "type:migration"
        forecast: "Migration"

  size:
    field: labels
    mappings:
      - jira: "size:S"
        forecast: "S"
      - jira: "size:M"
        forecast: "M"
      - jira: "size:L"
        forecast: "L"
      - jira: "size:XL"
        forecast: "XL"
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("✓ Created config at %s\n", configPath)
	fmt.Println("\nNext steps:")
	fmt.Println("1. Edit .forecast/config.yaml with your JIRA details")
	fmt.Println("2. Set JIRA_API_TOKEN environment variable")
	fmt.Println("3. Run 'forecast sync' to pull data from JIRA")

	return nil
}

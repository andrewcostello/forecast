package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	ProjectName    string                 `mapstructure:"project_name"`
	ProjectType    string                 `mapstructure:"project_type"`
	TeamSize       int                    `mapstructure:"team_size"`
	TeamCapacity   float64                `mapstructure:"team_capacity"` // Hours per day
	Source         SourceConfig           `mapstructure:"source"`         // Item source: jira (default) or authoritative yaml
	JIRA           JIRAConfig             `mapstructure:"jira"`
	JIRAInstances  map[string]JIRAConfig  `mapstructure:"jira_instances"` // Named JIRA instances for multi-JIRA support
	Projects       []ProjectConfig        `mapstructure:"projects"`       // Multiple project/epic tracking
	ItemTypes      []ItemTypeConfig       `mapstructure:"item_types"`
	ReferenceClass ReferenceClassConfig   `mapstructure:"reference_class"`
	JIRAMapping    JIRAMappingConfig      `mapstructure:"jira_mapping"`
}

// SourceConfig selects where forecast items come from.
//
//   - type: "jira" (or unset) — current behavior; sync pulls from JIRA.
//   - type: "yaml"            — yaml file is authoritative; sync reads it
//     directly and does not talk to JIRA. Use Path for a single-file project
//     (written to data.json), or Projects for multi-project layouts (each
//     entry written to data-{key}.json).
//
// Paths are resolved relative to the directory containing the config file when
// relative, or used as-is when absolute. Path and Projects are mutually
// exclusive; if both are set, Projects wins.
type SourceConfig struct {
	Type     string              `mapstructure:"type"`     // "jira" | "yaml"
	Path     string              `mapstructure:"path"`     // single-file yaml mode
	Projects []YAMLProjectConfig `mapstructure:"projects"` // multi-file yaml mode
}

// YAMLProjectConfig is one project in a yaml-authoritative multi-project setup.
type YAMLProjectConfig struct {
	Key  string `mapstructure:"key"`            // short key for CLI; drives data-{key}.json
	Name string `mapstructure:"name,omitempty"` // display name; defaults to Key
	Path string `mapstructure:"path"`           // path to this project's tasks yaml
}

// ProjectConfig defines a trackable project/initiative
type ProjectConfig struct {
	Name     string `mapstructure:"name"`      // Display name
	Epic     string `mapstructure:"epic"`      // Epic key (e.g., SMG-1688)
	Key      string `mapstructure:"key"`       // Short key for CLI (e.g., "monorepo")
	Capacity float64 `mapstructure:"capacity"` // Optional: team capacity for this project
	// JIRAInstance names a jira_instances entry that owns this project's
	// tickets. Empty = use the default jira: block.
	JIRAInstance string `mapstructure:"jira_instance"`
}

type JIRAConfig struct {
	URL              string   `mapstructure:"url"`
	Email            string   `mapstructure:"email"`
	APIToken         string   `mapstructure:"api_token"`
	ProjectKey       string   `mapstructure:"project_key"`
	// ProjectKeys lists additional issue-key prefixes (e.g. "FSG", "SMG")
	// owned by this JIRA instance. Used by GetJIRAInstanceForKey to route
	// per-ticket commands to the right instance. ProjectKey is always
	// considered first; ProjectKeys is for instances that span multiple
	// project prefixes.
	ProjectKeys      []string `mapstructure:"project_keys"`
	Epic             string   `mapstructure:"epic"`
	Labels           []string `mapstructure:"labels"`
	CycleTimeField   string   `mapstructure:"cycle_time_field"`   // Custom field ID for manual cycle time override (e.g., "customfield_10001")
	StoryPointsField string   `mapstructure:"story_points_field"` // Custom field ID for story points (e.g., "customfield_10004")
	// DoneStatuses lists Jira workflow statuses that should be treated as
	// "Done" for forecasting (cycle-time anchoring + completion counts).
	// Defaults to {"Done"} when unset. Use this to fold dev-complete-but-
	// not-yet-deployed states (e.g. "Awaiting Dev Deployment") into the
	// completed bucket so Monte Carlo has cycle-time samples while a
	// merge-to-main / deploy gate is still pending.
	DoneStatuses []string `mapstructure:"done_statuses"`
	// InProgressStatuses lists Jira workflow statuses that should be treated
	// as "In Progress" for forecasting (cycle-time start anchor + WIP counts).
	// Defaults to {"In Progress", "In Development"} when unset.
	InProgressStatuses []string `mapstructure:"in_progress_statuses"`
}

// EffectiveDoneStatuses returns the configured done-equivalent status list,
// or {"Done"} if unset. Always returns a non-empty slice.
func (j *JIRAConfig) EffectiveDoneStatuses() []string {
	if len(j.DoneStatuses) == 0 {
		return []string{"Done"}
	}
	return j.DoneStatuses
}

// EffectiveInProgressStatuses returns the configured in-progress-equivalent
// status list, or {"In Progress", "In Development"} if unset. Always returns
// a non-empty slice.
func (j *JIRAConfig) EffectiveInProgressStatuses() []string {
	if len(j.InProgressStatuses) == 0 {
		return []string{"In Progress", "In Development"}
	}
	return j.InProgressStatuses
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

	// Expand environment variables in API tokens (supports ${VAR} syntax in YAML)
	current.JIRA.APIToken = os.ExpandEnv(current.JIRA.APIToken)
	for name, inst := range current.JIRAInstances {
		inst.APIToken = os.ExpandEnv(inst.APIToken)
		current.JIRAInstances[name] = inst
	}

	return nil
}

// IsLoaded returns true if a config has been loaded with usable contents.
// JIRA configs need a URL or at least one project; authoritative-yaml configs
// need source.type=yaml and a non-empty path.
func IsLoaded() bool {
	if current == nil {
		return false
	}
	if current.JIRA.URL != "" || len(current.Projects) > 0 {
		return true
	}
	if strings.EqualFold(current.Source.Type, "yaml") &&
		(current.Source.Path != "" || len(current.Source.Projects) > 0) {
		return true
	}
	return false
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
	if strings.EqualFold(c.Source.Type, "yaml") {
		for _, yp := range c.Source.Projects {
			if yp.Key == keyOrEpic {
				p := yamlProjectToProjectConfig(yp)
				return &p
			}
		}
	}
	return nil
}

// GetAllProjects returns all configured projects. In yaml-authoritative mode
// with Source.Projects, the yaml projects are surfaced as synthetic
// ProjectConfigs so the existing multi-project run/report/dashboard flows
// work uniformly across JIRA and yaml sources.
func (c *Config) GetAllProjects() []ProjectConfig {
	if !strings.EqualFold(c.Source.Type, "yaml") || len(c.Source.Projects) == 0 {
		return c.Projects
	}
	out := make([]ProjectConfig, 0, len(c.Projects)+len(c.Source.Projects))
	out = append(out, c.Projects...)
	for _, yp := range c.Source.Projects {
		out = append(out, yamlProjectToProjectConfig(yp))
	}
	return out
}

func yamlProjectToProjectConfig(yp YAMLProjectConfig) ProjectConfig {
	name := yp.Name
	if name == "" {
		name = yp.Key
	}
	return ProjectConfig{Key: yp.Key, Name: name}
}

// ResolvePath resolves a config-relative path against the directory of the
// loaded config file. Absolute paths are returned unchanged. Returns p as-is
// when no config file is known (which happens in tests that construct Config
// values directly).
func ResolvePath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	cfgFile := viper.ConfigFileUsed()
	if cfgFile == "" {
		return p
	}
	return filepath.Join(filepath.Dir(cfgFile), p)
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

// GetJIRAInstanceForProject returns the JIRA instance that owns the given project.
func (c *Config) GetJIRAInstanceForProject(p *ProjectConfig) *JIRAConfig {
	if p == nil {
		return &c.JIRA
	}
	if p.JIRAInstance != "" {
		return c.GetJIRAInstance(p.JIRAInstance)
	}
	return c.GetJIRAInstanceForKey(p.Epic)
}

// GetJIRAInstanceForKey returns the JIRA instance that owns the given issue key,
// routed by its project-key prefix. Falls back to the default jira: block.
func (c *Config) GetJIRAInstanceForKey(issueKey string) *JIRAConfig {
	prefix := projectPrefix(issueKey)
	if prefix == "" {
		return &c.JIRA
	}
	if instanceClaimsPrefix(&c.JIRA, prefix) {
		return &c.JIRA
	}
	for name, inst := range c.JIRAInstances {
		if instanceClaimsPrefix(&inst, prefix) {
			out := c.JIRAInstances[name]
			return &out
		}
	}
	return &c.JIRA
}

// projectPrefix extracts the uppercased project-key prefix from an issue key
// (e.g. "SMG-1688" -> "SMG"). Returns "" for malformed keys.
func projectPrefix(issueKey string) string {
	idx := strings.IndexByte(issueKey, '-')
	if idx <= 0 {
		return ""
	}
	return strings.ToUpper(issueKey[:idx])
}

func instanceClaimsPrefix(j *JIRAConfig, prefix string) bool {
	if strings.EqualFold(j.ProjectKey, prefix) {
		return true
	}
	for _, k := range j.ProjectKeys {
		if strings.EqualFold(k, prefix) {
			return true
		}
	}
	return false
}

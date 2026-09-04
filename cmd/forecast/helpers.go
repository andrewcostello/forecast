package main

import (
	"fmt"
	"strings"

	"github.com/andrewcostello/forecast/internal/config"
	apperrors "github.com/andrewcostello/forecast/internal/errors"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/spf13/cobra"
)

// mustGetString returns the string flag value (empty if unset).
func mustGetString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// mustGetFloat returns the float64 flag value.
func mustGetFloat(cmd *cobra.Command, name string) float64 {
	v, _ := cmd.Flags().GetFloat64(name)
	return v
}

// mustGetBool returns the bool flag value.
func mustGetBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

// truncate shortens a string to the specified length, adding ".." if truncated
func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-2] + ".."
}

// splitAndTrim splits a string by separator and trims whitespace from each part
func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, p := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// getJiraClient creates a JIRA client from the loaded config
func getJiraClient() (*jira.Client, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, fmt.Errorf("failed to load config - run 'forecast init' first")
	}
	return jira.NewClient(&cfg.JIRA), nil
}

// getJiraClientForKey returns the JIRA client whose instance owns the given
// issue key (routed by project-key prefix). Use it for per-ticket commands so
// that, with multiple jira_instances configured, each ticket op hits the right
// instance. Falls back to the default jira: block when nothing claims the key.
func getJiraClientForKey(issueKey string) (*jira.Client, *config.JIRAConfig, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, nil, fmt.Errorf("failed to load config - run 'forecast init' first")
	}
	inst := cfg.GetJIRAInstanceForKey(issueKey)
	return jira.NewClient(inst), inst, nil
}

// printBanner prints a formatted section header
func printBanner(title string) {
	fmt.Println("\n" + strings.Repeat("━", 60))
	fmt.Printf("  %s\n", title)
	fmt.Println(strings.Repeat("━", 60))
}

// requireConfig returns the config or a user-friendly error
func requireConfig() (*config.Config, error) {
	cfg := config.Get()
	if cfg == nil || !config.IsLoaded() {
		return nil, apperrors.NoConfigError()
	}
	return cfg, nil
}

// requireProject resolves a project key and returns the project or an error
func requireProject(cfg *config.Config, projectKey string) (*config.ProjectConfig, error) {
	projects := cfg.GetAllProjects()
	available := make([]string, 0, len(projects))
	for _, p := range projects {
		if p.Key != "" {
			available = append(available, p.Key)
		} else {
			available = append(available, p.Epic)
		}
	}

	if projectKey == "" {
		return nil, apperrors.NoProjectError(available)
	}

	proj := cfg.GetProject(projectKey)
	if proj == nil {
		return nil, apperrors.ProjectNotFoundError(projectKey, available)
	}
	return proj, nil
}

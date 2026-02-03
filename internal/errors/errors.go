// Package errors provides user-friendly error types with suggestions.
package errors

import (
	"fmt"
	"strings"
)

// UserError represents an error with a suggestion for the user
type UserError struct {
	Message    string
	Suggestion string
	Cause      error
}

func (e *UserError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("%s\n\n  Suggestion: %s", e.Message, e.Suggestion)
	}
	return e.Message
}

func (e *UserError) Unwrap() error {
	return e.Cause
}

// Common errors with helpful suggestions

// NoConfigError is returned when no config file exists
func NoConfigError() error {
	return &UserError{
		Message:    "No configuration found",
		Suggestion: "Run 'forecast init' to set up your project",
	}
}

// ConfigLoadError is returned when config can't be parsed
func ConfigLoadError(cause error) error {
	return &UserError{
		Message:    fmt.Sprintf("Failed to load configuration: %v", cause),
		Suggestion: "Check your .forecast/config.yaml for syntax errors",
		Cause:      cause,
	}
}

// NoProjectError is returned when no project is specified or found
func NoProjectError(available []string) error {
	msg := "No project specified"
	suggestion := "Use 'forecast use <project>' to set a default, or specify with --project flag"

	if len(available) > 0 {
		suggestion += fmt.Sprintf("\n\n  Available projects: %s", strings.Join(available, ", "))
	}

	return &UserError{
		Message:    msg,
		Suggestion: suggestion,
	}
}

// ProjectNotFoundError is returned when a specified project doesn't exist
func ProjectNotFoundError(projectKey string, available []string) error {
	msg := fmt.Sprintf("Project '%s' not found", projectKey)
	suggestion := "Check the project key in your .forecast/config.yaml"

	if len(available) > 0 {
		suggestion += fmt.Sprintf("\n\n  Available projects: %s", strings.Join(available, ", "))
	}

	return &UserError{
		Message:    msg,
		Suggestion: suggestion,
	}
}

// NoDataError is returned when there's no data to process
func NoDataError(projectKey string) error {
	return &UserError{
		Message:    fmt.Sprintf("No data found for project '%s'", projectKey),
		Suggestion: "Run 'forecast sync' to pull data from JIRA first",
	}
}

// NoCycleTimeError is returned when there's no cycle time data for forecasting
func NoCycleTimeError() error {
	return &UserError{
		Message:    "No completed items with cycle time data",
		Suggestion: "Complete some items in JIRA and sync again, or log time on completed issues",
	}
}

// JIRAConnectionError is returned when JIRA connection fails
func JIRAConnectionError(cause error) error {
	suggestion := "Check your JIRA configuration:\n" +
		"  - Verify the URL is correct\n" +
		"  - Ensure your API token is valid\n" +
		"  - Confirm your email matches your JIRA account"

	if strings.Contains(cause.Error(), "401") {
		suggestion = "Authentication failed. Check your API token:\n" +
			"  1. Generate a new token at https://id.atlassian.com/manage-profile/security/api-tokens\n" +
			"  2. Update JIRA_API_TOKEN environment variable or .forecast/config.yaml"
	} else if strings.Contains(cause.Error(), "404") {
		suggestion = "Resource not found. Check:\n" +
			"  - JIRA URL is correct\n" +
			"  - Project key exists\n" +
			"  - You have access to the project"
	}

	return &UserError{
		Message:    fmt.Sprintf("JIRA connection failed: %v", cause),
		Suggestion: suggestion,
		Cause:      cause,
	}
}

// JIRAQueryError is returned when a JIRA query fails
func JIRAQueryError(jql string, cause error) error {
	return &UserError{
		Message:    fmt.Sprintf("JIRA query failed: %v", cause),
		Suggestion: fmt.Sprintf("Check that the epic exists and you have access.\n  Query: %s", jql),
		Cause:      cause,
	}
}

// InvalidArgumentError is returned for invalid command arguments
func InvalidArgumentError(message, suggestion string) error {
	return &UserError{
		Message:    message,
		Suggestion: suggestion,
	}
}

// FileWriteError is returned when a file can't be written
func FileWriteError(path string, cause error) error {
	return &UserError{
		Message:    fmt.Sprintf("Failed to write file '%s': %v", path, cause),
		Suggestion: "Check that you have write permissions to the directory",
		Cause:      cause,
	}
}

// Wrap wraps an error with a user-friendly message
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return &UserError{
		Message: message,
		Cause:   err,
	}
}

// WrapWithSuggestion wraps an error with a message and suggestion
func WrapWithSuggestion(err error, message, suggestion string) error {
	if err == nil {
		return nil
	}
	return &UserError{
		Message:    message,
		Suggestion: suggestion,
		Cause:      err,
	}
}

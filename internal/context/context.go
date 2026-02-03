// Package context manages the current project context for CLI commands.
package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const contextFile = ".forecast/context.json"

// Context represents the current working context
type Context struct {
	CurrentProject  string    `json:"current_project"`
	PreviousProject string    `json:"previous_project,omitempty"`
	LastSync        time.Time `json:"last_sync,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Load loads the current context from disk
func Load() (*Context, error) {
	data, err := os.ReadFile(contextFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &Context{}, nil
		}
		return nil, err
	}

	var ctx Context
	if err := json.Unmarshal(data, &ctx); err != nil {
		return &Context{}, nil // Return empty context on parse error
	}
	return &ctx, nil
}

// Save saves the context to disk
func (c *Context) Save() error {
	c.UpdatedAt = time.Now()

	// Ensure directory exists
	dir := filepath.Dir(contextFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(contextFile, data, 0644)
}

// SetProject sets the current project (preserves previous for toggle)
func SetProject(projectKey string) error {
	ctx, err := Load()
	if err != nil {
		ctx = &Context{}
	}
	// Store current as previous before switching (if different)
	if ctx.CurrentProject != "" && ctx.CurrentProject != projectKey {
		ctx.PreviousProject = ctx.CurrentProject
	}
	ctx.CurrentProject = projectKey
	return ctx.Save()
}

// GetPreviousProject returns the previous project key
func GetPreviousProject() string {
	ctx, err := Load()
	if err != nil {
		return ""
	}
	return ctx.PreviousProject
}

// ToggleProject switches to the previous project and returns the new current
func ToggleProject() (string, error) {
	ctx, err := Load()
	if err != nil {
		return "", err
	}
	if ctx.PreviousProject == "" {
		return "", nil // No previous project to toggle to
	}
	// Swap current and previous
	ctx.CurrentProject, ctx.PreviousProject = ctx.PreviousProject, ctx.CurrentProject
	if err := ctx.Save(); err != nil {
		return "", err
	}
	return ctx.CurrentProject, nil
}

// GetProject returns the current project key
func GetProject() string {
	ctx, err := Load()
	if err != nil {
		return ""
	}
	return ctx.CurrentProject
}

// SetLastSync records the last sync time
func SetLastSync(t time.Time) error {
	ctx, err := Load()
	if err != nil {
		ctx = &Context{}
	}
	ctx.LastSync = t
	return ctx.Save()
}

// GetLastSync returns the last sync time
func GetLastSync() time.Time {
	ctx, err := Load()
	if err != nil {
		return time.Time{}
	}
	return ctx.LastSync
}

// Clear clears the current context
func Clear() error {
	return os.Remove(contextFile)
}

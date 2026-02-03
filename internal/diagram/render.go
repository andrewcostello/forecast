package diagram

import (
	"bytes"
	"fmt"
	"os/exec"
)

// Renderer handles D2 diagram rendering
type Renderer struct {
	d2Binary string // Path to d2 binary
}

// NewRenderer creates a new diagram renderer
func NewRenderer() *Renderer {
	return &Renderer{
		d2Binary: "d2", // Assume d2 is in PATH
	}
}

// RenderSVG renders D2 code to SVG using the d2 CLI
func (r *Renderer) RenderSVG(d2Code string) ([]byte, error) {
	// Check if d2 is available
	if _, err := exec.LookPath(r.d2Binary); err != nil {
		return nil, fmt.Errorf("d2 CLI not found: %w (install from https://d2lang.com)", err)
	}

	// Run d2 with stdin/stdout
	cmd := exec.Command(r.d2Binary, "-", "-")

	var stdout, stderr bytes.Buffer
	cmd.Stdin = bytes.NewBufferString(d2Code)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("d2 render failed: %w: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// RenderToFile renders D2 code to a file
func (r *Renderer) RenderToFile(d2Code, outputPath string) error {
	// Check if d2 is available
	if _, err := exec.LookPath(r.d2Binary); err != nil {
		return fmt.Errorf("d2 CLI not found: %w (install from https://d2lang.com)", err)
	}

	// Write D2 code to temp file and render
	cmd := exec.Command(r.d2Binary, "-", outputPath)

	var stderr bytes.Buffer
	cmd.Stdin = bytes.NewBufferString(d2Code)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("d2 render failed: %w: %s", err, stderr.String())
	}

	return nil
}

// IsD2Available checks if the d2 CLI is available
func (r *Renderer) IsD2Available() bool {
	_, err := exec.LookPath(r.d2Binary)
	return err == nil
}

// RenderWithFallback tries to render with d2, falls back to built-in SVG
func (g *Generator) RenderWithFallback(diagramType DiagramType, renderer *Renderer) ([]byte, string, error) {
	// If d2 is available, use it
	if renderer != nil && renderer.IsD2Available() {
		d2Code := g.Generate(diagramType)
		svg, err := renderer.RenderSVG(d2Code)
		if err == nil {
			return svg, "image/svg+xml", nil
		}
		// Fall through to fallback on error
	}

	// Fallback to built-in SVG generation
	var svg string
	switch diagramType {
	case TypeBurndown:
		svg = g.GenerateBurndownSVG()
	case TypeFlow:
		svg = g.GenerateFlowSVG()
	case TypeTimeline:
		svg = g.GenerateTimelineSVG()
	default:
		svg = g.GenerateBurndownSVG()
	}

	return []byte(svg), "image/svg+xml", nil
}

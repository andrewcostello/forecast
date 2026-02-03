// Package terminal provides interactive terminal input utilities.
package terminal

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Prompter handles interactive terminal prompts
type Prompter struct {
	reader *bufio.Reader
}

// NewPrompter creates a new terminal prompter
func NewPrompter() *Prompter {
	return &Prompter{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Prompt asks for text input with a default value
func (p *Prompter) Prompt(prompt string, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

// PromptRequired asks for required text input
func (p *Prompter) PromptRequired(prompt string) (string, error) {
	for {
		fmt.Printf("%s: ", prompt)
		input, err := p.reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		input = strings.TrimSpace(input)
		if input != "" {
			return input, nil
		}
		fmt.Println("  This field is required. Please enter a value.")
	}
}

// PromptSecret asks for password/secret input (hidden)
func (p *Prompter) PromptSecret(prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)

	// Try to use terminal for hidden input
	if term.IsTerminal(int(os.Stdin.Fd())) {
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // New line after hidden input
		if err != nil {
			return "", err
		}
		return string(password), nil
	}

	// Fallback to regular input (for non-TTY environments)
	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

// PromptConfirm asks for yes/no confirmation
func (p *Prompter) PromptConfirm(prompt string, defaultYes bool) (bool, error) {
	defaultStr := "y/N"
	if defaultYes {
		defaultStr = "Y/n"
	}

	fmt.Printf("%s [%s]: ", prompt, defaultStr)
	input, err := p.reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultYes, nil
	}

	return input == "y" || input == "yes", nil
}

// PromptSelect asks user to select from options
func (p *Prompter) PromptSelect(prompt string, options []string, defaultIndex int) (int, string, error) {
	fmt.Printf("\n%s:\n", prompt)
	for i, opt := range options {
		marker := "  "
		if i == defaultIndex {
			marker = "> "
		}
		fmt.Printf("%s%d. %s\n", marker, i+1, opt)
	}

	for {
		defaultDisplay := ""
		if defaultIndex >= 0 && defaultIndex < len(options) {
			defaultDisplay = fmt.Sprintf("%d", defaultIndex+1)
		}

		var input string
		var err error
		if defaultDisplay != "" {
			input, err = p.Prompt("Select option", defaultDisplay)
		} else {
			input, err = p.PromptRequired("Select option")
		}
		if err != nil {
			return -1, "", err
		}

		// Parse selection
		var selected int
		if _, err := fmt.Sscanf(input, "%d", &selected); err == nil {
			if selected >= 1 && selected <= len(options) {
				return selected - 1, options[selected-1], nil
			}
		}
		fmt.Printf("  Please enter a number between 1 and %d\n", len(options))
	}
}

// PrintSuccess prints a success message
func PrintSuccess(format string, args ...interface{}) {
	fmt.Printf("✓ "+format+"\n", args...)
}

// PrintError prints an error message
func PrintError(format string, args ...interface{}) {
	fmt.Printf("✗ "+format+"\n", args...)
}

// PrintInfo prints an info message
func PrintInfo(format string, args ...interface{}) {
	fmt.Printf("→ "+format+"\n", args...)
}

// PrintWarning prints a warning message
func PrintWarning(format string, args ...interface{}) {
	fmt.Printf("⚠ "+format+"\n", args...)
}

// PrintHeader prints a section header
func PrintHeader(title string) {
	fmt.Printf("\n━━━ %s ━━━\n\n", title)
}

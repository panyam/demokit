package tui

import (
	"bufio"
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/panyam/demokit"
)

// FormPrompter collects a step's declared inputs from the user and
// returns the typed payload. Implementations may render a styled form
// (huh-backed) or a plain readline loop. Callers attach a custom
// prompter to a Renderer via Renderer.WithPrompter.
type FormPrompter interface {
	Prompt(stepID string, inputs []demokit.InputDef) map[string]any
}

// ReadlinePrompter prompts for inputs sequentially via stdin readline,
// applying a foreground color from the renderer's palette. On a Parse
// error it collects the rest of the line, prints the error inline, and
// re-prompts every field — using each just-typed valid value as the
// next default so the user only retypes the bad one (Enter to keep).
//
// This is the default prompter for tui.Renderer; swap in a richer
// implementation (e.g. one backed by github.com/charmbracelet/huh) via
// Renderer.WithPrompter if a styled form is desired.
type ReadlinePrompter struct {
	PromptColor color.Color
	ErrorColor  color.Color
}

// Prompt implements FormPrompter.
func (p *ReadlinePrompter) Prompt(stepID string, inputs []demokit.InputDef) map[string]any {
	if len(inputs) == 0 {
		return map[string]any{}
	}
	promptStyle := lipgloss.NewStyle().Foreground(p.PromptColor)
	errStyle := lipgloss.NewStyle().Foreground(p.ErrorColor)

	pending := make([]demokit.InputDef, len(inputs))
	copy(pending, inputs)
	stdin := bufio.NewReader(os.Stdin)

	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			fmt.Println(promptStyle.Render("  Re-enter values (Enter to keep [bracketed]):"))
		}
		result := map[string]any{}
		errored := false
		for i, in := range pending {
			label := in.Prompt
			if label == "" {
				label = in.Name
			}
			if in.Default != nil {
				fmt.Print(promptStyle.Render(fmt.Sprintf("  %s [%v]: ", label, in.Default)))
			} else {
				fmt.Print(promptStyle.Render(fmt.Sprintf("  %s: ", label)))
			}
			line, _ := stdin.ReadString('\n')
			line = strings.TrimRight(line, "\r\n")

			if line == "" && in.Default != nil {
				result[in.Name] = in.Default
				continue
			}

			parser := in.Parse
			if parser == nil {
				parser = func(s string) (any, error) { return s, nil }
			}
			val, err := parser(line)
			if err != nil {
				fmt.Println(errStyle.Render(fmt.Sprintf("  [error] %v", err)))
				errored = true
				continue
			}
			result[in.Name] = val
			pending[i].Default = val
		}
		if !errored {
			return result
		}
	}
}

package demokit

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// ColorRule maps a log line to an ANSI color code based on its content.
// The first matching rule wins. If no rule matches, the default color is used.
type ColorRule struct {
	// Contains is the substring to match in the log line.
	Contains string
	// DarkColor is the ANSI escape for dark terminal backgrounds.
	DarkColor string
	// LightColor is the ANSI escape for light terminal backgrounds.
	// If empty, DarkColor is used for both.
	LightColor string
}

// Common ANSI escape codes for use in ColorRule definitions.
const (
	ANSIRed          = "\033[31m"
	ANSIGreen        = "\033[32m"
	ANSIYellow       = "\033[33m"
	ANSIBlue         = "\033[34m"
	ANSICyan         = "\033[36m"
	ANSIGray         = "\033[37m"  // light gray — subdued but readable on dark backgrounds
	ANSIBrightRed    = "\033[91m"
	ANSIBrightGreen  = "\033[92m"
	ANSIBrightYellow = "\033[93m"
	ANSIBrightCyan   = "\033[96m"
	ANSIBrightWhite  = "\033[97m"
	ANSIDimCyan      = "\033[2;36m"
	ANSIDimBlue      = "\033[2;34m"
)

// NewColorLogger creates a *log.Logger with colorized output.
// Lines are colored based on the provided rules (first match wins).
// Dark/light terminal background is auto-detected via lipgloss.
//
// If rules is nil, all lines are written without color.
//
// Example:
//
//	logger := demokit.NewColorLogger("[mcp] ", []demokit.ColorRule{
//	    {Contains: "error", DarkColor: demokit.ANSIRed},
//	    {Contains: "[http] →", DarkColor: demokit.ANSIDimCyan, LightColor: demokit.ANSIDimBlue},
//	    {Contains: "MCP ", DarkColor: demokit.ANSIGreen},
//	})
func NewColorLogger(prefix string, rules []ColorRule) *log.Logger {
	return log.New(newColorWriter(os.Stderr, rules), prefix, log.LstdFlags)
}

type colorWriter struct {
	w     io.Writer
	light bool
	rules []ColorRule
}

func newColorWriter(w io.Writer, rules []ColorRule) *colorWriter {
	cw := &colorWriter{w: w, rules: rules}
	cw.light = !lipgloss.HasDarkBackground(os.Stdin, os.Stderr)
	return cw
}

func (cw *colorWriter) Write(p []byte) (int, error) {
	if len(cw.rules) == 0 {
		return cw.w.Write(p)
	}

	s := string(p)
	reset := "\033[0m"

	for _, r := range cw.rules {
		if strings.Contains(s, r.Contains) {
			color := r.DarkColor
			if cw.light && r.LightColor != "" {
				color = r.LightColor
			}
			_, err := fmt.Fprintf(cw.w, "%s%s%s", color, s, reset)
			return len(p), err
		}
	}

	// No rule matched — write without color.
	return cw.w.Write(p)
}

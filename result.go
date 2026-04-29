package demokit

import (
	"fmt"
	"time"
)

// ResultStatus controls how a step's result is rendered.
type ResultStatus int

const (
	StatusSuccess ResultStatus = iota // green / "Result"
	StatusError                       // red / "Error"
	StatusWarning                     // yellow / "Warning"
	StatusInfo                        // blue / "Info"
)

// DefaultLabel returns the default display label for a status.
func (s ResultStatus) DefaultLabel() string {
	switch s {
	case StatusError:
		return "Error"
	case StatusWarning:
		return "Warning"
	case StatusInfo:
		return "Info"
	default:
		return "Result"
	}
}

// StepResult carries the outcome of a step's run function.
// A nil *StepResult from Run means success with no message.
// The zero value is also success.
type StepResult struct {
	Status  ResultStatus // controls color/border styling
	Label   string       // custom title; empty = auto from Status
	Message string       // shown prominently (error text, info note, etc.)
	Err     error        // underlying error, if any
	Next    string       // step ID to jump to; empty = fall through to declaration order
}

// DisplayLabel returns Label if set, otherwise the default for the Status.
func (r *StepResult) DisplayLabel() string {
	if r.Label != "" {
		return r.Label
	}
	return r.Status.DefaultLabel()
}

// StepContext is passed to a step's run function. It carries the resolved
// input payload and visit count. Future fields may include trace metadata.
type StepContext struct {
	// Inputs holds the raw map payload collected from the renderer.
	// Values are typed (e.g. int, string) according to each InputDef's Parse.
	// Empty map if the step declared no inputs.
	Inputs map[string]any
	// Input is the coalesced typed payload returned by the step's Coalesce
	// function. If Coalesce was not set, Input == Inputs.
	Input any
	// Visits is the number of times this step has been entered, including
	// the current visit (so the first visit is 1).
	Visits int
}

// WaitOpts configures the renderer's WaitForStep prompt.
type WaitOpts struct {
	// AutoAcceptAfter, if > 0, advances the demo automatically after this
	// duration even without user input. Zero means wait indefinitely.
	AutoAcceptAfter time.Duration
	// ShowCountdown, when true, asks the renderer to display a visible
	// countdown / progress indicator while AutoAcceptAfter is in effect.
	ShowCountdown bool
}

// --- Convenience constructors for common result types ---

// Err creates an error result from an error.
func Err(err error) *StepResult {
	return &StepResult{Status: StatusError, Message: err.Error(), Err: err}
}

// Errf creates an error result from a formatted string.
func Errf(format string, args ...any) *StepResult {
	msg := fmt.Sprintf(format, args...)
	return &StepResult{Status: StatusError, Message: msg, Err: fmt.Errorf("%s", msg)}
}

// Warn creates a warning result.
func Warn(msg string) *StepResult {
	return &StepResult{Status: StatusWarning, Message: msg}
}

// Info creates an informational result.
func Info(msg string) *StepResult {
	return &StepResult{Status: StatusInfo, Message: msg}
}

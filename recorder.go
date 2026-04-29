package demokit

import (
	"encoding/json"
	"io"
	"os"
)

// TraceKind discriminates between recorded items.
type TraceKind string

const (
	// KindStep marks a step visit (with inputs, output, and result).
	KindStep TraceKind = "step"
	// KindSection marks a non-executable section visit.
	KindSection TraceKind = "section"
)

// TraceEntry is a single recorded item from an Execute run. The same
// entry shape covers steps and sections — only the fields relevant to
// the kind are populated. Designed for round-trip replay and for use
// as the input to documentation renderers (markdown, html).
type TraceEntry struct {
	Kind    TraceKind      `json:"kind"`
	Title   string         `json:"title,omitempty"`
	StepID  string         `json:"step_id,omitempty"`
	Visit   int            `json:"visit,omitempty"`
	Inputs  map[string]any `json:"inputs,omitempty"`
	Output  string         `json:"output,omitempty"`
	Status  ResultStatus   `json:"status,omitempty"`
	Label   string         `json:"label,omitempty"`
	Message string         `json:"message,omitempty"`
	Next    string         `json:"next,omitempty"` // resolved successor; empty = fall-through
	Body    string         `json:"body,omitempty"` // section body
}

// Recorder observes a demo as it runs. Implementations should retain
// entries cheaply; persistence (if any) belongs in Close.
type Recorder interface {
	Record(TraceEntry)
}

// MemoryRecorder collects entries in memory. Useful for tests and as a
// building block for other recorders.
type MemoryRecorder struct {
	Entries []TraceEntry
}

// Record appends to the in-memory slice.
func (r *MemoryRecorder) Record(e TraceEntry) {
	r.Entries = append(r.Entries, e)
}

// JSONFileRecorder writes a recorded trace to a JSON file at Close time.
// The file is overwritten on each Close.
type JSONFileRecorder struct {
	Path    string
	Entries []TraceEntry
}

// NewJSONFileRecorder constructs a recorder that will write to path.
func NewJSONFileRecorder(path string) *JSONFileRecorder {
	return &JSONFileRecorder{Path: path}
}

// Record appends to the in-memory buffer.
func (r *JSONFileRecorder) Record(e TraceEntry) {
	r.Entries = append(r.Entries, e)
}

// Close serializes the buffered entries to disk as a JSON array.
func (r *JSONFileRecorder) Close() error {
	f, err := os.Create(r.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(r.Entries)
}

// LoadTrace reads and decodes a JSON trace file produced by JSONFileRecorder.
func LoadTrace(path string) ([]TraceEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var entries []TraceEntry
	if err := json.NewDecoder(f).Decode(&entries); err != nil && err != io.EOF {
		return nil, err
	}
	return entries, nil
}

// closeRecorder flushes a recorder if it implements io.Closer. Used by
// Execute at the end of a run.
func closeRecorder(r Recorder) error {
	if c, ok := r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

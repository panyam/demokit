package demokit

import "github.com/panyam/demokit/events"

// This file holds the conversion + emit helpers that bridge
// demokit's internal types (StepDef, ArrowView, etc.) into the
// events package's public projection types. Keeping them
// separate from demokit.go's main flow makes the lifecycle hooks
// in RunLoop more readable.

// stepStartEvent builds the events.StepStart payload for a step
// visit. Projects arrows / refs / verbatims into their public
// shapes (no demokit-internal types in the event).
func stepStartEvent(visit int, s *StepDef) events.StepStart {
	return events.StepStart{
		Visit:     visit,
		StepID:    s.id,
		Title:     s.title,
		Note:      s.note,
		Arrows:    arrowsToEvents(s.Arrows()),
		Refs:      refsToEvents(s.Refs()),
		Verbatims: verbatimsToEvents(s.VerbatimBlocks()),
	}
}

// stepEndEvent builds the events.StepEnd payload from a Run's
// result. result == nil means "no message, success."
func stepEndEvent(visit int, result *StepResult) events.StepEnd {
	e := events.StepEnd{Visit: visit, Status: "ok"}
	if result == nil {
		return e
	}
	e.Status = statusString(result.Status)
	e.Message = result.Message
	if result.Err != nil {
		e.ErrorText = result.Err.Error()
	}
	return e
}

// arrowsToEvents projects ArrowView slice into events.Arrow.
func arrowsToEvents(in []ArrowView) []events.Arrow {
	if len(in) == 0 {
		return nil
	}
	out := make([]events.Arrow, len(in))
	for i, a := range in {
		out[i] = events.Arrow{From: a.From, To: a.To, Label: a.Label, Dashed: a.Dashed}
	}
	return out
}

// refsToEvents projects Ref slice into events.Ref.
func refsToEvents(in []Ref) []events.Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]events.Ref, len(in))
	for i, r := range in {
		out[i] = events.Ref{Name: r.Name, URL: r.URL}
	}
	return out
}

// verbatimsToEvents projects VerbatimView slice into events.Verbatim.
func verbatimsToEvents(in []VerbatimView) []events.Verbatim {
	if len(in) == 0 {
		return nil
	}
	out := make([]events.Verbatim, len(in))
	for i, vb := range in {
		variants := make([]events.Variant, len(vb.Variants))
		for j, v := range vb.Variants {
			variants[j] = events.Variant{
				Label: v.Label, Lang: v.Lang, Content: v.Content, IsDefault: v.IsDefault,
			}
		}
		out[i] = events.Verbatim{Label: vb.Label, Variants: variants}
	}
	return out
}

// inputsToEvents projects InputDef slice into typed events.Input
// values (StringInput / IntInput / ChoiceInput). The runtime
// Parse closure is dropped — consumers re-derive validation from
// the concrete type via type-switch. Unknown kinds project to
// StringInput.
func inputsToEvents(in []InputDef) []events.Input {
	if len(in) == 0 {
		return nil
	}
	out := make([]events.Input, len(in))
	for i, d := range in {
		switch d.Kind {
		case "int":
			out[i] = events.NewIntInput(d.Name, d.Prompt, d.Default)
		case "choice":
			out[i] = events.NewChoiceInput(d.Name, d.Prompt, d.Default, d.Options)
		default:
			out[i] = events.NewStringInput(d.Name, d.Prompt, d.Default)
		}
	}
	return out
}

// statusString maps ResultStatus to the public string form used
// in events.StepEnd.Status. Mirrors result.go's StringerImpl
// behavior but in lower-case to match what wire formats expect.
func statusString(s ResultStatus) string {
	switch s {
	case StatusError:
		return "error"
	case StatusWarning:
		return "warning"
	case StatusInfo:
		return "info"
	default:
		return "ok"
	}
}

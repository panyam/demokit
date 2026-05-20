package notebook

import (
	"sync"
	"time"
)

// InputResponse is what AwaitInput / AwaitInputBy returns.
//
// Source classifies how the wait ended:
//   - "user-submitted": the PromptCell emitted PromptSubmittedMsg.
//   - "cancelled": Stop was called, or the prompt cell was Removed,
//     while the wait was pending.
type InputResponse struct {
	Answers map[string]any
	Source  string
	At      time.Time
}

// rendezvous registers pending AwaitInput / AwaitInputBy calls.
// The model resolves them when a PromptCell submits; Stop drains
// them with Source: "cancelled" so no caller goroutine is left
// blocked.
//
// All channels are buffered cap 1 so resolveInput never blocks
// even if the awaiter has already returned via a different select
// arm (e.g. <-stopped).
type rendezvous struct {
	mu     sync.Mutex
	inputs map[CellID]chan InputResponse
}

func newRendezvous() *rendezvous {
	return &rendezvous{inputs: map[CellID]chan InputResponse{}}
}

// registerInput enrols a waiter keyed by the prompt cell's ID.
func (r *rendezvous) registerInput(id CellID) chan InputResponse {
	ch := make(chan InputResponse, 1)
	r.mu.Lock()
	r.inputs[id] = ch
	r.mu.Unlock()
	return ch
}

// removeInput drops the waiter for id without resolving it.
func (r *rendezvous) removeInput(id CellID) {
	r.mu.Lock()
	delete(r.inputs, id)
	r.mu.Unlock()
}

// resolveInput hands the waiter for id (if any) its response.
func (r *rendezvous) resolveInput(id CellID, answers map[string]any, source string) bool {
	r.mu.Lock()
	ch, ok := r.inputs[id]
	if ok {
		delete(r.inputs, id)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- InputResponse{Answers: answers, Source: source, At: time.Now()}:
	default:
	}
	return true
}

// cancelAll resolves every pending input waiter with
// Source: "cancelled". Called from Stop.
func (r *rendezvous) cancelAll() {
	r.mu.Lock()
	inputs := r.inputs
	r.inputs = map[CellID]chan InputResponse{}
	r.mu.Unlock()

	for _, ch := range inputs {
		select {
		case ch <- InputResponse{Source: "cancelled", At: time.Now()}:
		default:
		}
	}
}

// --- Notebook public API ---

// AwaitInputBy blocks until the cell with cellID emits a
// PromptSubmittedMsg, the cell is Removed, or Stop is called.
// The caller is responsible for having Appended the prompt cell.
// Use AwaitInput for the convenience that builds + appends the
// cell via the configured PromptFactory.
func (nb *Notebook) AwaitInputBy(cellID CellID) InputResponse {
	ch := nb.rdv.registerInput(cellID)
	select {
	case resp := <-ch:
		return resp
	case <-nb.stopped:
		nb.rdv.removeInput(cellID)
		return InputResponse{Source: "cancelled", At: time.Now()}
	}
}

// PromptFactory builds a prompt cell from a list of Inputs. The
// notebook package can't reference its own notebook/cells
// subpackage (cells imports notebook), so consumers wire a factory
// at construction time — cells.PromptFactory() returns one for the
// built-in PromptCell.
type PromptFactory func(id CellID, inputs []Input) Cell

// AwaitInput is the convenience that builds a prompt cell from
// the configured PromptFactory, Appends it, and waits for the
// user to submit. Panics if no factory was configured.
func (nb *Notebook) AwaitInput(inputs []Input) InputResponse {
	if nb.promptFactory == nil {
		panic("notebook: AwaitInput requires WithPromptFactory(...) at New() — use AwaitInputBy for caller-supplied cells")
	}
	id := nb.nextPromptID()
	cell := nb.promptFactory(id, inputs)
	if _, err := nb.Append(cell); err != nil {
		// Duplicate ID — shouldn't happen because nextPromptID is
		// monotonic per Notebook, but surface it via cancelled
		// rather than blocking forever.
		return InputResponse{Source: "cancelled", At: time.Now()}
	}
	return nb.AwaitInputBy(id)
}

package notebook

import (
	"sync"
	"time"
)

// AdvanceResponse is what AwaitAdvance returns.
//
// Source classifies how the wait ended:
//   - "user-enter": the user pressed Enter in SelectMode.
//   - "auto-advance": the deadline fired before any user input.
//   - "cancelled": Stop was called while the wait was pending.
type AdvanceResponse struct {
	Source string
	At     time.Time
}

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

// rendezvous registers pending AwaitAdvance / AwaitInput calls.
// The model resolves them when the user advances or a PromptCell
// submits; Stop drains them with Source: "cancelled" so no caller
// goroutine is left blocked.
//
// All channels are buffered cap 1 so resolveX never blocks even
// if the awaiter has already returned via a different select arm
// (e.g. deadline timer fired first).
type rendezvous struct {
	mu       sync.Mutex
	advances []chan AdvanceResponse
	inputs   map[CellID]chan InputResponse
}

func newRendezvous() *rendezvous {
	return &rendezvous{inputs: map[CellID]chan InputResponse{}}
}

// registerAdvance enrols a new pending advance and returns its
// resolution channel. FIFO: resolveAdvance pops the first.
func (r *rendezvous) registerAdvance() chan AdvanceResponse {
	ch := make(chan AdvanceResponse, 1)
	r.mu.Lock()
	r.advances = append(r.advances, ch)
	r.mu.Unlock()
	return ch
}

// removeAdvance unenrols ch (used when the deadline timer fired
// or Stop closed the global stop channel before any user advance).
func (r *rendezvous) removeAdvance(ch chan AdvanceResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, w := range r.advances {
		if w == ch {
			r.advances = append(r.advances[:i], r.advances[i+1:]...)
			return
		}
	}
}

// resolveAdvance hands the next pending awaiter its response.
// Returns true if there was one to resolve. Send is non-blocking
// because the chan is buffered cap 1.
func (r *rendezvous) resolveAdvance(source string) bool {
	r.mu.Lock()
	if len(r.advances) == 0 {
		r.mu.Unlock()
		return false
	}
	ch := r.advances[0]
	r.advances = r.advances[1:]
	r.mu.Unlock()
	select {
	case ch <- AdvanceResponse{Source: source, At: time.Now()}:
	default:
	}
	return true
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

// cancelAll resolves every pending advance and input with
// Source: "cancelled". Called from Stop.
func (r *rendezvous) cancelAll() {
	r.mu.Lock()
	advances := r.advances
	r.advances = nil
	inputs := r.inputs
	r.inputs = map[CellID]chan InputResponse{}
	r.mu.Unlock()

	for _, ch := range advances {
		select {
		case ch <- AdvanceResponse{Source: "cancelled", At: time.Now()}:
		default:
		}
	}
	for _, ch := range inputs {
		select {
		case ch <- InputResponse{Source: "cancelled", At: time.Now()}:
		default:
		}
	}
}

// --- Notebook public API ---

// AwaitAdvance blocks until the user advances (Enter in
// SelectMode), the deadline fires, or Stop is called.
//
// A zero deadline (time.Time{}) means no auto-advance — the call
// blocks until a user advance or Stop. Otherwise the call returns
// with Source: "auto-advance" when the deadline elapses.
func (nb *Notebook) AwaitAdvance(deadline time.Time) AdvanceResponse {
	ch := nb.rdv.registerAdvance()
	var timerC <-chan time.Time
	if !deadline.IsZero() {
		t := time.NewTimer(time.Until(deadline))
		defer t.Stop()
		timerC = t.C
	}
	select {
	case resp := <-ch:
		return resp
	case <-timerC:
		nb.rdv.removeAdvance(ch)
		return AdvanceResponse{Source: "auto-advance", At: time.Now()}
	case <-nb.stopped:
		nb.rdv.removeAdvance(ch)
		return AdvanceResponse{Source: "cancelled", At: time.Now()}
	}
}

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

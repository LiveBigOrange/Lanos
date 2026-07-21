package transfer

import (
	"errors"
	"fmt"
	"sort"
)

// ErrInvalidTransition is returned when a state transition is not allowed by
// the transfer state machine.
var ErrInvalidTransition = errors.New("transfer: invalid state transition")

// defaultTransitions defines every legal state change for a Lanos transfer.
//
// Lifecycle (PRD §5.3 / IMPLEMENTATION_ROADMAP P1-20):
//
//	pending (待确认) -> connecting (连接中) -> transferring (传输中) -> completed (已完成)
//	                                          |                    |
//	                                          +-> awaiting_resume --+-> connecting (resume)
//	                                          +-> failed / cancelled
//	pending -> cancelled (rejected) / failed (timeout)
//	failed  -> connecting (manual retry)
//
// Terminal states (no outgoing edges): completed, cancelled.
var defaultTransitions = map[Status]map[Status]struct{}{
	StatusPending: {
		StatusConnecting: {},
		StatusCancelled:  {},
		StatusFailed:     {},
	},
	StatusConnecting: {
		StatusTransferring:   {},
		StatusFailed:         {},
		StatusCancelled:      {},
		StatusAwaitingResume: {},
	},
	StatusTransferring: {
		StatusCompleted:      {},
		StatusFailed:         {},
		StatusCancelled:      {},
		StatusAwaitingResume: {},
	},
	StatusAwaitingResume: {
		StatusConnecting: {},
		StatusCancelled:  {},
		StatusFailed:     {},
	},
	StatusFailed: {
		StatusConnecting: {},
	},
	StatusCompleted: {},
	StatusCancelled: {},
}

// StateMachine enforces the valid lifecycle of a single transfer. It is not
// safe for concurrent use on its own; callers (the [Manager]) must hold the
// surrounding mutex when driving transitions.
type StateMachine struct {
	current Status
	table   map[Status]map[Status]struct{}
}

// NewStateMachine returns a StateMachine starting in [initial] using the
// default Lanos transition table.
func NewStateMachine(initial Status) *StateMachine {
	return &StateMachine{current: initial, table: defaultTransitions}
}

// State returns the current status.
func (sm *StateMachine) State() Status { return sm.current }

// IsTerminal reports whether the current state permits no further transitions
// (completed or cancelled).
func (sm *StateMachine) IsTerminal() bool {
	next, ok := sm.table[sm.current]
	return ok && len(next) == 0
}

// CanTransition reports whether moving to [next] is permitted from the current
// state.
func (sm *StateMachine) CanTransition(next Status) bool {
	allowed, ok := sm.table[sm.current]
	if !ok {
		return false
	}
	_, ok = allowed[next]
	return ok
}

// Transition moves to [next] if allowed. Returns [ErrInvalidTransition]
// otherwise; the current state is left unchanged.
func (sm *StateMachine) Transition(next Status) error {
	if !sm.CanTransition(next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, sm.current, next)
	}
	sm.current = next
	return nil
}

// ValidTransitions returns the states reachable from the current state, sorted
// for deterministic test output.
func (sm *StateMachine) ValidTransitions() []Status {
	allowed, ok := sm.table[sm.current]
	if !ok {
		return nil
	}
	out := make([]Status, 0, len(allowed))
	for s := range allowed {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

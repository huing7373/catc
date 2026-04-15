// Package fsm provides a minimal finite-state-machine primitive ported
// from the P2 server. It is intentionally small: states are plain string
// identifiers and transitions are validated via a declared table.
//
// The cat watch app domain (Mirror/Active/Sleep/…) will use this package
// from its domain layer.
package fsm

import (
	"errors"
	"sync"
)

// State is an opaque state identifier.
type State string

// Transition describes a legal move from one State to another. An empty
// From slice means "from any state". When more than one transition shares
// the same (From, Event), the FSM refuses to construct.
type Transition struct {
	Event string
	From  []State
	To    State
}

// ErrIllegalTransition is returned when Fire is called for an event that
// is not legal from the current state.
var ErrIllegalTransition = errors.New("fsm: illegal transition")

// FSM is a thread-safe finite state machine.
type FSM struct {
	mu       sync.RWMutex
	current  State
	byEvent  map[string][]Transition
}

// New builds an FSM rooted at initial. transitions may be empty; events
// can be added later via AddTransition (no-op table means any Fire is
// illegal).
func New(initial State, transitions []Transition) *FSM {
	f := &FSM{current: initial, byEvent: make(map[string][]Transition)}
	for _, t := range transitions {
		f.byEvent[t.Event] = append(f.byEvent[t.Event], t)
	}
	return f
}

// Current returns the current state.
func (f *FSM) Current() State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.current
}

// Fire attempts to apply the named event. If allowed, the state advances
// and (newState, nil) is returned; otherwise (current, ErrIllegalTransition).
func (f *FSM) Fire(event string) (State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.byEvent[event] {
		if len(t.From) == 0 {
			f.current = t.To
			return t.To, nil
		}
		for _, from := range t.From {
			if from == f.current {
				f.current = t.To
				return t.To, nil
			}
		}
	}
	return f.current, ErrIllegalTransition
}

package domain

import (
	"fmt"
	"strings"
	"time"
)

// AttemptCustodyFence is the typed custody-close fence of one Attempt: the
// durable record that the bound provider session was durably terminated
// BEFORE the result snapshot was taken. It is insert-once - a custody
// boundary is crossed once, and a second fence for the same Attempt means
// something is wrong.
//
// The fence attests exactly that termination observation, nothing more: it
// is not a writer-quiescence proof (a true quiescence mechanism is a larger
// design, an input to the final-tree proof work). Snapshots (receipts) and
// checks bind to bytes captured after this fence; a write that lands after
// it is detected against the retained version, never silently absorbed.
type AttemptCustodyFence struct {
	AttemptID AttemptID
	// SessionID is the daemon-owned session binding whose exit the fence
	// records. The fence never accepts a client-supplied binding.
	SessionID string
	// Detail carries the provider-exit evidence (termination state observed
	// at fence time), never provider prose.
	Detail   string
	FencedAt time.Time
}

// Validate checks the fence record's structural invariants.
func (f AttemptCustodyFence) Validate() error {
	if f.AttemptID.IsZero() {
		return fmt.Errorf("attempt id is required")
	}
	if strings.TrimSpace(f.SessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	if f.FencedAt.IsZero() {
		return fmt.Errorf("fenced-at is required")
	}
	return nil
}

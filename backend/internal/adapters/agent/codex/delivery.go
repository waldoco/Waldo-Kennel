package codex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PaneTerminal is the provider-neutral pane interface the Codex delivery
// policy drives. The tmux runtime satisfies it; the wiring layer binds a
// session's runtime handle into it. Keeping the interface here (not in
// ports) keeps terminal mechanics out of the agent-adapter contract.
type PaneTerminal interface {
	// Capture returns the most recent pane lines (plain, no SGR).
	Capture(ctx context.Context, lines int) (string, error)
	// CancelCopyMode best-effort exits tmux copy mode (a no-op elsewhere).
	CancelCopyMode(ctx context.Context)
	// PasteBuffer atomically pastes text with bracketed-paste framing.
	PasteBuffer(ctx context.Context, text string) error
	SendEnter(ctx context.Context) error
	// SendTab is the Codex keymap's queue-request key while a turn runs.
	SendTab(ctx context.Context) error
	// PaneDead reports an exited provider behind a kept pane.
	PaneDead(ctx context.Context) (bool, error)
}

// DeliveryOutcome reports how a message was acknowledged. The zero value is
// not a success.
type DeliveryOutcome int

const (
	DeliveryNone DeliveryOutcome = iota
	// DeliverySubmitted: the composer cleared and the message entered the
	// turn stream (a turn started, or the pane returned to idle without the
	// draft - an instantly-completed turn).
	DeliverySubmitted
	// DeliveryQueued: sent mid-turn via the keymap's queue key; the composer
	// cleared while the turn kept running.
	DeliveryQueued
)

func (o DeliveryOutcome) String() string {
	switch o {
	case DeliverySubmitted:
		return "submitted"
	case DeliveryQueued:
		return "queued"
	default:
		return "none"
	}
}

// DeliveryPendingError: the pane stayed unsteerable past the hold deadline;
// the message was never injected and must stay queued kennel-side.
type DeliveryPendingError struct {
	Reason UnsteerableReason
}

func (e *DeliveryPendingError) Error() string {
	return fmt.Sprintf("codex pane unsteerable (%s); message held, not injected", e.Reason)
}

// DeliveryUnknownError: keystrokes were sent but no acknowledgment was
// observed inside the bounded window, or the pane failed closed before
// dispatch. Capture preserves the pane evidence at decision time.
type DeliveryUnknownError struct {
	Phase   string
	Capture string
}

func (e *DeliveryUnknownError) Error() string {
	return fmt.Sprintf("codex delivery unknown at %s: no submission acknowledgment observed", e.Phase)
}

// ErrPaneDead refuses delivery to an exited provider.
var ErrPaneDead = errors.New("codex pane process exited; refusing to type into a dead provider")

// DelivererConfig bounds every wait in the delivery state machine. No bare
// sleeps: each wait is a bounded poll for an observable pane signal.
type DelivererConfig struct {
	// SettlePoll/SettleDeadline bound the post-paste wait for the composer to
	// demonstrably hold the pasted draft before the submit keystroke (the
	// fix for upstream issue #28167, where an immediate Enter is absorbed as
	// another paste event; a fixed delay is not a contract).
	SettlePoll     time.Duration
	SettleDeadline time.Duration
	// AckPoll/AckDeadline bound the post-submit wait for positive
	// acknowledgment (draft cleared plus turn evidence).
	AckPoll     time.Duration
	AckDeadline time.Duration
	// UnsteerablePoll/UnsteerableDeadline bound how long delivery holds a
	// message while the pane shows a modal state (approval, compact, review).
	UnsteerablePoll     time.Duration
	UnsteerableDeadline time.Duration
	// CaptureLines sizes each capture-pane read.
	CaptureLines int
}

// DefaultDelivererConfig carries production bounds.
func DefaultDelivererConfig() DelivererConfig {
	return DelivererConfig{
		SettlePoll:          100 * time.Millisecond,
		SettleDeadline:      3 * time.Second,
		AckPoll:             200 * time.Millisecond,
		AckDeadline:         10 * time.Second,
		UnsteerablePoll:     time.Second,
		UnsteerableDeadline: 30 * time.Second,
		CaptureLines:        120,
	}
}

// Deliverer is the state-aware Codex TUI send path: classify, dispatch with
// the keymap-correct submit key, and require positive acknowledgment before
// reporting success.
type Deliverer struct {
	cfg  DelivererConfig
	poll func(ctx context.Context, d time.Duration) error // injectable clock for tests
}

func NewDeliverer(cfg DelivererConfig) *Deliverer {
	return &Deliverer{cfg: cfg, poll: pollSleep}
}

func pollSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// Deliver injects message into the pane and returns only once submission is
// acknowledged, the message is honestly held (DeliveryPendingError), or the
// outcome is honestly unknown (DeliveryUnknownError). An empty message is a
// bare nudge: Enter on an idle pane, with the same acknowledgment contract.
func (d *Deliverer) Deliver(ctx context.Context, term PaneTerminal, message string) (DeliveryOutcome, error) {
	dead, err := term.PaneDead(ctx)
	if err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "liveness probe"}
	}
	if dead {
		return DeliveryNone, ErrPaneDead
	}

	state, reason, err := d.awaitSteerable(ctx, term)
	if err != nil {
		return DeliveryNone, err
	}
	switch state {
	case PaneStateUnknown:
		return DeliveryNone, &DeliveryUnknownError{Phase: "pre-dispatch classification"}
	case PaneStateUnsteerable:
		return DeliveryNone, &DeliveryPendingError{Reason: reason}
	}

	// Dispatch boundary: cancel copy mode, paste atomically, then WAIT FOR
	// THE COMPOSER TO HOLD THE DRAFT before submitting. The wait is the
	// ported issue-#28167 fix: the submit key fires only against observed
	// pane state, never against a timer assumption.
	term.CancelCopyMode(ctx)
	if message != "" {
		if err := term.PasteBuffer(ctx, message); err != nil {
			return DeliveryNone, &DeliveryUnknownError{Phase: "paste: " + err.Error()}
		}
		ok, capErr := d.awaitDraft(ctx, term, message)
		if capErr != nil {
			return DeliveryNone, &DeliveryUnknownError{Phase: "settle capture: " + capErr.Error()}
		}
		if !ok {
			// One recovery: the paste did not land. Re-paste once; a second
			// miss is honestly unknown rather than a silent Enter into an
			// empty composer.
			term.CancelCopyMode(ctx)
			if err := term.PasteBuffer(ctx, message); err != nil {
				return DeliveryNone, &DeliveryUnknownError{Phase: "re-paste: " + err.Error()}
			}
			ok, capErr = d.awaitDraft(ctx, term, message)
			if capErr != nil {
				return DeliveryNone, &DeliveryUnknownError{Phase: "settle capture after re-paste: " + capErr.Error()}
			}
			if !ok {
				capture, _ := term.Capture(ctx, d.cfg.CaptureLines)
				return DeliveryNone, &DeliveryUnknownError{Phase: "draft never visible after paste", Capture: capture}
			}
		}
	}

	// Re-classify against fresh pane state before the submit keystroke: the
	// paste window is long enough for a turn to finish (Enter would then
	// steer instead of queue) or a modal to appear (a keystroke would answer
	// it). The submit key follows the LIVE state, not the pre-paste one.
	freshCapture, err := term.Capture(ctx, d.cfg.CaptureLines)
	if err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "pre-submit capture: " + err.Error()}
	}
	freshState, freshReason := ClassifyPaneState(freshCapture)
	switch freshState {
	case PaneStateUnknown:
		return DeliveryNone, &DeliveryUnknownError{Phase: "pre-submit classification", Capture: freshCapture}
	case PaneStateUnsteerable:
		return DeliveryNone, &DeliveryPendingError{Reason: freshReason}
	}
	submit := term.SendEnter
	if freshState == PaneStateActiveTurn {
		submit = term.SendTab
	}
	if err := submit(ctx); err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "submit keystroke: " + err.Error()}
	}

	outcome, acked, lastCapture := d.awaitAck(ctx, term, message, freshState)
	if acked {
		return outcome, nil
	}
	// Bounded recovery: one nudge Enter (the existing sendConfirm contract),
	// then stop guessing.
	if err := term.SendEnter(ctx); err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "recovery nudge: " + err.Error(), Capture: lastCapture}
	}
	outcome, acked, lastCapture = d.awaitAck(ctx, term, message, freshState)
	if acked {
		return outcome, nil
	}
	return DeliveryNone, &DeliveryUnknownError{Phase: "no acknowledgment after submit + recovery nudge", Capture: lastCapture}
}

// awaitSteerable polls classification until the pane is steerable or the
// unsteerable hold deadline expires. Unknown fails immediately - there is no
// safe wait for "becomes recognizable".
func (d *Deliverer) awaitSteerable(ctx context.Context, term PaneTerminal) (PaneState, UnsteerableReason, error) {
	deadline := time.Now().Add(d.cfg.UnsteerableDeadline)
	for {
		capture, err := term.Capture(ctx, d.cfg.CaptureLines)
		if err != nil {
			return PaneStateUnknown, "", &DeliveryUnknownError{Phase: "classification capture: " + err.Error()}
		}
		state, reason := ClassifyPaneState(capture)
		if state != PaneStateUnsteerable {
			return state, reason, nil
		}
		if time.Now().After(deadline) {
			return state, reason, nil
		}
		if err := d.poll(ctx, d.cfg.UnsteerablePoll); err != nil {
			return PaneStateUnknown, "", err
		}
	}
}

// awaitDraft polls until the composer demonstrably holds the pasted draft.
func (d *Deliverer) awaitDraft(ctx context.Context, term PaneTerminal, message string) (bool, error) {
	deadline := time.Now().Add(d.cfg.SettleDeadline)
	for {
		capture, err := term.Capture(ctx, d.cfg.CaptureLines)
		if err != nil {
			return false, err
		}
		if DraftPresent(capture, message) {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		if err := d.poll(ctx, d.cfg.SettlePoll); err != nil {
			return false, err
		}
	}
}

// awaitAck polls for positive acknowledgment after the submit keystroke:
// the draft must be gone, and the pane must show turn evidence (a running
// turn) or a cleared idle composer (an instantly-finished turn). A queued
// message is acknowledged when the draft clears while the turn runs.
func (d *Deliverer) awaitAck(ctx context.Context, term PaneTerminal, message string, dispatchState PaneState) (DeliveryOutcome, bool, string) {
	deadline := time.Now().Add(d.cfg.AckDeadline)
	var lastCapture string
	for {
		capture, err := term.Capture(ctx, d.cfg.CaptureLines)
		if err == nil {
			lastCapture = capture
			if message == "" || !DraftPresent(capture, message) {
				state, _ := ClassifyPaneState(capture)
				switch state {
				case PaneStateActiveTurn:
					if dispatchState == PaneStateActiveTurn {
						return DeliveryQueued, true, lastCapture
					}
					return DeliverySubmitted, true, lastCapture
				case PaneStateIdle:
					return DeliverySubmitted, true, lastCapture
				}
			}
		}
		if time.Now().After(deadline) {
			return DeliveryNone, false, lastCapture
		}
		if perr := d.poll(ctx, d.cfg.AckPoll); perr != nil {
			return DeliveryNone, false, lastCapture
		}
	}
}

// DraftPresent reports whether the pane still shows the message as an
// unsubmitted composer draft. Submitted user messages KEEP their "›" prefix
// in the scrollback (run 35303652394's capture shows the submitted turn-1
// prompt rendered as a "›" line), so matching any "›" line would confuse a
// scrollback echo for a live draft. The composer is bottom-anchored: the
// LAST "›"-prefixed line in the capture is the composer's input row.
// Normalized whitespace on both sides; the composer re-wraps long input, so
// only the message's first line is matched, prefix-either-way to survive
// mid-line visual wraps.
func DraftPresent(capture, message string) bool {
	want := normalizeSpaces(firstLine(message))
	if want == "" {
		return false
	}
	composer := lastComposerBody(capture)
	if composer == "" {
		return false
	}
	return strings.HasPrefix(composer, want) || strings.HasPrefix(want, composer)
}

// lastComposerBody returns the text of the bottom-most composer input row.
func lastComposerBody(capture string) string {
	last := ""
	for _, line := range strings.Split(capture, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "›") {
			last = normalizeSpaces(strings.TrimSpace(strings.TrimPrefix(trimmed, "›")))
		}
	}
	return last
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func normalizeSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

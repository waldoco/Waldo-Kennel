package codex

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

// DeliveryPendingError: the pane stayed unsteerable past the hold deadline
// BEFORE any injection; the message was never injected and must stay queued
// kennel-side. An unsteerable pane discovered after the paste is NOT
// pending - it is injected-but-unsubmitted unknown (DeliveryUnknownError).
type DeliveryPendingError struct {
	Reason UnsteerableReason
}

func (e *DeliveryPendingError) Error() string {
	return fmt.Sprintf("codex pane unsteerable (%s); message held, not injected", e.Reason)
}

// DeliveryUnknownError: the message may have been injected but no
// acknowledgment was observed inside the bounded window, or the pane failed
// closed mid-dispatch. Callers must NOT retry: a retry pastes a duplicate.
// Capture preserves the pane evidence at decision time.
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

	// Pre-dispatch boundary: snapshot the pane so acknowledgment can be
	// anchored to evidence that POSTDATES this delivery. A cleared composer
	// alone never acks - the pane could have lost the draft spuriously.
	boundary, err := term.Capture(ctx, d.cfg.CaptureLines)
	if err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "boundary capture: " + err.Error()}
	}
	anchor := ackAnchor{
		line:   lastStableScrollbackRow(boundary),
		queued: countQueuedStructural(boundary, message),
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
		// Injection already happened: this is injected-but-unsubmitted
		// unknown, NOT a retryable pending hold.
		return DeliveryNone, &DeliveryUnknownError{Phase: "post-injection unsteerable (" + string(freshReason) + ")", Capture: freshCapture}
	}
	submit := term.SendEnter
	if freshState == PaneStateActiveTurn {
		submit = term.SendTab
	}
	if err := submit(ctx); err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "submit keystroke: " + err.Error()}
	}

	outcome, acked, lastCapture := d.awaitAck(ctx, term, message, freshState, anchor)
	if acked {
		return outcome, nil
	}

	// Bounded recovery, HIGH-2: NO unconditional Enter. Re-probe liveness and
	// re-classify first; a retry keystroke fires only against observed safe
	// state.
	//
	//  - Modal/overlay: a keystroke would answer it. Injected-but-unsubmitted
	//    unknown; stop.
	//  - Draft still in the composer: proof the first keystroke never landed
	//    (nothing it could have submitted). Retry the keymap-correct key for
	//    the LIVE state - Enter when idle, Tab mid-turn.
	//  - Draft gone with no ack evidence: the first keystroke may have
	//    submitted into a state we cannot confirm. Retyping Enter could
	//    queue a duplicate or answer a dialog. Honestly unknown; stop.
	dead, err = term.PaneDead(ctx)
	if err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "recovery liveness probe: " + err.Error(), Capture: lastCapture}
	}
	if dead {
		return DeliveryNone, ErrPaneDead
	}
	recCapture, err := term.Capture(ctx, d.cfg.CaptureLines)
	if err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "recovery capture: " + err.Error(), Capture: lastCapture}
	}
	recState, recReason := ClassifyPaneState(recCapture)
	switch recState {
	case PaneStateUnsteerable:
		return DeliveryNone, &DeliveryUnknownError{Phase: "post-submit unsteerable (" + string(recReason) + ")", Capture: recCapture}
	case PaneStateUnknown:
		return DeliveryNone, &DeliveryUnknownError{Phase: "post-submit classification", Capture: recCapture}
	}
	retryable := message == "" || DraftPresent(recCapture, message)
	if !retryable {
		return DeliveryNone, &DeliveryUnknownError{Phase: "no acknowledgment; draft gone (may have submitted)", Capture: recCapture}
	}
	retry := term.SendEnter
	if message != "" && recState == PaneStateActiveTurn {
		retry = term.SendTab
	}
	if err := retry(ctx); err != nil {
		return DeliveryNone, &DeliveryUnknownError{Phase: "recovery keystroke: " + err.Error(), Capture: recCapture}
	}
	outcome, acked, lastCapture = d.awaitAck(ctx, term, message, recState, anchor)
	if acked {
		return outcome, nil
	}
	return DeliveryNone, &DeliveryUnknownError{Phase: "no acknowledgment after submit + bounded retry", Capture: lastCapture}
}

// awaitSteerable polls classification until the pane is steerable or the
// hold deadline expires. Unknown polls on the same bound as unsteerable: a
// freshly launched TUI shows only its boot banner for the first seconds - no
// composer, no status footer - so unknown at first sight is "no evidence
// yet", not "never recognizable" (run 35311387153 failed a steer 26ms after
// launch on exactly this). The fail-closed posture is unchanged: a pane
// still unknown at the deadline returns unknown and Deliver refuses the
// keystroke.
func (d *Deliverer) awaitSteerable(ctx context.Context, term PaneTerminal) (PaneState, UnsteerableReason, error) {
	deadline := time.Now().Add(d.cfg.UnsteerableDeadline)
	for {
		capture, err := term.Capture(ctx, d.cfg.CaptureLines)
		if err != nil {
			return PaneStateUnknown, "", &DeliveryUnknownError{Phase: "classification capture: " + err.Error()}
		}
		state, reason := ClassifyPaneState(capture)
		if state != PaneStateUnsteerable && state != PaneStateUnknown {
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

// ackAnchor anchors acknowledgment to the pre-dispatch pane. Counts over a
// bounded capture window are NOT monotonic (a clearing draft shrinks the
// composer and pulls old scrollback - including an old identical echo -
// into the window from the top), so the anchor is POSITIONAL: the last
// stable (non-volatile) scrollback row above the composer at dispatch time.
// Only rows rendered BELOW that row postdate dispatch. Queued evidence is
// counted structurally (see countQueuedStructural) against the boundary.
type ackAnchor struct {
	line   string // normalized last stable scrollback row at dispatch
	queued int    // structural queued rows in the boundary capture
}

// awaitAck polls for positive acknowledgment after the submit keystroke.
// Idle dispatch (submitted): the draft must be gone AND a NEW "› <message>"
// echo row must render BELOW the boundary's last stable scrollback row
// (positional anchor - immune to capture-window shifts), with turn evidence
// tied to that row below it, or post-echo completion evidence between the
// row and the composer on a positively-idle pane (an instantly-finished
// turn; every row in that gap postdates the boundary). Active-turn dispatch
// (queued): the draft must be gone AND structural queued rows ("> "-prefixed,
// full-text, below the last working line, ABOVE the composer region) must
// exceed the boundary count while the original turn still runs. Anything
// else fails closed to unknown.
func (d *Deliverer) awaitAck(ctx context.Context, term PaneTerminal, message string, dispatchState PaneState, anchor ackAnchor) (DeliveryOutcome, bool, string) {
	deadline := time.Now().Add(d.cfg.AckDeadline)
	var lastCapture string
	for {
		capture, err := term.Capture(ctx, d.cfg.CaptureLines)
		if err == nil {
			lastCapture = capture
			state, _ := ClassifyPaneState(capture)
			if message == "" {
				// Bare nudge: acknowledgment is a turn starting on a pane
				// that was idle at dispatch.
				if dispatchState == PaneStateIdle && state == PaneStateActiveTurn {
					return DeliverySubmitted, true, lastCapture
				}
			} else if !DraftPresent(capture, message) {
				if dispatchState == PaneStateActiveTurn {
					if state == PaneStateActiveTurn && countQueuedStructural(capture, message) > anchor.queued {
						return DeliveryQueued, true, lastCapture
					}
				} else if submittedAck(capture, message, anchor, state) {
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

// submittedAck reports whether the pane proves the message entered the turn
// stream. The echo must render BELOW the boundary's last stable scrollback
// row: an old identical echo entering the capture window (a clearing draft
// shrinks the composer and reveals scrollback above) sits ABOVE the anchor
// and never counts. Evidence must then be tied to the new row: a working
// indicator, tool call, or finished-turn marker below it; or, for an
// instantly-finished turn (idle pane), completion content between the new
// row and the composer - every row in that gap postdates the boundary.
func submittedAck(capture, message string, anchor ackAnchor, state PaneState) bool {
	want := normalizeSpaces(firstLine(message))
	if want == "" {
		return false
	}
	lines := strings.Split(capture, "\n")
	ci := composerRowIndex(lines)
	if ci < 0 {
		return false
	}
	anchorIdx := -1
	if anchor.line != "" {
		anchorIdx = -1
		for i := len(lines) - 1; i >= 0; i-- {
			if normalizeSpaces(strings.TrimSpace(lines[i])) == anchor.line {
				anchorIdx = i
				break
			}
		}
		if anchorIdx < 0 {
			return false // boundary evidence scrolled away: fail closed
		}
	}
	newEcho := -1
	for i := anchorIdx + 1; i < ci; i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if !strings.HasPrefix(trimmed, "›") {
			continue
		}
		body := normalizeSpaces(strings.TrimSpace(strings.TrimPrefix(trimmed, "›")))
		if body == want {
			newEcho = i
		}
	}
	if newEcho < 0 {
		return false
	}
	for j := newEcho + 1; j < len(lines); j++ {
		if strings.Contains(lines[j], "esc to interrupt") ||
			strings.Contains(lines[j], "• Called") ||
			strings.Contains(lines[j], "└ ") ||
			doneMarker.MatchString(lines[j]) {
			return true
		}
	}
	if state == PaneStateIdle {
		for j := newEcho + 1; j < ci; j++ {
			if strings.TrimSpace(lines[j]) != "" {
				return true
			}
		}
	}
	return false
}

var doneMarker = regexp.MustCompile(`\bdone \d`)

var workingTimer = regexp.MustCompile(`\bWorking \(\d`)

// composerRowIndex returns the index of the bottom-most "›"-prefixed row -
// the live composer's input row - or -1 when no composer renders.
func composerRowIndex(lines []string) int {
	ci := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "›") {
			ci = i
		}
	}
	return ci
}

// volatileRow reports whether a row's text mutates between captures (the
// working indicator's elapsed timer), making it useless as an anchor.
func volatileRow(line string) bool {
	return strings.Contains(line, "esc to interrupt") || workingTimer.MatchString(line)
}

// lastStableScrollbackRow returns the normalized text of the last
// non-empty, non-volatile row above the composer - the positional dispatch
// anchor. Empty when the boundary shows no scrollback (every later row is
// then post-boundary).
func lastStableScrollbackRow(capture string) string {
	lines := strings.Split(capture, "\n")
	ci := composerRowIndex(lines)
	if ci < 0 {
		ci = len(lines)
	}
	for i := ci - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if t == "" || volatileRow(lines[i]) {
			continue
		}
		return normalizeSpaces(t)
	}
	return ""
}

// countQueuedStructural counts QUEUED-message renderings of the text: rows
// that (a) sit below the LAST working indicator, (b) sit ABOVE the live
// composer region, and (c) carry the provider's message structure (a
// "›"-prefixed row whose body is the message's first line, exactly). The
// composer region is excluded explicitly: a wrapped or truncated draft row
// that evades DraftPresent must never count as queued rendering. Queued
// rendering is the least verified ground truth; a mismatch fails closed to
// unknown rather than acking on the continuing original turn alone.
func countQueuedStructural(capture, message string) int {
	want := normalizeSpaces(firstLine(message))
	if want == "" {
		return 0
	}
	lines := strings.Split(capture, "\n")
	ci := composerRowIndex(lines)
	if ci < 0 {
		return 0
	}
	lastWorking := -1
	for i, line := range lines {
		if strings.Contains(line, "esc to interrupt") {
			lastWorking = i
		}
	}
	if lastWorking < 0 {
		return 0
	}
	count := 0
	for i := lastWorking + 1; i < ci; i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if !strings.HasPrefix(trimmed, "›") {
			continue
		}
		body := normalizeSpaces(strings.TrimSpace(strings.TrimPrefix(trimmed, "›")))
		if body == want {
			count++
		}
	}
	return count
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

package codex

import (
	"strings"
)

// PaneState classifies what the Codex TUI pane is doing right now, from a
// bounded capture-pane excerpt. It is the pre-dispatch gate for the
// state-aware send path: a message may only be injected when the pane is in a
// state whose submission semantics Kennel understands. Anything unrecognized
// fails closed as PaneStateUnknown.
//
// Patterns are grounded in the Codex TUI composer/status rendering verified
// against rust-v0.153.4 and rust-v0.155.0-alpha.2.6 (chat_composer.rs and the
// status footer), plus the real governed-session pane capture from e2e run
// 35303652394 (idle composer with an unsubmitted steered draft).
type PaneState int

const (
	// PaneStateUnknown is the fail-closed default: capture failed, layout
	// unrecognized, or the pane is in a state Kennel has no contract for.
	PaneStateUnknown PaneState = iota
	// PaneStateIdle is a live composer with no running turn: Enter submits.
	PaneStateIdle
	// PaneStateActiveTurn is a normal turn streaming: Tab queues, Enter
	// submits only in the idle window (codex keymap contract).
	PaneStateActiveTurn
	// PaneStateUnsteerable is a recognized state that must not receive prompt
	// keystrokes: an approval dialog, review or compact overlay, the startup
	// modal, or tmux copy mode. The daemon holds the message kennel-side and
	// re-classifies rather than injecting into a modal.
	PaneStateUnsteerable
)

func (s PaneState) String() string {
	switch s {
	case PaneStateIdle:
		return "idle"
	case PaneStateActiveTurn:
		return "active_turn"
	case PaneStateUnsteerable:
		return "unsteerable"
	default:
		return "unknown"
	}
}

// UnsteerableReason names why a pane was classified unsteerable, for logging
// and the delivery_pending surface. Empty when the state is steerable.
type UnsteerableReason string

const (
	ReasonApprovalDialog UnsteerableReason = "approval_dialog"
	ReasonCompact        UnsteerableReason = "compact"
	ReasonReviewOverlay  UnsteerableReason = "review_overlay"
	ReasonStartupModal   UnsteerableReason = "startup_modal"
)

// ClassifyPaneState inspects a capture-pane excerpt (most recent lines) and
// returns the pane state plus, when unsteerable, the reason. Classification
// order matters: modal overlays are checked first because an approval dialog
// or compact can render over an otherwise idle-looking composer; a running
// turn is checked before idle because a streaming pane still shows the
// composer region in the scrollback. Idle is a positive identification, not
// the absence of other signals: the composer prompt and the model footer must
// both be present.
func ClassifyPaneState(capture string) (PaneState, UnsteerableReason) {
	if strings.TrimSpace(capture) == "" {
		return PaneStateUnknown, ""
	}

	// Modal and overlay states first: they swallow prompt keystrokes.
	if isApprovalDialog(capture) {
		return PaneStateUnsteerable, ReasonApprovalDialog
	}
	if strings.Contains(capture, "Compacting conversation") ||
		strings.Contains(capture, "compacting conversation history") {
		return PaneStateUnsteerable, ReasonCompact
	}
	if isReviewOverlay(capture) {
		return PaneStateUnsteerable, ReasonReviewOverlay
	}

	// A running turn: codex renders the working status ("esc to interrupt")
	// directly ABOVE the live composer. The marker is live only as part of
	// the bottom-pane structure - above the composer row, separated from it
	// by nothing but blank or user-message rows (queued renders), with the
	// status footer below. No fixed tail window: a tall pasted draft can
	// push the marker arbitrarily far up the capture (tail-windowing it
	// false-idles mid-turn and Enter steers instead of queueing), while an
	// old scrollback marker always has turn output or a done marker between
	// it and the composer and is ignored.
	if isLiveWorkingIndicator(capture) && hasStatusFooter(capture) {
		return PaneStateActiveTurn, ""
	}

	// Idle is positive: the composer prompt line and the model/status footer
	// must both render, and no working indicator may be present.
	if hasComposerPrompt(capture) && hasStatusFooter(capture) {
		return PaneStateIdle, ""
	}
	return PaneStateUnknown, ""
}

// isLiveWorkingIndicator reports whether the pane's working indicator is
// LIVE: walking up from the live composer row (the bottom-most "›" row),
// the first content row is the working status. Blank rows and user-message
// rows ("›"-prefixed queued renders) may sit between them; any other row -
// turn output, a done marker - means the working line is old scrollback.
// Draft continuation rows render BELOW the composer's "›" row, so a tall or
// wrapped draft never hides the indicator from this walk.
func isLiveWorkingIndicator(capture string) bool {
	lines := strings.Split(capture, "\n")
	ci := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "›") {
			ci = i
		}
	}
	if ci <= 0 {
		return false
	}
	for i := ci - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if strings.Contains(lines[i], "esc to interrupt") {
			return true
		}
		if strings.HasPrefix(strings.TrimLeft(lines[i], " "), "›") {
			continue // queued user-message render between working line and composer
		}
		return false
	}
	return false
}

// hasComposerPrompt reports whether a codex composer input line (rendered
// with the "›" prompt) is present in the capture.
func hasComposerPrompt(capture string) bool {
	for _, line := range strings.Split(capture, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "›") {
			return true
		}
	}
	return false
}

// hasStatusFooter reports whether the codex status footer renders: the
// "<model> <mode> · <directory>" line at the bottom of an idle TUI. The
// middle-dot separator between model mode and directory is the stable token
// across 0.153.4 and 0.155.0-alpha.2.6 captures.
func hasStatusFooter(capture string) bool {
	lines := strings.Split(capture, "\n")
	// The footer is a bottom-anchored line; look at the tail only so a
	// scrollback-quoted separator does not pass as a live footer.
	start := len(lines) - 6
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, " · ") && strings.Contains(trimmed, "/") {
			return true
		}
	}
	return false
}

// isApprovalDialog recognizes the codex exec/tool approval overlay. Verified
// tokens across the certified builds: the dialog offers explicit
// proceed/decline choices and never coexists with the working status line.
func isApprovalDialog(capture string) bool {
	lower := strings.ToLower(capture)
	if strings.Contains(lower, "allow") && strings.Contains(lower, "deny") &&
		(strings.Contains(lower, "approval") || strings.Contains(lower, "command")) {
		return true
	}
	// on-request approval prompt wording used by the TUI permission dialog.
	if strings.Contains(lower, "would you like to run") ||
		strings.Contains(lower, "approve this command") {
		return true
	}
	return false
}

// isReviewOverlay recognizes the /review and transcript-review overlays,
// which replace the composer with a scrollable review surface.
func isReviewOverlay(capture string) bool {
	lower := strings.ToLower(capture)
	return strings.Contains(lower, "reviewing changes") ||
		strings.Contains(lower, "review mode") && strings.Contains(lower, "esc")
}

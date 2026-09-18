package codex

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeTerminal scripts capture-pane responses and records every operation in
// order. The last scripted capture repeats once the script is exhausted.
type fakeTerminal struct {
	captures []string
	ops      []string
	dead     bool
	pastes   []string
	enters   int
	tabs     int
	cancels  int
	// afterEnters, when >0, replaces the capture script once that many Enters
	// have been sent - models a pane that changes in response to the nudge,
	// independent of how many ack polls a wall-clock window happens to span.
	afterEnters   int
	afterEntersTo string
	afterTabs     int
	afterTabsTo   string
}

func (f *fakeTerminal) Capture(ctx context.Context, lines int) (string, error) {
	if len(f.captures) == 0 {
		return "", errors.New("no capture scripted")
	}
	out := f.captures[0]
	if len(f.captures) > 1 {
		f.captures = f.captures[1:]
	}
	return out, nil
}

func (f *fakeTerminal) CancelCopyMode(ctx context.Context) {
	f.cancels++
	f.ops = append(f.ops, "cancel")
}
func (f *fakeTerminal) PasteBuffer(ctx context.Context, text string) error {
	f.pastes = append(f.pastes, text)
	f.ops = append(f.ops, "paste")
	return nil
}
func (f *fakeTerminal) SendEnter(ctx context.Context) error {
	f.enters++
	f.ops = append(f.ops, "enter")
	if f.afterEnters > 0 && f.enters >= f.afterEnters {
		f.captures = []string{f.afterEntersTo}
	}
	return nil
}
func (f *fakeTerminal) SendTab(ctx context.Context) error {
	f.tabs++
	f.ops = append(f.ops, "tab")
	if f.afterTabs > 0 && f.tabs >= f.afterTabs {
		f.captures = []string{f.afterTabsTo}
	}
	return nil
}
func (f *fakeTerminal) PaneDead(ctx context.Context) (bool, error) {
	f.ops = append(f.ops, "probe")
	return f.dead, nil
}

func testDeliverer() *Deliverer {
	cfg := DelivererConfig{
		SettlePoll:          time.Millisecond,
		SettleDeadline:      50 * time.Millisecond,
		AckPoll:             time.Millisecond,
		AckDeadline:         50 * time.Millisecond,
		UnsteerablePoll:     time.Millisecond,
		UnsteerableDeadline: 20 * time.Millisecond,
		CaptureLines:        120,
	}
	return NewDeliverer(cfg)
}

const msg = "Run this shell command: for i in 1 2 3 4 5 6; do echo b4-$i; sleep 3; done"

// idleNoDraft: an idle composer holding only the placeholder - turn 1 done,
// no steered text yet.
const paneIdleNoDraft = `› Execute the following approved WorkUnit inside your isolated worktree.

  KENNEL_WORK_STATUS: reviewing

  done 9:04 AM

› Ask Codex to do anything

  gpt-6-astra default · /private/var/folders/x/worktrees/mer-1 · Create persistent fixt...`

// idleWithSteeredDraft is the run-35303652394 failure pane: the steered text
// sits in the composer, unsubmitted, above the idle footer.
var paneIdleWithSteeredDraft = strings.Replace(paneIdleNoDraft, "› Ask Codex to do anything", "> PLACEHOLDER", 1)

func init() {
	paneIdleWithSteeredDraft = strings.Replace(paneIdleNoDraft,
		"› Ask Codex to do anything",
		"› "+msg, 1)
}

// activeTurnRunning: a turn streaming, composer empty.
const paneActive = `› Execute the following approved WorkUnit inside your isolated worktree.

• Called kennel_governed.list_repository({})
  └ README.md

  Working (12s, esc to interrupt)

› Ask Codex to do anything

  gpt-6-astra default · /tmp/worktrees/mer-1 · session title`

var paneActiveWithDraft = strings.Replace(paneActive, "› Ask Codex to do anything", "› "+msg, 1)

// submittedAck: the steered message moved into the transcript as a new turn
// (scrollback "›" echo) and a turn is running - draft gone from composer.
const paneSubmittedAck = `› Execute the following approved WorkUnit inside your isolated worktree.

  done 9:04 AM

› Run this shell command: for i in 1 2 3 4 5 6; do echo b4-$i; sleep 3; done

  Working (3s, esc to interrupt)

› Ask Codex to do anything

  gpt-6-astra default · /private/var/folders/x/worktrees/mer-1 · Create persistent fixt...`

// paneQueuedAck: mid-turn queue acknowledgment - the original turn still
// runs and the steered message renders below the working line, gone from
// the composer input row.
const paneQueuedAck = `› Execute the following approved WorkUnit inside your isolated worktree.

• Called kennel_governed.list_repository({})
  └ README.md

  Working (14s, esc to interrupt)

› Run this shell command: for i in 1 2 3 4 5 6; do echo b4-$i; sleep 3; done

› Ask Codex to do anything

  gpt-6-astra default · /tmp/worktrees/mer-1 · session title`

// paneInstantAck: an instantly-finished turn - the echo row sits in the
// scrollback and the pane is already back to idle, no working indicator.
var paneInstantAck = strings.Replace(paneIdleNoDraft,
	"› Ask Codex to do anything",
	"› "+msg+"\n\n  done 9:07 AM\n\n› Ask Codex to do anything", 1)

// idleClearedAck: draft vanished with NO echo and NO turn evidence - the
// HIGH-1 false-positive pane. Must never ack.
var paneIdleClearedAck = paneIdleNoDraft

func TestDeliverIdleHappyPath(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleNoDraft,          // pre-dispatch classification
		paneIdleNoDraft,          // pre-dispatch boundary snapshot (no echo yet)
		paneIdleWithSteeredDraft, // settle: draft visible
		paneIdleWithSteeredDraft, // pre-submit re-classification (still idle)
		paneSubmittedAck,         // ack: draft gone, new echo + turn running
	}}
	outcome, err := testDeliverer().Deliver(context.Background(), term, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if outcome != DeliverySubmitted {
		t.Fatalf("outcome = %v, want submitted", outcome)
	}
	if term.enters != 1 || term.tabs != 0 || len(term.pastes) != 1 {
		t.Fatalf("enters=%d tabs=%d pastes=%d, want 1/0/1", term.enters, term.tabs, len(term.pastes))
	}
	// Ordering contract: cancel copy mode BEFORE paste BEFORE enter.
	want := []string{"probe", "cancel", "paste", "enter"}
	if strings.Join(term.ops, ",") != strings.Join(want, ",") {
		t.Fatalf("ops = %v, want %v", term.ops, want)
	}
}

func TestDeliverActiveTurnQueuesViaTab(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneActive,          // pre-dispatch: active turn
		paneActive,          // boundary snapshot (nothing queued yet)
		paneActiveWithDraft, // settle: draft visible
		paneActiveWithDraft, // pre-submit: still active
		paneQueuedAck,       // ack: draft gone, message queued below the working line
	}}
	outcome, err := testDeliverer().Deliver(context.Background(), term, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if outcome != DeliveryQueued {
		t.Fatalf("outcome = %v, want queued", outcome)
	}
	if term.tabs != 1 || term.enters != 0 {
		t.Fatalf("tabs=%d enters=%d, want 1/0", term.tabs, term.enters)
	}
}

func TestDeliverRefusesDeadPane(t *testing.T) {
	term := &fakeTerminal{dead: true}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	if !errors.Is(err, ErrPaneDead) {
		t.Fatalf("err = %v, want ErrPaneDead", err)
	}
	if len(term.pastes) != 0 || term.enters != 0 || term.tabs != 0 {
		t.Fatalf("keystrokes sent to dead pane: %v", term.ops)
	}
}

func TestDeliverFailsClosedOnUnknownLayout(t *testing.T) {
	term := &fakeTerminal{captures: []string{paneFixtureUnrecognized}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var unk *DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v, want DeliveryUnknownError", err)
	}
	if len(term.pastes) != 0 || term.enters != 0 {
		t.Fatalf("keystrokes sent against unknown layout: %v", term.ops)
	}
}

func TestDeliverHoldsWhileUnsteerableThenProceeds(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneFixtureApproval,      // approval dialog up
		paneFixtureApproval,      // still up
		paneIdleNoDraft,          // cleared -> idle
		paneIdleNoDraft,          // boundary snapshot
		paneIdleWithSteeredDraft, // settle
		paneIdleWithSteeredDraft, // pre-submit
		paneSubmittedAck,         // ack
	}}
	outcome, err := testDeliverer().Deliver(context.Background(), term, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if outcome != DeliverySubmitted {
		t.Fatalf("outcome = %v, want submitted", outcome)
	}
}

func TestDeliveryPendingWhenModalPersists(t *testing.T) {
	term := &fakeTerminal{captures: []string{paneFixtureApproval}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var pend *DeliveryPendingError
	if !errors.As(err, &pend) {
		t.Fatalf("err = %v, want DeliveryPendingError", err)
	}
	if pend.Reason != ReasonApprovalDialog {
		t.Fatalf("reason = %q, want approval_dialog", pend.Reason)
	}
	if len(term.pastes) != 0 || term.enters != 0 {
		t.Fatalf("keystrokes sent into approval dialog: %v", term.ops)
	}
}

// TestDeliverUnknownWhenPasteNeverLands is the upstream #28167 wedge: the
// paste is absorbed and the draft never renders. One re-paste, then honest
// unknown - never a bare Enter into an empty composer.
func TestDeliverUnknownWhenPasteNeverLands(t *testing.T) {
	term := &fakeTerminal{captures: []string{paneIdleNoDraft}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var unk *DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v, want DeliveryUnknownError", err)
	}
	if len(term.pastes) != 2 {
		t.Fatalf("pastes = %d, want exactly 2 (paste + one recovery)", len(term.pastes))
	}
	if term.enters != 0 || term.tabs != 0 {
		t.Fatalf("submit keystroke sent without a visible draft: %v", term.ops)
	}
	if unk.Capture == "" {
		t.Fatal("DeliveryUnknownError must preserve the pane evidence")
	}
}

// TestDeliverUnknownWhenDraftNeverSubmits reproduces run 35303652394: paste
// lands, Enter fires, and the draft just sits there. One recovery nudge, then
// honest unknown with the pane capture attached. The old path reported 200.
func TestDeliverUnknownWhenDraftNeverSubmits(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleNoDraft,          // classify: idle
		paneIdleNoDraft,          // boundary snapshot
		paneIdleWithSteeredDraft, // settle: draft visible
		paneIdleWithSteeredDraft, // pre-submit: idle
		paneIdleWithSteeredDraft, // ack polls + recovery: draft persists (script repeats)
	}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var unk *DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v, want DeliveryUnknownError", err)
	}
	if term.enters != 2 {
		t.Fatalf("enters = %d, want exactly 2 (submit + one nudge)", term.enters)
	}
	if len(term.pastes) != 1 {
		t.Fatalf("pastes = %d, want 1 (recovery never re-pastes a visible draft)", len(term.pastes))
	}
	if !strings.Contains(unk.Capture, "b4-$i") {
		t.Fatal("error must carry the pane evidence showing the stuck draft")
	}
}

// TestDeliverRecoveryNudgeSucceeds: the first Enter is absorbed (the
// historical 300ms race), the nudge submits.
func TestDeliverRecoveryNudgeSucceeds(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleNoDraft,
		paneIdleNoDraft,          // boundary snapshot
		paneIdleWithSteeredDraft, // settle
		paneIdleWithSteeredDraft, // pre-submit, then ack window + recovery probe repeat it: Enter absorbed
	}}
	term.afterEnters = 2
	term.afterEntersTo = paneSubmittedAck
	outcome, err := testDeliverer().Deliver(context.Background(), term, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if outcome != DeliverySubmitted {
		t.Fatalf("outcome = %v, want submitted", outcome)
	}
	if term.enters != 2 {
		t.Fatalf("enters = %d, want 2 (submit + nudge)", term.enters)
	}
}

// TestDraftPresentScrollbackEchoIsNotDraft pins the scrollback trap: a
// submitted message keeps its "›" prefix in the transcript; only the
// bottom-most composer row counts as a live draft.
func TestDraftPresentScrollbackEchoIsNotDraft(t *testing.T) {
	if DraftPresent(paneSubmittedAck, msg) {
		t.Fatal("scrollback echo of the submitted message must not count as a live draft")
	}
	if !DraftPresent(paneIdleWithSteeredDraft, msg) {
		t.Fatal("the bottom composer row holding the message IS a live draft")
	}
	if DraftPresent(paneIdleNoDraft, msg) {
		t.Fatal("placeholder text is not the draft")
	}
}

func TestDeliverEmptyMessageNudge(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleWithSteeredDraft, // classify: idle (draft from an earlier paste)
		paneIdleWithSteeredDraft, // boundary snapshot
		paneIdleWithSteeredDraft, // pre-submit
		paneSubmittedAck,         // ack after nudge
	}}
	outcome, err := testDeliverer().Deliver(context.Background(), term, "")
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if outcome != DeliverySubmitted {
		t.Fatalf("outcome = %v, want submitted", outcome)
	}
	if term.enters != 1 || len(term.pastes) != 0 {
		t.Fatalf("enters=%d pastes=%d, want 1/0", term.enters, len(term.pastes))
	}
}

// TestDeliverIdleInstantCompletion: the turn finishes inside the ack window
// - echo row in scrollback, pane already idle. The new echo row beyond the
// pre-dispatch boundary is what distinguishes this from a spuriously
// cleared composer.
func TestDeliverIdleInstantCompletion(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleNoDraft,
		paneIdleNoDraft,
		paneIdleWithSteeredDraft,
		paneIdleWithSteeredDraft,
		paneInstantAck,
	}}
	outcome, err := testDeliverer().Deliver(context.Background(), term, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if outcome != DeliverySubmitted {
		t.Fatalf("outcome = %v, want submitted", outcome)
	}
}

// TestDeliverClearedComposerWithoutEchoIsUnknown pins the HIGH-1 false
// positive: the draft vanished but NO new user-message echo or turn
// evidence exists. A cleared composer plus an idle footer is NOT an ack,
// and the recovery path must not fire a blind second Enter.
func TestDeliverClearedComposerWithoutEchoIsUnknown(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleNoDraft,
		paneIdleNoDraft,
		paneIdleWithSteeredDraft,
		paneIdleWithSteeredDraft,
		paneIdleClearedAck, // ack polls + recovery: draft gone, no echo (repeats)
	}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var unk *DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v, want DeliveryUnknownError", err)
	}
	if term.enters != 1 {
		t.Fatalf("enters = %d, want exactly 1 (no blind retry without draft proof)", term.enters)
	}
}

// TestDeliverPostPasteModalIsUnknownNotPending pins HIGH-3: a modal that
// appears AFTER injection is injected-but-unsubmitted unknown, never a
// retryable pending hold - a caller retry would paste a duplicate.
func TestDeliverPostPasteModalIsUnknownNotPending(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleNoDraft,
		paneIdleNoDraft,
		paneIdleWithSteeredDraft,
		paneFixtureApproval, // pre-submit re-classification: modal appeared
	}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var unk *DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v, want DeliveryUnknownError", err)
	}
	var pend *DeliveryPendingError
	if errors.As(err, &pend) {
		t.Fatal("post-injection modal must not surface as retryable pending")
	}
	if term.enters != 0 || term.tabs != 0 {
		t.Fatalf("keystroke sent into a modal: %v", term.ops)
	}
}

// TestDeliverPostSubmitModalStopsRetry pins HIGH-2: the submit keystroke
// fired, then a modal surfaced. Recovery must not press Enter into it.
func TestDeliverPostSubmitModalStopsRetry(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleNoDraft,
		paneIdleNoDraft,
		paneIdleWithSteeredDraft,
		paneIdleWithSteeredDraft,
		paneFixtureApproval, // ack polls + recovery: modal (repeats)
	}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var unk *DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v, want DeliveryUnknownError", err)
	}
	if term.enters != 1 {
		t.Fatalf("enters = %d, want exactly 1 (no Enter into the modal)", term.enters)
	}
}

// TestDeliverRecoveryUsesTabMidTurn pins HIGH-2: when the first queue key
// is absorbed and the draft still sits in the composer mid-turn, the
// recovery keystroke is Tab - never Enter, which would not queue.
func TestDeliverRecoveryUsesTabMidTurn(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneActive,
		paneActive,
		paneActiveWithDraft,
		paneActiveWithDraft,
		paneActiveWithDraft, // ack polls + recovery probe: Tab absorbed, draft persists (repeats)
	}}
	term.afterTabs = 2
	term.afterTabsTo = paneQueuedAck
	outcome, err := testDeliverer().Deliver(context.Background(), term, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if outcome != DeliveryQueued {
		t.Fatalf("outcome = %v, want queued", outcome)
	}
	if term.tabs != 2 || term.enters != 0 {
		t.Fatalf("tabs=%d enters=%d, want 2/0", term.tabs, term.enters)
	}
}

// TestDeliverOldEchoWindowShiftIsUnknown pins review round-3 HIGH-1: the
// same message was submitted EARLIER and its echo sat just above the
// capture window; when our draft vanishes spuriously, the shrinking
// composer reveals that OLD echo. A count-based anchor would false-ack
// (0->1, idle pane). The positional anchor (last stable scrollback row at
// dispatch) sits BELOW the revealed echo, so it never counts.
func TestDeliverOldEchoWindowShiftIsUnknown(t *testing.T) {
	// Boundary: the last stable scrollback row is the old turn's "done"
	// marker, which sits BELOW the old echo of the same message text.
	boundaryWithOldEcho := strings.Replace(paneIdleNoDraft,
		"  done 9:04 AM",
		"› "+msg+"\n\n  done 9:04 AM", 1)
	// Ack window: draft gone, pane idle, old echo revealed - nothing new
	// below the "done 9:04 AM" anchor row.
	term := &fakeTerminal{captures: []string{
		boundaryWithOldEcho, // classify: idle (old echo present in scrollback)
		boundaryWithOldEcho, // boundary snapshot: anchor = "done 9:04 AM"
		paneIdleWithSteeredDraft,
		paneIdleWithSteeredDraft,
		boundaryWithOldEcho, // ack polls + recovery: draft gone, only the OLD echo (repeats)
	}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var unk *DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v, want DeliveryUnknownError (old echo must not ack)", err)
	}
	if term.enters != 1 {
		t.Fatalf("enters = %d, want exactly 1", term.enters)
	}
}

// TestDeliverWrappedDraftIsNotQueuedEvidence pins review round-3 HIGH-2:
// mid-turn, the draft renders in the composer region as a bare continuation
// row (no "›" prefix on the text row), evading DraftPresent's bottom-row
// match. Generic text-below-the-working-line matching would accept it as a
// queued rendering; structural queued evidence must not.
func TestDeliverWrappedDraftIsNotQueuedEvidence(t *testing.T) {
	wrappedDraft := strings.Replace(paneActive,
		"› Ask Codex to do anything",
		"› \n"+msg+"\n\n› Ask Codex to do anything", 1)
	// Sanity: the text row sits below the working line but the bottom-most
	// "›" row is the empty composer prompt, so DraftPresent misses it.
	if DraftPresent(wrappedDraft, msg) {
		t.Fatal("precondition: wrapped draft must evade DraftPresent")
	}
	term := &fakeTerminal{captures: []string{
		paneActive,
		paneActive,
		paneActiveWithDraft, // settle: draft visible in the composer
		paneActiveWithDraft, // pre-submit
		wrappedDraft,        // ack polls + recovery: only a composer-region wrap (repeats)
	}}
	_, err := testDeliverer().Deliver(context.Background(), term, msg)
	var unk *DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("err = %v, want DeliveryUnknownError (composer wrap must not ack queued)", err)
	}
	if term.tabs != 1 {
		t.Fatalf("tabs = %d, want exactly 1 (no retry without draft proof)", term.tabs)
	}
}

// paneActiveTallDraft: a turn running while the composer holds a draft
// wrapped across MORE THAN 10 rows - the working indicator sits far above
// the footer. A tail-windowed "esc to interrupt" check false-idles here and
// Enter steers instead of queueing (review round-4 HIGH).
var paneActiveTallDraft = func() string {
	// The composer holds OUR message, its first line wrapped across rows.
	wrapped := "› Run this shell command: for i in 1 2"
	for i := 0; i < 12; i++ {
		wrapped += "\n  continuation of the wrapped draft row"
	}
	return strings.Replace(paneActive, "› Ask Codex to do anything", wrapped, 1)
}()

// TestDeliverActiveTurnWithTallDraftQueuesViaTab: the working indicator is
// more than 10 lines above the bottom; classification must still see the
// live bottom-pane structure and dispatch Tab.
func TestDeliverActiveTurnWithTallDraftQueuesViaTab(t *testing.T) {
	if got, _ := ClassifyPaneState(paneActiveTallDraft); got != PaneStateActiveTurn {
		t.Fatalf("ClassifyPaneState(paneActiveTallDraft) = %v, want active_turn", got)
	}
	term := &fakeTerminal{captures: []string{
		paneActiveTallDraft, // classify
		paneActiveTallDraft, // boundary
		paneActiveTallDraft, // settle: draft already visible (composer holds a draft)
		paneActiveTallDraft, // pre-submit
		paneQueuedAck,       // ack: queued
	}}
	outcome, err := testDeliverer().Deliver(context.Background(), term, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if outcome != DeliveryQueued {
		t.Fatalf("outcome = %v, want queued", outcome)
	}
	if term.tabs != 1 || term.enters != 0 {
		t.Fatalf("tabs=%d enters=%d, want 1/0 - Enter against an active turn is the bug", term.tabs, term.enters)
	}
}

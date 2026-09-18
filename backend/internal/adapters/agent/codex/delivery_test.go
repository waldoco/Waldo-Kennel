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

func (f *fakeTerminal) CancelCopyMode(ctx context.Context) { f.cancels++; f.ops = append(f.ops, "cancel") }
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
func (f *fakeTerminal) SendTab(ctx context.Context) error   { f.tabs++; f.ops = append(f.ops, "tab"); return nil }
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

// idleClearedAck: instant-completion path - draft gone, pane back to idle.
var paneIdleClearedAck = paneIdleNoDraft

func TestDeliverIdleHappyPath(t *testing.T) {
	term := &fakeTerminal{captures: []string{
		paneIdleNoDraft,          // pre-dispatch classification
		paneIdleWithSteeredDraft, // settle: draft visible
		paneIdleWithSteeredDraft, // pre-submit re-classification (still idle)
		paneSubmittedAck,         // ack: draft gone, turn running
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
		paneActiveWithDraft, // settle: draft visible
		paneActiveWithDraft, // pre-submit: still active
		paneActive,          // ack: draft cleared while turn runs => queued
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
		paneIdleWithSteeredDraft, // settle: draft visible
		paneIdleWithSteeredDraft, // pre-submit: idle
		paneIdleWithSteeredDraft, // ack polls: draft persists (script repeats)
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
		paneIdleWithSteeredDraft, // settle
		paneIdleWithSteeredDraft, // pre-submit, then ack window repeats it: Enter absorbed
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

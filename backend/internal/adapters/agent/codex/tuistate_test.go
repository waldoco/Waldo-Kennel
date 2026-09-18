package codex

import "testing"

// The idle fixture reproduces the real governed-session capture from e2e run
// 35303652394: turn 1 complete ("done 9:04 AM"), a steered message sitting as
// an unsubmitted composer draft, and the idle model/status footer. The daemon
// had reported delivery success while the draft never submitted - this exact
// pane is the bug the state-aware send path exists to fix.
const paneFixtureIdleWithDraft = `╭────────────────────────────────────────────────────╮
│ >_ OpenAI Codex (v0.155.0-alpha.2.6)               │
│                                                    │
│ model:     gpt-6-astra   /model to change          │
│ directory: /private/var/.../outcome-persistent2443-1 │
╰────────────────────────────────────────────────────╯

⚠ 1 startup issue · ctrl + t for details

› Execute the following approved WorkUnit inside your isolated worktree.

• Called kennel_governed.write_text_file({"path":"durable.txt","content":"PERSISTENT"})
  └ wrote durable.txt

• Created durable.txt containing exactly PERSISTENT, confirmed by governed readback.

  KENNEL_WORK_STATUS: reviewing

  done 9:04 AM

› Run this shell command: for i in 1 2 3 4 5 6; do echo b4-$i; sleep 3; done

  gpt-6-astra default · /private/var/folders/fs/sqsvy0pn0n72nwp8px8pwg_h0000gn/T/worktrees/outcome-persistent2443-1 · Create persistent fixt...`

const paneFixtureActiveTurn = `› Execute the following approved WorkUnit inside your isolated worktree.

• Called kennel_governed.list_repository({})
  └ README.md

  Working (1m 12s, esc to interrupt)

› 

  gpt-6-astra default · /tmp/worktrees/mer-1 · session title`

const paneFixtureApproval = `• Called kennel_governed.write_text_file({"path":"durable.txt"})

╭─ Approval required ─────────────────────────────╮
│ Would you like to run the following command?    │
│                                                 │
│   Allow   Deny                                  │
╰─────────────────────────────────────────────────╯`

const paneFixtureCompacting = `› prior user message

  Compacting conversation history...`

const paneFixtureEmpty = ""

// paneFixtureComposerMissing is a pane whose layout matches nothing codex
// renders - an unrecognized provider banner, for example. Fail closed.
const paneFixtureUnrecognized = `SomeOtherAgent v9.9

ready>`

// paneFixturePinned0153 is the real preserved pane from run 35307077099
// (pinned 0.153.4, evidence artifact): the same stuck-draft failure with a
// two-segment status footer (no session-title segment) and box-rule
// separators - the footer shape must still classify idle.
const paneFixturePinned0153 = `────────────────────────────

• Created durable.txt and confirmed its content is exactly PERSISTENT through governed readback.

  KENNEL_WORK_STATUS: reviewing

────────────────────────────


› Run this shell command: for i in 1 2 3 4 5 6; do echo b4-$i; sleep 3; done

  gpt-6-astra default · /private/var/folders/fs/sqsvy0pn0n72nwp8px8pwg_h0000gn/T/worktrees/outcome-persistent5725-1`

func TestClassifyPaneState(t *testing.T) {
	cases := []struct {
		name    string
		capture string
		state   PaneState
		reason  UnsteerableReason
	}{
		{"idle composer with unsubmitted draft (run 35303652394)", paneFixtureIdleWithDraft, PaneStateIdle, ""},
		{"pinned 0.153.4 stuck draft, two-segment footer (run 35307077099)", paneFixturePinned0153, PaneStateIdle, ""},
		{"active turn shows interrupt hint", paneFixtureActiveTurn, PaneStateActiveTurn, ""},
		{"approval dialog is unsteerable", paneFixtureApproval, PaneStateUnsteerable, ReasonApprovalDialog},
		{"compact overlay is unsteerable", paneFixtureCompacting, PaneStateUnsteerable, ReasonCompact},
		{"empty capture fails closed", paneFixtureEmpty, PaneStateUnknown, ""},
		{"unrecognized layout fails closed", paneFixtureUnrecognized, PaneStateUnknown, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, reason := ClassifyPaneState(tc.capture)
			if state != tc.state {
				t.Fatalf("state = %v, want %v", state, tc.state)
			}
			if reason != tc.reason {
				t.Fatalf("reason = %q, want %q", reason, tc.reason)
			}
		})
	}
}

// TestClassifyPaneStateModalBeatsIdle pins the ordering contract: an approval
// dialog rendering over an idle-looking composer must classify unsteerable,
// never idle.
func TestClassifyPaneStateModalBeatsIdle(t *testing.T) {
	capture := paneFixtureIdleWithDraft + "\n\nWould you like to run the following command?\n\n  Allow   Deny"
	state, reason := ClassifyPaneState(capture)
	if state != PaneStateUnsteerable || reason != ReasonApprovalDialog {
		t.Fatalf("state,reason = %v,%q; want unsteerable,approval_dialog", state, reason)
	}
}

// TestClassifyPaneStateActiveBeatsIdle pins the second ordering contract: a
// streaming pane still carries scrollback that looks like an idle composer,
// so the working indicator must win.
func TestClassifyPaneStateActiveBeatsIdle(t *testing.T) {
	state, _ := ClassifyPaneState(paneFixtureActiveTurn)
	if state != PaneStateActiveTurn {
		t.Fatalf("state = %v, want active_turn", state)
	}
}

// TestClassifyPaneStateOldWorkingMarkerIsIdle: an "esc to interrupt" line
// in scrollback (with completed-turn rows between it and the composer) is
// not a live indicator - the pane is idle and Enter is correct.
func TestClassifyPaneStateOldWorkingMarkerIsIdle(t *testing.T) {
	pane := "› earlier prompt\n\n  Working (30s, esc to interrupt)\n\n  done 9:01 AM\n\n› another prompt\n\n  done 9:04 AM\n\n› Ask Codex to do anything\n\n  gpt-6-astra default · /tmp/worktrees/mer-1 · title"
	got, _ := ClassifyPaneState(pane)
	if got != PaneStateIdle {
		t.Fatalf("ClassifyPaneState(old working marker) = %v, want idle", got)
	}
}

// TestClassifyPaneStateQueuedRowsBetweenWorkingAndComposer: queued user
// renders may sit between the live working line and the composer; the pane
// is still mid-turn.
func TestClassifyPaneStateQueuedRowsBetweenWorkingAndComposer(t *testing.T) {
	pane := "› earlier prompt\n\n• Called tool({})\n  └ out\n\n  Working (14s, esc to interrupt)\n\n› queued follow-up text\n\n› Ask Codex to do anything\n\n  gpt-6-astra default · /tmp/worktrees/mer-1 · title"
	got, _ := ClassifyPaneState(pane)
	if got != PaneStateActiveTurn {
		t.Fatalf("ClassifyPaneState(queued rows) = %v, want active_turn", got)
	}
}

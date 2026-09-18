# TUI send-path hardening (slice: feat/tui-send-path-hardening)

Staged after feat/codex-certified-binary-pin (pin promotes first; the live
verdict on the pinned binary stays a clean binary-drift-vs-send-race
experiment). Owner directive: port field-proven techniques from the
open-source prior art, invent our own where it falls short.

## Failure being fixed

Run 35303652394 verbatim: a steered instruction landed in the Codex TUI
composer and was never submitted - the draft sat unsubmitted for 2 minutes
while the daemon believed delivery succeeded (HTTP 200 from send-keys).
tmux accepted the keystrokes; the TUI never turned them into a turn. Delivery
without acknowledgment is indistinguishable from delivery.

## Ported techniques (source -> what we lift)

- AWS cli-agent-orchestrator (awslabs/cli-agent-orchestrator, docs/codex-cli.md):
  load-buffer/paste-buffer delivery with bracketed-paste framing instead of
  keystroke streaming; cancel tmux copy mode before delivery (copy mode eats
  submission keys); clear stale rolling output before sending; record a
  dispatch boundary in the captured output before the submit keystroke;
  refuse to type into a provider whose process appears exited.
- agent-conductor: Enter as a separate explicit keystroke, never combined
  with the text write (libtmux's combined enter=True is flaky - their bug,
  our pre-existing shape already splits them).
- orchestmux: sendPrompt/sendEnter separation with pane output piped to disk;
  we lift the separation and the retained pane evidence.
- openai/codex#28167: paste-buffer + immediate Enter wedges the composer
  (Enter read as another paste event); ~5s avoided it in that build. Proves
  our 300ms enterDelay is not a contract - so the submit wait is adaptive
  (bounded poll for a safe-to-submit signal), not a bigger sleep.
- openai/codex#16045: `-c mcp_servers={}` merges non-destructively and cannot
  clear servers - vindicates the session-scoped CODEX_HOME config.toml over
  inline -c replacement for governed confinement; keep it that way.
- Codex keymap contract (verified at rust-v0.153.4 and v0.155.0-alpha.2.6,
  tui/src/bottom_pane/chat_composer.rs): Enter submits immediately; Tab
  requests queueing while a task is running (Tab submits like Enter when
  idle, so input is never dropped). Queue, not steer, is the TUI's native
  mid-turn semantic.

## Design: state-aware dispatch with positive acknowledgment

Pane-state classification (from capture-pane, codex-specific classifier):
  idle            - composer prompt live, no running turn
  active_turn     - normal turn running (stream/spinner present)
  unsteerable     - approval dialog, review, manual compact, startup modal,
                    copy mode, dead/exited provider
  unknown         - capture failed or layout unrecognized (fail closed)

Dispatch by state:
  idle        -> cancel copy mode; bracketed-paste the text (load-buffer +
                 paste-buffer, no -l keystroke stream); adaptive wait for the
                 composer to be ready (poll capture beyond the 120ms
                 paste-suppression window, bounded); Enter.
  active_turn -> same paste; Tab (explicit queue semantics), never Enter.
  unsteerable -> do NOT inject; hold kennel-side and retry classification on a
                 bounded cadence; surface delivery_pending if the state
                 persists past the bound.
  unknown     -> delivery_unknown immediately; no keystrokes.

Positive acknowledgment after every dispatch (bounded poll of capture-pane):
  submission  = composer no longer holds the draft AND (a new user-message
                renders or a turn transition/spinner appears)
  queued      = codex's queued-message rendering contains the text
Absent ack inside the bound -> delivery_unknown: one bounded recovery
(re-classify and redeliver once); still absent -> honest error to the caller,
pane capture preserved. No ack, no success: the HTTP 200 contract changes
from "tmux accepted keys" to "submission was observed or explicitly unknown".

Placement: the codex-specific classifier + dispatch policy live behind the
agent-adapter seam (ports.AgentMessenger stays the boundary); tmux.Runtime
gains only provider-neutral primitives it lacks (load-buffer/paste-buffer,
copy-mode cancel, capture we already have). daemon's runtimeMessenger resolves
the session's harness and consults the adapter's delivery policy when one
exists, falling back to the current SendMessage for harnesses without one.

Explicitly not in this slice: app-server migration (turn/steer with
expectedTurnId removes this whole race class; post-launch per owner);
changing the send API contract; approval-dialog interaction.

## Test plan

- Classifier unit tests over recorded pane fixtures: idle composer, streaming
  turn, approval dialog, review, compact, copy mode, exited provider,
  unrecognized layout.
- Dispatch tests against a fake terminal: paste+Enter on idle, paste+Tab
  mid-turn, no keystrokes on unsteerable/unknown.
- Ack tests: composer-clear + turn-start observed -> success; draft persists
  past bound -> one recovery then delivery_unknown; queued render -> queued.
- Recovery: redelivery never double-pastes (dispatch-boundary marker checked
  before retry).
- e2e (self-hosted runner, post-promotion): the persistent-session proof's
  first steer is the live verdict for this exact path.

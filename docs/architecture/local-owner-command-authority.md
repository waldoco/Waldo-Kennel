# Local owner command authority

> **vNext extension note (2026-09-15):** S1 source implementation is accepted; packaged macOS/runtime proof remains open. The W1.3 replacement-only continuation below is historical. Extend the same authenticated envelope to answer, steer, interrupt, cancel, and replace under [ADR 0017](../adr/0017-persistent-mission-runtime-and-bounded-supervision.md).


State-changing recovery decisions that claim owner approval must not trust loopback HTTP, the `Host` header, CORS, or body-supplied actor labels. The desktop boundary is:

1. Electron main mints a 256-bit per-app-run owner-command token. It shares the app-run ID and this token with the app-owned daemon once, over the child's inherited stdin, in the same bounded startup envelope as the browser-runtime token. Tokens never enter argv, environment values, files, `running.json`, logs, or renderer JavaScript.
2. The context-isolated preload exposes a narrow typed `approveAttemptReplacement` IPC method, not the bearer. Electron main accepts it only from the primary window's main frame, validates the closed command shape, and asks for approval in a native main-process confirmation that displays the exact Outcome, predecessor, WorkUnit, Plan, Contract revision, and run generation. Only that confirmation sends the command to the daemon.
3. The daemon accepts owner-command creation only on its loopback listener, with an exact constant-time bearer check. The LAN listener blocks `/internal/` independently. Ordinary loopback callers cannot mint a durable decision. Renderer content can only propose the closed command and cannot bypass the native confirmation or access the bearer.
4. The resulting `AttemptReplacementDecision` is immutable and records the authenticated `local-owner:<app-run-id>` principal. Its request fingerprint binds that principal, `replace`, Outcome, predecessor Attempt, Plan, WorkUnit, Contract revision, run-intent generation, and idempotency key.
5. Storage verifies the predecessor's immutable bindings and the current run-intent generation and Contract revision in the same transaction before inserting. Exact request replay returns the prior decision; a changed fingerprint conflicts.

This wave creates authority only. W1.3 must consume the stored decision by ID, recheck every binding/currentness condition, and atomically create the successor Attempt/fence/receipt. No replacement is created here.

A standalone daemon has no app-run owner capability, so the route is not mounted. An app-spawned keep-alive daemon retains only that app run's capability; after the app exits, no caller has its bearer. A later app run cannot mint decisions through that old daemon and must explicitly restart it before owner commands are available. This is intentional fail-closed behavior until another authenticated owner transport is designed.


## Threat boundary

S1 protects owner decisions from unauthorized loopback/LAN callers and compromised renderer content. The daemon process and per-user `kennel.db` are trusted local control-plane state. A same-user process with arbitrary database-write access can already corrupt Attempts, receipts, policies, and executable state; S1 does not claim tamper-proof local storage or add a keyed decision MAC. The migration's insert/update/delete triggers prevent accidental or buggy invariant violations, not provenance forgery by a hostile database writer. Provenance is established at the guarded native confirmation and capability-authenticated daemon insertion. W1.3 may trust a valid immutable decision loaded from this trusted database. OS-keystore-backed integrity for the whole control plane would be a separate architecture project.

## vNext plugin, routine command, and reconnect extension

S1 distinguishes authenticated owner continuations from material authority transitions. Answers to current questions, ordinary turns, and steering inside an already-approved profile use the narrow typed owner capability and do not require a native dialog on every message. Contract/Plan approval, authority or effect widening, profile change, exceptional replacement, and Result Accept require a native confirmation that shows the exact delta.

A plugin never receives the owner bearer. It pairs through a one-time desktop challenge and receives a short-lived adapter capability bound to its digest, harness/version fingerprint, mission, app run, and allowed proposal/transport classes. It may carry owner input to S1 and render results; adapter authentication proves transport only. Owner content carried through it must include a separate short-lived owner proof minted by an authenticated owner surface and bound to exact content hash, mission, target/question generation, command class, and expiry. The plugin never receives the owner bearer. Without that proof the ingress is proposal-only and must round-trip through desktop/CLI. The daemon records adapter and owner authentication separately; the adapter cannot label its own request as owner-authorized or confirm a material change.

The earlier app-run keep-alive restart rule is superseded for vNext by an authenticated reconnect handshake. A new desktop run may attach to a healthy daemon, rotate its owner capability, and resume durable missions without restarting the daemon. The old capability is revoked; in-flight delivery is reconciled before new commands. Standalone and legacy routes remain fail closed.

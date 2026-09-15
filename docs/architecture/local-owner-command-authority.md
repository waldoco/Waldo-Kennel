# Local owner command authority

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

# S1 local-owner command authority evidence

Date: 2026-09-15
Baseline: `5f26b3e038334e5725566bde3339fee809ea17df` on `outcome-loop`

## Implemented source boundary

Electron main now mints a 256-bit owner-command bearer for each app run and hands it, the app-run ID, and the browser-runtime bearer to the app-owned daemon in one bounded base64url JSON envelope over inherited stdin. The bearer is not put in argv or a secret-valued environment variable. A narrow preload proposal is accepted only from the primary window main frame. Electron main validates the closed replacement shape, presents a cancel-default native prompt containing every action-driving identity, and authenticates the resulting daemon request itself.

The daemon mounts the replacement-decision route only when both the startup authority and decision store exist. The route requires loopback control semantics plus constant-time bearer authentication. The LAN listener continues to block all `/internal/` routes. Storage records an immutable decision only for a terminal predecessor bound to the current Outcome/Plan/WorkUnit/Contract/run generation while the desired run state is still `running`. Exact semantic replay returns the original record; changed semantics under the same request key conflict. No successor Attempt is created in S1.

## Source gates run on this package

Passed:

- `go test ./internal/ownercommand ./internal/httpd ./internal/storage/sqlite/store -run 'OwnerCommand|StartupSecrets|LANManagerBlocksLoopbackOnlyControlRoutes|AttemptReplacementDecision' -count=1`
- `go test ./internal/storage/sqlite -run '^Test(SessionListSucceedsOnBurnedMigrationHistory|UsageTablesKeepOnlyDurableCollectionState)$' -count=1`
- `npx vitest run --config vite.renderer.config.ts src/main/owner-command.test.ts src/preload.test.ts --maxWorkers=1` (27 tests)
- focused owner-command TypeScript compilation performed during implementation
- `git diff --cached --check`

The storage tests prove active predecessors are rejected, exact replay is stable, conflicting reuse fails, stale and paused run authority fails, and concurrent independent writer pools converge inside the production public method on one same-semantic decision, without caller retries, and reject a later conflict. Raw direct INSERT tests exercise valid insertion plus active predecessor, wrong Outcome/Plan/WorkUnit/generation/Contract and non-running latest intent; raw UPDATE/DELETE attempts hit the immutable guards. The database BEFORE INSERT trigger independently enforces these authority bindings.

## Review findings and unproven layers

Static source inspection found the owner bearer only in Electron main and daemon startup/auth code, with no successor-creation files in the package.

The full project TypeScript check is unproven in this environment: repeated isolated runs exceeded 110 seconds or exhausted the Node heap. A direct Vite build transformed 3,241 modules but then hit the repository's existing workspace-resolution problem (`react` could not be resolved from `packages/product-ui/src/TaskComposerView.tsx`). A daemon package compile-only gate also exceeded 120 seconds. These broader results are not called passing.

This package remains source-review pending and does not mark S1 accepted. Packaged macOS proof is still required before S1 is considered fully accepted: native confirmation screenshots in dark and light modes, direct renderer/subframe/cancel behavior in the real package, and runtime inspection showing the bearer absent from argv, environment values, files, run records, logs, and renderer globals. This source package does not claim those packaged results.

Focused source seams also cover unavailable/invalid stdin, distinct app-run capability separation, no-Origin loopback requests without capability on localhost/127.0.0.1/[::1], arbitrary and alternate Origin rejection, and LAN spoofed-Host blocking. Keep-alive lifecycle is fail-closed by per-run capability separation; the real app-exit/new-run restart behavior remains part of packaged proof. The immutable insert is the crash boundary: before commit no decision exists, after commit exact replay returns it.

The trusted-state boundary is explicit: SQL triggers protect invariants and programming accidents, not provenance against a hostile same-user writer with arbitrary `kennel.db` access. Native confirmation plus capability-authenticated daemon insertion establishes provenance; the daemon and database are trusted local control-plane state.

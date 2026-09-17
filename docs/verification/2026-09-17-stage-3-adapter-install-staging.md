# Stage 3 adapter install/upgrade staging evidence

- Baseline: `outcome-loop` at `c65051bce745ba05587446da259a2cb19b700f92`
- Design: `docs/handoffs/2026-09-17-stage-3-adapter-install-upgrade-freeze.md`
- Scope: S3.2-only staging and activation kernel

## Implemented boundary

- Typed trusted-release, operation state, drift and repair contracts. The release shape identifies the already-verified trust root; it deliberately does not claim that self-described metadata establishes trust.
- Private prepare-beside-active copies an opened regular non-symlink candidate, hashes while copying, checks exact size and digest, fsyncs, then moves it into an immutable digest/version generation directory.
- Release/manifest/artifact generation binding, monotonic sequence, target OS/architecture, expiry, S3.2 artifact verification and capability/protocol compatibility are fail-closed before activation.
- One process-local mutex plus an exclusive filesystem lease serialize installation. Each operation persists a request digest for exact replay/conflict behavior.
- Existing generation is quiesced before an atomic relative-symlink switch. The old generation is retained; successful health and pairing activation pins the new generation as last-known-good.
- Metadata-only health probe rechecks opened digest, version, live protocol and required classes. It has no owner command surface.
- Any health or pairing failure atomically restores the previous generation. Restart seeing `activated_pending_health` rolls back rather than guessing success.
- A pure safety-precedence drift evaluator covers rollback failure, tamper/local drift, mixed generation, rollback attempt, expired metadata, incomplete activation, required/protocol/source drift, optional degradation and upgrade availability.

## Explicit S3.3 seam

`ports.HarnessAdapterPairingActivator` is required after health. The only implementation included in this slice is `UnavailableHarnessAdapterPairing`, which returns `ErrHarnessAdapterPairingUnavailable`. There is no success stub. Until the reviewed S3.3 implementation supplies this port, every activation rolls back and reports the missing seam.

## Local gates

Focused normal and race tests cover fresh commit with a test-only pairing implementation, exact replay, changed-request conflict, S3.3-unavailable rollback, digest tamper, rollback release rejection, symlink rejection, restart recovery, last-known-good retention, concurrent exact callers and every drift precedence class.

## Unclaimed

- No trusted release network/resolver implementation, download, package marketplace or release publication.
- No S3.3 production pairing/new-generation implementation.
- No S3.5 mission/profile migration or UI.
- No real Mac install/upgrade proof yet. The design freeze's native Mac matrix remains the exit gate.
- This filesystem kernel assumes the Kennel-owned root and candidate are on the same filesystem; product wiring must enforce that deployment fact.

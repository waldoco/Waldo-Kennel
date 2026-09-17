# Stage 3 adapter install and upgrade staging design freeze

- Baseline: `outcome-loop` at `c65051bce745ba05587446da259a2cb19b700f92`
- Extends: S3.2 read-only discovery, manifest classification and supplied-artifact hashing
- Scope: contract and Mac evidence plan only; no installer production code
- Status: design frozen for implementation review

## Outcome

Kennel can install or upgrade only its own adapter artifact through a prepare/verify/activate/health/commit sequence, retain the previous generation until the new one is proved, and roll back deterministically after any pre-commit failure. Installation compatibility grants neither adapter transport authority nor owner authority.

## Security correction to the current architecture prose

A content digest proves bytes match an expected digest; it does not establish who authorized that digest or whether metadata is fresh. Executable installation therefore requires a **trusted release input** in addition to S3.2's content-addressed manifest. The first implementation may use a Kennel-pinned release index or bundled release record, but it must bind adapter ID, version, artifact digest, manifest digest, target OS/architecture, monotonically increasing release sequence and expiry. It must reject rollback, expired/frozen metadata and mixed metadata/artifact generations. Do not claim full TUF security unless the standard is actually implemented.

## State machine

| State | Durable meaning |
| --- | --- |
| `idle` | No staged mutation |
| `prepared` | Candidate copied to a private staging directory; active generation unchanged |
| `verified` | Trusted release, artifact digest, manifest digest, target, permissions and S3.2 compatibility all pass |
| `activated_pending_health` | Atomic active pointer selects candidate; previous generation retained; no active mission migrated |
| `committed` | Candidate passed bounded health checks and is last-known-good for new connections |
| `rolled_back` | Active pointer restored to previous generation; evidence records why |
| `action_needed` | Safe automatic rollback/cleanup cannot be proved; current facts and one repair action are retained |

Exact replay of one operation ID returns its durable state. Reusing an operation ID with changed artifact, release input, target or expected active generation conflicts before filesystem mutation.

## Pipeline

1. **Inventory.** Re-run S3.2 discovery. Record canonical active path, opened-file identity/digest, source, version, protocol fingerprint and active generation. Resolve symlinks and defend against path/file swaps.
2. **Acquire caller-supplied or trusted artifact.** Network acquisition, if later added, is a separate policy seam. Never invoke provider credentials or package-manager login during verification.
3. **Prepare beside active.** Copy by opened file handle into a daemon-owned `0700` staging root on the same filesystem as the active pointer. Candidate file is non-executable until verified. `fsync` file and directory before state advances.
4. **Verify trusted release.** Validate release source, sequence/freshness/expiry, adapter and manifest digests, target OS/architecture and exact artifact size. Reject rollback or mixed-generation metadata.
5. **Verify S3.2 compatibility.** Use the existing manifest classification for version/protocol/classes. `known_compatible` may continue. Optional degradation requires explicit policy. Required missing, protocol drift, digest tamper, invalid, unsupported version or supplied rollback block activation.
6. **Harden candidate.** Reject symlinks, non-regular files, group/world writable paths, unexpected ownership and executable bundles with undeclared files. Set final immutable-by-convention permissions before activation.
7. **Quiesce boundary.** Do not swap during an active adapter call or turn. Fence the old adapter generation. Active missions remain pinned to their current profile and do not migrate automatically.
8. **Activate atomically.** Install under an immutable digest/version directory and atomically replace a small active pointer/symlink on the same volume. Never overwrite the running artifact in place.
9. **Health-check without authority.** Start/probe the candidate in metadata/capability mode, confirm actual opened digest, version, protocol fingerprint and manifest classes, then complete S3.3 pairing for a new connection generation. Health checks must not submit owner content or production effects.
10. **Commit or roll back.** On success, mark candidate last-known-good for new connections and retain the prior generation for a bounded rollback window. On timeout/crash/mismatch, atomically restore the previous pointer, prove its digest/profile, and record rollback evidence.
11. **Garbage collect later.** Never delete active, pinned-by-mission, last-known-good or evidence-referenced generations. Cleanup is not part of the commit transaction.

## Drift classification

One deterministic classifier returns a primary class and one repair action:

| Classification | Meaning | Repair |
| --- | --- | --- |
| `in_sync` | Active opened digest/release/manifest/profile match expected generation | none |
| `upgrade_available` | Trusted newer compatible release exists; active remains valid | `stage_upgrade` |
| `optional_degradation` | Candidate lacks only optional capability | `review_degraded_capability` |
| `required_capability_missing` | Candidate cannot serve required profile | `keep_current_and_repair_compatibility` |
| `protocol_drift` | Live protocol does not match reviewed manifest | `keep_current_and_repair_compatibility` |
| `artifact_tamper` | Active or candidate opened bytes mismatch trusted digest | `quarantine_and_restore_last_known_good` |
| `local_modification` | Active installation differs from Kennel's recorded generation but has no trusted matching release | `review_local_drift` |
| `source_drift` | Canonical path/source/owner changed while bytes may match | `review_installation_source` |
| `rollback_attempt` | Candidate release sequence/version is older than the highest trusted installed release | `reject_candidate` |
| `metadata_expired` | Trusted release input is stale/expired | `refresh_release_metadata` |
| `mixed_generation` | Release, manifest and artifact do not bind to one generation | `reject_candidate` |
| `activation_incomplete` | Crash left prepared/pending state; active pointer truth must be reconciled | `reconcile_or_rollback` |
| `rollback_failed` | Previous generation cannot be restored/proved | `manual_repair` |

Precedence is safety-first: tamper/mixed/rollback/expired, incomplete activation, required/protocol, source/local drift, optional degradation, upgrade available, in sync.

## Crash and concurrency invariants

- Every durable state is written before or after one named filesystem boundary, never ambiguously across it.
- Restart reconciles operation record, active pointer and opened-file digest; it does not infer success from a copied file.
- Only one installer lease may prepare/activate per installation. A stale lease is fenced by generation, not wall-clock alone.
- Losing power before pointer swap leaves current active. Losing power after swap but before commit yields pending-health and rolls back unless candidate health can be positively re-proved.
- Rollback restores the pointer, not an in-place backup copy.
- Exact retries converge. Conflicting retries stop before mutation.
- Secrets never enter release metadata, argv, logs or evidence.

## Mac proof required for Stage 3 exit

Run against a real macOS user account and canonical Codex installation source, with the exact commit recorded. Evidence must include:

1. OS/architecture, canonical paths, ownership/permissions, source classification and pre-install active digest/version/protocol fingerprint.
2. Trusted release and manifest identifiers/digests, artifact digest and staging directory on the same volume. Do not publish secrets or provider credentials.
3. Fresh install into an empty Kennel adapter root, atomic activation, metadata-only health probe, S3.3 pairing and one S3.5 `connected` evaluation.
4. Upgrade to a newer compatible adapter, proving old generation remains present, active pointer changes atomically and the existing mission profile does not change mid-turn.
5. Real desktop quit/reopen after upgrade, proving same durable mission and expected pinned-profile behavior.
6. Injected failures at copy, verification, pre-swap, post-swap/pre-health and post-health/pre-commit; each restart result must match the state machine.
7. Digest tamper, symlink/path swap, wrong owner/mode, rollback candidate, expired metadata, mixed manifest/artifact and protocol/required-capability drift rejections.
8. Health failure rollback to the exact previous digest, followed by a successful reconnect using the previous generation.
9. Two concurrent installers with one activation winner; focused race results where applicable.
10. File tree and pointer snapshots before/prepared/activated/committed/rolled-back, operation ledger, sanitized logs and proof that active/pinned/last-known-good generations survive cleanup.
11. Explicit separation between fixture-backed tests, unsigned local artifacts and genuinely native Mac evidence. Signing/notarization is not claimed unless separately verified.

## Out of scope

- Installing/upgrading the Codex CLI or copying its credentials.
- Auto-downloading unreviewed artifacts.
- Generic marketplace/package-manager design.
- Changing an active mission profile without explicit migration authority.
- S3.3 pairing internals, owner commands, Result Acceptance, release publication or self-update of Kennel.
- In-place overwrite, broad filesystem cleanup, workflow changes, issues, releases or beta operations.

## Implementation ownership proposal

Add one narrow adapter-install operation domain/store and one filesystem adapter rooted in Kennel-owned directories. Reuse S3.2 verification and S3.3/S3.5 boundaries. Do not put install state in `HarnessConnection`, create a general package manager, or refactor discovery. Freeze exact trusted-release provenance before production code begins.

## Sources behind the freeze

- TUF attack model and metadata roles: https://theupdateframework.github.io/specification/latest/index.html
- Nix atomic upgrade/rollback model: https://nixos.org/guides/how-nix-works/
- VS Code extension runtime privilege warning: https://code.visualstudio.com/docs/configure/extensions/extension-runtime-security

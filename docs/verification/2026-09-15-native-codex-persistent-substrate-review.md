# Native Codex persistent substrate review guide

Review only slice 1. Reject the patch if it changes Outcome admission, session ownership, production behavior, UI, storage, migrations, provider parity, or external effects.

## Source review

- Confirm the opt-in test uses existing public Chat interfaces rather than provider wire calls.
- Confirm one actual detached disposable Git worktree and one provider thread span many turns.
- Confirm the proof names `codex_native_worktree_v1`, passes its exact hashed instructions as the Start and resume profile, uses workspace-write/on-request rather than legacy danger-full-access, and records hashed profile/configuration inputs plus inherited skill and negotiated protocol provenance.
- Confirm machine-written cwd/git-root evidence compares symlink-canonical paths so stock macOS `/var` and `/private/var` aliases do not create a false failure.
- Confirm the harness writes the deterministic Go fixture directly, its package regression produces the exact `TestValue` assertion, and the live loop observes that failing test before repair, reruns successfully, and verifies the repaired file.
- Confirm outside-worktree and network success sentinels cannot appear; an approval request is denied rather than treated as proof of refusal.
- Confirm client and provider IDs are asserted, not only logged.
- Confirm interrupt waits for provider-observed activity and targets the exact turn.
- Confirm the first controller is closed before resume, making it a fresh app-server process.
- Confirm resumed history proves all prior coding/boundary/interrupt turns and IDs and a post-resume turn proves continuity.
- Confirm a completed steer is followed by a distinct successful continuation turn in the same thread.
- Confirm typed request/answer coverage remains in the deterministic suite.
- Confirm the test cannot run or spend by default.

## Runtime repair review

See [native Codex runtime-package repair review](2026-09-15-native-codex-runtime-repair-review.md). Confirm protocol compatibility and runtime-companion viability are independently evidenced, selected/canonical executable identity is pinned, failure manifests survive, and generic local/DNS/tool failures cannot satisfy policy-negative probes.

## Evidence review

A source-only pass does not close slice 1. Require an authenticated real-provider run of both persistent substrate and steer tests, a pinned Codex version/binary hash, complete test log hash, IDs, marker evidence, and final workspace state. A skipped test is not a pass.

## Current local result

The deterministic package test passes in the Linux review worktree. The real-provider test is not run here because this environment has no Codex binary. Slice 1 remains open until the live evidence package is independently accepted.

## Revision-2 hardening checks

- Captured `requestUserInput` emits `ChatEventInputRequested`, preserves IDs/options/detail, returns the exact raw answers object, and refuses repeated/stale resolution.
- Cwd and git root are machine-written and compared to the exact disposable path.
- Turn 2 and post-resume each require command activity and create filesystem evidence derived from the prior marker.
- Two history reads return the same ordered, nonempty, unique event IDs and the expected started/user/completed plus exact client ID per turn.
- Interrupt observes command output first, completes within its phase bound, and never writes the terminal sentinel.
- Steer observes work in flight, preserves the exact thread/turn with one start/one completion, produces a machine-checked artifact from the appended guidance before completing, records timing and client IDs, and checks the steering client ID when provider history preserves it. It does not claim that steer cancels the already-running child command; interrupt owns that assertion.
- Live subprocess environment is allowlisted; the wrapper records binary/version/hash and rejects every SKIP or missing named PASS.

## Revision-4 frozen-contract reconciliation

- Rebases cleanly from approved baseline `f31cdca4ea55fb25903f7af55ec45c7abccdcc04`; the only manual merge was `docs/STATUS.md`, resolved to the frozen contract and current stage-1 review gate.
- Uses an actual detached Git worktree and explicit workspace-write/on-request native profile posture on both Start and resume; the exact instructions hashed as provenance are the ones executed.
- Canonicalizes observed and expected worktree paths before equality checks for macOS path aliases.
- Adds inspect/edit/failing-test/repair/rerun, outside-filesystem refusal, network refusal, post-steer continuation, installed/generated protocol fingerprint, and hashed profile/skill provenance.
- Retains the stage boundary: no governed Attempt admission, controller cutover, harness pairing, Outcome state, UI, migration, or stage 2+ code.

## Revision-3 evidence and input-shape checks

- Wrapper manifest records `source_dirty` from `git status --porcelain`.
- Hashed live log records every provider turn ID with its exact client message ID, marker content and SHA-256, and final `git status --short`.
- `requestUserInput` retains both `isSecret` and `isOther`. Secret fields project as password format. An `isOther` question keeps provider options as examples and an explicit other marker rather than an enum-only constraint. Exact raw answers still round-trip unchanged.

## Reconstructed checkpoint review additions (2026-09-16)

- Confirm the source base is exactly `f9095a19ac45175a3dcdbd982b005012a1f7ed12`; do not describe this tree as identical to the lost local checkpoint.
- Confirm the native profile is separate from governed `AttemptExecutionPolicy`, which remains rejected in App Server mode.
- Confirm Start/Resume evidence is labeled coarse and every detailed turn record is labeled not observable by the public provider protocol.
- Confirm receipts hash the exact newline-delimited fully written request and preserve request identity separately from actual write order.
- Confirm the compatibility diagnostic is the only first turn, is daemon-authored, and ordinary/model work is blocked until it passes.
- Confirm interrupt uses bounded item-ID drain plus continuous file stability. A late start followed by matching failed/canceled completion is accepted; success, output, mutation, unmatched activity or deadline residue fails.
- Require focused package, race, ports, vet, backend compile, shell syntax, diff/scope and Git-integrity evidence before a provider call.

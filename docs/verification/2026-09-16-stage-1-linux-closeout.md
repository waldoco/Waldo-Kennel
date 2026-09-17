# Stage 1 Linux closeout and contributor handoff

- Date: 2026-09-16
- Status: implementation complete; authenticated Linux live chain passed on the supported default path; final repository-wide verification is still running on a memory-limited host
- Recovery branch: `instinct/stage1-recovery-f9095a19`
- Current reviewed checkpoint: `643e955c16b7b8907d2ce4c7c285bedaa148d4b6`
- Reconstruction base: `f9095a19ac45175a3dcdbd982b005012a1f7ed12`
- Scope boundary: Stage 1 only. No Outcome cutover, Stage 2 implementation, release, merge, or PR is included.

This is a fresh reconstruction from the named base and surviving source/evidence. It is not an exact recovery of the lost local commits.

## Executive state

Stage 1 now supplies a working Codex app-server substrate on Linux for the default compatible path:

- exact native workspace-write profile at Start and Resume;
- multi-turn inspect, test, repair, rerun and continuation in one provider thread/worktree;
- filesystem and network boundary effects checked as hard outcomes;
- market-compatible tool-event handling, including Code Mode raw `exec` and standard raw `exec_command` fallback correlated by provider call ID and deduplicated against canonical command items;
- honest governance tiers: missing per-operation evidence is recorded as partial instead of blocking working turns;
- Stop that terminates the owned app-server process group when provider interrupt cannot prove process-level quiescence, then resumes the same thread in a fresh controller;
- fresh-process Resume, native history, post-resume work and steering;
- live-turn proof health based on active-turn liveness, not a short wall-clock deadline.

Authenticated Codex 0.154.0 with `gpt-5.5` passed the complete chain at source `643e955`. Luna passed canary and the first exact operation, then stalled on its second turn in repeated runs. Luna is therefore an honest optional/degraded capability result, not a blocker for the compatible default path.

## Timeline

### 2026-09-15: substrate definition and first proof work

1. The persistent-session architecture identified Stage 1 as the provider floor beneath later orchestration: one worktree, one persistent primary provider thread, many turns, steering, interruption, Resume and history.
2. The proof plan and review guide were written under `docs/verification/2026-09-15-native-codex-*`.
3. Early Mac/provider runs established protocol reachability but found proof defects rather than product failures:
   - fixture source passed through model/shell quoting and could fail for the wrong reason;
   - the steer proof incorrectly expected steering to cancel an already-running child command;
   - runtime-companion selection could choose a launcher without its native sidecar.
4. The proof was changed to use harness-owned fixture bytes, test same-turn steer incorporation, and bind selected/canonical runtime identity.
5. Commit `f9095a19` became the reconstruction base used today.

### 2026-09-16: reconstruction, compatibility correction and Linux proof

1. **Fresh Stage 1 reconstruction** (`da4f71d`)
   - Added a typed native sandbox profile separate from governed `AttemptExecutionPolicy`.
   - Added exact Start/Resume profile checks, per-turn request-integrity receipts, runtime hash and protocol digest evidence.
   - Extended the live chain across admission, expected test failure, repair, filesystem/network effects, interrupt, Resume, history and continuation.
   - Kept App Server governed Attempt injection rejected because that private tool boundary is not present.

2. **Provider-shape research and raw fallback** (`305a578`)
   - Exact Codex 0.154.0 source and live traces showed model metadata does not predict event behavior.
   - Added narrow fail-closed parsing for public `rawResponseItem/completed` records.
   - Supported Code Mode custom `exec` calls and standard function-call `exec_command` calls.
   - Paired call/output by `call_id`; delayed fallback completion until turn completion so canonical command items win without duplicate activities.

3. **Compatibility/governance split** (`305a578`)
   - The original proof blocked turns whenever exact provider operation attribution was unavailable.
   - Default behavior now judges compatibility on outcomes and effects.
   - Missing provider operation evidence records `compatible=true`, `governance=partial`, with an explicit reason.
   - Strict evidence remains a supported opt-in posture only where the protocol can provide it.
   - Codex's own sandbox remains enabled and boundary escapes remain hard failures.

4. **Interrupt correctness repair** (`305a578`)
   - Live runs falsified the assumption that `turn/completed: interrupted` or command completion means effects stopped. The shell appended after those notifications.
   - App-server now launches in an owned Linux process group.
   - Interrupt withholds the terminal interrupted event during a short grace period.
   - It then kills the full owned process group, not only the parent, to prevent an orphan shell from continuing mutations.
   - The service resumes the same provider thread with saved launch configuration before Stop returns.
   - Deterministic tests cover deferred reporting and forced shutdown.

5. **Long-turn and liveness correction** (`e4b8577`, `643e955`)
   - A fixed 90-second live-test deadline incorrectly classified slow models as failed.
   - The proof now has a broad safety cap and a 3-minute idle window reset by meaningful active-turn events: reasoning/message deltas, item lifecycle, command I/O, plans, diffs, usage carrying the active turn, and input/approval activity.
   - A follow-up Luna run exposed that thread/account events with no turn ID could create false heartbeats. Only events belonging to the active provider turn now reset liveness.
   - Product turns already had no wall-clock execution timeout; this makes the proof match that product behavior.

6. **Authenticated final default-path reseal**
   - Codex CLI: 0.154.0.
   - Binary SHA-256: `61b0194f3bb6534439c8d26a3ed57d0805f84b884588b761795323eeb92fcf70`.
   - Model/profile: `gpt-5.5`, medium reasoning, native workspace-write profile.
   - Result: PASS.
   - Live log SHA-256: `807a9d9557de9d6c8d2799e9484a21a1badd2bb76950ac6ee6c8c6dc010ec0cc`.
   - Stdout SHA-256: `31286ffac19930505572ff46fdd7d8eef28a4f4eef15af9c4f75a7ca3c0967ba`.
   - Proved: admission, expected failing assertion, repair, rerun, filesystem/network effects, interrupt effect stability, process-tree fail-safe diagnostic, fresh-process Resume, stable history, post-resume work/boundaries, same-turn steer and a later continuation turn.

## What was removed or relaxed

### Removed as default blockers

- **Exact per-operation evidence as an admission requirement.** Codex 0.154.0 does not guarantee it for every model/turn. Absence is now partial governance, not a broken product.
- **Model-name assumptions.** `gpt-5.5`, Luna and other Code Mode models cannot be classified safely from names or catalog metadata. Runtime behavior and negotiated capabilities decide.
- **Short wall-clock test deadlines.** Long thinking is normal. Silence, not duration, is the stall signal.
- **Trust in provider interrupt lifecycle as process quiescence.** Provider notifications are transcript facts, not proof that a child shell stopped.
- **Canonical-item-only command observation.** Raw public call/output events provide a deduplicated compatibility fallback when canonical items are absent.

### Deliberately retained

- Actual failed outcomes and changed effects remain hard failures.
- Filesystem/network escapes remain hard failures.
- Sandbox enforcement remains enabled.
- Parsing of ambiguous/malformed raw events fails closed.
- Governed `AttemptExecutionPolicy` remains separate from the native substrate and unsupported in App Server until its real private execution boundary exists.
- User Stop must stop effects, even if that requires killing and resuming the provider process.

## Reasoning and UI implications

Codex 0.154.0 exposes standard-stream reasoning summary/text deltas and section breaks, plus completed reasoning items. Kennel already normalizes these to reasoning deltas and settled reasoning text. The product can stream provider-supplied thinking summaries. It must not promise hidden chain-of-thought; raw reasoning was not observed on the authenticated account. The Resume raw-event blind spot affects internal raw response telemetry, not standard reasoning events.

The next UI/controller slice should present active-turn liveness from reasoning, message, item and command progress. A quiet turn can become “possibly stalled” with Stop/recover offered; it should not be auto-killed just for taking a long time.

## Current code map for contributors

- `backend/internal/adapters/chatdriver/codexappserver/driver.go`
  - Start/Resume profile mapping, runtime/protocol binding and app-server spawning.
- `backend/internal/adapters/chatdriver/codexappserver/conversation.go`
  - turn dispatch, raw command fallback, native policy evidence, interrupt lifecycle and event normalization coordination.
- `backend/internal/adapters/chatdriver/codexappserver/process_unix.go`
  - owned process-group setup and termination.
- `backend/internal/adapters/chatdriver/codexappserver/process_windows.go`
  - platform fallback. Mac behavior still requires the agreed boundary smoke.
- `backend/internal/adapters/chatdriver/codexappserver/rpc.go`
  - exact serialized transport receipts for turn/start.
- `backend/internal/adapters/chatdriver/codexappserver/live_test.go`
  - full authenticated Stage 1 journey and liveness gate.
- `backend/internal/service/chat/service.go`, `controller.go`
  - typed interrupt restart result and same-thread controller recovery.
- `scripts/prove-native-codex-substrate.sh`
  - sealed evidence runner and manifest.
- `docs/verification/2026-09-15-native-codex-persistent-substrate-review.md`
  - review checklist; this closeout supersedes its stale “live execution pending” status text.

## Build status and open gates

### Complete

- Stage 1 implementation commits pushed to the recovery branch.
- Focused adapter/service deterministic tests.
- Focused adapter race test.
- Authenticated real-provider default-path full chain.
- Luna behavioral probe with a documented second-turn stall.
- GitHub identity `Pin4sf` verified; recovery-branch remote SHA matched local at every checkpoint through `643e955`.

### Running or pending

- A full backend compile on this small Linux host. The parallel attempt was OOM-killed while compiling `anthropic-sdk-go`; a serialized fresh-cache `GOMAXPROCS=1`, `-p=1` run is active. This is infrastructure pressure, not a source failure, until the serialized result says otherwise.
- Repository-wide race, vet, generated-diff, shared/frontend/package gates.
- Final evidence archive and final remote SHA after this document is committed.
- Cheap Mac phase-boundary smoke. Mac is the shipping platform and process-group/path behavior differs; this should run before Stage 2 code.

Stage 1 is implementation-complete and live-proven on Linux's compatible default path, but the final Linux stamp remains pending the repository-wide gates and final package hash. No PR should be opened before owner review.

## Recommended next order

1. Finish and record the serialized Linux verification gates.
2. Commit/push this document and refreshed evidence archive; verify the recovery-branch SHA.
3. Give the owner the review package.
4. Run the structured Mac boundary smoke from the same branch.
5. After review and smoke, begin the Stage 2 architecture/ego audit before writing Stage 2 code.
6. Build W1.0/admission verdict and shared event vocabulary capability-first, with the compatibility/governance and liveness rules from Stage 1.

## Handoff invariants

A new agent must not:

- call this exact recovery of lost commits;
- merge, release, open a PR, modify issues, or start Stage 2 without fresh owner direction;
- weaken actual effect or sandbox failures into governance labels;
- turn missing evidence into a default blocker again;
- infer capabilities from model names;
- claim Luna passed the full chain;
- claim the Linux stamp or Mac support before their remaining gates land.

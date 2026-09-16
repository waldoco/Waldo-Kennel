# Native Codex persistent substrate proof plan

- Status: implementation and review package; live execution pending
- Baseline: `outcome-loop` at `f31cdca4ea55fb25903f7af55ec45c7abccdcc04`
- Slice: 1 only from the [persistent-session execution map](../roadmap/persistent-session-execution-map.md)

## Purpose

Prove the native Codex app-server substrate vNext needs without cutting over Outcome admission or building a second controller. The proof uses the existing Codex driver, persistent Chat contract, normalization, and history reader against one disposable git worktree.

## Existing grounded substrate

Current source already supplies:

- `thread/start` and absolute `cwd` binding in `driver.go`;
- `turn/start` with provider turn ID and `clientUserMessageId` in `conversation.go`;
- typed server requests through `ResolveRequest`;
- `turn/steer`, preserving expected turn ID and client ID, in `steer.go`;
- `turn/interrupt` in `conversation.go`;
- `thread/resume` against a stored thread ID in `driver.go`;
- settled `thread/read` history with stable synthesized `ProviderEventID` and recovered `ClientMessageID` in `history.go`;
- normalized thread, turn, item, command, diff, plan, usage, and request events.

These are code and pipe-test facts. Existing live tests prove one turn, fresh-process resume, and active-turn steering against a real Codex installation. They do not yet form one repeatable persistent-session proof.

## Smallest change

Extend `live_test.go` with one opt-in test, `TestLivePersistentCodexSubstrate`. It:
- creates the tiny Go fixture directly from typed harness-owned bytes before model work begins, so shell/model quoting cannot replace the intended assertion failure with a compile failure;

1. creates a seed repository and one actual detached disposable `git worktree`;
2. resolves and records the installed version, negotiated protocol digest, generated schema version/digest, capability degradation, named `codex_native_worktree_v1` profile-instruction hash, and inherited skill inventory hash;
3. starts one app-server thread with `accept-edits` / workspace-write confinement and records its provider thread ID;
4. inspects the repository, machine-records `pwd` and git root, compares their symlink-canonical paths to the exact worktree, writes a marker and a deliberately failing Go test, and requires the failed command event;
5. repairs that code in the same thread, reruns the test successfully, and requires a completed command event plus exact repaired content;
6. attempts one undeclared write outside the worktree and one undeclared network call without requesting wider authority; both must fail or ask, every request is denied, and neither success sentinel may exist;
7. starts a long local command, observes pre-interrupt output, interrupts the exact turn under a phase deadline, and proves the terminal sentinel was never written;
8. closes the controller/app-server process;
9. resumes the exact stored thread ID from a fresh app-server process and the same absolute cwd;
10. reads history twice and requires byte-identical ordered event IDs, nonempty uniqueness, and exact started/user/completed plus client-ID facts for the coding, boundary, and interrupt turns;
11. starts another command turn after resume and writes a filesystem result derived from the earlier turn.

The existing `TestLiveSteerKeepsTheTurnAndItsWork` stays part of the proof command. It uses an actual detached worktree and the same workspace-write posture. It proves `turn/steer` appends guidance to the active thread/turn, preserves that turn ID, observably incorporates the guidance into subsequent model work, carries a client ID where provider history preserves it, emits no second turn start, and accepts a new command turn afterward in the same thread. It does not require steering to cancel an already-running child command; the separate `turn/interrupt` proof owns active-command cancellation.

Typed request/answer remains a deterministic pipe/conformance gate, because a real model cannot be relied on to ask a specific question. A captured `item/tool/requestUserInput` frame must emit `ChatEventInputRequested` with question IDs, options, descriptions, `isSecret`/`isOther`, password/free-text semantics, and typed form schema; the exact raw `answers` object must return to the provider; repeat and stale resolution must return `ErrChatRequestNotPending`. The live proof fails on any unexpected request. Protocol negotiation and native runtime-companion viability are separate gates: the wrapper first resolves production's canonical runtime identity and runs `TestLiveCodexRuntimeCanary` alone before the longer journey.

## Commands and evidence

Static/repeatable gate:

```bash
cd backend
go test ./internal/adapters/chatdriver/codexappserver -count=1
```

Real-provider gate on a machine with authenticated Codex:

```bash
cd backend
KENNEL_CODEX_LIVE=1 go test ./internal/adapters/chatdriver/codexappserver \
  -run 'TestLive(PersistentCodexSubstrate|SteerKeepsTheTurnAndItsWork)$' \
  -count=1 -v
```

Use the evidence wrapper, which rejects every SKIP and requires both named PASS lines:

```bash
scripts/prove-native-codex-substrate.sh /path/to/evidence-directory
```

If Codex is not on `PATH`, set `KENNEL_CODEX_BIN` to the exact authenticated binary. The wrapper records:

- source commit and machine-recorded `source_dirty` from `git status --porcelain`;
- selected PATH executable plus canonical production-resolved runtime executable; version/hash/launch all bind the canonical target;
- PASS/FAIL/SKIP, exact exit status and complete-log hash even when setup or tests fail;
- profile name and hashed profile/configuration inputs, inherited skill inventory hash, installed/generated protocol versions and digests, negotiated degradation/missing-floor facts;
- `codex --version` and binary SHA-256;
- platform/architecture and Go version;
- test start/end time and exit status;
- provider thread and turn IDs from verbose logs;
- client message IDs and history event count;
- disposable workspace path during the run;
- marker file content/SHA-256, every provider turn ID with its exact client message ID, and final `git status --short` in the hashed log;
- complete stdout/stderr log hash.

The live subprocess receives an allowlist only: `HOME`, identity/path/shell/temp/locale/terminal OS facts; `CODEX_HOME`, `OPENAI_API_KEY`, `OPENAI_BASE_URL` when explicitly present; certificate paths; and proxy variables. No unrelated environment value is forwarded or logged. The evidence log may contain provider-visible response text, so review/redact it before sharing; never retain credentials, environment values, private provider transcript, or hidden reasoning.

## Acceptance gates

- One provider thread ID survives many distinct turn IDs.
- The workspace is an actual detached Git worktree, not only a repository-shaped temporary directory.
- Inspect/failing-test/repair/rerun turns share the exact absolute worktree and marker state.
- `accept-edits` maps the proof to workspace-write/on-request rather than the legacy default danger-full-access posture.
- An admitted local write and local curl-availability control pass first. An undeclared outside-worktree write and undeclared network command then fail with evidence naming the exact operation plus policy enforcement, or request authority that the test denies; generic missing-tool, DNS, host, or curl failures never count. No success sentinel exists.
- Active-turn steer retains the same turn ID, produces one completed turn, and permits a distinct continuation turn afterward.
- Interrupt targets the exact turn and produces interrupted completion.
- Closing the first controller really ends its app-server process; resume starts a fresh process and returns the same thread ID.
- Native history contains prior settled turns with stable provider event IDs and preserved/synthesized client IDs.
- A post-resume turn reads the same worktree state.
- Typed request/answer conformance remains green and never invents consent.
- No Outcome, Plan, WorkUnit, Attempt, MissionProjection, UI, provider-parity, migration, or production admission behavior changes.

## Protocol findings and deferred parameters

Observed protocol resolves these slice-1 parameters:

- provider thread ID is the durable resume key;
- provider turn ID is the exact steer/interrupt precondition;
- `clientUserMessageId` is the provider-carried delivery/idempotency key;
- `thread/read` is required after resume because resume does not replay old notifications;
- absolute cwd is mandatory; relevant development commands execute through native Codex tools in that worktree;
- a steer sent before provider `turn/started` can be refused, so provider acknowledgement, not local dispatch return, opens the steer window;
- request IDs and provider-offered answer shapes are the typed answer boundary.

Not encoded by this slice: durable daemon command envelopes, delivery-unknown reconciliation across a daemon crash, input/output manifests, governed Attempt cutover, attention state, Supervisor policy, or UI projection. Those belong to stages 2-9. Stage 1 only proves native provider compatibility; it does not claim that the current app-server `ValidateExecutionPolicy` admits a governed Attempt.

## Genuine decisions

No owner decision is required for this proof package. It introduces no external effect, persistent user data, budget default, migration, authority change, or production cutover. The real-provider test makes model calls only when a maintainer explicitly opts in with `KENNEL_CODEX_LIVE=1`.

## Reconstructed Stage 1 deterministic checkpoint (2026-09-16)

The earlier local checkpoint was lost before export. This checkpoint is a fresh reimplementation from immutable public base `f9095a19ac45175a3dcdbd982b005012a1f7ed12`, the revision-pinned Codex protocol, and archived evidence. It does not claim byte identity with the lost commits.

Before provider work, Kennel binds the exact native profile and requires a daemon-authored compatibility diagnostic. The profile is pinned after caller settings on every `turn/start`: workspace write, network off, no extra writable roots, `/tmp` excluded, and active `TMPDIR` excluded. Start and Resume record only the coarse fields publicly returned by Codex; detailed per-turn provider observation remains explicitly unavailable. Exact serialized turn requests receive full-write transport receipts with request ID, transport sequence, byte count, and SHA-256, without retaining prompt bytes.

The live journey requires deterministic admission, the intended failing test, repair and rerun, steer incorporation, a distinct continuation, exact-turn interrupt, fresh-process resume, stable history, post-resume continuation and cleanup. Interrupt completion is not treated as a lifecycle ordering fence. The acceptance helper drains late item frames by item ID for a bounded settle window, requires every observed effect item to terminate failed or canceled, and continuously requires the effect file to remain unchanged. Successful completion, unmatched activity, continuing output, file mutation or a terminal sentinel fails the proof.

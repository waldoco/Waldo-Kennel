# Kennel production-readiness program

- **Status:** Active program
- **Date:** 2026-09-14
- **Baseline:** `beta` at `629a9bd` (post-PR #178)
- **Authority order, unchanged:** [ROADMAP](../../ROADMAP.md) sets milestones and exit gates, [STATUS](../STATUS.md) records what is integrated on `beta`, ADRs define accepted semantics. This program orders the remaining production work and records the evidence rule for it. It does not widen any gate, close any issue, or claim anything the linked evidence does not show.
- **Architecture authority:** ADR 0008, ADR 0009, ADR 0011–0013, ADR 0015, ADR 0016, [kennel-build-program](kennel-build-program.md)

## 1. Why this program exists (root cause)

Kennel is approaching a public launch with a working governed core and a
recurring process failure: **stale evidence being read as current truth.**

Two observed instances in September 2026:

1. The 2026-09-13 build-order audit treated issue #115 (bounded repository
   capabilities for governed WorkUnits) as the main unbuilt demo blocker. The
   implementation had already merged through PR #127 (`64b5185` plus six
   hardening commits) and PR #129 (daemon-owned approved checks) *before* the
   audit was written; the audit quoted the PR #110 canary's `needs_you`
   outcome, which PR #127 fixed. The code was built; the belief was stale.
2. Codex compatibility was asserted from a version floor and a static
   capability table. That mechanism cannot see the failure it claims to
   prevent (shape drift under an experimental upstream protocol), and it
   advertised capabilities for builds never exercised (see ADR 0016).

The corrective rule for every claim in this program: **a capability is only
ever as current as its linked, dated, revision-pinned evidence.** Sections 8
and 9 make that rule mechanical.

The product reason for the program: launch requires a stranger to install
Kennel, connect a provider, and complete one governed Outcome loop with proof
— and every public claim on the launch page must be backed by that evidence.

## 2. Current state (verified 2026-09-14 against `beta` @ `629a9bd`)

### 2.1 Shipped and proven (code plus unit/conformance tests; packaged evidence where noted)

- Outcome paradigm core: immutable Contract/Plan, bounded WorkUnit DAG, Attempt, Evidence, Verification, explicit owner Acceptance; serial scheduler (concurrency 1 by construction); restart/replay fences.
- Codex governed execution: frozen-policy Attempts, private MCP with bounded read / scoped write, daemon-only approved checks, fail-closed provider-boundary policy mapping. One packaged live Attempt (2026-09-12/13) plus the #115 packaged canary.
- Governed repository capabilities (#115): see §2.3.
- Reasoning: owner-key OpenAI/Anthropic and native Codex packet-mode planning to `proposal_ready` (packaged, API-assisted); ADR 0015 interactive planning.
- Five provider identities with readiness probes: Codex, Claude Code, OpenCode, Cursor, Pi. Chat drivers: Codex app-server, Claude/OpenCode via ACP; Cursor/Pi refuse chat by design.
- Mission Control: daemon-derived WorkUnit detail (#177), historical Attempt inspection (#178), Result projection with measured per-file changes (#176).

### 2.2 Shipped but not proven in the final package

- The full UI-only planning → execution → Result → rework journey (issue #38 gate). Rehearsal script ready; not yet run on a packaged Mac build at current HEAD.
- First-user onboarding: fresh-profile install → connect provider → add repository → Outcome → Acceptance, without terminal or API rescue. Never rehearsed.
- macOS signing, notarization, DMG, install, update rehearsal (#117). Machinery exists (`frontend/forge.config.ts`); zero published releases or tags as of 2026-09-14.
- Claude/OpenCode chat and Cursor/Pi anything in a packaged build. `docs/STATUS.md` states the boundary verbatim.
- Linux: enforcement fails closed by design today; Acceptance cannot complete (#179, post-launch lane).

### 2.3 #115 status, explicitly

Implemented on `beta` (PR #127 + hardening + PR #129), all five acceptance
criteria met in code and unit/fault-injection tests, packaged canary recorded
2026-09-12 (`docs/verification/2026-09-12-issue-115-governed-repository-tools.md`).
Remaining: one fresh packaged canary at current HEAD (≈15 minutes, requires
the owner's Mac and Codex sign-in). The issue stays open until the owner
closes it; this program never modifies issues.

### 2.4 Missing

- Claude governed Attempts: no `ValidateExecutionPolicy` outside Codex.
- Pi governed anything: spawn/resume plus a reviewer adapter only.
- Parallel WorkUnit scheduling (#122; ADR 0009 fully specified, unimplemented).
- Context/Memory: Context Packet v0 is the honest first slice (#119/#120/#121 lane).
- Release CI: `foundation.yml` builds the packaged macOS app; no signing/notarization/publish workflow exists.
- Codex protocol negotiation: **this program's first PR** (§5).

## 3. Invariants (never weakened to finish a slice)

1. The provider never accepts the Outcome; owner-only Acceptance.
2. Fail closed with a named reason before side effects; no silent fallback between providers, models, protocols, or policy mappings.
3. The daemon, not the provider, runs approved checks.
4. The UI never implies authority, parallelism, or capability that is not shipped and proven.
5. Every status claim carries revision, date, and evidence link — or it is not made.
6. Slices land backend/domain truth before frontend projection, with narrow conformance tests, per the build-program rule.

## 4. The change gate: problem-first and first-principles, by construction

Every production change — code, protocol, UX, packaging, or release — passes
this gate **before implementation starts**, and the record lands with the
change (in the PR body for small work, in a linked design note or ADR for
large work). A change that cannot answer these questions is not ready, and a
reviewer is expected to stop it. This is the execution of the owner's rule:
no unnecessary patches, root cause first, ship only what a thoughtful senior
engineer would sign.

The gate record must answer, with evidence:

1. **User problem and utility.** Whose journey breaks or improves, and where
   in the Outcome loop? If no user-facing problem or capability is named, the
   change is not justified by "cleanup" alone.
2. **Observed evidence and provenance.** What was seen, where, at which
   revision, on which date? Beliefs without a revision-pinned source are
   hypotheses and are labeled as such (§1).
3. **Root cause.** Why does the problem exist — which contract, invariant, or
   assumption produces it? A fix that cannot name the mechanism is a patch
   over a symptom and is rejected.
4. **Governing invariants.** Which §3 invariants constrain the solution space?
   If the fix weakens one, the slice stops instead (build-program rule 10).
5. **Alternatives and tradeoffs.** At least one serious alternative, and why
   it loses. "No alternative exists" requires the search to be shown.
6. **Smallest durable architectural change.** The least that removes the root
   cause at the layer that owns it — not a larger change for elegance, not a
   smaller one that re-opens the problem downstream.
7. **Compatibility, security, privacy consequences.** What changes for
   existing users, stored state, protocols, credentials, and data custody?
   "None" must be argued, not assumed.
8. **Explicit non-goals.** What this change deliberately does not do, so
   review and future sessions cannot mistake scope.
9. **Failure and recovery behavior.** How the change fails when its
   assumptions break (named, fail-closed errors over silent fallback), and how
   a user or the system recovers.
10. **Measurable acceptance criteria.** Observable outcomes a test, canary, or
    rehearsal can prove — not "works better."
11. **Rollout and evidence plan.** How it ships (flag, migration, order), what
    evidence is captured at which revision, and where it is recorded.
12. **Checkpoint update.** Which §9 ledger row this changes, and the
    revision/date/link its status will cite.

### Review intensity follows risk

- **Tier 1 — no behavior change** (docs, tests, tooling, comments): gate
  answered in the PR body; normal review.
- **Tier 2 — behavior change in shipped paths**: full gate record, narrow
  conformance tests, owner review of the exact package before any push.
- **Tier 3 — authority, protocols, security, data custody, release**: full
  gate record plus an ADR, plus live or packaged evidence before any claim is
  made in STATUS or launch copy.

A proposal must also state what would falsify it: the evidence that would
change the decision. Review checks evidence freshness (revision + date) the
same way §8's ledger does. This mirrors the repository's existing public
practice — ADRs for semantics, conformance tests for boundaries, promotion
review for integration — and applies the same discipline top engineering teams
document publicly for design review (written proposals, explicit tradeoffs,
pre-agreed success metrics) without importing any private process.

### Reconciliation

[kennel-build-program](kennel-build-program.md)'s ten-step slice rule remains
the execution checklist for **how** a slice lands; this gate decides
**whether and what** before those steps run. P1 (§5) is the first change
executed under this gate and serves as the reference record: root cause in
§5.1, design and tradeoffs in §5.2, non-goals in §5.3, decision in ADR 0016.

### Banned patterns

- patching a symptom when the root cause is reachable;
- claiming capability from design, branch, or belief instead of linked evidence;
- fallbacks that hide failure (silent provider/protocol/policy substitution);
- UI that implies authority, parallelism, or readiness that is not proven;
- scope growth inside an approved package — approval covers the exact reviewed
  diff, nothing near it.

## 5. Problem P1 and its design: Codex protocol compatibility (this PR)

### 5.1 Root cause

Upstream marks Codex app-server experimental; its generated schema describes
one build and carries no stability promise. Kennel trusted a version floor
(≥ 0.146.0 chat, ≥ 0.153.4 native reasoning) and a static all-true capability
table. Version numbers cannot detect method drift, and the static table
over-advertised. Evidence gathered 2026-09-14: latest published CLI is 0.154.0;
its schema packaging already changed shape (flat → `v1`/`v2` bundles); upstream
docs describe unreleased methods; and a live handshake against 0.154.0 showed
the server accepting and returning fields its own schema omits — so payload
gating against the schema would false-positive while method gating is exact.

### 5.2 Design (implemented in this PR; decision recorded in ADR 0016)

- **Runtime surface negotiation.** The driver reads the installed build's own
  schema (`codex app-server generate-json-schema`, local, unauthenticated),
  parses the method unions, digests with the generator's algorithm, and caches
  per binary identity (path + mtime + size). Failures are not cached and fail
  closed.
- **Fail-closed floor.** Fourteen required method+direction pairs: session
  lifecycle (`initialize`, `thread/start`, `thread/resume`), turn control
  (`turn/start`, `turn/interrupt`), streaming notifications (`turn/started`,
  `turn/completed`, `item/started`, `item/completed`,
  `item/agentMessageDelta`), all three approval requests, and `model/list`.
  Missing any → refused as `ErrChatDriverIncompatible` naming the methods,
  before spawn (Probe) or before `thread/start` (Start/Resume/intelligence).
- **Independent optional degradation.** Each optional capability (steer,
  rollback, fork, rename, compaction, skills, rate limits, MCP reload,
  history, usage, diffs, plans, tools, interactive input) maps to its backing
  methods and switches off alone when they disappear; the UI consumes the
  negotiated set. New provider methods are always tolerated.
- **Provenance.** Installed version, live digest vs pin, degraded set, missing
  floor — exposed via `ports.ChatProtocolProvenanceDriver` and logged at
  session start/preflight.
- **Drift CI.** `codex-protocol-drift.yml` (weekly + manual): installs the
  latest published CLI, runs conformance, regenerates the bindings, fails on
  unreviewed diff. The 0.153.4 pin remains a test fixture only.

### 5.3 What this PR deliberately does not solve

- Payload-shape runtime gating (schema under-describes the server; payload truth stays with generated-type conformance and live activation checks).
- Persisted protocol provenance in Mission Control receipts (needs a schema migration; follow-up C1.6).
- The governed TUI execution adapter (CLI/hook transport, separate surface).
- Claude/Pi governed paths (Lanes C2/C3).

## 6. Production lanes and checkpoints

Order: Codex first (launch provider), then Claude Code, then Pi; orchestration,
context, UI, and release lanes proceed against the same evidence rule.

### Lane C1 — Codex production readiness

| ID | Checkpoint | Evidence required |
| --- | --- | --- |
| C1.1 | Runtime protocol negotiation + drift CI | This PR: unit/conformance tests, live fetch against latest published CLI, live handshake |
| C1.2 | Fresh #115 governed canary at current HEAD | Packaged Attempt on owner Mac; artifact only in leased worktree; restart persistence; recorded in `docs/verification/` |
| C1.3 | #38 packaged UI-only Outcome journey | Full planning → execution → Result → rework on packaged build, recorded; this doubles as launch demo footage |
| C1.4 | First-user onboarding rehearsal | Fresh-profile install → connect Codex → add repo → Outcome → Acceptance, no terminal/API rescue; gaps filed as exact slices |
| C1.5 | Install/update path (#117) | Signed + notarized DMG, published release, update canary, integrity checks, README link verification |
| C1.6 | Protocol provenance persistence (follow-up) | Migration + receipt/record fields surfacing digest, negotiated set, degraded capabilities in Mission Control |

### Lane C2 — Claude Code governed parity (after C1.3)

| ID | Checkpoint | Evidence required |
| --- | --- | --- |
| C2.1 | `ValidateExecutionPolicy` mapping: AttemptExecutionPolicy → `allowedTools`/`permissionMode` plus a daemon-owned `PreToolUse` hook admitting only the approved check vector | Unit + boundary tests mirroring the Codex codexpolicy suite; fail-closed on unmappable policy |
| C2.2 | Transport decision: ACP (owner's interactive login) vs Agent SDK (programmatic; usage bills from a separate SDK credit on subscription plans since 2026-06 — an economics decision for the owner) | Decision recorded in an ADR |
| C2.3 | Packaged live conformance run | Same bar as the Codex canary: init, hooks, clean exit, reconcile, artifact custody |

### Lane C3 — Pi managed-worker depth (after C2)

| ID | Checkpoint | Evidence required |
| --- | --- | --- |
| C3.1 | Plugin/hook surface for Pi (today: spawn/resume + reviewer adapter only; no hooks, no SessionInfo) | Adapter contract tests |
| C3.2 | Governed policy mapping or an explicit documented refusal | Boundary tests; STATUS updated either way |

### Lane O — Orchestration

| ID | Checkpoint | Evidence required |
| --- | --- | --- |
| O1 | #122 WorkspaceLease-safe parallel WorkUnit scheduling per ADR 0009 | Gate tests from #122; UI must not imply parallelism before they pass |
| O2 | Capability-driven routing/briefs (#123) and per-Attempt receipts with effective model, protocol digest, negotiated capability set, token usage | Benchmark + receipt schema evidence |

### Lane M — Context / Memory

| ID | Checkpoint | Evidence required |
| --- | --- | --- |
| M1 | Context Packet v0: bounded, immutable, provenance-bearing projection of canonical facts; Kennel is the sole writer; providers submit typed candidates only | #119-linked tests; token-budget proof |
| M2 | Grounded suggestions (#120), then owner-correctable Memory (#121) behind the admission gate | Admission/correction/deletion contract tests |

### Lane U — UI/UX simplification

| ID | Checkpoint | Evidence required |
| --- | --- | --- |
| U1 | Component/pattern study of the owner's supplied reference site; license check before any copied code or assets | Study notes + license record in `docs/research/` |
| U2 | Simplify the eight-step Outcome journey around those patterns | Playwright smoke gate stays green; visual evidence per changed screen |

### Lane R — Release

| ID | Checkpoint | Evidence required |
| --- | --- | --- |
| R1 | #117 publish + verify install/update (feeds C1.5) | As C1.5 |
| R2 | #118 security reporting route + repository protection | Verified private reporting route; protection settings recorded |
| R3 | Packaging/release CI (signing, notarization, publish) | Workflow run on a release candidate |
| R4 | Dependabot triage before release (#135–#175 open; several high-severity per dependency review) | Merge or documented defer per item |

## 7. Dependencies, rollout, failure handling

- C1.2–C1.5 require the owner's Mac and Codex sign-in; code lanes (C2.1, O1, M1) do not.
- C2 starts after C1.3 so the conformance harness pattern is copied from a proven Codex run.
- O1 must not ship UI affordances before its gates pass (#122 says this verbatim).
- Negotiated-degradation UX: when negotiation switches a capability off, the feature is hidden or explained as unavailable — never offered to fail at runtime. Floor refusal surfaces the exact missing methods.
- Upgrade handling: provider binary change (path/mtime/size) invalidates the cached surface; the next session renegotiates. Daemon restart renegotiates unconditionally.
- Rollback for this PR: revert restores version-floor behavior; no state, schema, or contract formats change.

## 8. Test and evidence requirements

- Every slice: narrow conformance/invariant tests plus the touched-area suites; repo-wide gates per the build-program rule.
- Packaged claims require packaged evidence: owner Mac, recorded, stored under `docs/verification/` with revision and package hash, matching the 2026-09-12/13 pattern.
- Protocol work: the drift CI is part of the evidence loop, not an optional extra.
- No claim in STATUS, ROADMAP, or launch copy may cite evidence older than the revision it describes without saying so.

## 9. Checkpoint ledger

Update rule: append a dated row (or update a row's status **with** revision,
date, and evidence link). Never mark Proven without a link. Never delete
history; supersede in place with a dated note.

| Checkpoint | Status | Proven at |
| --- | --- | --- |
| C1.1 Protocol negotiation + drift CI | In review (PR pending) | Tests + live 0.154.0 fetch/handshake, 2026-09-14, this PR |
| C1.2 Fresh #115 canary at HEAD | Open | — |
| C1.3 #38 packaged journey | Open (script ready) | — |
| C1.4 Onboarding rehearsal | Open | — |
| C1.5 Install/update (#117) | Open | — |
| C1.6 Provenance persistence | Open | — |
| C2.1 Claude policy mapping | Open | — |
| C2.2 Claude transport decision | Open | — |
| C2.3 Claude packaged conformance | Open | — |
| C3.1 Pi plugin/hook surface | Open | — |
| C3.2 Pi governed mapping/refusal | Open | — |
| O1 Parallel scheduling (#122) | Open (ADR 0009 specified) | — |
| O2 Routing + receipts (#123) | Open | — |
| M1 Context Packet v0 | Open | — |
| M2 Suggestions/Memory (#120/#121) | Open | — |
| U1 Reference-site study + license | Open (site pending from owner) | — |
| U2 Journey simplification | Open | — |
| R1–R4 Release lane | Open | — |

## 10. Traceability

Issues (live states verified 2026-09-14; this program changes none of them):
#38 demo gate · #35 evidence/result · #78 · #115 governed repo capabilities (§2.3)
· #116 historical inspection (inspect delivered by #178; engage open) ·
#117 install/update · #118 security reporting · #119 project understanding ·
#120 suggestions · #121 memory · #122 parallel scheduling · #123 orchestration
strategies · #179 Linux lane (post-launch) · #82 governed Waldo replies.
Milestones: <https://github.com/waldoco/Waldo-Kennel/milestones> (#117
launch-readiness track).
Decision records: ADR 0016 (this PR). Prior program: [kennel-build-program](kennel-build-program.md).

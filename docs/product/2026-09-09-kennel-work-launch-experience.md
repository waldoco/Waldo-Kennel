# Kennel Work launch experience and execution handoff

> **Superseded for vNext (2026-09-15):** Historical launch experience; current UI follows persistent mission runtime. Use [persistent mission runtime](../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../roadmap/persistent-session-execution-map.md). Retained for provenance.


Date: 2026-09-09. Status: implementation specification for review; not a claim of shipped or accepted behavior.

This companion refines the [execution plan](../superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md). ADRs 0010–0012, product architecture, ADRs 0008/0009 and repository engineering rules retain authority. Read this document before implementing Work UX, proof or delivery. It replaces software-only launch framing and fills the missing delivery contract; it does not authorize implementing every slice.

## 1. Scope and evidence correction

The owner clarified that the coding agent was assigned L2 only. The Wednesday branch nevertheless contains these commits:

| Commit | Source present | Assignment/acceptance interpretation |
|---|---|---|
| `9396c3844` | Integrated beta foundation through L1 recovery | Live execution/recovery conformance remains open |
| `0b867790c` | L2 reasoning settings, secrets, metrics, recovery | Assigned work; verify against L2 gates |
| `684b9c0a9` | L3-labelled grounding and replan implementation | Repository-focused L3 implementation present; live/general-context gates remain |
| `170230bf0` | L4-labelled schedule API and Plan/Run UI changes | Partial L4 implementation; direct WorkUnit graph and full experience incomplete |
| `d617b4ee3`, `c83684c11` | Follow-up corrections and evidence | Reported passing; independent follow-up review remains open |

Do not discard this work, duplicate it, cherry-pick it blindly, or call a slice complete from its commit title. Assignment history does not erase implemented code. Complete the missing behavior rather than restarting L2/L3. Before each slice, produce a delta map: requirement → existing code → missing behavior → test/evidence. Candidate commits are a reuse starting point. If an L2-only integration is desired, inspect cross-slice dependencies before splitting the branch; do not silently include L3/L4. Do not push, merge or start a subsequent slice from this document alone.

## 2. The product we are building

Kennel helps an owner turn an Outcome into a verified result they can use, while retaining authority over execution and acceptance. Outcomes are general: research, analysis, documents and software are examples, not separate responsibility models.

Launch claims must be narrower than the domain model: advertise only tested combinations of input, provider, capability and deliverable. A research Outcome needing live web access cannot silently run under a no-network policy or invent research. A document Outcome can use explicitly supplied local material. A software Outcome can use a Git repository. State unavailable capability before authorization and give an actionable path.

The complete loop is:

Project/context → Outcome/Contract → proposed Plan → owner authorization → governed execution → evidence/checks → owner review/rework → Acceptance → delivery.

Delivery may precede acceptance as a labelled draft export, but an accepted-result export must bind the accepted revision. Acceptance is a judgment; delivery is transfer of a particular artifact. Neither proves installation, merge, publication, or an external result not observed by Kennel.

Launch focus: Work, necessary Project/settings/setup, contextual clarification/replan, and technical inspection. Disable Home, Island/notch startup and standalone Waldo side chat in the launch experience. Preserve their data and code. Existing historical deep links should explain availability and offer Work navigation, not silently delete or migrate content. Contextual Outcome reasoning stays available; hiding side chat must not remove clarification.

## 3. Information architecture

```text
Work: Project filter + Outcome Board | List
  └─ Mission Control: one selected Outcome
      ├─ Contract / Plan & Graph / Evidence & Result
      ├─ Contextual question, feedback and decisions
      └─ WorkUnit detail
          └─ Attempt history / Session inspector
```

Reuse WorkShell, existing stage surfaces, board/list primitives, inspector, settings, typography and components. The existing Enter/Understand/Decide/Act/Prove stages become milestones within one selected Mission, not competing primary navigation. Preserve routes where practical; do not add a parallel Work application. Board/List selection and returning from inspection must retain Project, Outcome, filters and scroll position.

### Outcome Board and List

Both are projections of the same top-level Outcomes, not session lists. Default: active top-level Outcomes; filters include Project, needs attention and history. Proposed launch columns: Define, Ready to authorize, In progress, Needs you, Ready for review, Accepted. Map these labels from existing daemon facts; add a daemon projection if a fact is missing, not a stored renderer status. Suspended/unknown belongs in Needs you with a precise reason. Child Outcomes appear in their parent Mission by default, with an explicit all-Outcomes filter when supported.

A card/row shows title, Project, current milestone, concise blocker/next decision and proof progress when known. Clicking opens Mission Control. No automatic provider launch on navigation or drag. Omit drag-to-status unless the action maps to an existing validated command with explicit semantics. List offers the same actions and information, keyboard accessible.

### Mission Control: shared header and decisions

Always show Outcome title, current Contract/Plan revision, truthful state and one primary next action. Actions can be Clarify, Review Plan, Authorize, Start, Pause, Resolve, Review result, Request changes, Accept or Export. They must be derived from daemon eligibility. Authorization and execution are separate API operations; if a combined UI action is later offered, label both effects explicitly.

Keep a visible decision history: what changed, which revision was approved, what needs the owner. Provider transcript is optional inspection, not the source of canonical state. Missing reasoning credentials leads to settings and returns to the same draft without losing input.

### Contract and contextual reasoning

Capture desired result, observable criteria, constraints, non-goals, evidence expectations and delivery format/destination. Select context explicitly: registered repo/folder, supplied files and Project Brief. Sign-in does not attach context. Unsupported input paths are unavailable, not implied capabilities.

Reasoning may ask a material clarification; retain the full answer and prior proposal. Separate facts inspected from assumptions. No-key, refused, timeout, cancelled and interrupted states retain owner input and allow deliberate retry. No deterministic placeholder proposal or silent provider fallback.

Edits create revisions and expose their effect on approval/proof. Never modify approved authority in place. Feedback to replan must carry expected revision and replay identity; stale or duplicate responses cannot replace newer owner decisions.

### Plan and Mission Graph

A direct Outcome graph contains WorkUnits and dependency edges. A composed Outcome graph contains contributing Outcomes with navigation to their own Missions. Never mix these into an untyped graph. Each direct node opens a WorkUnit panel with objective, expected output, criterion text, dependency titles, capability scope, exact provider/model semantics, routing reason, latest Attempt, evidence and blockers. Raw IDs belong in technical detail; human-readable labels are primary.

Before approval, the graph is a proposed topology, not a runnable schedule. Afterwards overlay the daemon schedule: waiting for dependency proof, ready, executing, paused, failed, unresolved, or satisfied as supported by canonical facts. Do not invent percent complete. Show unknown explicitly. Node layout may be computed in React; execution eligibility may not.

Initial operating limit: serial execution behind current custody fencing. Branching edges indicate independence in the Plan, not simultaneous execution. Show this limit clearly. Parallelism requires a separately assigned scheduler/lease/integration slice under ADR0009; do not remove fences to enable it.

Graph controls: fit view, zoom, keyboard node navigation, accessible node labels and a full equivalent list. Select a node without launching it. Retain selection across CDC updates. Large plans remain bounded by the daemon's existing limit. Use existing graph machinery if suitable; record a concrete gap before adding dependencies.

Start/Continue invokes daemon selection, with the approved Plan and an idempotency key. Named-unit requests, if exposed, remain assertions checked by the daemon. Show why nothing is runnable. Double-click, reconnect and retries cannot duplicate execution. CDC updates schedule, Outcome, Attempt and proof queries; a stale screen must not be represented as current truth.

### Evidence, result, rework and acceptance

Display actual produced files/diffs and checks, bound to Contract/Plan/WorkUnit/Attempt. Distinguish provider claims, owner observations and independently observed checks. Unknown or failed checks remain visible. A command summary or arbitrary URL is not content integrity.

The daemon reconciles terminal runtime facts and proof before assigning success; provider completion alone never accepts an Outcome. Downstream WorkUnits consume retained, attributed upstream artifacts. Cancellation and ambiguous recovery prevent automatic continuation.

Request changes retains the current result/evidence, captures feedback and creates the appropriate new revision or Attempt through existing service rules. Acceptance is an explicit owner decision against the reviewed revision, with confirmation of what was and was not proved. Accepted history remains inspectable.

### Delivery: part of completion

Minimum local delivery: owner chooses Export result. For a document/research Outcome export the retained deliverable and a provenance/check manifest; for software export a portable patch or bundle whose application has been tested. Select the mechanism only after verifying it preserves required binary files, additions, deletions and modes; refuse unsupported cases. Do not silently omit dirty/untracked outputs.

L5 retains: repository/context identity, base/result revision where applicable, producing lineage, relative artifact paths, content digests, verification references and retention state. Use existing artifact/receipt storage where sufficient; add only missing fields with migrations. Delivery records identify the exact artifact version, format, destination and observed success/failure. Cancellation or partial export cannot be marked delivered. Resolve paths safely, reject traversal/symlink escapes, and ask before overwriting existing user files.

Export must not mutate the owner's source branch. Provide application/opening instructions and disclose prerequisites. Test reapplication into a disposable copy at the recorded base, including conflicts. Never infer merge/publication from export. PR creation, automatic integration, deployments and sending remain separately authorized future boundaries. Retain artifacts/workspace until a documented safe cleanup policy applies.

## 4. Ordered implementation packets

One assigned packet per agent task; stop at its handoff. Each packet starts by reading current STATUS and the old plan's corresponding slice. These additions refine, not erase, the existing narrow tests and invariants.

### L2 — close the assigned reasoning work

Entry: inspect `0b867790c` and relevant fixes, including dependencies on candidate later code. Paths: settings service/controllers/UI, secretstore, daemon/waldo_reasoning, LLM adapters, IntelligenceRun store/recovery.

Required: provider-bound credentials and environment override behavior; no cross-provider credential transfer; no key exposure in responses/logs; truthful readiness; bounded calls/retries; durable interrupted state; cancellation and late-response guards; actual request/provider/model/usage attribution. Test setup and provider switching with synthetic canaries, daemon launch without inherited env, and return to the preserved draft. Verify the previously reported fixes independently.

Exit: focused + required repository gates, real-daemon settings journey, evidence table for each advertised reasoning adapter. A real credential is required for live reasoning verification; the execution code-mode host is not a blanket blocker for settings/UI or direct HTTP-adapter tests. Report each blocker at its actual boundary. Hand back L2 evidence before starting L3.

### L3 — general grounded Contract/Plan and revision semantics

Entry: owner assigns L3 after L2 review. Audit existing `684b9c0a9`/follow-up code; do not rebuild it by default. Paths: intelligence repository_context/intake_adapter/llm, outcome plan_intelligence/plan, existing DTO/store and contextual intake UI.

Required: bounded, cancellable, secret-safe context; failures cannot be interpreted as permission to include ignored files; exact input digest; user material and Project Brief for non-repo context where supported; no false inspection claims. Clarifications/replan preserve substantive context and source attribution. Retain assumptions/blockers. Scope external research honestly. Replan must be idempotent for retries while a deliberate new request creates a new proposal.

Exit: grounded request test with distinctive local source/check, ignored/unignored sensitive files excluded, cancelled traversal bounded, changed context changes digest, stale response rejected, reload creates no new inference, explicit feedback creates immutable revision. Real reasoning produces a reviewed Contract/Plan for one software and one local-document example before both are advertised. No execution before authorization.

### L4 — Board/List and one complete Mission experience

Entry: owner assigns L4 after L3 review. Audit `170230bf0`; full Plan cards plus an API do not establish Mission Graph acceptance. Paths: WorkShell, _shell.work, OutcomeMissionControl, OutcomeDecideAuthorizeSurface, OutcomeRunSurface, useOutcome, event-transport, daemon scheduler/controller DTOs.

Required: Board and List contain Outcomes; shared selected Mission; full direct WorkUnit graph plus accessible list; per-node reasons/evidence links and exact binding; safe serial labels; daemon selection; CDC refresh; stale/error/offline states; integrated clarification/feedback; keyboard/back-navigation and context retention. Preserve historical composition inspection. Do not enable unwired decomposition controls or advertise the proposer.

Exit: actual daemon-backed desktop journey with A→B plus independent C, intentionally scrambled API array order, all nodes visible, dependency-blocked B cannot start, double click admits once, external Attempt/proof changes update the open screen, mutable Project preference cannot reroute an approved unit. Verify Board/List toggle, graph keyboard fallback and inspector return. Fixture tests alone are insufficient.

### L5 — execution, proof, continuation and usable delivery

Entry: reviewed L1/L4 behavior and explicitly assigned L5. Split into reviewable sub-packets: L5a terminal reconciliation/receipt + artifact handoff; L5b governed checks/evidence + serial continuation; L5c export/delivery. Each defines its contract before dependent UI work.

Required: own the missing AttemptSucceeded transition using truthful runtime/proof rules; bounded agent-facing submission through canonical validation, with submitted evidence treated as claims; independently executed checks; exact lineage; idempotent continuation; cancellation/restart safety; downstream artifact continuity; owner rework/acceptance; delivery contract above. No parallel event/status authority or unrestricted shell endpoint. Consult current proof APIs before adding another write surface.

Exit: A produces artifact, B consumes that exact artifact after restart, a failing check blocks progress, correction succeeds, owner reviews/accepts, exported artifact opens or patch reapplies and reproduces verified content. Duplicate ingestion/start/export never manufactures extra success. Test non-code file output as well as Git changes for the advertised product cut.

### L6 — focused launch shell and owner guidance

Disable Home, standalone side chat and Island automatic startup through reversible configuration; preserve legacy data. Start in Work; retain essential settings, projects and inspector. Add in-product first-Outcome guidance and an owner guide covering setup, authorization, questions, recovery, evidence, rework, acceptance and export. Do not hide contextual Outcome reasoning. Resolve empty/loading/offline/no-provider/history flows. L4 owns basic navigation; L6 polishes and removes distractions rather than introducing a new navigation model.

Exit: fresh install and returning owner can complete the loop without understanding session internals; unavailable historical links have safe navigation; disabling ambient surfaces starts no extra provider process.

### L7 — release decision and measured quality

Rehearse on the intended macOS package and isolated data. Only claim tested OS/architecture and provider/capability/mode combinations. Windows/Linux makers are packaging capability, not rehearsal evidence. Measure reasoning latency/tokens, spawn/recovery latency, context collection bounds and responsiveness with representative history before optimizing. No optimization removes authority/proof guards.

Exit: two advertised Outcome examples complete from setup through delivery with real provider evidence, interruption/rework coverage, no secret leakage and usable owner docs. Every journey is pass/fail/blocked. Missing credentials/runtime, blocked delivery or an incomplete normal UI journey means not release-ready, regardless of the date.

## 5. Decisions and scope limits

- General Outcome semantics are required; universal execution capability is not claimed.
- Board/List and Mission Graph are core, not optional cosmetic follow-up.
- Serial execution is the initial launch limit; parallel scheduling is substantive future work.
- Composition remains canonical. The automated proposer is unaccepted future work until explicitly assigned; oversized requests must offer scope reduction, not an unexplained dead end.
- Provider identities are preserved; admit only proven roles and effects. No provider-name fallback.
- Landing/mobile/cloud-auth surfaces are outside this local Work launch implementation; preserve them and avoid implying their release readiness.
- Delivery and owner documentation are included. Publishing/integration effects are separate.

## 6. Agent handoff template

Assigned slice/sub-packet only: <ID>. Base SHA: <verified revision>. Dependency SHAs: <list>. Read AGENTS, STATUS, this experience document and the matching execution-plan packet. First produce a current-code delta map; identify candidate code already present and reuse it where correct. Implement only this assignment. Do not delete unrelated surfaces/data, weaken invariants, or proceed into the next slice. No push/merge/deploy without authorization.

Return: base/head/branch; changed behavior; migrations/generated contracts; tests and exact evidence locations; pass/fail/blocked matrix; omissions; known risks; next dependency. State separately code present, automated verification, real-daemon verification, real-provider verification and owner acceptance. A blocked external test is not proof of a passing implementation. Keep logs redacted and durable evidence inside docs/verification where appropriate.

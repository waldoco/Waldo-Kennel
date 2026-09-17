# Pre-execution UX + technical-debt audit

> **Classification (2026-09-15):** Historical UX/debt audit; reuse findings remain references, while runtime topology follows ADR 0017. Use the [canonical documentation map](../README.md).


- **Date:** 2026-09-08
- **Status:** Required companion to the Outcome Control Plane MVP implementation plan
- **Original branch:** `feat/wt3-routing-outcome-first`, merged as PR99. Implement from latest beta.
- **Primary implementation plan:** `docs/superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md`
- **Architecture:** ADR 0010 + ADR 0011
- **Canonical product architecture:** `docs/product/kennel-v1-product-architecture.md`


## Post-PR99 source correction — 2026-09-08

This audit remains the UX reuse contract; its original “currently” statements below describe the pre-merge audit unless reverified here. The [execution plan](../superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md) and [STATUS](../STATUS.md) now own remaining-work ordering.

- Confirmed still present: Run/Plan review selects first WorkUnit; Run sends Project harness; direct/composed Mission surfaces remain separate components; proof form writes Outcome-scoped evidence.
- Corrected interpretation: the daemon no longer selects a hidden Codex fallback for exact-bound Attempts. It rejects a conflicting legacy harness supplied by the UI. Remove that normal-client field, not the exact-binding check.
- New connection gap: service schedule has exact WorkUnit proof requirements; Outcome-level UI proof does not satisfy them. Expose the existing schedule and bind proof correctly rather than weakening validation.
- Final source/test correction: exact binding is stored and checked at Outcome admission, but Manager.Spawn does not invoke its resolver. Explicit and provider-default launch semantics both fail a recording-adapter regression by inheriting the mutable Project model. L1a is the first code fix.
- Newly implemented: real Contract/Plan LLM adapters and IntelligenceRun persistence. Do not rebuild the old session-backed analyzer or deterministic floor.
- Fresh frontend baseline: 23 failures across NewTaskDialog, TaskComposer, Sidebar and SwitchAgentDialog. See the [verification record](../verification/2026-09-08-post-pr99-launch-baseline.md); distinguish stale fixture assumptions from production bugs before changing tests.

## 1. Verdict

The Outcome Control Plane MVP plan is aligned with the existing planned/current Waldo Kennel Work UX. The implementation should **evolve the current five-stage Work surface set rather than invent a second UX**.

The current renderer already contains most of the intended product structure:

```text
WorkShell
  Enter
  Understand
    AdaptiveIntakeSurface
    IntakeContractReview
  Decide & Authorize
    OutcomeDecideAuthorizeSurface
  Act & Observe
    OutcomeRunSurface
    docked OutcomeAttemptTerminalPanel
  Prove & Close
    OutcomeProveCloseSurface

plus
  OutcomesOverviewSurface
  Sidebar Outcome tree
  OutcomeMissionControl for composed Outcomes
  CreateProjectFlow / CreateProjectAgentSheet
```

The primary debt risk is not missing UI. It is accidentally preserving two competing mental models:

1. Outcome-first Work surfaces; and
2. inherited project/orchestrator/session-first navigation and launch semantics.

The execution session must remove that ambiguity rather than add more screens.

## 2. Frozen UX spine

The MVP keeps the existing five-stage Work mental model:

```text
Enter
  ↓
Understand
  ↓
Decide & Authorize
  ↓
Act & Observe
  ↓
Prove & Close
```

These are product stages, not independent routes/products. `WorkShell` remains the persistent Work chrome and `/work` remains the normal Outcome destination.

Do **not** introduce a parallel wizard such as:

```text
New Outcome → Contract page → Plan page → Mission page → Run page
```

when those responsibilities already fit the current stage surfaces.

Stage transitions are driven by durable daemon facts. The frontend must not persist a second lifecycle database.

## 3. Surface-by-surface reuse contract

| Existing surface | MVP disposition | Required change | Must not become |
| --- | --- | --- | --- |
| `CreateProjectFlow` | **Keep** | Preserve Project/Workspace chooser, Git preflight, error recovery | a new onboarding wizard |
| `CreateProjectAgentSheet` | **Keep + reframe** | Worker = execution preference; Orchestrator field = coordinator/planning preference; both optional; readiness truthful | a session launcher |
| `WorkShell` | **Keep** | Preserve current Figma-derived chrome and docked terminal behavior; adapt labels/controls to Outcome semantics | another session board shell |
| `AdaptiveIntakeSurface` | **Keep** | Use IntelligenceRun provenance; no execution spawn | generic task composer |
| `IntakeContractReview` | **Keep** | Continue editable owner-reviewed Contract proposal | hidden auto-approval |
| `OutcomeDecideAuthorizeSurface` | **Keep + extend** | Show intelligent Plan, WorkUnits, assumptions, routing/provider/model reasoning, capabilities, replan/edit, Approve Plan | second Plan page |
| `OutcomeRunSurface` | **Refactor into direct-Outcome Mission Control body** | Present WorkUnits/Attempts and next action; exact-bound execution; keep terminal drill-down | a SessionsBoard clone |
| `OutcomeMissionControl` | **Generalize** | Become the one Mission Control concept with direct and decomposed variants | decomposition-only competing destination |
| `OutcomeAttemptTerminalPanel` | **Keep** | Stay docked inside Outcome context | primary navigation target |
| `OutcomeProveCloseSurface` | **Keep** | Evidence/verification/acceptance against Contract criteria | provider-completion screen |
| `OutcomesOverviewSurface` | **Keep** | Global Outcome Board/List; truthful attention state | global session dashboard |
| `Sidebar` | **Refactor hierarchy** | Projects → Outcomes first; execution/session details subordinate/legacy | orchestrator/session tree as primary model |
| legacy session routes | **Keep as deep-link/history compatibility** | Reachable from Attempt/Inspector/history only | normal new-work destination |
| unfinished Home | **Hide/disable as primary beta navigation** | Preserve code if still needed later | competing unfinished product surface |

## 4. Project onboarding contract

Do not redesign project creation during the MVP reset.

Preserve:

- current Workspace vs Project chooser;
- native folder picker;
- Git/repository validation and initialization recovery;
- installed/authorized/supported provider inventory;
- model selection components;
- current design-system primitives and translations.

Change semantics only where required:

### Worker field

Meaning:

> Default execution preference for Outcomes in this Project.

It is **not** authorization and does not launch anything.

### Coordinator/orchestrator field

User-facing meaning becomes:

> Preferred coordinator/planning intelligence for this Project.

It may be empty. It does not create a persistent orchestrator session during Project registration.

The old persisted field may remain as a compatibility seam during the MVP if changing it everywhere would increase risk. User-facing copy and new-work behavior must follow the new semantics.

### Project creation postcondition

After project creation:

```text
Project exists
provider preferences may exist
zero Outcome execution Attempts
zero execution AgentSessionRefs
zero mandatory persistent orchestrator session
```

The next normal action is creating/opening an Outcome.

## 5. Understand / Contract UX contract

`AdaptiveIntakeSurface` remains the entry experience.

Expected interaction:

```text
user states desired result
  ↓
intent is persisted
  ↓
Waldo/intelligence analyzes available Project context
  ↓
if materially ambiguous: ask one useful question
otherwise: propose Contract
  ↓
owner edits/reviews
  ↓
owner confirms
```

Avoid questionnaire UX. Ask only material questions.

ADR0012 removed the deterministic/offline proposal floor. Show reasoning setup, failure and explicit retry without claiming a canned proposal is understanding or silently changing provider.

Loading/waiting/refusal/expiry/cancellation states remain first-class UI states.

## 6. Decide & Authorize UX contract

Do not create a separate Plan route.

Evolve `OutcomeDecideAuthorizeSurface` in place.

The review should answer:

- What work will be done?
- In what order?
- What assumptions were made?
- Which WorkUnit is responsible for which expected evidence?
- Which provider/model is recommended for each WorkUnit?
- Why was it recommended?
- What capabilities/effects are required?
- Where will Kennel stop and ask?
- What exactly am I authorizing?

Required actions:

```text
Edit/revise/replan
Approve Plan
```

Remove session-centric copy such as “start sessions” where it implies the user is authorizing a session rather than a Plan.

Approval is visually and semantically distinct from proposal generation.

## 7. One Mission Control, not two

This is the most important UX-debt prevention rule discovered in this audit.

Today:

- `OutcomeRunSurface` is the direct Outcome Act & Observe surface;
- `OutcomeMissionControl` is primarily the composed/decomposition surface.

The MVP must not leave these as two unrelated concepts.

Target concept:

```text
Outcome Mission Control
  Contract / summary context
  Current attention / next action
  Plan / WorkUnits
  Attempt lineage under each WorkUnit
  Evidence / verification progress
  terminal/session drill-down
```

Rendering varies by Outcome shape:

### Direct Outcome

```text
Mission Control
  WorkUnit 1
    current/past Attempts
  WorkUnit 2
    current/past Attempts
  ...
```

For the serialized MVP, ordering is shown truthfully. No fake parallel graph.

### Decomposed Outcome

```text
Mission Control
  contribution layer
    Outcome A
    Outcome B
    Outcome C
  selected contributor → its own WorkUnits/Attempts
```

Implementation may generalize `OutcomeMissionControl`, compose `OutcomeRunSurface` inside it, or rename/refactor components. The **product must expose one Mission Control mental model**.

Do not ship a `Mission Control` button that opens one representation while `Act & Observe` opens another unrelated representation of the same Outcome.

## 8. Act & Observe information hierarchy

Primary information is not a list of sessions.

Order of importance:

1. Outcome current state / next action;
2. WorkUnit state and dependency/serial order;
3. blockers / decisions / action required;
4. evidence/proof progress;
5. selected provider/model and Attempt lineage;
6. terminal/native session details only on drill-down.

Existing `SessionsBoardGridView` / `SessionsListView` visual primitives may be reused if useful, but the semantic rows/cards must represent WorkUnits/Attempts rather than presenting the experience as a generic Sessions board.

No guessed percentage-complete from provider text.

## 9. Sidebar / navigation hierarchy

Target hierarchy:

```text
Project
  Outcome
    (optional contributing Outcome)
```

Normal Outcome click:

```text
/work?project=<id>&outcome=<id>&stage=<derived>
```

Never:

```text
Outcome click → /projects/<id>/sessions/<id>
```

Sessions remain accessible from:

- active Attempt terminal/inspector;
- historical compatibility views;
- explicit low-level/debug paths where retained.

The project-level orchestrator shortcut must not remain a dominant normal-work affordance once coordinator preference is no longer a persistent Project session.

Browser back/forward must preserve the Outcome context around terminal inspection.

## 10. Board/List contract

Global/project Board and List are Outcome projections.

They prioritize:

- Outcome title/desired result;
- attention state;
- Waldo next-action summary;
- Plan progress;
- proof coverage;
- blockers/action required.

Session count/provider activity can appear as subordinate metadata when useful.

The global empty state CTA must lead through Project selection/registration to Outcome capture, not generic session creation.

## 11. Prove & Close contract

Keep the current final stage.

Required distinction:

```text
execution finished ≠ Outcome accepted
```

The UI must show:

- Contract criteria;
- evidence linked to each criterion;
- verification result/state;
- unresolved/failed proof;
- explicit owner action to accept, reject/request more work, or continue.

Accepted Outcomes remain inspectable with full lineage.

## 12. Loading, empty, failure, stale and recovery states

Every stage must have intentional states for:

- initial loading;
- empty/not-yet-created;
- intelligence unavailable;
- intelligence running;
- intelligence rejected/expired/cancelled;
- stale Contract/Plan revision;
- no valid provider candidate;
- provider not ready after Plan approval;
- execution running;
- execution waiting for input;
- execution failed;
- runtime state unknown/unconfirmed;
- daemon/app restart recovery;
- evidence incomplete;
- verification failed;
- accepted/history.

Do not solve these by redirecting to a generic Sessions screen.

`unknown` and `unconfirmed` remain honest product states.

## 13. UX debt guardrails

The implementation session must obey all of the following unless an evidence-backed architecture correction is documented:

1. **No second lifecycle.** Daemon facts derive stage/attention; frontend does not persist another lifecycle state machine.
2. **No second Work shell.** Reuse `WorkShell` and the current `/work` destination.
3. **No second Contract/Plan/Mission screens.** Extend existing stage surfaces.
4. **No new wizard for Outcome creation.** Adaptive intake remains conversational/minimal.
5. **No session-first automatic navigation.** Terminal/session inspection is explicit drill-down.
6. **No dead controls.** Hide/disable controls whose target is not truthful in the MVP.
7. **No visual fake concurrency.** Serialized MVP renders serial execution honestly.
8. **No provider-brand policy in UI.** UI renders daemon routing/readiness facts.
9. **No redesign for redesign's sake.** Preserve current design tokens, typography, spacing system, product-ui primitives, motion/reduced-motion behavior, and Figma-derived shell structure.
10. **Preserve accessibility.** Keyboard reachability, focus behavior, semantic labels, back/forward, and screen-reader status messages remain part of acceptance.
11. **Preserve i18n.** New visible copy goes through the current translation system; do not scatter hardcoded production strings.
12. **One action, one authority meaning.** “Approve Plan” authorizes execution; “Open terminal” inspects; “Accept Outcome” closes responsibility. Avoid ambiguous CTAs such as “Start session” in the primary flow.
13. **Progressive disclosure.** Provider/session/runtime detail stays available without dominating normal supervision.
14. **Historical compatibility is not new-work UX.** Old routes/data may remain readable without remaining visible as primary navigation.

## 14. Technical-debt guardrails

1. **Keep the Go daemon/SQLite canonical.** Do not create a Node/Python parallel control plane.
2. **Reuse existing domain lineage.** Outcome → ContractRevision → PlanRevision → WorkUnit → Attempt → AgentSessionRef → Evidence → Verification → Acceptance.
3. **IntelligenceRun is non-authoritative.** Do not create a second execution abstraction around it.
4. **Ports/adapters stay narrow.** Provider SDK/CLI differences do not leak into Outcome services/UI.
5. **Generated contracts remain generated.** OpenAPI/TypeScript and sqlc are regenerated from source.
6. **Migrations are additive.** Do not rewrite merged migrations.
7. **Replace before delete.** Gate/remove AO authority paths only after their Outcome-first replacement works.
8. **Contain compatibility.** Legacy session APIs/routes may remain, but new services must not depend on them as responsibility truth.
9. **One routing authority point.** Recommendation happens in Plan formation; approval freezes binding; Attempt executes the frozen binding and never reroutes.
10. **No duplicated readiness logic.** Provider inventory/readiness/capabilities are daemon facts reused by onboarding, planning and execution admission.
11. **No transcript parsing as canonical state.** Receipts/evidence/structured callbacks remain the continuity path.
12. **No premature full scheduler rewrite.** Ship the truthful serial vertical loop first; then implement ADR 0008/0009 parallel leases/DAG scheduling.

## 15. Existing UI mismatches that the implementation must explicitly remove

The audit found concrete current code that still reflects the old mental model:

### Project setup

`CreateProjectAgentSheet` still exposes fields named `workerAgent` and `orchestratorAgent`. Internal compatibility names may remain temporarily, but user-facing meaning and side effects must become preference-only.

### Direct execution

`OutcomeRunSurface` currently reads mutable Project worker roles and passes a harness into `startAttempt`. This must be removed. Start must execute the exact approved WorkUnit binding.

### Session-board semantics

`OutcomeRunSurface` currently adapts Attempt lineage through generic Sessions Board/List primitives. Reuse visual primitives only if the result reads as WorkUnit/Attempt supervision rather than “manage your sessions.”

### Mission Control split

`OutcomeMissionControl` currently represents composed Outcomes while direct Outcomes use `OutcomeRunSurface`. Generalize to one Mission Control product concept.

### WorkShell view switch

`WorkShell` currently uses a `SessionsViewSwitch` and an `outcomeRunViewMode` name. Internal compatibility can be changed incrementally, but user-facing Board/List semantics must be Outcome/WorkUnit-oriented.

### Sidebar

The sidebar currently maintains both Outcome hierarchy and live orchestrator/worker session navigation. Outcomes become primary. Session/orchestrator controls must be demoted to inspection/history/legacy paths.

## 16. Test matrix required before MVP acceptance

### Project setup

- create Project with no provider configured;
- create Project with worker/coordinator preferences;
- no persistent execution/orchestrator session created as a side effect;
- provider not ready is remediation, not silent fallback.

### Understand

- capture → intelligence → optional question → Contract review;
- zero execution Attempts/AgentSessionRefs;
- refresh/restart restores intake state;
- missing/invalid reasoning configuration has actionable remediation and explicit retry; no canned fallback.

### Decide & Authorize

- Plan proposal visible without execution;
- routing reason/provider/model visible;
- edit/replan does not execute;
- stale revision blocks approval safely;
- approval freezes exact binding.

### Act & Observe

- Mission Control opens after approval;
- serialized WorkUnits display truthfully;
- exact-bound Attempt starts;
- Project preference changed afterward does not affect it;
- terminal opens as docked drill-down and closes back to the same Outcome;
- restart does not duplicate Attempt;
- unknown runtime remains unknown.

### Prove & Close

- evidence linked to criteria;
- verification visible;
- provider completion does not accept;
- owner can accept/request more work;
- accepted Outcome/history remains inspectable.

### Navigation

- Outcome click always opens Outcome stage;
- global empty CTA enters Outcome capture;
- no normal create/confirm/approve flow navigates to session route;
- legacy session deep links remain readable if retained;
- unfinished Home is not presented as a complete beta destination.

### UX quality

- keyboard navigation/focus states;
- loading/empty/error/action-required views;
- reduced motion behavior;
- no broken translation keys;
- no overlapping/duplicate top bars;
- no dead Mission/Graph/terminal buttons;
- no unsupported concurrency implication.

## 17. Execution rule for the next session

The next implementation session must treat this document and the main implementation plan as one package.

Before coding, read:

1. ADR 0010;
2. ADR 0011;
3. `docs/product/kennel-v1-product-architecture.md`;
4. `docs/product/2026-09-08-outcome-control-plane-mvp-reset.md`;
5. `docs/product/2026-09-08-pre-execution-ux-and-tech-debt-audit.md`;
6. `docs/superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md`;
7. the WT3 routing plan.

When an implementation choice appears ambiguous, prefer in order:

1. canonical Outcome/authority truth;
2. reuse of the existing five-stage Work UX;
3. reuse of existing components/chassis;
4. smallest reversible refactor;
5. only then a new abstraction or surface.

Any new top-level route, lifecycle state, canonical object, provider fallback, or Mission/Plan/Contract surface created during implementation requires an explicit explanation of why the existing architecture cannot represent the requirement.

## 18. Final falsification check

The MVP has accumulated UX/technical debt if any of these are true after implementation:

- the user can create the same work through both an Outcome flow and an equally prominent session/task flow;
- there are two Mission Control concepts for direct vs decomposed Outcomes;
- Project creation secretly starts the coordinator/orchestrator execution session;
- a Plan page and Decide & Authorize page duplicate one another;
- direct Outcome supervision is still fundamentally a Sessions Board;
- the renderer owns lifecycle truth the daemon also owns;
- changing Project worker preference changes an already-approved Attempt;
- the user must open provider transcripts to understand routine Outcome state;
- an agent ending successfully makes the UI imply the Outcome is accepted;
- the UI implies parallelism the current scheduler cannot deliver.

If any is true, stop and fix the architectural/UX conflict before calling the MVP complete.

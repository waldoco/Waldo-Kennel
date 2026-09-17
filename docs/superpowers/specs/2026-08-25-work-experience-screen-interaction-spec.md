# Work Experience — screen and interaction specification

> **Superseded for vNext (2026-09-15):** Historical Work UX specification; its session topology is superseded. Use [persistent mission runtime](../../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../../roadmap/persistent-session-execution-map.md). Retained for provenance.


- **Status:** Canonical Work UX companion specification
- **Original date:** 2026-08-25
- **Consolidated:** 2026-09-04
- **Parent:** `2026-08-25-work-control-plane-canonical-flow-design.md`
- **Architecture authority:** `docs/product/kennel-v1-product-architecture.md`
- **Implementation truth:** `docs/STATUS.md`

This specification fixes the information hierarchy, interaction semantics, and required failure states for the Work experience. It does not claim the final Graph/Project Brief/scheduler are already shipped.

## 1. User job

The user should be able to state a result, correct Waldo's understanding, approve a bounded way of achieving it, supervise multiple/retried agents without becoming their dispatcher, and decide whether the desired state is actually complete.

The user should not need to:

- translate goals into provider-specific prompts;
- keep a coordinator model session permanently open;
- infer progress from terminals;
- remember which session owns which responsibility;
- reconstruct scope after a provider/context restart;
- accept work because an agent says “done”;
- understand every backend object before creating useful work.

## 2. Interface mental model

```text
Projects
  ↓
Project Brief + Board/List of Outcomes
  ↓
Outcome Mission Control
    Contract | Graph
  ↓
WorkUnit
  ↓
Attempt / Session Inspector
```

The same Waldo agent appears throughout; its working context narrows with navigation depth.

Never label a Session as the Outcome/task. One Outcome can have no Session, one Attempt, many concurrent WorkUnits, failed/recovery Attempts, and several providers over time.

## 3. Project rail and onboarding

### Project rail

Show Projects first. Under an expanded Project, show only a small number of active/pinned/attention-worthy top-level Outcomes plus `View all outcomes`. Do not recursively render all contributing Outcomes or provider Sessions in the rail.

### Add Project

Offer three clear entry modes:

- **Open Repository**
- **Open Folder**
- **Start New**

After selection, show:

- Project name/path;
- detected Git/repository state;
- default provider preference;
- readiness status;
- setup/run/test configuration when known;
- parallel-write capability.

Provider inventory comes from the daemon and includes Codex, Claude Code, OpenCode, Cursor, Pi. Non-ready providers remain visible with a specific reason/remediation when possible.

Never silently replace the user's selected provider.

### Non-Git warning

A non-Git folder can still be registered. Explain that parallel isolated write WorkUnits require a supported workspace strategy; v1 normally uses Git worktrees. Offer Git initialization where appropriate. Do not block Project Brief, Outcome drafting, research, or honest single-writer use.

### Start New

Create the Project directory/config, allow the Project Brief to be drafted immediately, and initialize Git before enabling parallel write execution when the user accepts that path.

## 4. Project Brief

Project Brief is a first-class Project surface separate from Outcomes.

Show current revision and allow edit/revision history for:

- purpose/direction;
- product/technical context;
- architecture/conventions;
- important constraints;
- setup/run/test expectations;
- provenance/last update.

Changing the Brief should not visually imply current Outcomes were silently rewritten. If Waldo detects a material conflict, show a proposal such as `Review affected Outcome Contract`.

There is no special “Primary Outcome.” Pin/Focus is a navigation preference on ordinary Outcomes.

## 5. Board and List

Board/List are projections of the same Outcome store.

Recommended active Board columns:

```text
NEEDS YOU | IN PROGRESS | WAITING | READY FOR REVIEW
```

Accepted/Archived work is accessible through history/filter, not kept as active “Done” clutter by default.

### Outcome card hierarchy

1. Outcome title / desired-state shorthand;
2. Waldo's one-line current state or requested decision;
3. attention state;
4. Plan progress (structural execution);
5. proof coverage (criteria proved);
6. blocker/recovery detail when relevant;
7. provider/session summary only as subordinate execution context;
8. last meaningful change.

Do not show guessed “agent 63% complete.”

### Progress semantics

- **Outcome:** segmented/ring Plan progress may show WorkUnit completion/active structure.
- **WorkUnit:** discrete state (queued, waiting, runnable, working, needs-you, failed, recovered, completed/retained, verified as applicable).
- **Session:** runtime dot/state only.
- **Proof:** criterion coverage count/progress.
- **Acceptance:** explicit user closure.

A valid UI state is `Plan 6/6 · Proof 5/7` and the Outcome remains open.

### Composed Outcomes

Default Board shows top-level active Outcomes. A decomposed parent card may summarize `3 contributing Outcomes · 1 needs attention · 5/7 criteria proved`.

Contributors live primarily inside Mission Control. If a child needs user judgment, parent attention rolls up and points to the exact child rather than duplicating the entire hierarchy on the Board.

## 6. New Outcome flow

The first interaction is natural language, not a blank schema form.

Compact modal/sheet:

- selected Project;
- “What would you like to make true?”;
- Analyze/Continue;
- cancel.

Waldo can ask one material clarification at a time. Show why the answer matters when asking.

The Contract proposal then shows/editably structures desired state, success criteria, Evidence expectations, review method, constraints/non-goals, authority, and stop conditions.

A model proposal is not canonical until daemon validation/user confirmation. Do not derive Contract/Plan approval from transcript text.

## 7. Mission proposal and authorization

The product distinguishes two proposal shapes:

### Decomposition proposal

Use when the desired responsibility itself separates into independent Outcomes. Show contribution to parent criteria, authority narrowing, dependencies if any, and independent proof/acceptance consequences.

### Plan proposal

Use when one direct Outcome needs multiple execution nodes. Show:

- WorkUnits;
- dependency arrows;
- why each unit exists;
- provider/role capability assignment;
- workspace/write/effect implications;
- evidence/review responsibility;
- concurrency/cost guidance where known.

Authorization is explicit when the proposal requires new material authority/cost/effects. The UI must not silently rewrite an authorized Plan when routing/topology changes materially.

## 8. Mission Control

Selecting an Outcome opens Mission Control as a right-side/docked/expandable workspace depending on layout. The semantic toggle is:

```text
Contract | Graph
```

not `List | Graph`.

### Contract mode

Show:

- Outcome title + Contract revision + current state;
- current desired state;
- success criteria + proof coverage;
- contributing Outcomes when decomposed;
- constraints/non-goals;
- authority / pause triggers;
- current Plan summary when direct;
- Evidence / Verification / Acceptance state;
- revision/staleness warnings.

### Graph mode

Do not render relationships not backed by daemon state.

Direct Outcome:

```text
WorkUnit A ──► WorkUnit B ──┐
                            ├─► WorkUnit D
WorkUnit C ─────────────────┘
```

Decomposed Outcome:

1. contribution layer;
2. selected contributing Outcome;
3. that Outcome's WorkUnit DAG.

### WorkUnit card

Default card is compact:

```text
● Readiness API
  Working

  Codex · 14m
  3 files · 2 checks
```

Expand only when needed:

```text
Readiness API
├── Attempt 1
│   └── Claude Code × failed
└── Attempt 2
    └── Codex ● working
```

A Mission with five WorkUnits and twenty Sessions/retries still shows roughly five primary execution nodes.

### Failure/recovery

When an Attempt fails, keep WorkUnit/Outcome identity stable. Show recovery lineage: `Attempt 1 failed → Attempt 2 recovery working`.

Do not create a new Outcome card because a provider process failed.

## 9. Session Inspector

Selecting an Attempt/Session opens deep technical detail:

- provider transcript/native chat;
- terminal;
- diff/files;
- worktree/branch/commit;
- browser/preview;
- runtime status/logs;
- process controls;
- provider-native child/subagent details when exposed;
- receipt/reconciliation facts.

Breadcrumb:

```text
Project / Outcome / WorkUnit / Attempt / Provider
```

Normal supervision should not require this view.

### Direct instruction

A normal user answer to a responsibility-level `Needs You` item is recorded at Outcome/DecisionRequest level and routed to affected execution. Directly steering one Session is an explicit advanced action. A material scope/authority/effect change becomes the appropriate Contract/Plan/permission decision rather than being hidden in chat.

## 10. Project Waldo

Project Waldo is a persistent Project-scoped relationship over bounded episodes, not one immortal provider context.

Header shows explicit scope such as:

```text
Waldo · Waldo-Kennel
Context: Project
```

or:

```text
Waldo · Waldo-Kennel
Context: Outcome “Provider reliability”
```

Waldo may explain state, draft Outcomes/Contracts/Plans, answer Project questions, summarize receipts/evidence, or route decisions. Canonical mutations still go through daemon APIs.

The conversation should use bounded Project context/receipts rather than replaying all prior transcripts.

## 11. Provider selection and readiness UX

Use one daemon inventory/readiness projection across onboarding, Settings, WorkUnit assignment, reviewer selection, and provider handoff.

Useful statuses:

- Ready;
- needs auth/config;
- not installed;
- installed but role capability unavailable;
- probing/temporarily unhealthy.

A provider may be ready for worker execution but not structured coordinator/review/switch roles. Explain the exact unavailable capability rather than hiding the provider or allowing a broken action.

Cross-provider “switch” should be phrased as handoff/retry when a new Attempt is created. Preserve/preview the continuation packet when material.

## 12. Needs You and Decision Requests

Surface consequences, not raw provider output.

Bad:

> Claude needs input.

Good:

> API migration needs your choice between compatibility strategies.

Each decision view should contain:

- exact Outcome/WorkUnit affected;
- recommendation;
- alternatives;
- consequence of deferral;
- authority/cost/effect change if any;
- inspect path.

Answering one decision should update all affected execution through canonical daemon state rather than asking the user to copy the answer between terminals.

## 13. Prove & Close

Show criteria with exact Evidence/Verification state. Distinguish deterministic checks, producer self-checks, independent sessions/providers, and owner walkthrough truthfully.

When ready, present explicit Accept/Rework/Reopen actions.

Provider/CI/verifier success may make the Outcome `Ready for Review`; only the user can Accept.

For composed Outcomes, the UI may batch review ergonomically while preserving separate immutable Acceptance decisions per responsibility.

## 14. External activity

Observed external provider sessions may appear in Project Activity/Sessions with a clear `Observed` label.

Offer explicit actions:

- attach as research/support;
- create WorkUnit;
- create Outcome;
- propose Project learning candidate;
- leave as Project Activity.

Never auto-attach because text/repo similarity looks high.

## 15. Waldo Island

Island is an ambient projection of daemon attention state.

Collapsed example:

```text
! 1 needs you · 2 outcomes active
```

Expanded items name consequence and Outcome first. Provider/session identity is secondary.

Island may deep-link to Mission Control/Session Inspector. Closing it never changes execution truth.

## 16. Failure/degraded states that must exist

- daemon unavailable: preserve local draft, disable canonical mutation, show reconnect;
- provider missing/auth required: exact remediation, no silent fallback;
- stale Contract/Plan: show impact and require explicit revision/re-authorization where needed;
- dependency blocked: name upstream WorkUnit;
- workspace provisioning failure: name failed stage and retained debris;
- provider state unknown: show `Unconfirmed`, not Done/Dead;
- external effect outcome unknown: block unsafe retry and request reconciliation/decision;
- offline/stale renderer projection: label last confirmed snapshot;
- cleanup failure: surface inspect action; never hide by force-delete.

## 17. Figma semantic anchors

Re-open the actual Figma design before pixel implementation. These IDs freeze semantics:

- Board `3253:35386`
- Contract drawer `3253:36397`
- Mission Graph `3264:37187`
- Session Inspector area around `3144:26031`

The current Kennel design system in `DESIGN.md` remains visual authority. This spec governs object/interaction meaning.

## 18. UX acceptance test

A successful dogfood run lets the user:

1. define and authorize a real Outcome;
2. see multiple WorkUnits progress/recover truthfully;
3. answer only material decisions;
4. understand retained changes through Waldo/receipts;
5. inspect raw Sessions only when desired;
6. see Plan completion and proof as separate;
7. explicitly Accept only after reviewing current evidence.

If routine work still requires watching every terminal, the product has not passed its own UX thesis.

# Interactive planning UX contract

> **Superseded for vNext (2026-09-15):** Historical planning UX contract; fresh Contract/planning thread topology follows ADR 0017. Use [persistent mission runtime](../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../roadmap/persistent-session-execution-map.md). Retained for provenance.


The simplest useful shape is one inline step inside Mission Control:

```text
Confirmed Outcome
      ↓
Choose planner + Read this repository
      ↓
Short planning conversation
      ↓
Review Plan
      ↓ owner approval
Serial WorkUnit supervision
```

## What the owner sees

- A single planner picker. Ready choices are selectable; unavailable choices explain the one next action. There is no automatic fallback.
- One default context choice: **Read this repository**. An optional supplied-document packet appears only when one is already approved.
- A familiar compact composer. The planner's ordinary text stays brief.
- Clarifications as a question with a recommended answer and a few alternatives.
- Contract-change suggestions as **Review Contract change**. Accepting the suggestion opens the existing Contract editor; it never edits or reconfirms behind the owner's back.
- A Plan card only after a valid structured proposal is compiled. WorkUnits, dependencies, provider/model binding, authority, checks, assumptions, and blockers remain reviewable before **Approve Plan**.

## Interaction budget

The ordinary path should feel like one conversation, not configuration work:

- keep the confirmed Contract collapsed to a one-line goal plus criteria count;
- require an explicit planner choice, or remember the owner's last explicit choice when it is still admissible;
- show the context choice once, before planning starts;
- keep the latest planner question and composer in the primary reading path, with earlier turns collapsed;
- make **Review Plan** the sole primary action when a proposal is ready;
- keep provider IDs, model provenance, digests, grants, routing, and raw run details behind **Details**.

Do not add a stepper, full-screen wizard, required transcript review, or repeated confirmation for read-only planning. Delight comes from continuity, useful defaults, immediate acknowledgement, and preserving the owner's place—not decorative motion.

## State mapping

| Daemon fact | UI treatment |
|---|---|
| no ready candidate | compact setup state; no Start button |
| `active` + `waitingOn=owner` | composer enabled |
| `active` + `waitingOn=provider` | one in-place thinking indicator; composer disabled |
| `active` + `waitingOn=owner` + `PLANNING_REPLY_AMBIGUOUS` | compact interrupted-reply notice with one **Try again** action; never auto-resubmit |
| provider/model mismatch | concise binding-changed notice and one **Start fresh** action |
| clarification turn | short question card |
| Contract-change proposal turn | review action into existing Contract editor |
| `proposal_ready` | Plan review is primary; conversation remains readable |
| `superseded` | explain Contract changed; one **Start fresh** action |
| `cancelled` | read-only history with **Start new planning** |

## Progressive disclosure

The main surface shows Outcome, planner, latest exchange, and Plan summary. Repository digest, IntelligenceRun identity, exact routing rationale, grants, checks, and full WorkUnit details stay available in disclosure rows. Provider transcript or terminal is never required for ordinary planning.

The transition into execution reuses the same surface: after **Approve Plan**, the Plan card becomes the approved summary and the existing WorkUnit schedule appears directly beneath it. There is no second handoff screen and no need to copy prompts into a provider UI.

## Safety language

Use one concise disclosure near context selection:

> A bounded snapshot of this repository is sent to your selected reasoning provider. The planner cannot run commands, edit files, or use network tools while planning.

Plan approval and execution remain visibly separate actions. A generated Plan is ready for review, not accepted and not running.

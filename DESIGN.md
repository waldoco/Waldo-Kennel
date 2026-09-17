# Kennel design contract

**Current product topology:** Outcome -> Contract -> Plan -> WorkUnit -> Attempt -> Result. Sessions and harnesses are execution details, not the information architecture. The runtime contract is [persistent mission runtime](docs/architecture/persistent-mission-runtime.md); the screen/data contract is [Work experience](docs/product/experience.md).

## Product shape

Kennel uses two layers:

1. **Portfolio:** a calm Board/List answering which Outcomes are moving, need the owner, or are done. `Needs You` is durable state, never a toast.
2. **Focused Outcome:** one place to answer one question, grant exact authority, inspect evidence, or take control. Contract, Plan, Execution, Result and History share one inspector pattern.

Chat is clarification transport and the escape hatch for unanticipated content. It is not the application shell. Every task type shares a stable skeleton: identity, attention, one next action, live state, freshness and proof. Typed modules add code, research or data details. The renderer reads durable state; no LLM runs on render, hover, polling or inspector open.

## Work surface

- Board and List use the same server-owned attention and next-action projection. Cards stay short; details belong in the focused inspector.
- The inspector preserves identity and selection across live updates. Its layers are summary, session/terminal activity, and explicit takeover. It exposes disconnected, stale, pending, refused and delivery-unknown states.
- `Needs You` renders the exact generation-bound question or choice, context, recommendation/options, and answer delivery state.
- Plan authorization shows the frozen Contract binding, WorkUnit graph, checks, permissions, context provenance and digest before authority is granted.
- Result leads with auto-collected criterion-to-evidence proof. Manual owner walkthrough is a disclosed fallback.

## Mission Control canvas

Mission Control is in launch scope. Keep the two graph meanings separate:

- a direct Outcome graph is WorkUnits plus Plan dependency edges and daemon Schedule state;
- a composed Outcome graph is contributing Outcomes plus decomposition dependencies.

Never imply concurrency while the project custody fence serializes execution. The current list-like graph remains the semantic/accessibility fallback. The intended visual layer is a spatial canvas with stable nodes, routed dependency edges, fit/zoom/pan, selected-node inspector, and dominant Running/Needs-You paths. `@xyflow/react` (React Flow) is the preferred direction **pending a measured accessibility, bundle and performance spike**; it is not an implemented dependency or settled fact. If the spike fails, measured DOM nodes with an SVG edge layer are the bounded fallback.

Node faces show only title, typed state/attention, freshness and one next action. Criteria, attempts, activity, artifacts, checks and terminal controls stay in the shared inspector. Topology swaps atomically only after a new Plan revision is authorized.

## Color canon

Color is rare and semantic. Four saturated status colors are canonical:

- **Blue:** active selection, live execution, focus and primary action.
- **Orange:** Needs You, warning and owner action required.
- **Green:** proven, accepted or verified success.
- **Red:** destructive action, failed/refused state or unsafe boundary.

Neutral near-black/gray surfaces carry structure. Color always has a text/icon equivalent. Do not spend status colors on navigation, decorative gradients or progress dots. No shimmer prose, continuous “thinking” animation, confetti, 3D effects or error shake.

## Shared interaction rules

- Semantic elements, full keyboard paths, visible focus, reduced motion and non-hover access are required.
- Known content stays visible during refresh. Loading never replaces it with fake content.
- Live swaps are restrained (140-180 ms); no animation may move focus or resize graph nodes as summaries arrive.
- Generated summaries are cached, non-authoritative and labeled with source sequence/time. Stale summaries say what they are through; deterministic facts remain available if generation fails.
- Raw provider activity is inspectable in terminal/activity. It never becomes authoritative state or Outcome acceptance.

## Settings information architecture

Settings separates configuration from authority:

- **General:** appearance, behavior, shortcuts and onboarding replay.
- **Intelligence:** provider/model/readiness and credential status.
- **Repository context:** bounded project/context defaults.
- **Harnesses & Authority:** adapter identity/version/digest, verified connection state, requested/granted capability classes and scope, pairing approval/denial, active connections, revoke and durable receipts.
- **Updates and Help.** Mobile-device pairing remains separate from harness authority.

A generic composer is never an adapter control plane. “Connected” requires verified pairing/proof state. Destructive/revocation actions state their custody and command consequences.

## Retained implementation constraints

Use existing `components/ui` and `kennel-product-ui`; do not create a third component system. React Query owns authoritative DTOs. Zustand/local state owns only viewport, selection, disclosure, drafts and modals. Existing legacy session/AO screens are compatibility or drill-down views and do not define new Outcome-first design.

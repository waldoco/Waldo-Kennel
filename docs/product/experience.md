# Kennel Work experience

Kennel uses a calm portfolio and one focused Outcome surface. The renderer projects durable daemon state; conversation is clarification transport, not the shell. Current planning service and UI exist, but the literal installed/provider-verified `/mission` command does not yet exist.

## Lifecycle

- **Understand:** capture an Outcome, resolve material questions and freeze a Contract revision.
- **Plan and authorize:** conduct a durable planning conversation, produce a typed Plan/WorkUnit DAG and approve its exact Contract binding and capabilities.
- **Run:** follow server-owned WorkUnit state, answer generation-bound Needs-You requests, inspect activity or take control.
- **Prove and decide:** bind evidence and verification to criteria and immutable results; the owner accepts, requests rework or reopens.

## UX invariants

- `Needs You` survives navigation/restart and carries one exact next action.
- Authority, execution state, freshness and acceptance are backend facts, never inferred by the renderer.
- Raw activity, typed durable facts and intelligence summaries stay visibly distinct.
- Generated summaries run only at durable transitions, persist provenance and never hide deterministic fallback facts.
- Legacy session records remain labeled drill-down/history, not Outcome authority.

## Data/transport legend

- **Row:** durable domain row/projection from Kennel's backend.
- **DTO:** current renderer API response named from `frontend/src/api/schema.ts` or hook type.
- **CDC:** event connection invalidates/patches React Query; direct fetch is recovery.
- **IR:** `IntelligenceRun`, invoked at a state transition, validated and persisted. Never invoked by render, hover, polling or inspector open.
- **Deterministic:** server/client presentation over durable facts, with no LLM.

React Query owns every authoritative DTO below. Zustand owns only local view, selection, disclosure, viewport, draft and modal state.

## Screen 1 - Onboarding and first Outcome

**Actual files:** `OnboardingTour.tsx`, `CreateProjectFlow`, `GeneralSettingsSection.tsx`, `ReasoningSettingsSection.tsx`, `AdaptiveIntakeSurface.tsx`.

| Component | Information and exact source | Fetch/update path | Intelligence | Gap |
|---|---|---|---|---|
| `OnboardingProgress` (refactor current step UI) | Local named steps + persisted onboarding status | Zustand/local; persisted settings mutation on skip/complete | Deterministic | Need distinct `skipped` vs `completed` and completion Outcome ID if absent. |
| Agent/harness inventory rows | Installed/authorized agent probes and reasoning readiness currently consumed by onboarding/settings | Existing inventory/settings fetch; refresh on explicit remediation and provider events | Deterministic | Current probe is not verified harness pairing. Add connection projection. |
| Pairing approval module | Pairing intent identity, version, digest, requested capability classes, scope, expiry, proof state | **New** pairing GET/mutations + CDC transitions | Deterministic authority rendering | **Missing public renderer path.** |
| Project form | Name, repo path, branch/provider defaults; existing project create DTO | Existing Create Project mutation -> workspace invalidation | Deterministic | No major gap; ensure repository validation consequence is typed. |
| First Outcome composer | User statement, project ID, intake session | Existing intake create/capture APIs in `AdaptiveIntakeSurface` | Input is owner-authored | No gap. |
| Analysis progress | Durable intake status + analysis request from `useIntakeAnalysisRequest` | Fetch + current request/status updates; CDC preferred | IR runs at capture/answer transition and persists proposal | Add CDC if still poll/manual refresh. |
| Clarification card | Durable intake clarification: question/reason/recommendation; alternatives if schema adds them | Intake snapshot fetch; answer mutation with expected revision | IR produced/persisted at analysis transition | Add typed alternatives if absent. |
| Contract/authority review | Durable intake proposal + `authorityCeiling` | Intake fetch; local edit draft; confirm mutation creates Outcome/Contract | Proposal may be IR-generated; rendering deterministic | No major model gap. |
| Completion check | Confirmed Outcome/Contract and route to Plan | Confirm response + Outcome query | Deterministic | Persist onboarding completion only here; current tour ends too early. |

**Harness/provider activity effect:** readiness/pairing events update prerequisite rows; intake IR status updates progress/proposal. Raw harness output is not shown in onboarding.

## Screen 2 - New Outcome / Understand

**Actual files:** `AdaptiveIntakeSurface.tsx`, `IntakeAnalysisWaiting.tsx`, `IntakeContractReview.tsx`, `OutcomeIntakeAgentRoles.tsx`, `GlobalNewTaskDialog.tsx`, `CommandPalette.tsx`, `Sidebar.tsx`.

| Component | Information/source | Fetch vs stream and update | Intelligence | Gap |
|---|---|---|---|---|
| Project-scoped `New Outcome` button | Active project from route + `useWorkspaceQuery` | Fetch workspace; navigation only | Deterministic | Add visible header action and explicit project-context API between palette/dialog/surface. |
| Centered `OutcomeComposer` | Owner statement, project name, optional defaults | Local draft; capture mutation; project fetch | None | Presentation only. |
| Project switcher | Workspace project DTOs | Existing `useWorkspaceQuery`; navigation | None | None. |
| Run preferences disclosure | Project agent/default config | Existing project/settings DTO | None | Ensure only supported defaults render. |
| Analysis state | Intake session + analysis request/provenance | Existing intake/request query; CDC/poll while IR active | IR at capture and each material answer; persisted | Prefer typed IR status/event rather than generic pending. |
| Typed clarification | Session `needs_user` clarification | Fetch full card; answer mutation; CDC to analyzing/ready | IR produces question at transition | Alternatives/consequence schema if missing. |
| Proposal summary | title, desired state, criteria, review, bounds, facets, authority | Durable intake proposal DTO | Atomic fetch after IR; local draft edits until confirm | IR-generated proposal persisted | None. |
| Full editor | Same proposal fields | Local draft only; confirm sends validated proposal | None while editing | None. |
| Authority card | Proposal `authorityCeiling`, effective project/policy bounds | Intake DTO + policy projection | Deterministic | Add effective denied-by-policy explanations if response lacks them. |
| Confirm/cancel | Expected revision/request key and validation | Existing mutations; response is authoritative | None | Auto-route on confirm; no data gap. |

**Session-status updates:** capture -> analyzing -> needs_user/ready/failed via durable intake status. UI never streams analyzer tokens.

## Screen 3 - Portfolio Board/List

**Actual files:** `OutcomesOverviewSurface.tsx`, `OutcomeMissionWorkspace.tsx`, `useMissionAttention.ts`, `useOutcome.ts`, `useOutcomeRunState.ts`, `outcome-dashboard-presentation.ts`.

| Component | Information/source | Fetch/update | Intelligence | Gap |
|---|---|---|---|---|
| Project header/new action | active project/workspace | Workspace fetch | None | Presentation only. |
| Board/List mode | user local preference | Zustand | None | None. |
| Search/filter controls | fetched Outcome titles/goals, projects, lanes, accepted/contributor flags | Client filtering over current page; refetch on project change | None | For >24 rows, backend search/cursors and total counts required to claim complete results. |
| Lane headers/counts | typed run-state lane per Outcome | Existing project Outcomes + `useProjectRunStates`; CDC invalidation | Deterministic server classification | Need authoritative totals if page is partial. |
| Outcome identity/title | `OutcomeResponse` | `useProjectOutcomes`; Outcome mutations/CDC invalidate | None | Cross-project portfolio may need consolidated endpoint. |
| Attention chip/reason | `ControllersOutcomeRunStateResponse.state`, blocker, `attentionReason` | `useProjectRunStates`; CDC | Deterministic | Replace arbitrary reason text with typed code + params. |
| Card summary | Contract desired state fallback; proposed `portfolioSummary` | Existing Outcome fetch for fallback; summary updates on Contract/Plan/result events | Optional IR at those transitions, persisted | **Generated summary field/provenance missing.** |
| Next action | lifecycle state, question/proof/run state | Should arrive in portfolio projection; CDC | Deterministic server decision table | **Typed `{code,target}` missing/inconsistent.** |
| Freshness | domain `updatedAt`, transition/event sequence, connection state | DTO timestamp + events hook; local relative-time display | None | Add `lastTransitionAt/eventSequence` where absent. |
| Proof teaser | proof readiness/counts | Current proof DTO is per Outcome; summary should be in portfolio projection | Deterministic | Efficient portfolio proof summary likely missing. |
| Card/list row | composition of above | Fetch + CDC; never local authority inference | Summary may be IR; rest deterministic | Consolidated projection recommended. |
| Split workspace | selected Outcome route, width/expanded/focus | Route + Zustand/local | None | None. |

**Harness/provider activity effect:** Activity changes Attempt/session rows -> server recomputes run attention/freshness -> CDC -> only affected card updates. Do not stream provider lines into portfolio cards.

## Screen 4 - Focused Outcome detail + typed Needs-You

**Actual files:** `OutcomeMissionPanel.tsx`, `OutcomeRunSurface.tsx`, `OutcomeAttemptTerminalPanel.tsx`, `OutcomeRunControls.tsx`.

| Component | Information/source | Fetch/update | Intelligence | Gap |
|---|---|---|---|---|
| Detail header | project + Outcome title/current revision | `useWorkspaceQuery`, `useOutcome` | None | None. |
| Freshness/disconnected banner | events connection, last successful projection timestamp | `useEventsConnection`, query metadata/domain timestamp | None | Standard freshness field needed across responses. |
| State strip | Outcome run state, typed attention, next action | `useOutcomeRunState`; CDC/refetch | Deterministic | Typed next-action object/reason code needed. |
| Four-tab nav | local tab/route state | Zustand/route | None | Presentation only. |
| `NeedsYouCard` | current durable question/choice, source WorkUnit/Attempt/session, reason/options/recommendation/generation | **New** `useOutcomeQuestion`/choice hook; CDC created/superseded/answered | IR may normalize raw provider request at `needs_input`; persisted | **Launch-critical missing projection and mutation.** |
| Answer status rail | durable owner command queued/sent/acknowledged/refused/delivery_unknown | New answer command GET + CDC; no optimistic ack | None | **Missing public path/reconciliation.** |
| Run controls | run state/freshness/admission | Existing `useOutcomeRunState` + command mutation | None | None, but hide unsupported actions. |
| WorkUnit mission canvas | server mission projection | New consolidated projection or existing Plan/Schedule/Attempts migration join | Node summary IR optional; node state deterministic | **Session/Attempt status-to-WorkUnit mapping missing.** |
| Shared inspector summary | Plan objective, state, attention, summary, artifacts/checks, criteria | Projection + proof/artifact DTOs | Transition-triggered WorkUnit summary IR, persisted | Summary DTO/source sequence missing. |
| Live session | terminal/session stream and current responsibility | Existing terminal bridge/Attempt session refs | Raw stream only, clearly non-authoritative | Ensure WorkUnit projection exposes session ID/status/responsibility. |
| Takeover | provider/session control state and receipt | Existing terminal/takeover path if supported; event-confirmed | None | Add explicit responsibility/return-control projection if current path does not expose it. |

**Provider activity effect:** raw activity can stream to terminal; typed session phase/last activity and attention update mission projection via CDC. Only state transitions trigger durable summaries.

## Screen 5 - Plan authorization + Planning conversation

**Actual files:** `OutcomeDecideAuthorizeSurface.tsx`, `MissionPlanView.tsx`, `MissionPlanningConversation.tsx`, `PlanningAgentPicker.tsx`, `PlanningContextGrant.tsx`, `usePlanning.ts`, `useOutcome.ts`.

| Component | Information/source | Fetch/update | Intelligence | Gap |
|---|---|---|---|---|
| Plan header/status | `PlanRevisionResponse.summary/status/number/contractRevision` | `useOutcomePlan`; planning finalize/approve invalidates | Plan proposal from IR/planning service, persisted | None. |
| WorkUnit list/graph | Plan WorkUnits/dependsOn/routing/bindings; Schedule after approval | Plan + `useOutcomeSchedule` | None in render | None. |
| Capability approval card | Plan grants, Contract authority ceiling, admission result | Plan/Outcome DTO; approval mutation; policy change invalidation | Deterministic | Plain-language capability descriptors may need typed presentation metadata. |
| Assumptions/blockers | Plan fields | Plan fetch after planning IR | IR-produced/persisted with Plan | None. |
| Authorize/Open Run | exact Plan/current Contract/admission | existing approve mutation + route | None | None. |
| Planner picker | `PlanningCandidateResponse` | `usePlanningCandidates`; refresh on provider inventory/readiness | Deterministic admission/candidate ranking unless backend uses IR | Ensure ranking reason is typed/provenance. |
| Context grant | planning scope, repository/supplied packet, digest | durable `PlanningSessionResponse` | None | None. |
| Planning timeline | durable `PlanningTurnResponse[]` | `usePlanningSession`; currently 3s poll while provider owns turn; CDC preferred | Provider/IR generates turns, persisted | Add complete CDC event coverage to remove fast polling. |
| Clarification card | turn clarification reason/recommendation/options | Planning DTO | IR/provider-generated at turn transition, persisted | None if alternatives complete. |
| Contract-change proposal | typed proposal turn | Planning DTO | IR-generated/persisted | None. |
| Composer | owner message + expected session revision/request key | local draft; existing send mutation | None | None. |
| Planning status | waitingOn/status/binding/effective model | Planning session | CDC/poll | None | Event coverage/freshness. |

**Harness/provider activity effect:** candidate readiness changes picker; provider reply creates a durable turn; planner waiting state updates atomically. Never expose raw chain-of-thought.

## Screen 6 - Per-Outcome Mission Control Run canvas

**Actual files:** `MissionWorkUnitGraph.tsx`, `MissionPlanView.tsx`, `OutcomeRunSurface.tsx`, `OutcomeRunBoardAdapters.tsx`, `OutcomeAttemptTerminalPanel.tsx`, `dependency-layers.ts`. The inner Kanban is replaced; main portfolio Board/List remains.

| Component | Information/source | Fetch/update | Intelligence | Gap |
|---|---|---|---|---|
| `MissionAttentionStrip` | counts of `needs_choice`/`needs_input` nodes | WorkUnit mission projection | CDC | None in render | Depends on new projection/question taxonomy. |
| Canvas controls | viewport/fit/zoom, Canvas/List | Zustand/local; graph topology determines bounds | None | Library install decision for `@xyflow/react`. |
| WorkUnit node | title, schedule state, attention kind, next action, current Attempt/session, criterion count | Plan + **new `WorkUnitMissionNode` projection** | Initial fetch + CDC patch/invalidate by WorkUnit generation | Optional persisted summary not on node face | **Server session-to-WorkUnit mapping required.** |
| Dependency edge | Plan `dependsOn` only | Plan fetch; topology updates only on authorized revision | None | None. |
| Ordered List | exact same node adapter/data | Same React Query data; local view toggle | None | None. |
| Loading/unknown | query states, Plan availability, connection/freshness | React Query/events | None | Standard generation/freshness fields. |
| Inspector header/objective | WorkUnit/Attempt projection + frozen Plan | Fetch/CDC | None | None after projection. |
| Latest run summary | persisted `WorkUnitRunSummary` through event sequence/source digest | Summary included in projection or separate hook; CDC on ready | IR only at phase boundary, attention, complete/fail, material artifact/check batch | **Summary row/IR trigger path missing.** |
| Activity chips | typed Attempt observations/tool/change/check events | Attempts/observations/proof DTOs | Raw activity stream/fetch; bounded virtualization | None | Efficient per-WorkUnit activity endpoint may be needed. |
| Contract coverage | WorkUnit criterion IDs + Contract-bound proof criteria | Plan + `useOutcomeProof` | CDC on proof/evidence | None | None. |
| Session layer/takeover | session ID/status/responsibility and terminal | projection + terminal bridge | raw stream + event-confirmed responsibility | None | Responsibility/return-control fields if absent. |
| Plan revision banner | active authorized plan vs proposed/current | Outcome + Plan + run freshness | CDC/fetch; atomic topology swap only after authority changes | Deterministic diff | Need stable Plan generation/revision in projection. |

**Provider activity effect:** terminal streams raw bytes; provider/session activity creates durable Attempt events; server projection changes phase/status/last activity; CDC updates node without refit or selection loss; meaningful transition queues summary IR.

## Screen 7 - Harnesses & Authority / Settings

**Actual files:** no current launch-grade pairing page. Relevant existing files: `SettingsDialog.tsx`, `GlobalSettingsForm.tsx`, `ReasoningSettingsSection.tsx`, `GeneralSettingsSection.tsx`, `MobileDevicesSection.tsx`. `ConnectMobileModal` is separate phone pairing.

| Component | Information/source | Fetch/update | Intelligence | Gap |
|---|---|---|---|---|
| Harness task rows | adapter/provider identity, version, verified connection state, last seen, mission/project scope | **New** connections DTO | Fetch + CDC heartbeat/proof/revoke | None | **Missing public renderer path.** |
| Pair button/request | create/list pairing intent | New pairing mutation/DTO | Mutation + CDC | None | Missing. |
| Approval Card | executable/version/digest/capability classes/scope/expiry/challenge | Pairing intent durable row | Fetch + CDC; expected digest/expiry on action | None; authority is deterministic | Missing. |
| Capability chips/matrix | granted/requested capability classes + plain effect descriptors | Pairing/connection DTO | Atomic with intent/connection | None | Add typed display metadata if backend only exposes codes. |
| Pairing progress | requested -> challenge -> proof -> connected/refused/expired | durable pairing transitions | CDC, direct fetch on reconnect | None | Missing public events. |
| Approve/Deny/Revoke | intent/connection ID, expected digest/generation, owner proof; revoke receipt/custody consequences | New mutations + CDC | None | Missing. |
| Connection inspector | last proof, protocol, missions, current commands/custody | connection/command projection | Fetch + CDC | None | Missing consolidated safe projection. |
| Intelligence settings | provider/model/readiness/credential status | Existing settings hooks | Fetch/mutation/provider readiness update | None | Keep separate from connection authority. |
| Mobile devices | real mobile bridge/device state | Existing `MobileDevicesSection` | existing fetch/events | None | Product scope decision, not data gap. |

**Harness/provider activity effect:** verified proof/heartbeat updates last seen and state; command/custody changes inspector; it must not cause an LLM summary or silently broaden scope.

## Screen 8 - Result, evidence, delivery, rework

**Actual files:** `OutcomeProveCloseSurface.tsx`, `OutcomeDeliveryPanel.tsx`, `OutcomeDocumentsPanel.tsx`, `useOutcome.ts`, `useOutcomeArtifacts.ts`.

| Component | Information/source | Fetch/update | Intelligence | Gap |
|---|---|---|---|---|
| Result summary | `OutcomeProofResponse.result/status`, criterion/check/evidence counts, limitations | `useOutcomeProof`; CDC after attempts/evidence/verification | Mostly deterministic; optional persisted narrative IR | Add source-bound summary only if needed. |
| Criterion card | Contract criterion text, readiness, evidence, verifications, independence/result | proof DTO | Fetch + CDC | None | None. |
| Evidence summary | raw evidence/check/verification set + source refs | proof DTO; persisted summary extension | Deterministic formatter for structured checks; IR only for long unstructured evidence at set transition | **Generated summary/provenance fields missing.** |
| Raw evidence disclosure | source refs, producer, timestamp, contradicting/supporting | proof ledger | Fetch/CDC | None | None. |
| Manual evidence/verification forms | owner inputs and exact target criterion | local draft + existing mutations | None | None; hide until invoked. |
| Limitations/gaps | proof/result projection | fetch/CDC | Could be IR-produced during verification, persisted | Ensure not omitted if summary IR fails. |
| Accept/Rework | proof readiness, owner summary, disposition, target | existing acceptance mutation | None | Labeled re-entry candidate DTO would avoid raw IDs. |
| Re-entry picker | Attempts/WorkUnits/Plans/Contracts and recommended target | current proof/lineage + proposed typed candidates | Fetch on Rework; recommendation may be deterministic or persisted IR | **Renderer-friendly labeled candidates/recommendation likely missing.** |
| Delivery artifact picker | eligible retained artifacts + accepted/draft eligibility | existing deliveries/proof, but current form asks raw Attempt ID/version | Fetch + delivery mutation + CDC | None | **Eligible-artifacts endpoint/projection missing.** |
| Delivery status/history | durable delivery rows | `useOutcomeDeliveries`; CDC/poll | None | Ensure event coverage. |
| Supplied documents | selected snapshots, revisions/change detection/approval | `useOutcomeDocumentContext` | fetch + explicit select/approve mutations | None | None. |

**Harness/provider activity effect:** checks/artifacts/receipts update proof; completion queues evidence-summary IR when needed; acceptance remains owner-only and never follows provider "done."

---


## Cross-surface update contract

Provider information has three classes: raw activity streams only to terminal/activity; validated typed facts persist and update UI through events/refetch; intelligence runs only at named transitions over bounded durable inputs and persists provenance. Live events identify aggregate, Outcome, Plan generation, monotonic revision/sequence, time and reason. Reconnect refetches source truth before saying “live.”

The launch-blocking backend gaps are: (1) typed generation-safe Needs-You question/answer and delivery states, (2) public renderer-safe harness connection/authority APIs, and (3) a server-owned WorkUnit MissionProjection. Transition summaries, typed next actions/freshness and Result helpers follow as production-clarity work.

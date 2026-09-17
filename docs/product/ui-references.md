# UI reference adoption table

These references supply presentation patterns, not Kennel authority or architecture.

## Beautiful UI

| Reference | Adopt | Kennel mapping | Do not copy |
|---|---|---|---|
| Approval Card | Compact request, reason, bounded choices, primary and deny actions | Typed Needs-You, pairing review, capability approval | Prose-inferred permissions; every choice/grant is durable, typed and generation-bound |
| Task Rows | Identity, state, freshness and one next action | Portfolio List, Mission Control List, Attempt lineage | Local “running/done”; collapsing Needs Choice and Needs Input |
| Tool Chips | Compact secondary evidence | Inspector Activity and Result disclosure | Raw tool-call flood or tool success as Outcome acceptance |
| Selection Actions | Action for the selected object | Inspector Answer/Review/Open Session footer | Hover-only or unsupported bulk actions |
| Prompt Bar | Bounded composer with scope disclosure | New Outcome, planning and “Something else” answer | Model/source/slash-command controls in every composer |
| Tabbed reasoning/chat | Separate durable result from optional activity | Planning details and terminal/activity | Private chain-of-thought or streamed prose as state |
| Harness demo | Dense identity/status/action grammar | Harness rows and connection inspector | Generic send box as control plane; unverified “connected” |

Promote shared visual primitives such as ApprovalCard, MissionStatusChip, TaskRow, ActivityChip, BoundedComposer, SelectionActionBar and SessionResponsibilityChip. Domain adapters remain under Outcome components.

## Transitions.dev

| Pattern | Adopted use | Timing | Rejected use |
|---|---|---:|---|
| Text/icon state swap | queued -> acknowledged; Ready -> Running -> Needs You | 140-160 ms | Looping thinking after durable state exists |
| Panel reveal | Inspector and nested terminal | 180 ms, 8 px max | Spring overshoot or focus-stealing auto-open |
| Accordion | Provenance, raw evidence, criteria | 160 ms | Changing live graph-node height |
| Notification badge | Needs-You count | 140 ms | Repeated pulse |
| Card resize | Freeform answer reveal | 160 ms | Summary-driven graph resize |
| Success check | Pairing verified or answer acknowledged | 150 ms once | Confetti/success theatre |
| Toast | Background completion with no visible destination | 160 ms | Replacing inline current-surface status |

Do not copy shimmer text, animated gradients, reasoning streams, skeletons for known topology, 3D tilt, smoky delete, matrix loaders or continuous thinking animation.

## Astryx release checklist

Use the checklist, not a third style system:

- semantic element first;
- focus-visible and complete keyboard path;
- disabled, pending, error, empty, stale, disconnected and reduced-motion states;
- text/icon redundancy for color, sufficient contrast and target size in both themes;
- documented controlled/uncontrolled behavior;
- consequence-labeled confirmation for destructive actions;
- no hover-only capability;
- known content remains visible during refresh.

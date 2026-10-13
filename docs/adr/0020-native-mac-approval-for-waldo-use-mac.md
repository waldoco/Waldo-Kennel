# ADR 0020: native Mac approval for Waldo machine requests

## October 13, 2026: build and merge directed by the owner

Status: Ashish directed this build to proceed to merge on 2026-10-13, explicitly overriding the Shivansh lane gate ("v0.3.0 / use_mac: not accepted, no code. Needs a Waldo-side proposal reviewed before code"). Bilateral acceptance of contract v0.3.0 is still not recorded; this note records the owner's override, not that acceptance. Number renumbered from 0019, which `outcome-loop` already uses for the governed capture source plane.

## October 9, 2026: use_mac / v0.3.0 build disposition

Status: owner-directed implementation; contract and Waldo-side behavior are pending Shivansh lane acceptance. This amendment supersedes the historical raw-shell prohibition and delegate_to_machine name below for this build.

Waldo exposes one model-visible machine tool, use_mac. The bridge transports shell_exec, session_query and session_control requests. Pairing authenticates transport only. Every shell command, session start, session message and session stop requires a new native Mac approval for the exact request. Session reads and interruption require no prompt. Chat replies and bridge frames never authorize an effect. No automatic push, merge or deploy is granted by use_mac.

Settled owner decisions:
- D1: native Mac dialog only. Remote owner-proof transport remains a future ADR.
- D2: full shell strings, owner's login shell, registered Kennel project cwd only; reject other cwd before showing a dialog.
- D3: only status and a UTF-8 output tail of at most 6144 bytes cross. Full output stays on the Mac; no transcript push.
- D4: message_session requires approval every time and refuses blocked sessions.
- D5: with the app closed, wait then expire; a standalone daemon cannot approve.
- Timing clarified by Ashish in-session: 15 minutes to approve/start. Unapproved commands expire at that deadline. Execution that starts before it receives a separate timeout, two minutes by default and ten minutes maximum. A completed result remains admissible afterward; started work is never mislabeled expired or blindly repeated after uncertainty.

Deny is the native dialog's default and cancel action. Approval grants one exact command once, never a standing grant. The main process keeps the KennelOwner bearer out of the renderer. Shell stdin is closed, environment is scrubbed, process groups are killed on timeout, local capture is capped at 1 MiB, and a durable started marker precedes the effect. An interrupted crash without a known result becomes outcome_unknown.

use_mac belongs to PRIVILEGED_ACTION_TOOLS. A turn that read external content cannot request it. Device result follow-ups start externally tainted, use a narrow read-only tool set, and produce exactly one durable final reply per request_id.

The v0.2.3 contract stays byte-identical; v0.3.0 is a new bilateral document. Existing pairings must re-pair. Frozen machine_state_query and notify_local behavior remains unchanged except required version negotiation.


## Relationship to ADR 0018

ADR 0018's K0 taxonomy stays frozen in the v0.2.3 document. The v0.3.0 extension admits local effects only after exact authenticated owner approval on the Mac. The device principal still grants no owner authority. The falsifier of a device causing a local effect beyond notification without exact owner proof is satisfied by the KennelOwner-token native decision and durable, request-bound one-shot authorization.

The previous no-raw-shell beta rule is reversed for this owner-directed build. The bridge still carries no approval path; a WhatsApp/Telegram reply is not owner proof. v0.2.3 sections 7 and 8 remain historical bytes; the new version permits a bounded output tail under pairing consent, with full transcripts kept local and re-pairing required to widen capabilities.

## Implementation order and checks

Author mirrored v0.3.0 contract and golden vectors before executable behavior. Then add asynchronous transport/recovery, per-device revocation fencing, consent/capability negotiation, durable native approval, a separate shell runner, existing-session adapters and signed state events. Extend Waldo issuance, taint/classification, durable owner result return and migration checks. Verify Go/TypeScript vectors against the same fixture, all touched suites, full repository gates and every handoff local acceptance case before requesting a push.

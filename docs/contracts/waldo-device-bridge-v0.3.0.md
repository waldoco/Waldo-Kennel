# Waldo device bridge contract v0.3.0

Status: owner-directed build; pending Shivansh lane bilateral acceptance.
Document SHA-256 is recorded in waldo-device-bridge-v0.3.0.sha256.
This is a new document. v0.2.3 remains byte-identical and historical.

## Authority and pairing

The paired device authenticates transport, never owner approval. Backend-origin frames use authenticated TLS only (Option A); every device-origin frame is ed25519 signed. Chat replies never approve effects. Shell execution and session start/message/stop require a native Mac dialog displaying the exact request, registered project folder, session, reason and red risk flags. Deny is the default and cancel action. Approval is once, for exactly one request. Session query and interruption require no prompt. No use_mac path automatically pushes, merges or deploys.

Capabilities are a nonempty, duplicate-free ordered subset of:
machine_state_query, notify_local, shell_exec, session_query, session_control.
They are declared at pairing and may be narrowed at connect, never widened without re-pairing. Existing v0.2.3 devices must re-pair; old persisted rows are retained but cannot authenticate as v0.3.0.

Redeem POST /devices/redeem has exactly six fields: code, device_pubkey, label, declared_capabilities, contract_version, session_consent. The first five retain v0.2.3 limits. contract_version is 0.3.0. session_consent is a boolean recording explicit owner consent for session status and bounded tails to cross; session_query or session_control requires true. Pairing returns device_id, owner_id, accepted_contract_version. Codes remain 32 random bytes, 600-second expiry, single-use digest custody, and the existing rate limits. The connect target is exactly /devices/connect?contract_version=0.3.0&declared_capabilities= followed by the ordered comma-separated subset.

## Envelope, canonicalization and signing

All v0.2.3 strict UTF-8, duplicate-key rejection, integer encoding, Unicode-code-point key sorting, compact canonical JSON, base64url and LF-joined signing conventions remain. Identifier bounds remain 128 ASCII letters/digits/underscore/hyphen; message_id remains a ULID. Frame limit is 8192 UTF-8 bytes for the COMPLETE signed frame. Replay window is 300 seconds, heartbeat 30 seconds. One command is in flight per device. Existing nonce, fingerprint and result/receipt custody protections remain.

HTTP signature base: decimal timestamp LF nonce LF method LF exact request target LF lowercase SHA-256 of raw body.
Device frame signature base: decimal timestamp LF type LF message_id LF nonce LF lowercase SHA-256 of canonical frame with signature omitted.
Device envelopes add timestamp, nonce, signature; backend envelopes never do.

Command envelope remains exactly contract_version,type,message_id,device_id,owner_id,command_id,revision,idempotency_key,class,expires_at,payload. revision is 1. Ack, result and command receipt retain their v0.2.3 reference fields. Heartbeat is unchanged except version and negotiated capability vocabulary.

## Deadlines and execution custody

For new machine requests, expires_at is the approval/start deadline, 900 seconds by default and maximum. Unapproved or not-started requests cannot start at or after that deadline and return expired. Ashish clarified this rule in-session on October 9: once approved and started before the deadline, execution receives a SEPARATE timeout, 120 seconds by default and at most 600 seconds. Completed results may arrive after expires_at. Never replace a started outcome with expired.

Ack accepted is sent immediately after durable command admission, off the execution path. The accepted request may wait for native approval for the full TTL while heartbeats, receipts and reads continue. With no desktop dialog consumer, it waits then expires. A standalone daemon has no owner authority and fails closed. Before any effect, persist a started marker. Recovery of started-without-result work returns outcome_unknown and NEVER repeats the effect. Recovery of pending unapproved work after expires_at returns expired.

Shell stdin is closed; the allowlisted environment contains no inherited credentials. cwd must resolve to a registered Kennel project before any dialog. Timeout kills the process group. Local capture is capped at 1048576 bytes. At most a valid UTF-8 tail of 6144 bytes crosses; reduce it further if JSON escaping and envelope overhead would exceed 8192 signed bytes. truncated indicates any loss. Full retained local capture is visible only in Kennel.

Frozen K0 command classes retain their existing payload, result and timeout semantics except version negotiation.

## Closed new payloads

Text fields reject invalid UTF-8. project is a registered project ID/name (1..128 UTF-8 bytes), reason is 1..512 bytes, command/prompt is 1..4096 bytes. A complete command frame exceeding 8192 bytes is rejected before approval.

shell_exec: exactly project, command, reason, timeout_seconds. timeout_seconds is an integer 1..600, normalized to 120 by the tool when absent.

session_query:
- list_sessions: exactly action, project.
- read_session: exactly action, project, session_id.

session_control:
- start_session: exactly action, project, reason, agent, prompt. agent is claude-code or codex.
- message_session: exactly action, project, reason, session_id, prompt.
- interrupt_session or stop_session: exactly action, project, reason, session_id.

Every session target must belong to the registered project. Existing session/chat services are the only execution engines. Never type into a blocked session or answer a provider permission prompt. start/message/stop use the same exact-request native approval rail; interrupt stops the active turn without approval.

## Results and state events

result payload keeps status (the existing wire discriminator):
- completed: exactly status,exit_code,output_tail,truncated, optionally session_id for session control. exit_code is a signed 32-bit integer; output_tail is valid UTF-8, 0..6144 bytes; truncated is boolean.
- denied, expired, outcome_unknown: exactly status.
- answered for session_query: exactly status,answer. List answer is sessions (at most 20 records of exactly session_id,project,agent,status) and truncated. Read answer is exactly session_id,status,output_tail,truncated.
- answered, delivered and failed for K0 retain v0.2.3 shapes. Existing processing_failed can report a refusal before a new command effect; it never proves an ambiguous effect did not occur.

session_event is a device-origin signed state change, with the same reference-free envelope fields as heartbeat: contract_version,type,message_id,device_id,owner_id,timestamp,nonce,signature,payload.
Payload is exactly session_id,project,event. event is needs_input,blocked_on_tool,turn_finished or exited. No transcript or output is attached. Emit state changes within one heartbeat, persist them in the durable outbox and re-sign retries with fresh timestamp/nonce.

A session-event receipt is backend-origin, exactly contract_version,type,message_id,device_id,owner_id,payload; payload is exactly event_message_id,received_at. Its receipt custody and conflicting-fingerprint rules match results. It never references or authorizes a command.

## Waldo issuance and return

One model-visible use_mac maps run_command to shell_exec, list/read to session_query, and start/message/interrupt/stop to session_control. It validates action-specific inputs and frame size and returns request_id and status sent to your Mac for approval immediately (a request receipt, never completion). Internal dispatch maintains owner isolation, request idempotency, capability checks, per-device limits and one in-flight command.

use_mac is privileged: an externally tainted turn cannot call it. Mac approval does not override this rejection. Results are external content. Bridge durable custody keeps the bounded result and originating request_id until the correct owner DO durably admits it. Owner lookup uses signed directory-derived UUID/do_name, never a caller-supplied owner name. Owner result admission deduplicates request_id with a fingerprint and recovers across crashes. A narrow, externally tainted system follow-up enters the existing final outbox with one stable delivery key. Duplicate result means one reply. Ordinary session_event produces a short fixed notice with no model turn; an explicitly watched session may use the same narrow follow-up policy.

## Appendix A: normative golden vectors

waldo-device-bridge-v0.3.0.vectors.json is mirrored byte-identically with this document. It records canonical frames, raw-body digests, exact LF signing bases and ed25519 signatures for every new/changed shape. The fixed public test seed is 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f; it is not a live credential and must never be accepted as a real pairing code. Both Go and TypeScript verify the same vectors and enforce the complete frame ceiling. Signature-free command/receipt vectors attest canonical bytes and shape under TLS rather than inventing backend signing.

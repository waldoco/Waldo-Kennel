# Post-pair adapter command ingress and native confirmation design

- Status: design packet only, no implementation
- Dependency: reviewed S3.1 connection kernel, frozen S3.3 pairing transport, and reviewed S3.4 owner-proof kernel/mint path
- Rule: the pairing server remains data-only and retains only `request_challenge` and `prove`

## Separate authenticated channel

Add a new `harnesscommand` local service on its own protected local endpoint. Do not add an RPC to `adapters/harnesspairing.Server` and do not expose `Coordinator.Issue`.

A command frame carries:

- S3.1 connection bearer plus its exact connection binding: connection ID, installation, adapter digest, harness identity/version, protocol fingerprint, mission, app run, connection generation, and one transport class;
- owner proof ID and bearer;
- exact command bytes, target ID, target/question generation, and owner command class;
- an adapter request key used only for idempotent lookup, never as authority.

The ingress applies these steps in order:

1. verify local peer identity before reading or responding;
2. parse one bounded frame with a closed schema;
3. call `harnessconnection.Kernel.Authenticate` with the full S3.1 tuple and required transport class;
4. canonicalize the command using the command type's versioned canonical encoder and compute SHA-256 inside the daemon;
5. map transport class to owner command class using the closed mapping and require an exact match;
6. validate current mission, app run, target identity, target/question generation, and command-specific revision before authority consumption;
7. atomically consume the owner proof and create the durable ingress/claim record;
8. dispatch only from that durable claim through existing generation, revision, quiescence, delivery-unknown, and immutable-claim fences.

Every rejection returns one generic response shape. Field-specific diagnostics remain daemon-local and must not reveal either bearer or verifier.

## Dual authority references

Use an append-only `command_authority_claims` record rather than overloading owner principal, capability fingerprint, or provider correlation fields. Its immutable fields are:

- claim ID and adapter request key/fingerprint;
- owner proof ID;
- harness connection ID and authenticated generation;
- connection binding fingerprint and authenticated transport class;
- app run and mission;
- command content digest, target ID, target generation, owner command class;
- confirmation record ID for material classes, empty for routine classes;
- command-specific destination record ID and state;
- created and updated timestamps.

No bearer or verifier enters this record. Governed command/control, replacement, approval, and Acceptance rows reference the authority claim ID. A uniqueness constraint on owner proof ID enforces one destination claim. A uniqueness constraint on `(connection_id, generation, adapter_request_key)` converges exact retries while a fingerprint mismatch conflicts.

## No crash window

Do not call the current `VerifyAndConsume` and then separately insert a governed claim. Add one store transaction whose semantic operation is `ConsumeOwnerProofAndCreateAuthorityClaim`:

- re-check proof pending state, expiry, and the entire expected tuple inside the transaction;
- change `consumed_at` only while inserting the immutable authority claim and command-specific durable claim/outbox row;
- commit all three or none.

Dispatch begins only from the committed outbox/claim. A crash before commit leaves the proof usable. A crash after commit leaves a durable claimed or delivery-unknown item that recovery reconciles; it never silently reissues a proof or redelivers without the existing effect fence. If a destination cannot share the same SQLite transaction, the authority claim remains `action_needed` and blocks effect until a reviewed reconciler completes the durable destination binding.

## Routine target mapping

Each command type gets its own canonical encoder and current-state check:

- `turn`: session ID plus controller generation and expected revision;
- `steer`: session ID, active provider turn ID, controller generation, expected revision;
- `answer`: exact open question ID and question generation;
- `interrupt`: session ID, active provider turn ID, controller generation;
- `cancel`: a distinct type per target. Chat turn interrupt and Outcome Attempt cancellation must never share a generic target mapping.

The implementation review must freeze these encoders and mappings before adding handlers.

## Native material confirmation

Add a daemon-owned `native_confirmations` store populated only by an authenticated desktop main-process confirmation flow. Renderer and adapter data may propose a transition but cannot mark confirmation complete.

A confirmation record binds:

- confirmation ID, authenticated app run, mission, material class;
- daemon-computed canonical content digest;
- exact target and generation/revision;
- capability delta or effect summary where applicable;
- native presentation/decision evidence reference;
- created, expiry, and consumed timestamps.

The material proof mint route accepts only a confirmation ID plus the command tuple. The daemon loads the record, validates current target/revision and exact tuple, and atomically consumes the confirmation while creating the owner proof. It never accepts a request-supplied free-form confirmation reference. `replace`, Plan/Contract approval or widening, and Result Accept remain unavailable until their target mappings and native confirmation producers are reviewed.

## Required test matrix

- paired transport plus exact owner proof creates one durable authority claim;
- either authority alone creates no owner-authorized claim;
- every connection and proof tuple mutation fails with the same wire response;
- replay and concurrent duplicates have one proof consumer and one durable result;
- changed semantics under the same adapter request key conflict;
- injected crashes before and after transaction commit recover without silent redelivery;
- restart retains consumed proof and non-secret dual references;
- routine commands never request native confirmation;
- each material class rejects missing, expired, consumed, wrong-tuple, stale-target, or renderer/adapter-created confirmation;
- scans prove no bearer/verifier reaches DB public columns, API/log output, argv/env, renderer, repository, or plugin storage;
- macOS separately proves protected local peer identity and native confirmation provenance.

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
7. enter one SQLite write transaction and re-read the owner proof, current harness connection row, and current target/revision; validate the proof pending/expiry/tuple, the connection unexpired/unrevoked/current generation and complete binding, and the target's current generation/revision inside that transaction;
8. in the same transaction, consume the owner proof and insert the immutable authority claim, immutable canonical command payload, deterministic destination mapping, and destination outbox/claim; commit all state transitions or none;
9. dispatch only from that committed payload and destination through existing generation, revision, quiescence, delivery-unknown, and immutable-claim fences.

Every rejection returns one generic response shape. Field-specific diagnostics remain daemon-local and must not reveal either bearer or verifier.

## Dual authority references

Use an append-only `command_authority_claims` record rather than overloading owner principal, capability fingerprint, or provider correlation fields. Its immutable fields are:

- claim ID and adapter request key/fingerprint;
- owner proof ID;
- harness connection ID and the exact current generation re-read and validated inside the claim transaction;
- immutable snapshot/fingerprint of the complete validated connection row binding, its expiry and revocation state, and authenticated transport class;
- app run and mission;
- command content digest, target ID, target generation, owner command class;
- canonical encoder version plus immutable canonical command bytes (or a complete typed payload with a deterministic canonical encoding), whose in-transaction digest must equal the proof-bound digest;
- deterministic destination type and destination record ID;
- confirmation record ID for material classes, empty for routine classes;
- command-specific destination record ID and state;
- created and updated timestamps.

No bearer or verifier enters this record. Governed command/control, replacement, approval, and Acceptance rows reference the authority claim ID. A uniqueness constraint on owner proof ID enforces one destination claim. A uniqueness constraint on `(connection_id, generation, adapter_request_key)` converges exact retries while a fingerprint mismatch conflicts.

## One transaction closes authority TOCTOU and crash windows

Do not call the current `VerifyAndConsume` and then separately insert a governed claim. Add one store transaction whose semantic operation is `ValidateAuthoritiesAndCreateCommandClaim`.

Inside one SQLite transaction, re-read and validate the owner proof, current harness connection row/generation/revocation/expiry and complete binding, current target/revision, then insert immutable authority claim plus immutable canonical command payload and destination outbox; commit all state transitions or none.

The transaction must:

- re-check proof pending state, expiry, and the entire expected tuple;
- re-read the connection row rather than relying on the earlier transport-authentication result, require that it remains unexpired and unrevoked, and match its current generation and complete binding to the command frame and owner proof;
- re-read the command-specific target and require its current generation/revision and destination mapping to match;
- canonicalize or validate the versioned canonical payload inside the transaction boundary and require its digest to equal the proof-bound content digest;
- change `consumed_at` only while inserting the immutable authority claim, canonical payload, deterministic destination, and command-specific durable claim/outbox row;
- record the exact connection row/generation validated, so later rotation or revocation cannot rewrite the historical authority evidence;
- commit every state transition and insert, or none.

This transaction serializes connection rotation/revocation, proof consumption, and claim creation through the same SQLite writer. Rotation or revocation that commits first makes the claim fail. A claim that commits first retains the exact authority snapshot it validated; later revocation governs future claims and does not erase history.

Dispatch begins only from the committed outbox's canonical payload and deterministic destination. A crash before commit leaves the proof usable. A crash after commit leaves a durable claimed or delivery-unknown item that recovery reconciles from persisted semantics; it never asks the adapter to restate the command, silently reissues a proof, or redelivers without the existing effect fence. If a destination cannot share the same SQLite transaction, the authority claim remains `action_needed` and blocks effect until a reviewed reconciler creates the deterministic destination from that persisted canonical payload.

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
- connection rotation/revocation racing claim creation serializes to either a rejected claim or a claim retaining the exact validated connection snapshot;
- target generation/revision changes racing claim creation serialize to rejection or an exact immutable destination claim;
- injected crashes before and after transaction commit recover from the persisted canonical payload and deterministic destination without adapter restatement or silent redelivery;
- a canonical payload/digest mismatch fails before proof consumption or claim creation;
- restart retains consumed proof, canonical command payload, deterministic destination, and non-secret dual references;
- routine commands never request native confirmation;
- each material class rejects missing, expired, consumed, wrong-tuple, stale-target, or renderer/adapter-created confirmation;
- scans prove no bearer/verifier reaches DB public columns, API/log output, argv/env, renderer, repository, or plugin storage;
- macOS separately proves protected local peer identity and native confirmation provenance.

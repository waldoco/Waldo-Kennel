# Stage 3 post-pair command ingress implementation

- Baseline: `outcome-loop` `6c11583bec922d323ae81037041a17523ca0811f`
- Frozen design: `1635ad9cf30886ea31acd656a5136b206e6e7175`

## Formal S3.4 target amendment

The approved S3.4 proof's `targetId + int64 targetGeneration` could not represent the existing governed string fences without loss. Migration 0148 replaces those columns with `target_digest`, the SHA-256 of a closed, versioned `OwnerProofTarget` canonical JSON shape:

- turn: session ID, controller generation, expected revision;
- steer: session ID, controller generation, expected revision, provider turn ID;
- answer: durable question ID and question generation;
- interrupt: session ID, controller generation, provider turn ID.

Unused fields, material classes and unfrozen cancel targets are rejected. Existing short-lived pending proofs are invalidated at upgrade rather than guessed into the new tuple. The authenticated owner mint route accepts the typed target and stores only its digest.

## Durable transaction-readable targets

Migration 0148 also adds `chat_command_targets`, an independently named current-policy row binding session ID, controller generation, expected plan revision, and the capability fingerprint used by governed dispatch. `ClaimChatControllerGeneration` updates the session generation and this target row atomically under the same SQLite writer; ungovened controllers clear any prior governed target in that transaction. The Chat service validates the execution policy and derives the capability fingerprint before making the generation/policy current. Ingress re-reads this row, not immutable attempt/session lineage. Deterministic claim-first and mutation-first races cover controller generation and expected revision.

Provider approval and structured-input requests now create an independently named `owner_answer_questions` row in the same projection transaction as their timeline activity. Resolution and controller-failure cleanup update the target row through the same store writer. Answer ingress re-reads the pending row and exact generation inside the claim transaction.

## Command authority transaction

`ValidateAuthoritiesAndCreateCommandClaim` holds `Store.writeMu` and one SQLite transaction. It re-reads and validates the current connection row (complete binding, current generation, bearer, class, expiry and revocation), owner proof (bearer, tuple, pending and expiry), and current command target. It then inserts the immutable authority claim, canonical payload, deterministic destination/outbox and consumes the proof. All commit or none.

Exact retries converge by connection generation plus adapter request key and request fingerprint. Changed semantics conflict. Material classes remain unavailable.

Deterministic barriers prove rotation, revocation, controller-generation, expected-revision, provider-turn, and question-target mutation cannot interleave past the in-transaction reads. Mutation-first races reject the claim. Claim-first races commit the exact connection snapshot before the mutation proceeds. Payload mismatch and stale question leave the proof unconsumed. Restart/retry reads the persisted canonical payload and deterministic destination.

## Separate transport

`adapters/harnesscommand.Server` is a new protected local service, not an RPC on the frozen pairing server. It verifies local peer identity, parses one bounded closed frame, authenticates the S3.1 connection, and invokes the transactional store, returning one generic failure shape. It has no pairing coordinator and cannot call `Coordinator.Issue`.

This source slice defines the service but does not claim production listener lifecycle or macOS peer proof. Those require the separately reviewed daemon/platform wiring.

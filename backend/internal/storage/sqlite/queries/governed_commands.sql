-- name: InsertGovernedCommandClaim :execrows
INSERT INTO governed_commands (
    id, session_id, idempotency_key, request_fingerprint, command_class, state,
    controller_generation, expected_revision, capability_fingerprint,
    provider_conversation_id, client_message_id, provider_turn_id,
    provider_event_id, provider_cursor, replay_strategy,
    reconciliation_outcome, quiescence, quiescence_evidence_ref,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: GetGovernedCommand :one
SELECT id, session_id, idempotency_key, request_fingerprint, command_class, state,
    controller_generation, expected_revision, capability_fingerprint,
    provider_conversation_id, client_message_id, provider_turn_id,
    provider_event_id, provider_cursor, replay_strategy,
    reconciliation_outcome, quiescence, quiescence_evidence_ref,
    created_at, updated_at
FROM governed_commands WHERE id = ?;

-- name: GetGovernedCommandByIdempotencyKey :one
SELECT id, session_id, idempotency_key, request_fingerprint, command_class, state,
    controller_generation, expected_revision, capability_fingerprint,
    provider_conversation_id, client_message_id, provider_turn_id,
    provider_event_id, provider_cursor, replay_strategy,
    reconciliation_outcome, quiescence, quiescence_evidence_ref,
    created_at, updated_at
FROM governed_commands WHERE session_id = ? AND idempotency_key = ?;

-- name: AdvanceGovernedCommand :execrows
UPDATE governed_commands SET
    state = sqlc.arg(next_state),
    provider_turn_id = sqlc.arg(provider_turn_id),
    provider_event_id = sqlc.arg(provider_event_id),
    provider_cursor = sqlc.arg(provider_cursor),
    reconciliation_outcome = sqlc.arg(reconciliation_outcome),
    quiescence = sqlc.arg(quiescence),
    quiescence_evidence_ref = sqlc.arg(quiescence_evidence_ref),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
  AND state = sqlc.arg(expected_state)
  AND controller_generation = sqlc.arg(expected_controller_generation)
  AND expected_revision = sqlc.arg(expected_revision)
  AND capability_fingerprint = sqlc.arg(expected_capability_fingerprint)
  AND sqlc.arg(updated_at) > created_at;

-- name: ListUnsettledGovernedCommands :many
SELECT id, session_id, idempotency_key, request_fingerprint, command_class, state,
    controller_generation, expected_revision, capability_fingerprint,
    provider_conversation_id, client_message_id, provider_turn_id,
    provider_event_id, provider_cursor, replay_strategy,
    reconciliation_outcome, quiescence, quiescence_evidence_ref,
    created_at, updated_at
FROM governed_commands
WHERE state IN ('claimed','dispatching','delivery_unknown')
ORDER BY created_at ASC, id ASC;

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

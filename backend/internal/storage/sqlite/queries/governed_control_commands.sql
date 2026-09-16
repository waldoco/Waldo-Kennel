-- name: InsertGovernedControlCommandClaim :execrows
INSERT INTO governed_control_commands (
 id,session_id,idempotency_key,request_fingerprint,command_class,state,
 controller_generation,expected_revision,capability_fingerprint,provider_conversation_id,
 client_message_id,provider_turn_id,request_instance_id,quiescence,quiescence_evidence_ref,created_at,updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;

-- name: GetGovernedControlCommand :one
SELECT * FROM governed_control_commands WHERE id = ?;

-- name: GetGovernedControlCommandByKey :one
SELECT * FROM governed_control_commands WHERE session_id = ? AND idempotency_key = ?;

-- name: AdvanceGovernedControlCommand :execrows
UPDATE governed_control_commands SET state=sqlc.arg(next_state), quiescence=sqlc.arg(quiescence),
 quiescence_evidence_ref=sqlc.arg(quiescence_evidence_ref), updated_at=sqlc.arg(updated_at)
WHERE governed_control_commands.id=sqlc.arg(id) AND governed_control_commands.state=sqlc.arg(expected_state)
 AND governed_control_commands.controller_generation=sqlc.arg(expected_controller_generation)
 AND governed_control_commands.expected_revision=sqlc.arg(expected_revision)
 AND governed_control_commands.capability_fingerprint=sqlc.arg(expected_capability_fingerprint)
 AND (sqlc.arg(expected_state) <> 'claimed' OR governed_control_commands.controller_generation=(SELECT sessions.controller_generation FROM sessions WHERE sessions.id=governed_control_commands.session_id))
 AND sqlc.arg(updated_at) > governed_control_commands.created_at;

-- name: ListUnsettledGovernedControlCommands :many
SELECT * FROM governed_control_commands
WHERE state IN ('claimed','dispatching','delivery_unknown')
ORDER BY created_at,id;

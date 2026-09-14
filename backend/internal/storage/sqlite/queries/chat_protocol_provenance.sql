-- Persisted protocol-negotiation provenance for Chat sessions. See migration 0136.

-- name: NextChatProtocolProvenanceSeq :one
SELECT COALESCE(MAX(seq), 0) + 1 FROM chat_protocol_provenance WHERE session_id = ?;

-- name: InsertChatProtocolProvenance :exec
INSERT INTO chat_protocol_provenance (
    session_id, seq, harness, provider, installed_version, generated_from,
    protocol_digest, generated_digest, matches_generated,
    degraded_capabilities, missing_floor, negotiated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- Latest episode at or before a binding time: the negotiation the work under
-- review actually ran on. A later episode must never answer for an older
-- binding.
-- name: ChatProtocolProvenanceAtOrBefore :one
SELECT * FROM chat_protocol_provenance
WHERE session_id = ? AND negotiated_at <= ?
ORDER BY negotiated_at DESC, seq DESC
LIMIT 1;

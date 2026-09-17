-- The answer target is Kennel's durable request-card instance, not a provider
-- protocol generation. Add the truthfully named column without renaming the
-- 0141 table: SQLite rewrites unrelated historical triggers during RENAME.
-- Existing answer claims retain their local identity through the one-time copy.
-- +goose Up
ALTER TABLE governed_control_commands ADD COLUMN request_instance_id TEXT NOT NULL DEFAULT '';
UPDATE governed_control_commands
SET request_instance_id = target_generation
WHERE command_class = 'answer' AND request_instance_id = '';
-- +goose Down
-- Additive compatibility column intentionally retained. SQLite DROP COLUMN
-- reparses unrelated historical triggers and is unsafe on deployed schemas.
SELECT 1;

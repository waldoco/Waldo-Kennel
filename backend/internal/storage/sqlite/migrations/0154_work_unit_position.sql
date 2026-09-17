-- +goose Up
-- +goose StatementBegin
-- Freeze one canonical serial position for every WorkUnit. Historical rows are
-- ordered by insertion rowid inside each immutable Plan. Direct legacy writers
-- receive the next position; canonical writers always supply it explicitly.
ALTER TABLE work_units ADD COLUMN position INTEGER;

-- Existing Plans retain their historical lexical-ID tie-break by leaving this
-- nullable. Inventing an order during migration would silently rewrite approved
-- execution policy. Every new canonical Plan persists all positive positions.
CREATE UNIQUE INDEX idx_work_units_plan_position
    ON work_units (plan_revision_id, position)
    WHERE position IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- Schema-preserving because SQLite cannot safely drop this authority column
-- while immutable dependent tables and triggers reference work_units.

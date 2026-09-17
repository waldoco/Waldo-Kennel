-- +goose Up
ALTER TABLE work_units ADD COLUMN role TEXT;
CREATE TABLE work_unit_inputs(
 work_unit_id TEXT NOT NULL REFERENCES work_units(id),
 from_work_unit_id TEXT NOT NULL REFERENCES work_units(id),
 required TEXT NOT NULL CHECK(length(trim(required)) > 0),
 position INTEGER NOT NULL CHECK(position > 0),
 PRIMARY KEY(work_unit_id, from_work_unit_id),
 UNIQUE(work_unit_id, position)
);
CREATE TRIGGER work_unit_inputs_immutable_update BEFORE UPDATE ON work_unit_inputs BEGIN SELECT RAISE(ABORT,'work unit inputs are immutable'); END;
CREATE TRIGGER work_unit_inputs_immutable_delete BEFORE DELETE ON work_unit_inputs BEGIN SELECT RAISE(ABORT,'work unit inputs are immutable'); END;
-- +goose Down
DROP TRIGGER IF EXISTS work_unit_inputs_immutable_delete;
DROP TRIGGER IF EXISTS work_unit_inputs_immutable_update;
DROP TABLE IF EXISTS work_unit_inputs;

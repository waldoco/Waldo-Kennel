-- W1.2 schema is installed conditionally by reconcileAdmissionSchema so burned
-- legacy ledgers remain upgradeable. Existing rows are explicitly unknown.
-- +goose Up
SELECT 1;
-- +goose Down
SELECT 1;

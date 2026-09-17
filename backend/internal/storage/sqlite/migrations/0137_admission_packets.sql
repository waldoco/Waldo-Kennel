-- W1.1 admission schema is installed by reconcileAdmissionSchema. SQLite cannot
-- conditionally ALTER work_units when an older burned ledger omitted 0100.
-- +goose Up
SELECT 1;
-- +goose Down
SELECT 1;

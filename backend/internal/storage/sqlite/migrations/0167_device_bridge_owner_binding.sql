-- Connect to Waldo: which Waldo owner this Mac pairs with, learned from the owner-scoped
-- pairing code (wp1.<owner>.<code>) instead of boot configuration. One row at most.
-- Holds an owner identifier only: no code, no key bytes. Rebinding to another owner is
-- decided by the repository inside the same IMMEDIATE transaction that checks the bound
-- owner's activation, so a paired or in-flight identity is never orphaned.
-- Additive: 0165 is untouched; a boot-configured owner keeps working without this row.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE device_bridge_owner_binding (
    singleton  INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
    owner_id   TEXT NOT NULL CHECK (owner_id <> ''),
    bound_at   TIMESTAMP NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- The binding names the owner of retained activation evidence; no automatic rollback.

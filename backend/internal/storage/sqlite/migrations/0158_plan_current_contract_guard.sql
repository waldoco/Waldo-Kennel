-- S3.2 authority-integrity race fence: a Plan revision is only ever minted
-- against the Outcome's CURRENT Contract revision. The planning-source guard
-- already fences session-sourced inserts; sessionless one-shot inserts had no
-- same-transaction fence, so a Contract advance committing between the
-- pre-evaluation recheck and the Plan append could insert a Plan bound to a
-- superseded Contract. This guard fires for every plan_revisions insert,
-- inside the append's own transaction.

-- +goose Up
-- +goose StatementBegin
DROP TRIGGER IF EXISTS plan_revisions_current_contract_guard;
CREATE TRIGGER plan_revisions_current_contract_guard
BEFORE INSERT ON plan_revisions
BEGIN
    SELECT CASE WHEN NEW.contract_revision_number <> (
        SELECT o.current_revision_number FROM outcomes o WHERE o.id = NEW.outcome_id
    ) THEN RAISE(ABORT, 'plan revision must bind the current Contract revision') END;
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS plan_revisions_current_contract_guard;
-- +goose StatementEnd

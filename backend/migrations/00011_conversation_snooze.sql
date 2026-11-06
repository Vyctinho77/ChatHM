-- +goose Up
ALTER TABLE conversations
    ADD COLUMN snoozed_until timestamptz,
    ADD COLUMN snoozed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT conversations_closed_not_snoozed CHECK (status <> 'closed' OR snoozed_until IS NULL);

CREATE INDEX conversations_snooze_due_idx ON conversations (snoozed_until)
    WHERE snoozed_until IS NOT NULL AND status <> 'closed';

-- +goose Down
DROP INDEX conversations_snooze_due_idx;
ALTER TABLE conversations DROP CONSTRAINT conversations_closed_not_snoozed,
    DROP COLUMN snoozed_until, DROP COLUMN snoozed_by;

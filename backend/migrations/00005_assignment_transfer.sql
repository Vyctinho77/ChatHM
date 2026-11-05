-- +goose Up
-- Uma atribuição também pode terminar por transferência para outra pessoa.
ALTER TABLE conversation_assignments DROP CONSTRAINT conversation_assignments_ended_reason_check;
ALTER TABLE conversation_assignments
    ADD CONSTRAINT conversation_assignments_ended_reason_check CHECK (ended_reason IN ('closed', 'transferred'));

-- +goose Down
UPDATE conversation_assignments SET ended_reason = 'closed' WHERE ended_reason = 'transferred';
ALTER TABLE conversation_assignments DROP CONSTRAINT conversation_assignments_ended_reason_check;
ALTER TABLE conversation_assignments
    ADD CONSTRAINT conversation_assignments_ended_reason_check CHECK (ended_reason IN ('closed'));

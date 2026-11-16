-- +goose Up
-- Quando quem transfere autoriza, o lembrete pessoal vai junto: fica registrado no novo atendimento
-- (histórico permanente, mesmo que quem recebeu edite ou apague o próprio lembrete depois).
ALTER TABLE conversation_assignments
    ADD COLUMN handover_note  text CHECK (handover_note IS NULL OR length(btrim(handover_note)) BETWEEN 1 AND 240),
    ADD COLUMN handover_color text CHECK (handover_color IS NULL OR handover_color IN ('olive', 'amber', 'sky', 'rose', 'violet', 'neutral')),
    ADD COLUMN handover_by    uuid REFERENCES users (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE conversation_assignments
    DROP COLUMN handover_by,
    DROP COLUMN handover_color,
    DROP COLUMN handover_note;

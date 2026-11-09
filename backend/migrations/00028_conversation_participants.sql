-- +goose Up
-- Participantes: outras pessoas (de outras equipes, ou administradores) incluídas no mesmo atendimento.
-- Elas veem a conversa e respondem sem assumir; o responsável continua o mesmo.
CREATE TABLE conversation_participants (
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    added_by        uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, user_id)
);
CREATE INDEX conversation_participants_user_idx ON conversation_participants (user_id);

-- Só o backend acessa.
ALTER TABLE conversation_participants ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE conversation_participants FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE conversation_participants FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE conversation_participants FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE conversation_participants;

-- +goose Up

-- Edição e "apagar para todos": a mensagem continua na conversa, marcada.
ALTER TABLE messages
    ADD COLUMN edited_at  timestamptz,
    ADD COLUMN deleted_at timestamptz;

-- "Apagar para mim": esconde a mensagem só para quem apagou (a conversa é compartilhada pela equipe).
CREATE TABLE message_hidden (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    message_id uuid NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, message_id)
);
CREATE INDEX message_hidden_message_idx ON message_hidden (message_id);

-- Só o backend acessa, com o usuário autenticado.
ALTER TABLE message_hidden ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE message_hidden FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE message_hidden FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE message_hidden FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE message_hidden;
ALTER TABLE messages DROP COLUMN deleted_at, DROP COLUMN edited_at;

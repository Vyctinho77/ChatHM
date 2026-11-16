-- +goose Up
-- Menções (@pessoa) e conversas de clientes compartilhadas no chat interno.
CREATE TABLE internal_mentions (
    message_id uuid NOT NULL REFERENCES internal_messages (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (message_id, user_id)
);
CREATE INDEX internal_mentions_user_idx ON internal_mentions (user_id, message_id);

-- Só o backend acessa.
ALTER TABLE internal_mentions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE internal_mentions FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE internal_mentions FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE internal_mentions FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- Cartão de conversa de cliente: só o id da conversa fica na mensagem (payload.sharedConversationId); o nome do
-- contato é resolvido para cada pessoa, e só para quem já pode ver aquela conversa.
ALTER TABLE internal_messages DROP CONSTRAINT internal_messages_kind_check;
ALTER TABLE internal_messages
    ADD CONSTRAINT internal_messages_kind_check CHECK (kind IN ('text', 'image', 'audio', 'document', 'location', 'conversation'));

-- +goose Down
DELETE FROM internal_messages WHERE kind = 'conversation';
ALTER TABLE internal_messages DROP CONSTRAINT internal_messages_kind_check;
ALTER TABLE internal_messages
    ADD CONSTRAINT internal_messages_kind_check CHECK (kind IN ('text', 'image', 'audio', 'document', 'location'));
DROP TABLE internal_mentions;

-- +goose Up
-- Reações às mensagens (como no WhatsApp): uma por pessoa e mensagem; trocar de emoji substitui, "reagir de novo" remove.
-- O escritório tem UMA conta no WhatsApp, então todas as reações dele (feitas aqui ou no celular) são uma só: reactor = 'office'.
-- Quem do escritório reagiu pelo ChatHM fica em user_id (só informação). Do cliente (ou, em grupo, de cada integrante) o reactor
-- é o identificador dele no WhatsApp.
CREATE TABLE message_reactions (
    message_id uuid        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    reactor    text        NOT NULL CHECK (char_length(reactor) BETWEEN 1 AND 200),
    emoji      text        NOT NULL CHECK (char_length(emoji) BETWEEN 1 AND 24),
    user_id    uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, reactor)
);
ALTER TABLE message_reactions ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS message_reactions;

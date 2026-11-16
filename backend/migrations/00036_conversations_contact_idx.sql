-- +goose Up
-- A lista de contatos e o histórico do contato procuram as conversas de um contato (abertas ou não). O índice único que existia
-- só cobre a conversa aberta; as demais faziam uma leitura sequencial da tabela de conversas para cada contato.
CREATE INDEX conversations_contact_idx ON conversations (contact_id);

-- +goose Down
DROP INDEX conversations_contact_idx;

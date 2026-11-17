-- +goose Up
-- A conferência de mensagens repetidas passou a olhar só o ID do WhatsApp (o mesmo chat pode chegar por dois endereços: número e @lid).
-- Sem este índice, cada mensagem recebida (e cada uma das milhares do histórico) varreria a tabela inteira.
CREATE INDEX IF NOT EXISTS messages_external_idx ON messages (external_id);

-- +goose Down
DROP INDEX IF EXISTS messages_external_idx;

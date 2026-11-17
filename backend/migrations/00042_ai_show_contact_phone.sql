-- +goose Up
-- A IA que cobra no Asaas precisa casar o cliente do Asaas com o contato do ChatHM, e o telefone é a ligação mais segura. Esta opção
-- mostra à IA só o campo "telefone" do CADASTRO do contato (o que ele já tem na ficha); números que aparecem dentro de mensagens
-- continuam ocultos pela opção "Ocultar telefones". Desligada por padrão; nunca vale para ChatGPT/claude.ai (login) nem no modo estrito.
ALTER TABLE ai_settings ADD COLUMN show_contact_phone boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE ai_settings DROP COLUMN IF EXISTS show_contact_phone;

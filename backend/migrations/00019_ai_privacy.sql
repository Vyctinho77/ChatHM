-- +goose Up

-- Privacidade das respostas do MCP: e-mails ocultos por padrão e modo estrito (nomes reduzidos, sem observações internas).
-- Credenciais e dados financeiros são sempre ocultados (sem opção). Clientes OAuth recebem tudo ocultado, sempre.
ALTER TABLE ai_settings
    ADD COLUMN mask_emails    boolean NOT NULL DEFAULT true,
    ADD COLUMN strict_privacy boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE ai_settings DROP COLUMN strict_privacy, DROP COLUMN mask_emails;

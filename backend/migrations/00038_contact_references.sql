-- +goose Up
-- Referências do cliente em outros sistemas: o CNPJ/CPF e o id do cliente no Asaas. Ligam a conversa ao cadastro financeiro sem casar por telefone
-- à mão (os agentes de cobrança e de consulta usam). Só dígitos no documento; o id do Asaas tem a forma cus_...
-- Vários contatos (números de WhatsApp) podem ter o mesmo documento: não é único.
ALTER TABLE contacts
    ADD COLUMN tax_id    text NOT NULL DEFAULT '' CHECK (tax_id ~ '^([0-9]{11}|[0-9]{14})?$'),
    ADD COLUMN asaas_id  text NOT NULL DEFAULT '' CHECK (asaas_id ~ '^(cus_[A-Za-z0-9]{6,40})?$');

CREATE INDEX contacts_tax_id_idx ON contacts (tax_id) WHERE tax_id <> '';
CREATE INDEX contacts_asaas_id_idx ON contacts (asaas_id) WHERE asaas_id <> '';

-- +goose Down
DROP INDEX IF EXISTS contacts_asaas_id_idx;
DROP INDEX IF EXISTS contacts_tax_id_idx;
ALTER TABLE contacts DROP COLUMN IF EXISTS asaas_id, DROP COLUMN IF EXISTS tax_id;

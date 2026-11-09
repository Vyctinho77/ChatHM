-- +goose Up
-- Nome que aparece acima das mensagens da pessoa (app e WhatsApp). Vazio = usa o nome da conta.
ALTER TABLE users ADD COLUMN signature_name text NOT NULL DEFAULT '' CHECK (char_length(signature_name) <= 40);

-- +goose Down
ALTER TABLE users DROP COLUMN signature_name;

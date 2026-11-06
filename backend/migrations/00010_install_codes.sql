-- +goose Up
-- Código de instalação de uso único, gerado ao entregar a instância a um novo cliente.
-- Enquanto existir um código não usado, só ele libera a configuração inicial (vale mais que SETUP_TOKEN).
CREATE TABLE install_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash  bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at    timestamptz
);

-- +goose Down
DROP TABLE install_codes;

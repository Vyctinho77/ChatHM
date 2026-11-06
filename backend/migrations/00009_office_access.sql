-- +goose Up
-- Token do escritório: a equipe usa o token (enviado pelo administrador) para criar a própria conta.
-- Uma linha só (um escritório por servidor); o token fica cifrado para o administrador poder revê-lo.
CREATE TABLE office_access (
    id             boolean PRIMARY KEY DEFAULT true CHECK (id),
    token_enc      bytea NOT NULL,
    allowed_domain text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    rotated_at     timestamptz
);

-- +goose Down
DROP TABLE office_access;

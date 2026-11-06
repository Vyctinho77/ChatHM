-- +goose Up
-- Acesso sem terminal: convites (código de uso único para a pessoa configurar o autenticador)
-- e códigos de recuperação (uso único, caso o celular seja perdido).
CREATE TABLE invites (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash   bytea NOT NULL,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    failures    integer NOT NULL DEFAULT 0,
    created_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
-- No máximo um convite aberto por pessoa: emitir outro invalida o anterior.
CREATE UNIQUE INDEX invites_one_open_per_user ON invites (user_id) WHERE used_at IS NULL;

CREATE TABLE recovery_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  bytea NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recovery_codes_user_idx ON recovery_codes (user_id) WHERE used_at IS NULL;

-- +goose Down
DROP TABLE recovery_codes;
DROP TABLE invites;

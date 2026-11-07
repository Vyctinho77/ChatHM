-- +goose Up

-- Acesso por IA (MCP): configuração do escritório (uma linha) e tokens pessoais.
CREATE TABLE ai_settings (
    id              boolean PRIMARY KEY DEFAULT true CHECK (id),
    enabled         boolean NOT NULL DEFAULT false,      -- "Permitir acesso por IA" (desligado por padrão)
    mask_documents  boolean NOT NULL DEFAULT true,       -- oculta CPF/CNPJ nas respostas
    mask_phones     boolean NOT NULL DEFAULT false,      -- oculta o meio dos telefones
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      uuid REFERENCES users (id) ON DELETE SET NULL
);
INSERT INTO ai_settings (id) VALUES (true);

-- O token (chm_...) só existe em texto na hora da criação; aqui fica o SHA-256 e um prefixo para identificação.
CREATE TABLE api_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    prefix       text NOT NULL,
    token_hash   bytea NOT NULL UNIQUE,
    scopes       text[] NOT NULL DEFAULT '{read}',
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at   timestamptz
);
CREATE INDEX api_tokens_user_idx ON api_tokens (user_id);

-- Só o backend acessa, com o usuário autenticado.
ALTER TABLE ai_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_tokens ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE ai_settings, api_tokens FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE ai_settings, api_tokens FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE ai_settings, api_tokens FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE api_tokens;
DROP TABLE ai_settings;

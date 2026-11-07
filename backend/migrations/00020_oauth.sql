-- +goose Up

-- OAuth 2.1 para aplicativos de IA hospedados por terceiros (ChatGPT, claude.ai). Fluxo: registro dinâmico (só para
-- endereços de retorno em uma lista do administrador), consentimento na tela do ChatHM, código com PKCE (S256),
-- token de acesso curto + token de renovação com rotação e detecção de reuso.
ALTER TABLE ai_settings
    ADD COLUMN allow_oauth boolean NOT NULL DEFAULT false,
    ADD COLUMN oauth_redirect_hosts text[] NOT NULL DEFAULT '{chatgpt.com,platform.openai.com,chat.openai.com,claude.ai,claude.com}';

CREATE TABLE oauth_clients (
    client_id     text PRIMARY KEY,
    name          text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    redirect_uris text[] NOT NULL CHECK (cardinality(redirect_uris) BETWEEN 1 AND 5),
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Uma concessão = uma pessoa autorizou um aplicativo, com um nível (read | write | send).
CREATE TABLE oauth_grants (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    client_id    text NOT NULL REFERENCES oauth_clients (client_id) ON DELETE CASCADE,
    scopes       text[] NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at   timestamptz
);
CREATE INDEX oauth_grants_user_idx ON oauth_grants (user_id);

CREATE TABLE oauth_codes (
    code_hash      bytea PRIMARY KEY,
    grant_id       uuid NOT NULL REFERENCES oauth_grants (id) ON DELETE CASCADE,
    redirect_uri   text NOT NULL,
    code_challenge text NOT NULL,
    resource       text NOT NULL DEFAULT '',
    expires_at     timestamptz NOT NULL,
    used_at        timestamptz
);

CREATE TABLE oauth_refresh_tokens (
    token_hash bytea PRIMARY KEY,
    grant_id   uuid NOT NULL REFERENCES oauth_grants (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX oauth_refresh_grant_idx ON oauth_refresh_tokens (grant_id);

-- Tokens de acesso OAuth vivem em api_tokens, ligados à concessão.
ALTER TABLE api_tokens
    ADD COLUMN kind     text NOT NULL DEFAULT 'personal' CHECK (kind IN ('personal', 'oauth')),
    ADD COLUMN grant_id uuid REFERENCES oauth_grants (id) ON DELETE CASCADE;

-- Só o backend acessa.
ALTER TABLE oauth_clients ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_refresh_tokens ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE oauth_clients, oauth_grants, oauth_codes, oauth_refresh_tokens FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE oauth_clients, oauth_grants, oauth_codes, oauth_refresh_tokens FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE oauth_clients, oauth_grants, oauth_codes, oauth_refresh_tokens FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE api_tokens DROP COLUMN grant_id, DROP COLUMN kind;
DROP TABLE oauth_refresh_tokens;
DROP TABLE oauth_codes;
DROP TABLE oauth_grants;
DROP TABLE oauth_clients;
ALTER TABLE ai_settings DROP COLUMN oauth_redirect_hosts, DROP COLUMN allow_oauth;

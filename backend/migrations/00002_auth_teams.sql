-- +goose Up

CREATE TABLE users (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email                  citext NOT NULL UNIQUE,
    name                   text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    password_hash          text NOT NULL,
    role                   text NOT NULL CHECK (role IN ('admin', 'agent')),
    is_active              boolean NOT NULL DEFAULT true,
    theme                  text NOT NULL DEFAULT 'system' CHECK (theme IN ('light', 'dark', 'system')),
    -- MFA (TOTP). Segredos cifrados com AES-256-GCM; a chave fica fora do banco.
    mfa_enabled            boolean NOT NULL DEFAULT false,
    mfa_secret_enc         bytea,
    mfa_pending_secret_enc bytea,
    mfa_last_step          bigint NOT NULL DEFAULT 0, -- evita reuso do mesmo código
    password_changed_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at          timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT mfa_enabled OR mfa_secret_enc IS NOT NULL)
);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE teams (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        citext NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 80),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER teams_updated_at BEFORE UPDATE ON teams
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE team_members (
    team_id    uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX team_members_user_idx ON team_members (user_id);

-- Sessões server-side. O cookie carrega o token; aqui guardamos apenas o SHA-256.
CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,
    csrf_token   text NOT NULL,
    mfa_verified boolean NOT NULL DEFAULT false,
    mfa_failures integer NOT NULL DEFAULT 0,
    ip           inet,
    user_agent   text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- Tentativas de login para limitar força bruta por e-mail e por IP.
CREATE TABLE login_attempts (
    id         bigserial PRIMARY KEY,
    email      citext NOT NULL,
    ip         inet,
    success    boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_attempts_email_idx ON login_attempts (email, created_at) WHERE NOT success;
CREATE INDEX login_attempts_ip_idx ON login_attempts (ip, created_at) WHERE NOT success;

-- Trilha de auditoria. Nunca registrar senhas, tokens ou conteúdo de mensagens.
CREATE TABLE audit_logs (
    id            bigserial PRIMARY KEY,
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    action        text NOT NULL,
    target_type   text NOT NULL DEFAULT '',
    target_id     text NOT NULL DEFAULT '',
    ip            inet,
    metadata      jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_created_idx ON audit_logs (created_at);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_user_id, created_at);

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE login_attempts;
DROP TABLE sessions;
DROP TABLE team_members;
DROP TABLE teams;
DROP TABLE users;

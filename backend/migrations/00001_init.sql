-- +goose Up
-- Migração base: extensões e utilitários. As tabelas da Fase 1
-- (users, teams, team_members, sessions, audit_logs) vêm na próxima migração.
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;    -- e-mails case-insensitive
CREATE EXTENSION IF NOT EXISTS unaccent;  -- normalização para classificação

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS set_updated_at();
DROP EXTENSION IF EXISTS unaccent;
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;

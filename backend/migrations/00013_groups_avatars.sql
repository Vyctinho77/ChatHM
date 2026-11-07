-- +goose Up

-- Grupos do WhatsApp viram contatos do tipo "group" (wa_id = JID do grupo, name = assunto).
-- Foto de perfil: miniatura baixada do WhatsApp e servida por /contacts/{id}/avatar.
ALTER TABLE contacts
    ADD COLUMN kind              text NOT NULL DEFAULT 'person' CHECK (kind IN ('person', 'group')),
    ADD COLUMN avatar            bytea,
    ADD COLUMN avatar_version    bigint NOT NULL DEFAULT 0,
    ADD COLUMN avatar_checked_at timestamptz;

-- +goose Down
ALTER TABLE contacts
    DROP COLUMN avatar_checked_at,
    DROP COLUMN avatar_version,
    DROP COLUMN avatar,
    DROP COLUMN kind;

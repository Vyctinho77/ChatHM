-- +goose Up
-- Chat interno do escritório: um grupo para a equipe tirar dúvidas. Nada aqui passa pelo WhatsApp nem aparece
-- para clientes, no MCP ou nas sugestões da IA. A sala tem tipo para, no futuro, existirem salas por equipe.
CREATE TABLE internal_rooms (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind       text NOT NULL CHECK (kind IN ('office')),
    name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX internal_rooms_one_office ON internal_rooms (kind) WHERE kind = 'office';
INSERT INTO internal_rooms (kind, name) VALUES ('office', 'Escritório');

CREATE TABLE internal_messages (
    id         uuid PRIMARY KEY,
    -- Ordem estável (e cursor de leitura): created_at pode empatar.
    seq        bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    room_id    uuid NOT NULL REFERENCES internal_rooms (id) ON DELETE CASCADE,
    author_id  uuid NOT NULL REFERENCES users (id),
    kind       text NOT NULL CHECK (kind IN ('text', 'image', 'audio', 'document', 'location')),
    body       text NOT NULL DEFAULT '' CHECK (char_length(body) <= 16000),
    payload    jsonb NOT NULL DEFAULT '{}',
    quoted_id  uuid REFERENCES internal_messages (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at  timestamptz,
    deleted_at timestamptz,
    deleted_by uuid REFERENCES users (id) ON DELETE SET NULL
);
CREATE INDEX internal_messages_room_seq_idx ON internal_messages (room_id, seq DESC);

CREATE TABLE internal_attachments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  uuid NOT NULL UNIQUE REFERENCES internal_messages (id) ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    mime_type   text NOT NULL,
    size_bytes  bigint NOT NULL CHECK (size_bytes >= 0),
    sha256      text NOT NULL,
    file_name   text NOT NULL DEFAULT '',
    scan_status text NOT NULL CHECK (scan_status IN ('clean', 'skipped')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Até onde cada pessoa já leu (base do selo de não lidas).
CREATE TABLE internal_reads (
    room_id       uuid NOT NULL REFERENCES internal_rooms (id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    last_read_seq bigint NOT NULL DEFAULT 0,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, user_id)
);

-- Só o backend acessa.
-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['internal_rooms', 'internal_messages', 'internal_attachments', 'internal_reads'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('REVOKE ALL ON TABLE %I FROM PUBLIC', t);
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
            EXECUTE format('REVOKE ALL ON TABLE %I FROM anon', t);
        END IF;
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
            EXECUTE format('REVOKE ALL ON TABLE %I FROM authenticated', t);
        END IF;
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE internal_reads;
DROP TABLE internal_attachments;
DROP TABLE internal_messages;
DROP TABLE internal_rooms;

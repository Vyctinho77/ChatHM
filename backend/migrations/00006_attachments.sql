-- +goose Up
-- Anexos (áudio, imagem, documento) das mensagens. O binário fica no armazenamento
-- privado (disco cifrado ou S3); aqui só os metadados e o resultado da verificação.
CREATE TABLE attachments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  uuid NOT NULL UNIQUE REFERENCES messages (id) ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    mime_type   text NOT NULL,
    size_bytes  bigint NOT NULL CHECK (size_bytes >= 0),
    sha256      text NOT NULL,
    file_name   text NOT NULL DEFAULT '',
    scan_status text NOT NULL CHECK (scan_status IN ('clean', 'skipped')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE attachments;

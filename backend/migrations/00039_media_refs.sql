-- +goose Up
-- Referências de mídia do WhatsApp: o endereço, a chave e os hashes que permitem baixar o arquivo de uma mensagem (e pedir ao celular que o
-- reenvie se o WhatsApp já o apagou dos servidores). O histórico traz a referência mesmo sem o arquivo; guardada, ela deixa o servidor baixar
-- em segundo plano, devagar e dentro do teto de espaço. A referência tem a CHAVE do arquivo: fica cifrada (secretbox, ENCRYPTION_KEY).
CREATE TABLE media_refs (
    message_id         uuid PRIMARY KEY REFERENCES messages (id) ON DELETE CASCADE,
    kind               text NOT NULL CHECK (kind IN ('image', 'audio', 'document', 'sticker')),
    ref_enc            bytea NOT NULL,
    -- pending: falta baixar · retry_requested: o celular foi chamado a reenviar · done: arquivo guardado
    -- unavailable: não há mais como obter · skipped: acima do limite de tamanho ou do teto de espaço
    status             text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'retry_requested', 'done', 'unavailable', 'skipped')),
    attempts           integer NOT NULL DEFAULT 0,
    next_attempt_at    timestamptz NOT NULL DEFAULT now(),
    retry_requested_at timestamptz,
    last_error         text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 200),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX media_refs_due_idx ON media_refs (next_attempt_at) WHERE status = 'pending';
CREATE INDEX media_refs_retry_idx ON media_refs (retry_requested_at) WHERE status = 'retry_requested';

-- Só o backend acessa.
ALTER TABLE media_refs ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE media_refs FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE media_refs FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE media_refs FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS media_refs;

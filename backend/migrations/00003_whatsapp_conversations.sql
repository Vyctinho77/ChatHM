-- +goose Up

-- Conexão do escritório com o WhatsApp (MVP: uma única conexão).
-- As chaves do dispositivo ficam em device_enc, cifradas com AES-256-GCM (chave fora do banco).
CREATE TABLE whatsapp_connections (
    id           smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    provider     text NOT NULL DEFAULT 'whatsmeow',
    status       text NOT NULL DEFAULT 'disconnected'
                 CHECK (status IN ('disconnected', 'pending_qr', 'connected', 'error')),
    phone        text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
    device_jid   text NOT NULL DEFAULT '',
    device_enc   bytea,
    last_error   text NOT NULL DEFAULT '',
    connected_at timestamptz,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
INSERT INTO whatsapp_connections (id) VALUES (1);
CREATE TRIGGER whatsapp_connections_updated_at BEFORE UPDATE ON whatsapp_connections
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Pre-keys do protocolo Signal (chaves privadas), cifradas.
CREATE TABLE whatsapp_prekeys (
    device_jid text    NOT NULL,
    key_id     integer NOT NULL,
    key_enc    bytea   NOT NULL,
    uploaded   boolean NOT NULL DEFAULT false,
    PRIMARY KEY (device_jid, key_id)
);

CREATE TABLE contacts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    wa_id      text NOT NULL UNIQUE,              -- identificador no WhatsApp (número ou LID)
    phone      text NOT NULL DEFAULT '',          -- E.164, quando conhecido
    name       text NOT NULL DEFAULT '',          -- nome exibido (push name ou editado)
    company    text NOT NULL DEFAULT '' CHECK (length(company) <= 120),
    note       text NOT NULL DEFAULT '' CHECK (length(note) <= 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER contacts_updated_at BEFORE UPDATE ON contacts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE conversations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    contact_id      uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'in_progress', 'closed')),
    assignee_id     uuid REFERENCES users (id) ON DELETE SET NULL,
    unread_count    integer NOT NULL DEFAULT 0 CHECK (unread_count >= 0),
    last_message_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    closed_at       timestamptz
);
-- No máximo uma conversa aberta por contato (sem cópias; regra 3 do blueprint).
CREATE UNIQUE INDEX conversations_one_open_per_contact ON conversations (contact_id) WHERE status <> 'closed';
CREATE INDEX conversations_last_message_idx ON conversations (last_message_at DESC);

CREATE TABLE messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    chat_jid        text NOT NULL,
    external_id     text NOT NULL,
    direction       text NOT NULL CHECK (direction IN ('in', 'out')),
    kind            text NOT NULL CHECK (kind IN ('text', 'audio', 'image', 'document', 'location', 'unsupported')),
    body            text NOT NULL DEFAULT '',      -- texto ou legenda
    payload         jsonb NOT NULL DEFAULT '{}',   -- metadados de mídia / localização
    sender_name     text NOT NULL DEFAULT '',
    status          text CHECK (status IN ('pending', 'sent', 'delivered', 'read', 'failed')),
    sent_at         timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- Deduplicação: o mesmo evento do provedor nunca gera duas mensagens.
    UNIQUE (chat_jid, external_id)
);
CREATE INDEX messages_conversation_idx ON messages (conversation_id, sent_at);

-- +goose Down
DROP TABLE messages;
DROP TABLE conversations;
DROP TABLE contacts;
DROP TABLE whatsapp_prekeys;
DROP TABLE whatsapp_connections;

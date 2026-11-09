-- +goose Up
-- Envios automáticos pela IA (docs/mcp-automacao.md, fase 6a): fila com aprovação, categorias, biblioteca de PDFs e ritmo.

-- Chave mestra do administrador (desligada por padrão) e provedor da IA de cada token pessoal (para a logo nas mensagens).
ALTER TABLE ai_settings ADD COLUMN allow_automation boolean NOT NULL DEFAULT false;
ALTER TABLE api_tokens ADD COLUMN provider text NOT NULL DEFAULT ''
    CHECK (provider IN ('', 'claude', 'chatgpt', 'grok', 'gemini', 'other'));

-- Cliente que respondeu PARAR (ou foi marcado pela equipe): a IA nunca mais inclui.
ALTER TABLE contacts ADD COLUMN automation_opt_out_at timestamptz;

CREATE TABLE automation_settings (
    id              boolean PRIMARY KEY DEFAULT true CHECK (id),
    paused          boolean NOT NULL DEFAULT false,
    signature       text    NOT NULL DEFAULT '' CHECK (char_length(signature) <= 60),
    hourly_cap      integer NOT NULL DEFAULT 30   CHECK (hourly_cap BETWEEN 1 AND 500),
    daily_cap       integer NOT NULL DEFAULT 150  CHECK (daily_cap BETWEEN 1 AND 3000),
    gap_min_s       integer NOT NULL DEFAULT 8    CHECK (gap_min_s BETWEEN 1 AND 300),
    gap_max_s       integer NOT NULL DEFAULT 25   CHECK (gap_max_s BETWEEN 1 AND 600),
    dialog_hours    integer NOT NULL DEFAULT 12   CHECK (dialog_hours BETWEEN 0 AND 168),
    client_daily    integer NOT NULL DEFAULT 1    CHECK (client_daily BETWEEN 1 AND 10),
    paused_reason   text    NOT NULL DEFAULT '' CHECK (char_length(paused_reason) <= 200),
    timezone        text    NOT NULL DEFAULT 'America/Manaus',
    allowed_domains text[]  NOT NULL DEFAULT '{}',
    office_fields   jsonb   NOT NULL DEFAULT '{}'::jsonb,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    CHECK (gap_max_s >= gap_min_s)
);
INSERT INTO automation_settings (id) VALUES (true);

CREATE TABLE automation_categories (
    key               text PRIMARY KEY CHECK (key ~ '^[a-z_]{2,30}$'),
    label             text NOT NULL,
    enabled           boolean NOT NULL DEFAULT false,
    level             smallint NOT NULL DEFAULT 1 CHECK (level BETWEEN 1 AND 3), -- 1 = aprovação; 2 e 3 chegam nas fases 6b e 6c
    window_start      time NOT NULL DEFAULT '08:00',
    window_end        time NOT NULL DEFAULT '18:00',
    weekdays          smallint[] NOT NULL DEFAULT '{1,2,3,4,5}',
    min_interval_days integer NOT NULL DEFAULT 5 CHECK (min_interval_days BETWEEN 0 AND 365),
    guidelines        text NOT NULL DEFAULT '' CHECK (char_length(guidelines) <= 2000),
    sort              integer NOT NULL DEFAULT 0
);
INSERT INTO automation_categories (key, label, min_interval_days, guidelines, sort) VALUES
    ('das', 'DAS, guias e impostos', 20,
     'Avise sobre guia disponível ou vencimento próximo, com tom cordial e direto. Diga a competência e a data de vencimento. Anexe a guia em PDF quando houver.', 1),
    ('cobranca', 'Cobrança de honorários', 7,
     'Lembre com educação e sem pressão. Nunca ameace, cite protesto, negativação ou cobrança judicial. Informe a competência e o valor. Dados de pagamento só pelos campos fixos do escritório.', 2),
    ('prazos', 'Prazos e obrigações', 7,
     'Avise sobre o prazo de uma obrigação acessória ou entrega de documentos, dizendo o que falta e até quando.', 3),
    ('aviso', 'Avisos do escritório', 3,
     'Comunicados gerais do escritório (mudança de horário, feriado, recesso). Curto e informativo.', 4);

-- PDFs prontos para enviar. Cada um é de um cliente ou marcado como compartilhável (a IA só anexa o do próprio destinatário).
CREATE TABLE automation_documents (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 150),
    storage_key      text NOT NULL,
    mime_type        text NOT NULL,
    size_bytes       bigint NOT NULL,
    sha256           text NOT NULL,
    owner_contact_id uuid REFERENCES contacts (id) ON DELETE CASCADE,
    shareable        boolean NOT NULL DEFAULT false,
    uploaded_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (shareable OR owner_contact_id IS NOT NULL)
);
CREATE INDEX automation_documents_owner_idx ON automation_documents (owner_contact_id);

CREATE TABLE automation_sends (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    token_id      uuid REFERENCES api_tokens (id) ON DELETE SET NULL,
    created_by    uuid REFERENCES users (id) ON DELETE SET NULL, -- dono do token (ou quem criou pela tela)
    provider      text NOT NULL DEFAULT '',
    app_name      text NOT NULL DEFAULT '',
    category      text NOT NULL REFERENCES automation_categories (key),
    reason        text NOT NULL CHECK (char_length(reason) BETWEEN 5 AND 300),
    text          text NOT NULL CHECK (char_length(text) BETWEEN 1 AND 1200),
    document_id   uuid REFERENCES automation_documents (id) ON DELETE SET NULL,
    contact_id    uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    dedupe_key    text NOT NULL CHECK (char_length(dedupe_key) BETWEEN 1 AND 80),
    status        text NOT NULL DEFAULT 'pending_approval'
                  CHECK (status IN ('pending_approval', 'approved', 'sending', 'sent', 'failed', 'cancelled', 'rejected', 'skipped')),
    status_detail text NOT NULL DEFAULT '',
    flags         text[] NOT NULL DEFAULT '{}',
    approved_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    approved_at   timestamptz,
    edited        boolean NOT NULL DEFAULT false,
    scheduled_for timestamptz,
    sent_at       timestamptz,
    message_id    uuid,
    attempts      integer NOT NULL DEFAULT 0
);
-- Mesmo cliente + assunto + chave não repete enquanto houver um ativo ou enviado.
CREATE UNIQUE INDEX automation_sends_dedupe ON automation_sends (contact_id, category, dedupe_key)
    WHERE status IN ('pending_approval', 'approved', 'sending', 'sent');
CREATE INDEX automation_sends_status_idx ON automation_sends (status, scheduled_for);
CREATE INDEX automation_sends_contact_idx ON automation_sends (contact_id, created_at DESC);

-- Ritmo: quando o próximo envio pode sair (com variação aleatória entre um e outro).
CREATE TABLE automation_state (
    id              boolean PRIMARY KEY DEFAULT true CHECK (id),
    next_allowed_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO automation_state (id) VALUES (true);

-- Só o backend acessa.
ALTER TABLE automation_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE automation_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE automation_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE automation_sends ENABLE ROW LEVEL SECURITY;
ALTER TABLE automation_state ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE automation_settings, automation_categories, automation_documents, automation_sends, automation_state FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE automation_settings, automation_categories, automation_documents, automation_sends, automation_state FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE automation_settings, automation_categories, automation_documents, automation_sends, automation_state FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE automation_state;
DROP TABLE automation_sends;
DROP TABLE automation_documents;
DROP TABLE automation_categories;
DROP TABLE automation_settings;
ALTER TABLE contacts DROP COLUMN automation_opt_out_at;
ALTER TABLE api_tokens DROP COLUMN provider;
ALTER TABLE ai_settings DROP COLUMN allow_automation;

-- +goose Up
-- Raia da IA (docs/mcp-raia.md): a IA age livremente DENTRO de uma raia, cercada por poucas regras que o servidor aplica.
-- Tudo nasce desligado e em "sombra" (a IA escreve, o servidor guarda o rascunho e nada é enviado ao cliente).

CREATE TABLE ai_lanes (
    key                 text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]{1,30}$'),
    label               text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 60),
    enabled             boolean NOT NULL DEFAULT false,
    mode                text NOT NULL DEFAULT 'shadow' CHECK (mode IN ('shadow', 'live')),
    -- Pode escrever primeiro a um cliente (cobrança) ou só responder a conversas que já existem na raia (atendimento inicial)?
    can_initiate        boolean NOT NULL DEFAULT false,
    -- A fila (assunto) que define as conversas da raia. Vazio = sem limite por fila (a raia vale para qualquer contato que o token enxergue).
    segment_id          uuid REFERENCES segments (id) ON DELETE SET NULL,
    -- Para onde a conversa vai quando a IA passa para um humano (vazio = fica onde está).
    handoff_segment_id  uuid REFERENCES segments (id) ON DELETE SET NULL,
    max_turns           integer NOT NULL DEFAULT 12 CHECK (max_turns BETWEEN 1 AND 50),
    max_consecutive     integer NOT NULL DEFAULT 3  CHECK (max_consecutive BETWEEN 1 AND 10),
    contact_daily       integer NOT NULL DEFAULT 10 CHECK (contact_daily BETWEEN 1 AND 50),
    human_quiet_minutes integer NOT NULL DEFAULT 60 CHECK (human_quiet_minutes BETWEEN 0 AND 1440),
    -- Procura conversas paradas (o cliente escreveu e ninguém da equipe iniciou o atendimento) há pelo menos N horas. 0 = não procura.
    stale_hours         integer NOT NULL DEFAULT 0  CHECK (stale_hours BETWEEN 0 AND 720),
    -- Links só destes domínios. Lista vazia = qualquer link.
    allowed_domains     text[]  NOT NULL DEFAULT '{}',
    guidelines          text    NOT NULL DEFAULT '' CHECK (char_length(guidelines) <= 6000),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid REFERENCES users (id) ON DELETE SET NULL
);
ALTER TABLE ai_lanes ENABLE ROW LEVEL SECURITY;

INSERT INTO ai_lanes (key, label, can_initiate, max_consecutive, contact_daily, human_quiet_minutes, stale_hours, allowed_domains, guidelines) VALUES
  ('certificado', 'Certificado digital: atendimento inicial', false, 3, 10, 60, 24, '{}',
   'Você é uma atendente mínima do escritório. Oriente o cliente, em linguagem simples, sobre o que fazer para emitir ou renovar o certificado digital e peça só o que um atendente pediria no início. Se o cliente mandar foto ou documento, ou pedir uma pessoa, passe para um atendente.'),
  ('cobranca', 'Cobrança de honorários', true, 1, 2, 120, 0, '{asaas.com}',
   'Lembre com educação. O link de pagamento do Asaas é o único permitido. Se o cliente disser que já pagou, mandar comprovante ou contestar, passe para um atendente.');

-- Cada token do atendente autônomo pode ficar preso a uma raia: só a raia vale para ele.
ALTER TABLE api_tokens ADD COLUMN lane_key text REFERENCES ai_lanes (key) ON DELETE SET NULL;

-- Conversas que a IA (ou o servidor) passou para um humano. Enquanto aberta (e sem resposta humana depois), a IA fica em silêncio.
CREATE TABLE ai_handoffs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    lane_key        text REFERENCES ai_lanes (key) ON DELETE SET NULL,
    token_id        uuid REFERENCES api_tokens (id) ON DELETE SET NULL,
    reason          text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 40),
    summary         text NOT NULL DEFAULT '' CHECK (char_length(summary) <= 600),
    source          text NOT NULL CHECK (source IN ('ai', 'server')),
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_handoffs_conversation_idx ON ai_handoffs (conversation_id, created_at DESC);
ALTER TABLE ai_handoffs ENABLE ROW LEVEL SECURITY;

-- Modo sombra: o que a IA enviaria. Nada vai ao cliente; a equipe confere o que a IA faria antes de a raia valer.
CREATE TABLE ai_lane_drafts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lane_key        text NOT NULL REFERENCES ai_lanes (key) ON DELETE CASCADE,
    conversation_id uuid REFERENCES conversations (id) ON DELETE CASCADE,
    contact_id      uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    token_id        uuid REFERENCES api_tokens (id) ON DELETE SET NULL,
    body            text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1600),
    created_at      timestamptz NOT NULL DEFAULT now(),
    outcome         text CHECK (outcome IN ('approved', 'edited', 'discarded')),
    reviewed_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at     timestamptz
);
CREATE INDEX ai_lane_drafts_lane_idx ON ai_lane_drafts (lane_key, created_at DESC);
ALTER TABLE ai_lane_drafts ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS ai_lane_drafts;
DROP TABLE IF EXISTS ai_handoffs;
ALTER TABLE api_tokens DROP COLUMN IF EXISTS lane_key;
DROP TABLE IF EXISTS ai_lanes;

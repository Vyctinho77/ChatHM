-- +goose Up
-- MCP: fila de respostas pendentes (o que o agente de IA já tratou) e aviso automático por webhook.

-- O que cada agente (pessoa dona do token + provedor) já avisou ou resolveu em cada conversa. "seen_until" é a última mensagem do cliente
-- tratada: uma resposta mais nova faz a conversa voltar para a fila de pendentes.
CREATE TABLE ai_reply_tracking (
    agent_key       text NOT NULL CHECK (char_length(agent_key) BETWEEN 3 AND 120),
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    status          text NOT NULL CHECK (status IN ('notified', 'resolved')),
    note            text NOT NULL DEFAULT '' CHECK (char_length(note) <= 300),
    seen_until      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_key, conversation_id)
);

-- Só o backend acessa.
ALTER TABLE ai_reply_tracking ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE ai_reply_tracking FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE ai_reply_tracking FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE ai_reply_tracking FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- Aviso automático por webhook: o ChatHM avisa um endereço do administrador quando o cliente responde, sem esperar o agente perguntar.
-- A mensagem em si NÃO vai por padrão: só quem escreveu, em qual conversa e fila (o agente lê o resto pelo MCP). O segredo fica cifrado.
CREATE TABLE ai_webhook (
    id                   boolean PRIMARY KEY DEFAULT true CHECK (id),
    enabled              boolean NOT NULL DEFAULT false,
    url                  text NOT NULL DEFAULT '' CHECK (char_length(url) <= 500),
    secret_enc           bytea,
    only_ai_replies      boolean NOT NULL DEFAULT true,   -- só respostas a mensagens enviadas por IA
    include_preview      boolean NOT NULL DEFAULT false,  -- inclui um trecho (ocultando dados sensíveis); desligado por padrão
    consecutive_failures integer NOT NULL DEFAULT 0,
    last_delivery_at     timestamptz,
    last_status          text NOT NULL DEFAULT '' CHECK (char_length(last_status) <= 40),
    last_error           text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 300),
    disabled_reason      text NOT NULL DEFAULT '' CHECK (char_length(disabled_reason) <= 200),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    updated_by           uuid REFERENCES users (id) ON DELETE SET NULL
);
INSERT INTO ai_webhook (id) VALUES (true);

-- Um aviso pendente por conversa: respostas em rajada se juntam num só (message_count), enviado alguns segundos depois da primeira.
CREATE TABLE ai_webhook_deliveries (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id  uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    first_message_id uuid,
    last_message_id  uuid,
    message_count    integer NOT NULL DEFAULT 1,
    first_at         timestamptz NOT NULL DEFAULT now(),
    status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts         integer NOT NULL DEFAULT 0,
    next_attempt_at  timestamptz NOT NULL,
    last_error       text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 300),
    created_at       timestamptz NOT NULL DEFAULT now(),
    sent_at          timestamptz
);
CREATE UNIQUE INDEX ai_webhook_pending_conversation ON ai_webhook_deliveries (conversation_id) WHERE status = 'pending';
CREATE INDEX ai_webhook_due_idx ON ai_webhook_deliveries (next_attempt_at) WHERE status = 'pending';

ALTER TABLE ai_webhook ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_webhook_deliveries ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE ai_webhook, ai_webhook_deliveries FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE ai_webhook, ai_webhook_deliveries FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE ai_webhook, ai_webhook_deliveries FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE ai_webhook_deliveries;
DROP TABLE ai_webhook;
DROP TABLE ai_reply_tracking;

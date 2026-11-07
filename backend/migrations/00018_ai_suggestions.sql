-- +goose Up

-- A IA nunca envia mensagem a cliente sozinha: ela deixa uma sugestão (rascunho) ou um pedido de envio, e uma
-- pessoa aprova na conversa. O envio real usa o caminho normal do app, como essa pessoa.
ALTER TABLE ai_settings ADD COLUMN allow_send boolean NOT NULL DEFAULT false;

CREATE TABLE ai_suggestions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,         -- dono do token = quem aprova
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    token_id        uuid REFERENCES api_tokens (id) ON DELETE SET NULL,
    intent          text NOT NULL CHECK (intent IN ('draft', 'send')),
    text            text NOT NULL CHECK (length(btrim(text)) BETWEEN 1 AND 4000),
    reply_to        uuid REFERENCES messages (id) ON DELETE SET NULL,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sending', 'sent', 'used', 'discarded', 'expired')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    decided_at      timestamptz,
    expires_at      timestamptz NOT NULL
);
CREATE INDEX ai_suggestions_pending_idx ON ai_suggestions (user_id, conversation_id) WHERE status = 'pending';

ALTER TABLE ai_suggestions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE ai_suggestions FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE ai_suggestions FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE ai_suggestions FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE ai_suggestions;
ALTER TABLE ai_settings DROP COLUMN allow_send;

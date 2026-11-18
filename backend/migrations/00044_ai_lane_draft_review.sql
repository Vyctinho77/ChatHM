-- +goose Up
-- Rascunhos da raia em sombra: quem revisa pode enviar (inclusive editado) ou descartar. Guardamos o agente que escreveu (para a mensagem sair
-- com o nome e a logo dele), o texto final e a mensagem enviada.
ALTER TABLE ai_lane_drafts
    ADD COLUMN provider   text NOT NULL DEFAULT '',
    ADD COLUMN app        text NOT NULL DEFAULT '',
    ADD COLUMN agent      text NOT NULL DEFAULT '',
    ADD COLUMN final_body text,
    ADD COLUMN message_id uuid;
CREATE INDEX ai_lane_drafts_pending_idx ON ai_lane_drafts (created_at DESC) WHERE outcome IS NULL;

-- +goose Down
DROP INDEX IF EXISTS ai_lane_drafts_pending_idx;
ALTER TABLE ai_lane_drafts DROP COLUMN IF EXISTS message_id, DROP COLUMN IF EXISTS final_body, DROP COLUMN IF EXISTS agent, DROP COLUMN IF EXISTS app, DROP COLUMN IF EXISTS provider;

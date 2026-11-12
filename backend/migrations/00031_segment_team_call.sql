-- +goose Up
-- Conversa em andamento em que o cliente passa a falar do assunto de outra equipe: a equipe do segmento entra como participante
-- (sem tirar a conversa de quem atende). O segmento pode desligar isso, e cada assunto chama a equipe no máximo uma vez por conversa.
ALTER TABLE segments ADD COLUMN auto_include boolean NOT NULL DEFAULT true;

CREATE TABLE conversation_segment_calls (
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    segment_id      uuid NOT NULL REFERENCES segments (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, segment_id)
);

-- Só o backend acessa.
ALTER TABLE conversation_segment_calls ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE conversation_segment_calls FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE conversation_segment_calls FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE conversation_segment_calls FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE conversation_segment_calls;
ALTER TABLE segments DROP COLUMN auto_include;

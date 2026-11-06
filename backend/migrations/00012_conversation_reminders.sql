-- +goose Up
CREATE TABLE conversation_reminders (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    text text NOT NULL CHECK (length(btrim(text)) BETWEEN 1 AND 240),
    color text NOT NULL DEFAULT 'olive' CHECK (color IN ('olive', 'amber', 'sky', 'rose', 'violet', 'neutral')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, conversation_id)
);
CREATE INDEX conversation_reminders_conversation_idx ON conversation_reminders(conversation_id);

-- Only the backend accesses personal notes, with its authenticated viewer.
ALTER TABLE conversation_reminders ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE conversation_reminders FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE conversation_reminders FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE conversation_reminders FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE conversation_reminders;

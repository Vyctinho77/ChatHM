-- +goose Up
ALTER TABLE messages DROP CONSTRAINT messages_kind_check;
ALTER TABLE messages ADD CONSTRAINT messages_kind_check
    CHECK (kind IN ('text', 'audio', 'image', 'document', 'location', 'sticker', 'unsupported'));

-- +goose Down
UPDATE messages SET kind = 'unsupported', payload = payload || '{"label":"Figurinha"}'::jsonb
WHERE kind = 'sticker';
ALTER TABLE messages DROP CONSTRAINT messages_kind_check;
ALTER TABLE messages ADD CONSTRAINT messages_kind_check
    CHECK (kind IN ('text', 'audio', 'image', 'document', 'location', 'unsupported'));

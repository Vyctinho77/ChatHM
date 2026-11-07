-- +goose Up
-- Nome público não significa que a pessoa está salva na agenda do escritório.
ALTER TABLE contacts ADD COLUMN whatsapp_saved boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE contacts DROP COLUMN whatsapp_saved;

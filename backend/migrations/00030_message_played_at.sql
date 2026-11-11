-- +goose Up
-- Mensagem de voz enviada pelo escritório e ouvida pelo cliente (recibo "played" do WhatsApp), separado do recibo de leitura.
ALTER TABLE messages ADD COLUMN played_at timestamptz;

-- +goose Down
ALTER TABLE messages DROP COLUMN played_at;

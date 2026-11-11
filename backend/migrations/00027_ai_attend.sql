-- +goose Up
-- Atendente autônomo: a IA envia e responde como um atendente (escopo "attend"). Chave mestra do administrador, desligada por padrão.
ALTER TABLE ai_settings ADD COLUMN allow_attend boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE ai_settings DROP COLUMN allow_attend;

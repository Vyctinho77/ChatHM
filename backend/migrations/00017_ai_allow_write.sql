-- +goose Up

-- O administrador precisa liberar explicitamente que a IA faça alterações (padrão: só leitura).
ALTER TABLE ai_settings ADD COLUMN allow_write boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE ai_settings DROP COLUMN allow_write;

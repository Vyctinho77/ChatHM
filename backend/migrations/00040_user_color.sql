-- +goose Up
-- Cor do nome de cada pessoa da equipe (uma chave da paleta: verde, azul, roxo...). Vazio = automática (a tela escolhe uma cor estável a partir do id).
-- A chave, e não o código da cor, para o app ajustar o tom ao tema claro/escuro sem perder o contraste.
ALTER TABLE users ADD COLUMN color text NOT NULL DEFAULT '' CHECK (color ~ '^[a-z]{0,16}$');

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS color;

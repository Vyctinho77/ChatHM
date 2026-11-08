-- +goose Up
-- Aparência do chat por pessoa: intensidade do papel de parede e cores dos balões ({"wallpaper":40,"out":"#5f6e24","in":"#ffffff"}).
ALTER TABLE users ADD COLUMN chat_appearance jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE users DROP COLUMN chat_appearance;

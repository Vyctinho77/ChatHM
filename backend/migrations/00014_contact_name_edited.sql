-- +goose Up

-- Nome definido pela equipe: o assunto atual do grupo no WhatsApp não o sobrescreve mais.
ALTER TABLE contacts ADD COLUMN name_edited boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE contacts DROP COLUMN name_edited;

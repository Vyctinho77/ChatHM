-- +goose Up
-- Perfil "gestor" (manager): supervisiona as equipes de que participa.
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'manager', 'agent'));

-- +goose Down
UPDATE users SET role = 'agent' WHERE role = 'manager';
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'agent'));

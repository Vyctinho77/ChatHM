-- +goose Up
CREATE TABLE agenda_items (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 kind text NOT NULL CHECK (kind IN ('appointment','deadline')),
 title text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
 scheduled_date date NOT NULL,
 scheduled_time time,
 team_id uuid NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
 conversation_id uuid REFERENCES conversations(id) ON DELETE SET NULL,
 note text NOT NULL DEFAULT '' CHECK (length(note)<=2000),
 done boolean NOT NULL DEFAULT false,
 created_by uuid REFERENCES users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (kind='appointment' OR scheduled_time IS NULL)
);
CREATE INDEX agenda_items_team_date_idx ON agenda_items(team_id,scheduled_date);
CREATE INDEX agenda_items_conversation_idx ON agenda_items(conversation_id);
CREATE INDEX agenda_items_creator_idx ON agenda_items(created_by);
CREATE TRIGGER agenda_items_updated_at BEFORE UPDATE ON agenda_items
 FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE agenda_items ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE agenda_items FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='anon') THEN
  REVOKE ALL ON TABLE agenda_items FROM anon;
 END IF;
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN
  REVOKE ALL ON TABLE agenda_items FROM authenticated;
 END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE agenda_items;

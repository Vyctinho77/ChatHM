-- +goose Up

-- Segmentos (filas). Um segmento pode estar vinculado a uma equipe; sem equipe, a fila
-- fica visível a todos (fila "não roteada"). Conversas sem segmento estão na fila geral.
CREATE TABLE segments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key        text NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9_]{1,40}$'),
    name       citext NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 80),
    -- RESTRICT: excluir uma equipe não pode tornar a fila visível a todos sem querer.
    team_id    uuid REFERENCES teams (id) ON DELETE RESTRICT,
    is_active  boolean NOT NULL DEFAULT true,
    position   integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER segments_updated_at BEFORE UPDATE ON segments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX segments_team_idx ON segments (team_id);

-- Palavras e expressões configuráveis. `normalized` guarda a frase já normalizada
-- (sem acentos, sem caixa, sem palavras de ligação, plural simples) — gerada pelo Go.
CREATE TABLE segment_keywords (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    segment_id uuid NOT NULL REFERENCES segments (id) ON DELETE CASCADE,
    phrase     text NOT NULL CHECK (length(phrase) BETWEEN 1 AND 120),
    normalized text NOT NULL CHECK (length(normalized) > 0),
    priority   integer NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 100),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (segment_id, normalized)
);

-- Marca de sementes já aplicadas (evita recriar segmentos padrão apagados de propósito).
CREATE TABLE seed_flags (
    name       text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE conversations
    ADD COLUMN segment_id     uuid REFERENCES segments (id) ON DELETE SET NULL,
    ADD COLUMN classification text NOT NULL DEFAULT 'none' CHECK (classification IN ('none', 'keyword', 'conflict')),
    ADD COLUMN classified_at  timestamptz;
CREATE INDEX conversations_segment_idx ON conversations (segment_id) WHERE status <> 'closed';
CREATE INDEX conversations_assignee_idx ON conversations (assignee_id) WHERE status <> 'closed';

-- Histórico de atendimentos. O índice único parcial garante, no próprio banco, que
-- uma conversa nunca tenha dois responsáveis ativos ao mesmo tempo.
CREATE TABLE conversation_assignments (
    id              bigserial PRIMARY KEY,
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users (id),
    team_id         uuid REFERENCES teams (id) ON DELETE SET NULL,
    segment_id      uuid REFERENCES segments (id) ON DELETE SET NULL,
    assigned_at     timestamptz NOT NULL DEFAULT now(),
    ended_at        timestamptz,
    ended_reason    text CHECK (ended_reason IN ('closed'))
);
CREATE UNIQUE INDEX conversation_assignments_one_active
    ON conversation_assignments (conversation_id) WHERE ended_at IS NULL;
CREATE INDEX conversation_assignments_user_idx ON conversation_assignments (user_id, assigned_at);

-- Envio e recibos.
ALTER TABLE messages
    ADD COLUMN sender_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN delivered_at   timestamptz,
    ADD COLUMN read_at        timestamptz,
    ADD COLUMN error          text NOT NULL DEFAULT '';
CREATE INDEX messages_external_out_idx ON messages (external_id) WHERE direction = 'out';

-- Etiquetas simples: não alteram o segmento.
CREATE TABLE tags (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       citext NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 30),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE conversation_tags (
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    tag_id          uuid NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, tag_id)
);
CREATE INDEX conversation_tags_tag_idx ON conversation_tags (tag_id);

-- +goose Down
DROP TABLE conversation_tags;
DROP TABLE tags;
DROP INDEX messages_external_out_idx;
ALTER TABLE messages DROP COLUMN error, DROP COLUMN read_at, DROP COLUMN delivered_at, DROP COLUMN sender_user_id;
DROP TABLE conversation_assignments;
DROP INDEX conversations_assignee_idx;
DROP INDEX conversations_segment_idx;
ALTER TABLE conversations DROP COLUMN classified_at, DROP COLUMN classification, DROP COLUMN segment_id;
DROP TABLE seed_flags;
DROP TABLE segment_keywords;
DROP TABLE segments;

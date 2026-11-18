-- +goose Up
-- Medição do corretor de acentos e de erros de digitação do campo de mensagem (docs/corretor.md): por camada do corretor e por par
-- "palavra digitada -> palavra corrigida", quantas vezes a correção foi aplicada e quantas foi desfeita (Backspace).
-- NÃO guarda texto de mensagem, frase, conversa, cliente nem quem digitou: só palavras soltas (letras, até 40) e contadores.
CREATE TABLE writing_stats (
    layer      text   NOT NULL CHECK (layer IN ('curated', 'extended', 'suffix', 'typo_tr', 'typo_ins', 'typo_dup', 'typo_key')),
    from_word  text   NOT NULL CHECK (char_length(from_word) BETWEEN 1 AND 40),
    to_word    text   NOT NULL CHECK (char_length(to_word) BETWEEN 1 AND 40),
    applied    bigint NOT NULL DEFAULT 0 CHECK (applied >= 0),
    undone     bigint NOT NULL DEFAULT 0 CHECK (undone >= 0),
    first_seen timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (layer, from_word, to_word)
);
ALTER TABLE writing_stats ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS writing_stats;

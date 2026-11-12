-- +goose Up
-- Estado de cada pessoa sobre uma conversa: o que ela já leu e o que adiou só para si.
-- Sem linha, a pessoa enxerga o estado geral da conversa (contador e adiamento de conversations).
CREATE TABLE conversation_user_state (
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    read_through    timestamptz NOT NULL DEFAULT now(), -- mensagens do cliente até aqui já foram lidas por esta pessoa
    snoozed_until   timestamptz,                        -- adiamento só desta pessoa
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX conversation_user_state_snooze_due_idx ON conversation_user_state (snoozed_until)
    WHERE snoozed_until IS NOT NULL;

-- +goose Down
DROP TABLE conversation_user_state;

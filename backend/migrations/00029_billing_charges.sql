-- +goose Up
-- Cobranças para os envios automáticos (docs/mcp-automacao.md, seção 14): o link de pagamento nunca é escrito pela IA;
-- ela aponta uma cobrança (charge_id) e o servidor insere o link verdadeiro. As cobranças vêm de um gateway (Asaas, ...) ou de importação.

-- Conexão com cada gateway. A chave de API fica cifrada (secretbox, ENCRYPTION_KEY) e nunca volta para a tela nem para a IA.
CREATE TABLE billing_gateways (
    provider     text PRIMARY KEY CHECK (provider ~ '^[a-z_]{2,20}$'),
    enabled      boolean NOT NULL DEFAULT false,
    sandbox      boolean NOT NULL DEFAULT false,
    config_enc   bytea,
    last_sync_at timestamptz,
    last_attempt_at timestamptz,
    last_error   text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 300),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   uuid REFERENCES users (id) ON DELETE SET NULL
);

CREATE TABLE billing_charges (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider       text NOT NULL CHECK (provider ~ '^[a-z_]{2,20}$'),   -- asaas, ..., ou manual (importação)
    external_id    text NOT NULL CHECK (char_length(external_id) BETWEEN 1 AND 120),
    contact_id     uuid REFERENCES contacts (id) ON DELETE SET NULL,   -- NULL = ainda não vinculada a um cliente
    link_manual    boolean NOT NULL DEFAULT false,                      -- vínculo feito pela equipe: a sincronização não troca
    customer_ref   text NOT NULL DEFAULT '',
    customer_name  text NOT NULL DEFAULT '',
    customer_phone text NOT NULL DEFAULT '',                            -- só dígitos
    description    text NOT NULL DEFAULT '' CHECK (char_length(description) <= 300),
    amount_cents   bigint NOT NULL CHECK (amount_cents > 0),
    due_date       date NOT NULL,
    status         text NOT NULL CHECK (status IN ('open', 'overdue', 'paid', 'cancelled')),
    payment_url    text NOT NULL DEFAULT '',
    billing_type   text NOT NULL DEFAULT '',
    synced_at      timestamptz NOT NULL DEFAULT now(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, external_id)
);
CREATE INDEX billing_charges_contact_idx ON billing_charges (contact_id, due_date DESC);
CREATE INDEX billing_charges_open_idx ON billing_charges (status, due_date) WHERE status IN ('open', 'overdue');

ALTER TABLE automation_sends ADD COLUMN charge_id uuid REFERENCES billing_charges (id) ON DELETE SET NULL;
-- Valor e vencimento que o administrador viu ao aprovar: se mudarem antes de sair, o envio é pulado e precisa de nova aprovação.
ALTER TABLE automation_sends ADD COLUMN charge_amount_cents bigint;
ALTER TABLE automation_sends ADD COLUMN charge_due_date date;
CREATE INDEX automation_sends_charge_idx ON automation_sends (charge_id) WHERE charge_id IS NOT NULL;

-- Só o backend acessa.
ALTER TABLE billing_gateways ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing_charges ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE billing_gateways, billing_charges FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON TABLE billing_gateways, billing_charges FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON TABLE billing_gateways, billing_charges FROM authenticated;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE automation_sends DROP COLUMN charge_due_date;
ALTER TABLE automation_sends DROP COLUMN charge_amount_cents;
ALTER TABLE automation_sends DROP COLUMN charge_id;
DROP TABLE billing_charges;
DROP TABLE billing_gateways;

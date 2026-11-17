package httpapi

import (
	"context"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func scalar[T any](t *testing.T, w *world, q string, args ...any) T {
	t.Helper()
	var v T
	if err := w.e.pool.QueryRow(context.Background(), q, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return v
}

// A mesma mensagem entregue pelo número e depois pelo @lid (o histórico sincronizado traz um, o ao vivo o outro) entra uma vez só.
func TestSameMessageThroughPhoneAndLIDIsStoredOnce(t *testing.T) {
	w := setupWorld(t)
	p := itoa(int(phoneSeq.Add(1)))
	live := inbound("DUP-"+p, p, "Cliente Dois Enderecos", "folha de pagamento")
	w.e.wa.Receive(live)

	again := live // mesmo ID, agora pelo endereço oculto
	again.ChatJID = "98765432109876@lid"
	again.Historical = true
	w.e.wa.Receive(again)
	eventually(t, "mensagem gravada", func() bool {
		return scalar[int](t, w, `SELECT count(*) FROM messages WHERE external_id = $1`, "DUP-"+p) >= 1
	})
	if n := scalar[int](t, w, `SELECT count(*) FROM messages WHERE external_id = $1`, "DUP-"+p); n != 1 {
		t.Fatalf("a mesma mensagem foi gravada %d vezes", n)
	}
	if n := scalar[int](t, w, `SELECT count(*) FROM contacts WHERE name = 'Cliente Dois Enderecos'`); n != 1 {
		t.Fatalf("o contato foi criado %d vezes", n)
	}

	// Mensagem diferente no mesmo chat continua entrando; e o mesmo ID em GRUPO diferente não é tratado como cópia de chat individual.
	other := inbound("OUTRA-"+p, p, "Cliente Dois Enderecos", "segunda mensagem")
	w.e.wa.Receive(other)
	eventually(t, "segunda mensagem", func() bool {
		return scalar[int](t, w, `SELECT count(*) FROM messages WHERE external_id = $1`, "OUTRA-"+p) == 1
	})
}

// Dados que já estavam repetidos: a prévia só conta; aplicar une o contato @lid ao do número, deixa uma cópia de cada mensagem e
// apaga os arquivos das cópias.
func TestDedupeCleansDuplicatesUnifiesContactsAndRemovesFiles(t *testing.T) {
	w := setupWorld(t)
	ctx := context.Background()
	phone := itoa(int(phoneSeq.Add(1)))
	lid := "55501234567890"

	// Contato do número, com uma imagem recebida (arquivo A).
	img := testPNG(32, 32)
	mk := func(id, chat, wa, ph, name string) {
		m := inbound(id, phone, name, "")
		m.ChatJID, m.ContactWAID, m.ContactPhone = chat, wa, ph
		m.Kind = whatsapp.KindImage
		m.Payload = map[string]any{"mimeType": "image/png"}
		m.Download = func(context.Context) ([]byte, error) { return append(append([]byte{}, img...), []byte(id)...), nil }
		w.e.wa.Receive(m)
		eventually(t, "anexo "+id, func() bool {
			return scalar[int](t, w, `SELECT count(*) FROM attachments a JOIN messages m ON m.id = a.message_id WHERE m.external_id = $1`, id) == 1
		})
	}
	mk("IMG-PN", phone+"@s.whatsapp.net", phone, "+"+phone, "Cliente Real")
	mk("IMG-LID", lid+"@lid", lid+"@lid", "", "Cliente Real") // o mesmo cliente, pelo @lid (outro contato)
	// Texto só do contato @lid (não repetido) e o repetido: o ID do arquivo vira o mesmo da imagem do número.
	w.e.wa.Receive(func() whatsapp.InboundMessage {
		m := inbound("SO-LID", phone, "Cliente Real", "mensagem só no lid")
		m.ChatJID, m.ContactWAID, m.ContactPhone = lid+"@lid", lid+"@lid", ""
		return m
	}())
	eventually(t, "texto no lid", func() bool {
		return scalar[int](t, w, `SELECT count(*) FROM messages WHERE external_id = 'SO-LID'`) == 1
	})
	// Reproduz o defeito antigo: a imagem do @lid passa a ter o MESMO ID da imagem do número.
	if _, err := w.e.pool.Exec(ctx, `UPDATE messages SET external_id = 'IMG-PN' WHERE external_id = 'IMG-LID'`); err != nil {
		t.Fatal(err)
	}
	// A correspondência @lid → número, que o WhatsApp mantém.
	if _, err := w.e.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS whatsmeow_lid_map (lid text PRIMARY KEY, pn text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = w.e.pool.Exec(context.Background(), `DROP TABLE IF EXISTS whatsmeow_lid_map`) })
	if _, err := w.e.pool.Exec(ctx, `INSERT INTO whatsmeow_lid_map (lid, pn) VALUES ($1, $2)`, lid, phone); err != nil {
		t.Fatal(err)
	}
	// O contato @lid só tem conversa fechada (como em produção); o do número também, para não haver duas abertas.
	_, _ = w.e.pool.Exec(ctx, `UPDATE conversations SET status = 'closed'`)

	filesBefore := countFiles(t, w.e.storageDir)
	contactsBefore := scalar[int](t, w, `SELECT count(*) FROM contacts WHERE name = 'Cliente Real'`)
	if contactsBefore != 2 {
		t.Fatalf("preparação: esperava 2 contatos, veio %d", contactsBefore)
	}

	// Prévia: conta e não grava.
	adm := w.admin
	r := adm.do("GET", "/api/v1/maintenance/duplicates", nil)
	expect(t, r, 200)
	if r.body["applied"] != false || r.body["contactsMerged"] != float64(1) || r.body["duplicateMessages"] != float64(1) || r.body["filesRemoved"] != float64(1) {
		t.Fatalf("prévia: %v", r.body)
	}
	if scalar[int](t, w, `SELECT count(*) FROM contacts WHERE name = 'Cliente Real'`) != 2 || scalar[int](t, w, `SELECT count(*) FROM messages WHERE external_id = 'IMG-PN'`) != 2 || countFiles(t, w.e.storageDir) != filesBefore {
		t.Fatal("a prévia não pode gravar nada")
	}

	// Aplicar exige a confirmação digitada.
	expect(t, adm.do("POST", "/api/v1/maintenance/duplicates", map[string]any{"confirm": "sim"}), 422)
	r = adm.do("POST", "/api/v1/maintenance/duplicates", map[string]any{"confirm": "CORRIGIR"})
	expect(t, r, 200)
	if r.body["applied"] != true || r.body["contactsMerged"] != float64(1) || r.body["duplicateMessages"] != float64(1) || r.body["filesRemoved"] != float64(1) {
		t.Fatalf("aplicada: %v", r.body)
	}
	if n := scalar[int](t, w, `SELECT count(*) FROM contacts WHERE name = 'Cliente Real'`); n != 1 {
		t.Fatalf("contatos depois: %d", n)
	}
	if n := scalar[int](t, w, `SELECT count(*) FROM messages WHERE external_id = 'IMG-PN'`); n != 1 {
		t.Fatalf("cópias da imagem depois: %d", n)
	}
	// A mensagem que só existia pelo @lid continua, agora na mesma pessoa do número.
	if n := scalar[int](t, w, `SELECT count(*) FROM messages m JOIN conversations c ON c.id = m.conversation_id JOIN contacts ct ON ct.id = c.contact_id WHERE m.external_id = 'SO-LID' AND ct.wa_id = $1`, phone); n != 1 {
		t.Fatalf("a mensagem só do @lid deveria ficar com o contato do número: %d", n)
	}
	if got := countFiles(t, w.e.storageDir); got != filesBefore-1 {
		t.Fatalf("arquivos: %d antes, %d depois (esperava um a menos)", filesBefore, got)
	}
	// A imagem que ficou continua abrindo.
	if n := scalar[int](t, w, `SELECT count(*) FROM attachments a JOIN messages m ON m.id = a.message_id WHERE m.external_id = 'IMG-PN'`); n != 1 {
		t.Fatalf("o anexo que ficou: %d", n)
	}

	// Rodar de novo não acha mais nada (idempotente).
	r = adm.do("GET", "/api/v1/maintenance/duplicates", nil)
	expect(t, r, 200)
	if r.body["contactsMerged"] != float64(0) || r.body["duplicateMessages"] != float64(0) || r.body["contactsRenamed"] != float64(0) {
		t.Fatalf("segunda prévia: %v", r.body)
	}
	// Só administrador.
	expect(t, w.ana.do("GET", "/api/v1/maintenance/duplicates", nil), 403)
}

// Contato @lid com número conhecido e sem outro contato: só ganha o número.
func TestDedupeGivesPhoneToLIDOnlyContact(t *testing.T) {
	w := setupWorld(t)
	ctx := context.Background()
	lid, phone := "77701234567890", itoa(int(phoneSeq.Add(1)))
	m := inbound("LID-ONLY", phone, "Cliente Oculto", "folha de pagamento")
	m.ChatJID, m.ContactWAID, m.ContactPhone = lid+"@lid", lid+"@lid", ""
	w.e.wa.Receive(m)
	eventually(t, "contato @lid", func() bool {
		return scalar[int](t, w, `SELECT count(*) FROM contacts WHERE wa_id = $1`, lid+"@lid") == 1
	})
	if _, err := w.e.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS whatsmeow_lid_map (lid text PRIMARY KEY, pn text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = w.e.pool.Exec(context.Background(), `DROP TABLE IF EXISTS whatsmeow_lid_map`) })
	_, _ = w.e.pool.Exec(ctx, `INSERT INTO whatsmeow_lid_map (lid, pn) VALUES ($1, $2)`, lid, phone)

	r := w.admin.do("POST", "/api/v1/maintenance/duplicates", map[string]any{"confirm": "CORRIGIR"})
	expect(t, r, 200)
	if r.body["contactsRenamed"] != float64(1) {
		t.Fatalf("resultado: %v", r.body)
	}
	if scalar[string](t, w, `SELECT phone FROM contacts WHERE wa_id = $1`, phone) != "+"+phone {
		t.Fatal("o contato deveria ter ganhado o telefone")
	}
}

// Um contato @lid que já tem número conhecido no WhatsApp passa a entrar pelo número já na chegada da mensagem.
func TestInboundFromLIDUsesKnownPhoneContact(t *testing.T) {
	w := setupWorld(t)
	ctx := context.Background()
	lid, phone := "88801234567890", itoa(int(phoneSeq.Add(1)))
	if _, err := w.e.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS whatsmeow_lid_map (lid text PRIMARY KEY, pn text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = w.e.pool.Exec(context.Background(), `DROP TABLE IF EXISTS whatsmeow_lid_map`) })
	_, _ = w.e.pool.Exec(ctx, `INSERT INTO whatsmeow_lid_map (lid, pn) VALUES ($1, $2)`, lid, phone)
	w.receiveFrom(phone, "Cliente Conhecido", "primeira, pelo número")
	m := inbound("VIA-LID", phone, "Cliente Conhecido", "segunda, pelo lid")
	m.ChatJID, m.ContactWAID, m.ContactPhone = lid+"@lid", lid+"@lid", ""
	w.e.wa.Receive(m)
	eventually(t, "segunda mensagem", func() bool {
		return scalar[int](t, w, `SELECT count(*) FROM messages WHERE external_id = 'VIA-LID'`) == 1
	})
	if n := scalar[int](t, w, `SELECT count(*) FROM contacts WHERE name = 'Cliente Conhecido'`); n != 1 {
		t.Fatalf("a pessoa virou %d contatos", n)
	}
}

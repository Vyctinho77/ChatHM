package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// histMedia monta uma mensagem de mídia do HISTÓRICO: sem download na hora, só com a referência (aqui, o ID externo).
func histMedia(id, phone, name string, kind whatsapp.Kind, data []byte, age time.Duration) whatsapp.InboundMessage {
	m := inbound(id, phone, name, "")
	m.Historical = true
	m.Timestamp = time.Now().Add(-age)
	m.Kind = kind
	m.Payload = map[string]any{"sizeBytes": uint64(len(data)), "historical": true}
	switch kind {
	case whatsapp.KindImage:
		m.Payload["mimeType"] = "image/png"
	case whatsapp.KindAudio:
		m.Payload["mimeType"] = "audio/ogg"
		m.Payload["durationSec"] = uint32(1)
	case whatsapp.KindDocument:
		m.Payload["mimeType"], m.Payload["fileName"] = "application/pdf", "guia.pdf"
	}
	m.MediaRef = &whatsapp.MediaRef{Kind: kind, Proto: []byte(id)}
	return m
}

type refState struct {
	status, state, lastErr string
	attempts               int
	next                   time.Time
}

func refOf(t *testing.T, e *env, ext string) refState {
	t.Helper()
	var r refState
	err := e.pool.QueryRow(t.Context(), `
		SELECT r.status, coalesce(m.payload->>'mediaState', ''), r.last_error, r.attempts, r.next_attempt_at
		  FROM media_refs r JOIN messages m ON m.id = r.message_id WHERE m.external_id = $1`, ext).Scan(&r.status, &r.state, &r.lastErr, &r.attempts, &r.next)
	if err != nil {
		t.Fatalf("referência de %s: %v", ext, err)
	}
	return r
}

func chatOf(phone string) string { return phone + "@s.whatsapp.net" }

// O histórico entra sem arquivo; o importador baixa, devagar, do mais recente para o mais antigo.
func TestHistoryMediaIsImportedInTheBackground(t *testing.T) {
	e := setupWorld(t).e // com o WhatsApp conectado: o importador só trabalha conectado
	ctx := t.Context()
	png, wav, pdf := testPNG(40, 30), testWAV(), []byte("%PDF-1.4 historico")
	phone := itoa(int(phoneSeq.Add(1)))
	for _, c := range []struct {
		id   string
		kind whatsapp.Kind
		data []byte
		age  time.Duration
	}{{"H-velho-pdf", whatsapp.KindDocument, pdf, 90 * 24 * time.Hour}, {"H-meio-img", whatsapp.KindImage, png, 40 * 24 * time.Hour}, {"H-novo-audio", whatsapp.KindAudio, wav, 2 * 24 * time.Hour}} {
		e.wa.PutMedia(c.id, c.data)
		if created, err := e.svcs.Conversations.Ingest(ctx, histMedia(c.id, phone, "Cliente Historico", c.kind, c.data, c.age)); err != nil || !created {
			t.Fatalf("%s: %v %v", c.id, created, err)
		}
		if attachmentCount(t, e, c.id) != 0 || refOf(t, e, c.id).status != "pending" {
			t.Fatalf("%s: o histórico entra sem arquivo e com a referência pendente", c.id)
		}
	}
	// Uma rodada baixa UM arquivo, o mais recente primeiro.
	e.svcs.Conversations.ImportMediaOnce(ctx)
	if attachmentCount(t, e, "H-novo-audio") != 1 || attachmentCount(t, e, "H-meio-img") != 0 {
		t.Fatal("a primeira rodada deveria baixar só o mais recente (o áudio)")
	}
	e.svcs.Conversations.ImportMediaOnce(ctx)
	e.svcs.Conversations.ImportMediaOnce(ctx)
	for _, id := range []string{"H-novo-audio", "H-meio-img", "H-velho-pdf"} {
		if attachmentCount(t, e, id) != 1 {
			t.Fatalf("%s deveria ter o arquivo", id)
		}
		if r := refOf(t, e, id); r.status != "done" || r.state != "" {
			t.Fatalf("%s: %+v", id, r)
		}
	}
	// A referência (com a chave do arquivo) fica cifrada no banco: o conteúdo guardado não é o ID em texto puro.
	var enc []byte
	if err := e.pool.QueryRow(ctx, `SELECT r.ref_enc FROM media_refs r JOIN messages m ON m.id = r.message_id WHERE m.external_id = 'H-meio-img'`).Scan(&enc); err != nil || string(enc) == "H-meio-img" || len(enc) < 20 {
		t.Fatalf("a referência deveria estar cifrada: %v", err)
	}
	// Nada novo a baixar: a rodada não faz nada.
	e.svcs.Conversations.ImportMediaOnce(ctx)

	// Teto de espaço: acima dele o arquivo fica só como metadados.
	e.svcs.Conversations.SetHistoryMediaBudget(1)
	e.wa.PutMedia("H-sem-espaco", pdf)
	if _, err := e.svcs.Conversations.Ingest(ctx, histMedia("H-sem-espaco", phone, "Cliente Historico", whatsapp.KindDocument, pdf, time.Hour)); err != nil {
		t.Fatal(err)
	}
	e.svcs.Conversations.ImportMediaOnce(ctx)
	if r := refOf(t, e, "H-sem-espaco"); r.status != "skipped" || attachmentCount(t, e, "H-sem-espaco") != 0 {
		t.Fatalf("acima do teto: %+v", r)
	}
	// Arquivo grande demais para o tipo.
	e.svcs.Conversations.SetHistoryMediaBudget(2 << 30)
	big := histMedia("H-gigante", phone, "Cliente Historico", whatsapp.KindImage, png, time.Hour)
	big.Payload["sizeBytes"] = uint64(50 << 20)
	if _, err := e.svcs.Conversations.Ingest(ctx, big); err != nil {
		t.Fatal(err)
	}
	e.svcs.Conversations.ImportMediaOnce(ctx)
	if r := refOf(t, e, "H-gigante"); r.status != "skipped" {
		t.Fatalf("arquivo enorme: %+v", r)
	}
}

// O WhatsApp já apagou o arquivo do servidor: o importador pede ao CELULAR que o envie de novo, um pedido por mensagem.
func TestExpiredHistoryMediaIsRequestedFromThePhone(t *testing.T) {
	e := setupWorld(t).e // com o WhatsApp conectado: o importador só trabalha conectado
	ctx := t.Context()
	png := testPNG(40, 30)
	phone := itoa(int(phoneSeq.Add(1)))
	ingest := func(id string, age time.Duration) {
		t.Helper()
		if _, err := e.svcs.Conversations.Ingest(ctx, histMedia(id, phone, "Cliente Reenvio", whatsapp.KindImage, png, age)); err != nil {
			t.Fatal(err)
		}
	}

	// 1) Celular tem o arquivo: pedido, espera, resposta, arquivo guardado.
	e.wa.PhoneHasMedia("R-ok", png)
	ingest("R-ok", 10*24*time.Hour)
	e.svcs.Conversations.ImportMediaOnce(ctx)
	reqs := e.wa.MediaRetryRequests()
	if len(reqs) != 1 || reqs[0].ExternalID != "R-ok" || reqs[0].ChatJID != chatOf(phone) || reqs[0].FromMe {
		t.Fatalf("pedido de reenvio: %+v", reqs)
	}
	if r := refOf(t, e, "R-ok"); r.status != "retry_requested" || r.state != "retrying" {
		t.Fatalf("esperando o celular: %+v", r)
	}
	e.svcs.Conversations.ImportMediaOnce(ctx) // não repete o pedido
	if len(e.wa.MediaRetryRequests()) != 1 {
		t.Fatal("um pedido só por mensagem")
	}
	e.wa.RespondMediaRetry("R-ok", chatOf(phone))
	if attachmentCount(t, e, "R-ok") != 1 {
		t.Fatal("o arquivo reenviado pelo celular deveria ser guardado")
	}
	if r := refOf(t, e, "R-ok"); r.status != "done" || r.state != "" {
		t.Fatalf("depois do reenvio: %+v", r)
	}

	// 2) Celular também não tem mais: indisponível, marcado como expirado.
	ingest("R-sem", 9*24*time.Hour)
	e.svcs.Conversations.ImportMediaOnce(ctx)
	e.wa.RespondMediaRetry("R-sem", chatOf(phone))
	if r := refOf(t, e, "R-sem"); r.status != "unavailable" || r.state != "expired" || attachmentCount(t, e, "R-sem") != 0 {
		t.Fatalf("celular sem o arquivo: %+v", r)
	}

	// 3) Resposta de algo que não pedimos é ignorada.
	e.wa.RespondMediaRetry("NUNCA-PEDI", chatOf(phone))

	// 4) O celular nunca responde: em 24 horas vira indisponível.
	ingest("R-mudo", 8*24*time.Hour)
	e.svcs.Conversations.ImportMediaOnce(ctx)
	if refOf(t, e, "R-mudo").status != "retry_requested" {
		t.Fatal("deveria estar esperando")
	}
	if _, err := e.pool.Exec(ctx, `UPDATE media_refs SET retry_requested_at = now() - interval '25 hours' WHERE message_id = (SELECT id FROM messages WHERE external_id = 'R-mudo')`); err != nil {
		t.Fatal(err)
	}
	e.svcs.Conversations.ImportMediaOnce(ctx)
	if r := refOf(t, e, "R-mudo"); r.status != "unavailable" || r.state != "expired" {
		t.Fatalf("sem resposta em 24 h: %+v", r)
	}

	// 5) O pedido falha (celular sem conexão): tenta de novo mais tarde, sem gastar o pedido; depois de várias, desiste.
	e.wa.FailMediaRetry(errors.New("sem conexão"))
	ingest("R-falha", 7*24*time.Hour)
	e.svcs.Conversations.ImportMediaOnce(ctx)
	r := refOf(t, e, "R-falha")
	if r.status != "pending" || r.attempts != 1 || !r.next.After(time.Now().Add(4*time.Minute)) || r.lastErr == "" {
		t.Fatalf("pedido que falhou: %+v", r)
	}
	for i := 0; i < 4; i++ {
		_, _ = e.pool.Exec(ctx, `UPDATE media_refs SET next_attempt_at = now() WHERE message_id = (SELECT id FROM messages WHERE external_id = 'R-falha') AND status = 'pending'`)
		e.svcs.Conversations.ImportMediaOnce(ctx)
	}
	if r := refOf(t, e, "R-falha"); r.status != "unavailable" || r.state != "expired" {
		t.Fatalf("depois de várias falhas: %+v", r)
	}
}

// Nova sincronização do histórico: as mensagens antigas (sem arquivo e sem referência) ganham a referência e o arquivo é baixado.
func TestRedeliveredHistoryGivesReferencesToOldMessages(t *testing.T) {
	e := setupWorld(t).e // com o WhatsApp conectado: o importador só trabalha conectado
	ctx := t.Context()
	png := testPNG(40, 30)
	phone := itoa(int(phoneSeq.Add(1)))
	old := histMedia("S-antiga", phone, "Cliente Sync", whatsapp.KindImage, png, 70*24*time.Hour)
	old.MediaRef = nil // como as 8,7 mil mídias de antes: entraram sem referência
	if _, err := e.svcs.Conversations.Ingest(ctx, old); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM media_refs`).Scan(&n)
	if n != 0 {
		t.Fatal("sem referência não há o que guardar")
	}
	// O administrador pede o histórico de novo; agora vem a referência.
	e.wa.PutMedia("S-antiga", png)
	if created, err := e.svcs.Conversations.Ingest(ctx, histMedia("S-antiga", phone, "Cliente Sync", whatsapp.KindImage, png, 70*24*time.Hour)); err != nil || created {
		t.Fatalf("mensagem repetida: %v %v", created, err)
	}
	if refOf(t, e, "S-antiga").status != "pending" {
		t.Fatal("a mensagem antiga deveria ganhar a referência")
	}
	e.svcs.Conversations.ImportMediaOnce(ctx)
	if attachmentCount(t, e, "S-antiga") != 1 {
		t.Fatal("o arquivo deveria ser baixado")
	}
	// Entregar de novo uma mensagem que já tem arquivo não cria referência nenhuma.
	_, _ = e.svcs.Conversations.Ingest(ctx, histMedia("S-antiga", phone, "Cliente Sync", whatsapp.KindImage, png, 70*24*time.Hour))
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM media_refs`).Scan(&n)
	if n != 1 {
		t.Fatalf("referências = %d", n)
	}
}

// Mídia recebida ao vivo cujo download falhou: a referência fica para tentar de novo, e a mensagem ganha o arquivo quando der.
func TestLiveMediaThatFailedToDownloadIsRetriedLater(t *testing.T) {
	e := setupWorld(t).e // com o WhatsApp conectado: o importador só trabalha conectado
	ctx := t.Context()
	png := testPNG(40, 30)
	phone := itoa(int(phoneSeq.Add(1)))
	m := inbound("L-falhou", phone, "Cliente Ao Vivo", "")
	m.Kind = whatsapp.KindImage
	m.Payload = map[string]any{"mimeType": "image/png", "sizeBytes": uint64(len(png))}
	m.Download = func(context.Context) ([]byte, error) { return nil, errors.New("sem rede") }
	m.MediaRef = &whatsapp.MediaRef{Kind: whatsapp.KindImage, Proto: []byte("L-falhou")}
	if _, err := e.svcs.Conversations.Ingest(ctx, m); err != nil {
		t.Fatal(err)
	}
	eventually(t, "estado de falha", func() bool {
		var n int
		_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM media_refs r JOIN messages m ON m.id = r.message_id WHERE m.external_id = 'L-falhou'`).Scan(&n)
		return n == 1 && refOf(t, e, "L-falhou").state == "failed"
	})
	if r := refOf(t, e, "L-falhou"); r.status != "pending" || !r.next.After(time.Now().Add(3*time.Minute)) {
		t.Fatalf("deveria tentar de novo só daqui a alguns minutos: %+v", r)
	}
	e.svcs.Conversations.ImportMediaOnce(ctx) // ainda não é hora
	if attachmentCount(t, e, "L-falhou") != 0 {
		t.Fatal("antes da hora não tenta")
	}
	e.wa.PutMedia("L-falhou", png)
	_, _ = e.pool.Exec(ctx, `UPDATE media_refs SET next_attempt_at = now()`)
	e.svcs.Conversations.ImportMediaOnce(ctx)
	if attachmentCount(t, e, "L-falhou") != 1 || refOf(t, e, "L-falhou").state != "" {
		t.Fatalf("o arquivo deveria chegar e o estado de falha sair: %+v", refOf(t, e, "L-falhou"))
	}
}

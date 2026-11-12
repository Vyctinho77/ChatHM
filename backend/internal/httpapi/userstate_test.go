package httpapi

// Não lidas e adiamento individuais: cada pessoa tem o seu estado da conversa.

import (
	"testing"
	"time"
)

func unreadOf(t *testing.T, c *client, name string) int {
	t.Helper()
	return int(mustConv(t, c, name)["unreadCount"].(float64))
}

func TestIndividualUnread(t *testing.T) {
	w := setupWorld(t)
	id, phone := w.attended(t, "Lidas por pessoa")
	path := "/api/v1/conversations/" + id
	w.receiveFrom(phone, "Lidas por pessoa", "Oi, tem novidade?")
	if a, b := unreadOf(t, w.ana, "Lidas por pessoa"), unreadOf(t, w.admin, "Lidas por pessoa"); a != 1 || b != 1 {
		t.Fatalf("antes de ler: ana=%d admin=%d", a, b)
	}
	// Ana lê: só o contador dela zera.
	expect(t, w.ana.do("POST", path+"/read", nil), 204)
	if a, b := unreadOf(t, w.ana, "Lidas por pessoa"), unreadOf(t, w.admin, "Lidas por pessoa"); a != 0 || b != 1 {
		t.Fatalf("ana leu: ana=%d admin=%d (esperado 0 e 1)", a, b)
	}
	// Chega outra mensagem: para quem já leu conta só a nova; para quem não leu, as duas.
	w.receiveFrom(phone, "Lidas por pessoa", "E então?")
	if a, b := unreadOf(t, w.ana, "Lidas por pessoa"), unreadOf(t, w.admin, "Lidas por pessoa"); a != 1 || b != 2 {
		t.Fatalf("segunda mensagem: ana=%d admin=%d (esperado 1 e 2)", a, b)
	}
	expect(t, w.admin.do("POST", path+"/read", nil), 204)
	if b := unreadOf(t, w.admin, "Lidas por pessoa"); b != 0 {
		t.Fatalf("admin leu: %d", b)
	}
	if a := unreadOf(t, w.ana, "Lidas por pessoa"); a != 1 {
		t.Fatalf("a leitura do admin mexeu no contador da ana: %d", a)
	}
	// Ler de novo, sem nada novo, não muda nada; o detalhe da conversa traz o mesmo número.
	expect(t, w.admin.do("POST", path+"/read", nil), 204)
	r := w.ana.do("GET", path, nil)
	expect(t, r, 200)
	if r.body["unreadCount"].(float64) != 1 {
		t.Fatalf("detalhe da ana: %v", r.body["unreadCount"])
	}
	// Responder atende para todos.
	w.receiveFrom(phone, "Lidas por pessoa", "Ainda aí?")
	expect(t, w.send(w.ana, id, "Já respondo"), 201)
	if a, b := unreadOf(t, w.ana, "Lidas por pessoa"), unreadOf(t, w.admin, "Lidas por pessoa"); a != 0 || b != 0 {
		t.Fatalf("depois de responder: ana=%d admin=%d", a, b)
	}
	// Quem não vê a conversa não consegue marcar como lida.
	expect(t, w.bia.do("POST", path+"/read", nil), 404)
}

func TestIndividualUnreadRealtime(t *testing.T) {
	w := setupWorld(t)
	id, phone := w.attended(t, "Tempo real lidas")
	w.receiveFrom(phone, "Tempo real lidas", "Mensagem nova")
	wsAna, wsAdmin := w.ana.websocket(t), w.admin.websocket(t)
	eventually(t, "websockets conectados", func() bool { return w.e.svcs.Hub.Count() == 2 })
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/read", nil), 204)
	eventually(t, "ana vê zero", func() bool {
		u, ok := wsAna.last("conversation.updated")["unreadCount"].(float64)
		return ok && u == 0
	})
	if wsAdmin.count("conversation.updated") != 0 {
		t.Fatal("a leitura da ana foi anunciada ao admin")
	}
	w.receiveFrom(phone, "Tempo real lidas", "Outra")
	eventually(t, "cada um recebe o seu contador", func() bool {
		a, _ := wsAna.last("conversation.updated")["unreadCount"].(float64)
		b, _ := wsAdmin.last("conversation.updated")["unreadCount"].(float64)
		return a == 1 && b == 2
	})
}

func TestPersonalSnooze(t *testing.T) {
	w := setupWorld(t)
	id, phone := w.attended(t, "Adiar para mim")
	path := "/api/v1/conversations/" + id
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	me := map[string]string{"until": until, "scope": "me"}
	expect(t, w.ana.do("POST", path+"/snooze", map[string]string{"until": until, "scope": "xx"}), 422)
	expect(t, w.bia.do("POST", path+"/snooze", me), 404) // quem não vê a conversa não adia
	// Qualquer pessoa que vê a conversa pode adiar para si, sem ser a responsável.
	expect(t, w.admin.do("POST", path+"/snooze", me), 204)
	c := mustConv(t, w.admin, "Adiar para mim")
	if c["snoozedUntil"] == nil || c["snoozedFor"] != "me" {
		t.Fatalf("admin deveria ver a conversa adiada para si: %v", c)
	}
	c = mustConv(t, w.ana, "Adiar para mim")
	if c["snoozedUntil"] != nil || c["snoozedFor"] != nil {
		t.Fatalf("a ana não adiou: %v", c)
	}
	var global bool
	if err := w.e.pool.QueryRow(t.Context(), `SELECT snoozed_until IS NOT NULL FROM conversations WHERE id=$1`, id).Scan(&global); err != nil || global {
		t.Fatalf("o adiamento individual mexeu na conversa: %v %v", global, err)
	}
	// A ana continua conseguindo responder.
	expect(t, w.send(w.ana, id, "Segue comigo"), 201)
	// O cliente escreve: volta para quem tinha adiado.
	w.receiveFrom(phone, "Adiar para mim", "Voltei")
	if c := mustConv(t, w.admin, "Adiar para mim"); c["snoozedUntil"] != nil {
		t.Fatalf("mensagem do cliente não devolveu a conversa: %v", c)
	}
	// Retomar só para mim.
	expect(t, w.admin.do("POST", path+"/snooze", me), 204)
	expect(t, w.admin.do("POST", path+"/resume", map[string]string{"scope": "me"}), 204)
	if c := mustConv(t, w.admin, "Adiar para mim"); c["snoozedUntil"] != nil {
		t.Fatalf("não retomou: %v", c)
	}
	// O adiamento geral continua só para quem atende (ou gestor) e vale para todos.
	expect(t, w.admin.do("POST", path+"/snooze", me), 204)
	expect(t, w.ana.do("POST", path+"/snooze", map[string]string{"until": until}), 204)
	if c := mustConv(t, w.admin, "Adiar para mim"); c["snoozedFor"] != "all" {
		t.Fatalf("o geral deveria prevalecer: %v", c)
	}
	expect(t, w.ana.do("POST", path+"/resume", nil), 204)
	if c := mustConv(t, w.admin, "Adiar para mim"); c["snoozedFor"] != "me" {
		t.Fatalf("o adiamento do admin deveria continuar: %v", c)
	}
	// Vencido: volta sozinho, só para quem adiou.
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE conversation_user_state SET snoozed_until = now() - interval '1 second' WHERE conversation_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if n, err := w.e.svcs.Conversations.ResumeDue(t.Context()); err != nil || n < 1 {
		t.Fatalf("ResumeDue: %d %v", n, err)
	}
	if c := mustConv(t, w.admin, "Adiar para mim"); c["snoozedUntil"] != nil {
		t.Fatalf("vencido não voltou: %v", c)
	}
	// Encerrar limpa os adiamentos individuais.
	expect(t, w.admin.do("POST", path+"/snooze", me), 204)
	expect(t, w.ana.do("POST", path+"/close", nil), 204)
	var left bool
	if err := w.e.pool.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM conversation_user_state WHERE conversation_id=$1 AND snoozed_until IS NOT NULL)`, id).Scan(&left); err != nil || left {
		t.Fatalf("adiamento individual sobrou após encerrar: %v %v", left, err)
	}
}

package httpapi

// Chat interno do escritório: envio, papéis, leitura, tempo real, moderação e anexos.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/vyctinho77/chathm/backend/internal/auth"
)

const chatPath = "/api/v1/internal-chat"

func chatSend(t *testing.T, c *client, text string) map[string]any {
	t.Helper()
	r := c.do("POST", chatPath+"/messages", map[string]string{"id": uuid.NewString(), "text": text})
	expect(t, r, 201)
	return r.body
}

func chatMessages(t *testing.T, c *client, query string) ([]map[string]any, bool) {
	t.Helper()
	r := c.do("GET", chatPath+"/messages"+query, nil)
	expect(t, r, 200)
	var out []map[string]any
	for _, m := range r.body["messages"].([]any) {
		out = append(out, m.(map[string]any))
	}
	hasMore, _ := r.body["hasMore"].(bool)
	return out, hasMore
}

func chatUnread(t *testing.T, c *client) int {
	t.Helper()
	r := c.do("GET", chatPath+"/room", nil)
	expect(t, r, 200)
	return int(r.body["unread"].(float64))
}

func TestInternalChatSendRolesAndRules(t *testing.T) {
	w := setupWorld(t)
	w.e.createUser("gestor@x.com", auth.RoleManager)
	gestor := w.e.client().mustLogin("gestor@x.com")

	expect(t, w.e.client().do("GET", chatPath+"/room", nil), 401)
	expect(t, w.e.client().do("GET", chatPath+"/messages", nil), 401)

	first := chatSend(t, w.admin, "Alguém sabe o prazo da DCTF?")
	if first["authorName"] != "adm" || first["authorRole"] != "admin" || first["authorId"] == "" || first["authorActive"] != true {
		t.Fatalf("author not identified: %v", first)
	}
	if first["text"] != "Alguém sabe o prazo da DCTF?" || first["kind"] != "text" {
		t.Fatalf("unexpected message: %v", first)
	}
	reply := w.ana.do("POST", chatPath+"/messages", map[string]string{"id": uuid.NewString(), "text": "Dia 15, segundo a Receita.", "replyTo": first["id"].(string)})
	expect(t, reply, 201)
	if reply.body["authorRole"] != "agent" || reply.body["authorName"] != "ana" {
		t.Fatalf("agent role/name wrong: %v", reply.body)
	}
	q, _ := reply.body["quoted"].(map[string]any)
	if q["id"] != first["id"] || q["author"] != "adm" || q["text"] != "Alguém sabe o prazo da DCTF?" {
		t.Fatalf("quote not resolved: %v", reply.body["quoted"])
	}
	if m := chatSend(t, gestor, "Confirmado."); m["authorRole"] != "manager" {
		t.Fatalf("manager role wrong: %v", m)
	}

	// A sala é uma só, na ordem em que as mensagens chegaram, e todos enxergam o mesmo.
	for _, c := range []*client{w.admin, w.ana, w.bia, gestor} {
		ms, hasMore := chatMessages(t, c, "")
		if len(ms) != 3 || hasMore || ms[0]["text"] != "Alguém sabe o prazo da DCTF?" || ms[2]["text"] != "Confirmado." {
			t.Fatalf("history differs: %v", ms)
		}
	}

	// Idempotência: repetir o mesmo envio não duplica; outra pessoa não pode reutilizar o identificador.
	id := uuid.NewString()
	body := map[string]string{"id": id, "text": "Uma vez só"}
	expect(t, w.ana.do("POST", chatPath+"/messages", body), 201)
	expect(t, w.ana.do("POST", chatPath+"/messages", body), 200)
	expect(t, w.bia.do("POST", chatPath+"/messages", body), 409)
	if ms, _ := chatMessages(t, w.ana, ""); len(ms) != 4 {
		t.Fatalf("duplicate stored: %d", len(ms))
	}

	// Validação.
	expect(t, w.ana.do("POST", chatPath+"/messages", map[string]string{"text": "   "}), 422)
	expect(t, w.ana.do("POST", chatPath+"/messages", map[string]string{"text": strings.Repeat("a", 4001)}), 422)
	expect(t, w.ana.do("POST", chatPath+"/messages", map[string]string{"id": "nao-e-uuid", "text": "oi"}), 422)
	expect(t, w.ana.do("POST", chatPath+"/messages", map[string]string{"text": "oi", "replyTo": uuid.NewString()}), 422)
	expect(t, w.ana.do("POST", chatPath+"/messages", map[string]string{"kind": "image", "text": "x"}), 422)
	expect(t, w.ana.do("POST", chatPath+"/messages", map[string]any{"kind": "location", "lat": 0, "lng": 0}), 422)
	loc := w.ana.do("POST", chatPath+"/messages", map[string]any{"kind": "location", "lat": -23.55, "lng": -46.63, "name": "Escritório"})
	expect(t, loc, 201)
	if loc.body["lat"] != -23.55 || loc.body["name"] != "Escritório" {
		t.Fatalf("location lost: %v", loc.body)
	}
	csrf := w.ana.csrf
	w.ana.csrf = ""
	expect(t, w.ana.do("POST", chatPath+"/messages", map[string]string{"text": "sem csrf"}), 403)
	w.ana.csrf = csrf

	// Nada vai para o WhatsApp.
	if len(w.e.wa.Sent()) != 0 {
		t.Fatal("internal chat sent a WhatsApp message")
	}
}

func TestInternalChatPagination(t *testing.T) {
	w := setupWorld(t)
	for i := 1; i <= 5; i++ {
		chatSend(t, w.admin, fmt.Sprintf("mensagem %d", i))
	}
	page, hasMore := chatMessages(t, w.ana, "?limit=2")
	if len(page) != 2 || !hasMore || page[0]["text"] != "mensagem 4" || page[1]["text"] != "mensagem 5" {
		t.Fatalf("latest page wrong: %v %v", page, hasMore)
	}
	older, hasMore := chatMessages(t, w.ana, fmt.Sprintf("?limit=2&before=%d", int64(page[0]["seq"].(float64))))
	if len(older) != 2 || !hasMore || older[0]["text"] != "mensagem 2" || older[1]["text"] != "mensagem 3" {
		t.Fatalf("older page wrong: %v %v", older, hasMore)
	}
	oldest, hasMore := chatMessages(t, w.ana, fmt.Sprintf("?limit=2&before=%d", int64(older[0]["seq"].(float64))))
	if len(oldest) != 1 || hasMore || oldest[0]["text"] != "mensagem 1" {
		t.Fatalf("oldest page wrong: %v %v", oldest, hasMore)
	}
}

func TestInternalChatUnreadAndRealtime(t *testing.T) {
	w := setupWorld(t)
	chatSend(t, w.admin, "Mensagem antiga")
	// Quem já está no escritório começa a contar do primeiro acesso; o que existia antes não vira selo.
	if chatUnread(t, w.ana) != 0 || chatUnread(t, w.bia) != 0 {
		t.Fatal("history counted as unread on first access")
	}
	anaWS, biaWS, adminWS := w.ana.websocket(t), w.bia.websocket(t), w.admin.websocket(t)
	eventually(t, "chat sockets", func() bool { return w.e.svcs.Hub.Count() == 3 })

	chatSend(t, w.admin, "Reunião às 15h")
	m := chatSend(t, w.admin, "Traga os relatórios")
	eventually(t, "message events", func() bool {
		return anaWS.count("internal.message.created") == 2 && biaWS.count("internal.message.created") == 2 && adminWS.count("internal.message.created") == 2
	})
	if ev := anaWS.last("internal.message.created"); ev["text"] != "Traga os relatórios" || ev["authorRole"] != "admin" || ev["id"] != m["id"] {
		t.Fatalf("event payload wrong: %v", ev)
	}
	if chatUnread(t, w.ana) != 2 || chatUnread(t, w.bia) != 2 {
		t.Fatal("unread should count other people's messages")
	}
	if chatUnread(t, w.admin) != 0 {
		t.Fatal("own messages counted as unread")
	}

	// Ler zera só o contador de quem leu, e só as abas dessa pessoa recebem o aviso.
	read := w.ana.do("POST", chatPath+"/read", nil)
	expect(t, read, 200)
	if read.body["unread"] != float64(0) {
		t.Fatalf("read did not clear: %v", read.body)
	}
	eventually(t, "read event", func() bool { return anaWS.count("internal.read") == 1 })
	if ev := anaWS.last("internal.read"); ev["unread"] != float64(0) {
		t.Fatalf("read event wrong: %v", ev)
	}
	if biaWS.count("internal.read") != 0 || adminWS.count("internal.read") != 0 {
		t.Fatal("read event leaked to other people")
	}
	if chatUnread(t, w.ana) != 0 || chatUnread(t, w.bia) != 2 {
		t.Fatal("read marker is not per person")
	}

	// Mensagem apagada deixa de contar.
	gone := chatSend(t, w.admin, "Engano")
	expect(t, w.admin.do("DELETE", chatPath+"/messages/"+gone["id"].(string), nil), 204)
	if chatUnread(t, w.bia) != 2 {
		t.Fatal("deleted message still unread")
	}

	// Quem entra agora começa sem selo, apesar de todo o histórico.
	w.e.createUser("novo@x.com", auth.RoleAgent)
	novo := w.e.client().mustLogin("novo@x.com")
	if chatUnread(t, novo) != 0 {
		t.Fatal("new person starts with history as unread")
	}
	chatSend(t, w.admin, "Boas-vindas")
	if chatUnread(t, novo) != 1 {
		t.Fatal("new person misses new messages")
	}
}

func TestInternalChatEditAndModeration(t *testing.T) {
	w := setupWorld(t)
	w.e.createUser("gestor@x.com", auth.RoleManager)
	gestor := w.e.client().mustLogin("gestor@x.com")
	biaWS := w.bia.websocket(t)
	eventually(t, "socket", func() bool { return w.e.svcs.Hub.Count() == 1 })

	mine := chatSend(t, w.ana, "Texto original")
	id := mine["id"].(string)
	path := chatPath + "/messages/" + id

	// Edição: só a autora, só texto.
	edited := w.ana.do("PATCH", path, map[string]string{"text": "Texto corrigido"})
	expect(t, edited, 200)
	if edited.body["text"] != "Texto corrigido" || edited.body["editedAt"] == nil {
		t.Fatalf("edit not applied: %v", edited.body)
	}
	eventually(t, "update event", func() bool { return biaWS.count("internal.message.updated") == 1 })
	if ev := biaWS.last("internal.message.updated"); ev["text"] != "Texto corrigido" {
		t.Fatalf("update event wrong: %v", ev)
	}
	for _, other := range []*client{w.bia, gestor, w.admin} {
		expect(t, other.do("PATCH", path, map[string]string{"text": "Alterado por outra pessoa"}), 403)
	}
	expect(t, w.ana.do("PATCH", path, map[string]string{"text": " "}), 422)
	loc := w.ana.do("POST", chatPath+"/messages", map[string]any{"kind": "location", "lat": -23.5, "lng": -46.6})
	expect(t, loc, 201)
	expect(t, w.ana.do("PATCH", chatPath+"/messages/"+loc.body["id"].(string), map[string]string{"text": "x"}), 422)
	expect(t, w.ana.do("PATCH", chatPath+"/messages/"+uuid.NewString(), map[string]string{"text": "x"}), 404)
	// Passadas 24 horas, a edição fecha.
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE internal_messages SET created_at = now() - interval '25 hours' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	expect(t, w.ana.do("PATCH", path, map[string]string{"text": "Tarde demais"}), 422)

	// Apagar: atendente só a própria; gestor e administrador qualquer uma, com auditoria sem o conteúdo.
	expect(t, w.bia.do("DELETE", path, nil), 403)
	expect(t, gestor.do("DELETE", path, nil), 204)
	var moderated int
	var metadata string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT count(*), coalesce(max(metadata::text), '') FROM audit_logs WHERE action = 'internal_chat.message.moderate' AND target_id = $1`, id).Scan(&moderated, &metadata); err != nil {
		t.Fatal(err)
	}
	if moderated != 1 || strings.Contains(metadata, "Texto") {
		t.Fatalf("moderation audit wrong: %d %s", moderated, metadata)
	}
	ms, _ := chatMessages(t, w.bia, "")
	var deleted map[string]any
	for _, m := range ms {
		if m["id"] == id {
			deleted = m
		}
	}
	if deleted == nil || deleted["deleted"] != true || deleted["text"] != "" || deleted["authorName"] != "ana" {
		t.Fatalf("deleted message wrong: %v", deleted)
	}
	expect(t, gestor.do("DELETE", path, nil), 204) // repetir é inofensivo
	expect(t, w.ana.do("PATCH", path, map[string]string{"text": "de volta"}), 422)

	// A autora apaga a própria sem auditoria de moderação; o admin apaga a de um gestor.
	own := chatSend(t, w.ana, "Vou apagar")
	expect(t, w.ana.do("DELETE", chatPath+"/messages/"+own["id"].(string), nil), 204)
	other := chatSend(t, gestor, "Do gestor")
	expect(t, w.admin.do("DELETE", chatPath+"/messages/"+other["id"].(string), nil), 204)
	var total int
	if err := w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action = 'internal_chat.message.moderate'`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("moderation audits = %d, want 2", total)
	}
	expect(t, w.ana.do("DELETE", chatPath+"/messages/"+uuid.NewString(), nil), 404)
}

func TestInternalChatAttachments(t *testing.T) {
	w := setupWorld(t)
	pic := testPNG(8, 8)
	fields := func(kind string, extra map[string]string) map[string]string {
		f := map[string]string{"id": uuid.NewString(), "kind": kind}
		for k, v := range extra {
			f[k] = v
		}
		return f
	}
	up := w.ana.upload(chatPath+"/messages", fields("image", map[string]string{"text": "Olhem este print"}), "print.png", pic)
	expect(t, up, 201)
	if up.body["kind"] != "image" || up.body["caption"] != "Olhem este print" || up.body["authorName"] != "ana" {
		t.Fatalf("image message wrong: %v", up.body)
	}
	url, _ := up.body["url"].(string)
	if !strings.HasPrefix(url, chatPath+"/attachments/") {
		t.Fatalf("attachment url wrong: %q", url)
	}
	// Todas as pessoas autenticadas leem o anexo; sem sessão, não.
	for _, c := range []*client{w.admin, w.bia, w.dani} {
		status, header, data := c.raw(url)
		if status != 200 || header.Get("Content-Type") != "image/png" || len(data) == 0 || header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("attachment read failed: %d %v", status, header)
		}
	}
	if status, _, _ := w.e.client().raw(url); status != 401 {
		t.Fatalf("anonymous attachment status = %d", status)
	}

	doc := w.bia.upload(chatPath+"/messages", fields("document", nil), "contrato.pdf", []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF"))
	expect(t, doc, 201)
	if doc.body["fileName"] != "contrato.pdf" {
		t.Fatalf("document name lost: %v", doc.body)
	}
	_, header, _ := w.ana.raw(doc.body["url"].(string))
	if header.Get("Content-Type") != "application/octet-stream" || !strings.Contains(header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("document must download, not render: %v", header)
	}

	// Tipos bloqueados, arquivo ausente e sticker (só existe no chat de clientes).
	expect(t, w.ana.upload(chatPath+"/messages", fields("document", nil), "programa.exe", []byte("MZ")), 422)
	expect(t, w.ana.upload(chatPath+"/messages", fields("image", nil), "", nil), 422)
	expect(t, w.ana.upload(chatPath+"/messages", fields("sticker", nil), "s.webp", pic), 422)

	// Apagar a mensagem derruba o anexo, mesmo para quem tinha o endereço.
	expect(t, w.ana.do("DELETE", chatPath+"/messages/"+up.body["id"].(string), nil), 204)
	if status, _, _ := w.admin.raw(url); status != 404 {
		t.Fatalf("deleted attachment status = %d", status)
	}
	var left int
	if err := w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM internal_attachments WHERE message_id = $1`, up.body["id"]).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("attachment row kept after delete")
	}
	if len(w.e.wa.Sent()) != 0 {
		t.Fatal("internal attachment went to WhatsApp")
	}
}

func TestInternalChatStaysOutOfCustomerData(t *testing.T) {
	w := setupWorld(t)
	chatSend(t, w.admin, "Cliente Fulano está reclamando da fatura")
	for _, c := range []*client{w.admin, w.ana} {
		r := c.do("GET", "/api/v1/conversations", nil)
		expect(t, r, 200)
		if strings.Contains(fmt.Sprint(r.body), "reclamando") {
			t.Fatal("internal message leaked into the conversation list")
		}
		s := c.do("GET", "/api/v1/search?q=reclamando", nil)
		expect(t, s, 200)
		if strings.Contains(fmt.Sprint(s.body), "reclamando") {
			t.Fatal("internal message leaked into search")
		}
	}
}

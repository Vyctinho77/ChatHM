package httpapi

// Testes de integração da Fase 2: conexão WhatsApp (provedor simulado),
// ingestão idempotente, permissões e eventos em tempo real.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tempo esgotado esperando: %s", what)
}

func inbound(id, phone, name, text string) whatsapp.InboundMessage {
	return whatsapp.InboundMessage{
		ExternalID: id, ChatJID: phone + "@s.whatsapp.net", ContactWAID: phone, ContactPhone: "+" + phone,
		PushName: name, Timestamp: time.Now(), Kind: whatsapp.KindText, Body: text,
	}
}

// wsClient abre o WebSocket com o cookie de sessão do cliente HTTP e coleta eventos.
type wsClient struct {
	mu     sync.Mutex
	events []map[string]any
	conn   *websocket.Conn
}

func (c *client) websocket(t *testing.T) *wsClient {
	t.Helper()
	u, _ := url.Parse(c.base)
	h := http.Header{}
	for _, ck := range c.hc.Jar.Cookies(u) {
		h.Add("Cookie", ck.Name+"="+ck.Value)
	}
	h.Set("Origin", c.origin)
	wsURL := "ws" + strings.TrimPrefix(c.base, "http") + "/api/v1/ws"
	conn, resp, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("websocket: %v (status %d)", err, status)
	}
	w := &wsClient{conn: conn}
	go func() {
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var ev map[string]any
			_ = json.Unmarshal(data, &ev)
			w.mu.Lock()
			w.events = append(w.events, ev)
			w.mu.Unlock()
		}
	}()
	t.Cleanup(func() { conn.CloseNow() })
	return w
}

func (w *wsClient) count(typ string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, e := range w.events {
		if e["type"] == typ {
			n++
		}
	}
	return n
}

func (w *wsClient) last(typ string) map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := len(w.events) - 1; i >= 0; i-- {
		if w.events[i]["type"] == typ {
			d, _ := w.events[i]["data"].(map[string]any)
			return d
		}
	}
	return nil
}

func TestWhatsAppConnectionFlow(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", auth.RoleAdmin)
	e.createUser("ana@x.com", auth.RoleAgent)
	admin := e.client().mustLogin("adm@x.com")
	agent := e.client().mustLogin("ana@x.com")
	adminWS := admin.websocket(t)
	agentWS := agent.websocket(t)
	eventually(t, "2 conexões ws", func() bool { return e.svcs.Hub.Count() == 2 })

	r := agent.do("GET", "/api/v1/whatsapp/connection", nil)
	expect(t, r, 200)
	if r.body["state"] != "disconnected" {
		t.Fatalf("estado inicial = %v", r.body)
	}

	// Somente admin gerencia a conexão.
	expect(t, agent.do("POST", "/api/v1/whatsapp/connect", nil), 403, "forbidden")
	expect(t, agent.do("POST", "/api/v1/whatsapp/disconnect", nil), 403, "forbidden")

	r = admin.do("POST", "/api/v1/whatsapp/connect", nil)
	expect(t, r, 202)
	eventually(t, "QR para o admin", func() bool {
		q := admin.do("GET", "/api/v1/whatsapp/connection", nil)
		qr, _ := q.body["qr"].(map[string]any)
		return q.body["state"] == "pending_qr" && qr != nil && strings.HasPrefix(qr["code"].(string), "chathm-fake-qr")
	})
	// Atendente vê o estado, nunca o QR (nem pela API nem pelo WebSocket).
	r = agent.do("GET", "/api/v1/whatsapp/connection", nil)
	if r.body["state"] != "pending_qr" || r.body["qr"] != nil {
		t.Fatalf("atendente viu: %v", r.body)
	}
	eventually(t, "evento de QR para admin", func() bool { return adminWS.count("whatsapp.qr") > 0 })
	if agentWS.count("whatsapp.qr") != 0 {
		t.Fatal("atendente recebeu evento de QR")
	}

	// Idempotente enquanto aguarda leitura.
	expect(t, admin.do("POST", "/api/v1/whatsapp/connect", nil), 202)

	e.wa.CompletePairing()
	eventually(t, "conectado", func() bool {
		s := agentWS.last("whatsapp.status")
		return s != nil && s["state"] == "connected" && s["phone"] == "+5511900000000"
	})
	expect(t, admin.do("POST", "/api/v1/whatsapp/connect", nil), 409, "already_connected")

	var state, phone string
	_ = e.pool.QueryRow(context.Background(), `SELECT status, phone FROM whatsapp_connections`).Scan(&state, &phone)
	if state != "connected" || phone != "+5511900000000" {
		t.Fatalf("persistido: %s %s", state, phone)
	}

	// Queda transitória fica visível como erro; desvincular pelo celular desconecta.
	e.wa.Drop(false)
	eventually(t, "erro visível", func() bool { return agentWS.last("whatsapp.status")["state"] == "error" })
	e.wa.Drop(true)
	eventually(t, "desconectado", func() bool { return agentWS.last("whatsapp.status")["state"] == "disconnected" })

	// Reconectar e desconectar pelo painel.
	expect(t, admin.do("POST", "/api/v1/whatsapp/connect", nil), 202)
	e.wa.CompletePairing()
	eventually(t, "reconectado", func() bool { return e.svcs.WhatsApp.Status().State == whatsapp.StateConnected })
	r = admin.do("POST", "/api/v1/whatsapp/disconnect", nil)
	expect(t, r, 200)
	if r.body["state"] != "disconnected" {
		t.Fatalf("após desconectar: %v", r.body)
	}

	var n int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action LIKE 'whatsapp.%'`).Scan(&n)
	if n != 3 {
		t.Fatalf("auditoria whatsapp = %d, esperado 3", n)
	}
}

func TestQRExpires(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", auth.RoleAdmin)
	admin := e.client().mustLogin("adm@x.com")
	expect(t, admin.do("POST", "/api/v1/whatsapp/connect", nil), 202)
	eventually(t, "aguardando QR", func() bool { return e.svcs.WhatsApp.Status().State == whatsapp.StatePendingQR })
	e.wa.ExpirePairing()
	eventually(t, "expirado", func() bool {
		s := e.svcs.WhatsApp.Status()
		return s.State == whatsapp.StateDisconnected && strings.Contains(s.LastError, "expirou")
	})
	if e.svcs.WhatsApp.CurrentQR() != nil {
		t.Fatal("QR deveria ser descartado")
	}
}

func TestRestartRestoresSession(t *testing.T) {
	e := setup(t, opts{waSession: true})
	eventually(t, "sessão restaurada no boot", func() bool {
		return e.svcs.WhatsApp.Status().State == whatsapp.StateConnected
	})
}

func TestIngestDedupAndRealtime(t *testing.T) {
	e := setup(t)
	e.createUser("ana@x.com", auth.RoleAgent)
	agent := e.client().mustLogin("ana@x.com")
	ws := agent.websocket(t)
	eventually(t, "ws", func() bool { return e.svcs.Hub.Count() == 1 })

	msg := inbound("ABC123", "5592998112040", "Rafael", "quero emitir uma nota")
	e.wa.Receive(msg)
	e.wa.Receive(msg) // reentrega do mesmo evento
	e.wa.Receive(inbound("ABC124", "5592998112040", "Rafael", "é urgente"))

	list := agent.do("GET", "/api/v1/conversations", nil)
	expect(t, list, 200)
	convs := list.body["conversations"].([]any)
	if len(convs) != 1 {
		t.Fatalf("conversas = %d, esperado 1", len(convs))
	}
	c := convs[0].(map[string]any)
	contact := c["contact"].(map[string]any)
	if c["status"] != "new" || c["unreadCount"].(float64) != 2 || c["segment"] != "geral" ||
		contact["name"] != "Rafael" || contact["phone"] != "+55 92 99811-2040" {
		t.Fatalf("conversa = %v", c)
	}
	if c["lastMessage"].(map[string]any)["text"] != "é urgente" {
		t.Fatalf("última mensagem = %v", c["lastMessage"])
	}
	id := c["id"].(string)

	msgs := agent.do("GET", "/api/v1/conversations/"+id+"/messages", nil)
	expect(t, msgs, 200)
	if n := len(msgs.body["messages"].([]any)); n != 2 {
		t.Fatalf("mensagens = %d, esperado 2 (sem duplicata)", n)
	}
	eventually(t, "2 eventos de mensagem", func() bool { return ws.count("message.created") == 2 })
	time.Sleep(100 * time.Millisecond)
	if n := ws.count("message.created"); n != 2 {
		t.Fatalf("eventos de mensagem = %d, esperado 2", n)
	}

	// Mensagem enviada pelo próprio celular: sai como "out" e não conta como não lida.
	out := inbound("ABC125", "5592998112040", "", "Bom dia!")
	out.FromMe = true
	e.wa.Receive(out)
	c2 := agent.do("GET", "/api/v1/conversations/"+id, nil)
	if c2.body["unreadCount"].(float64) != 2 || c2.body["lastMessage"].(map[string]any)["direction"] != "out" {
		t.Fatalf("após mensagem do celular: %v", c2.body)
	}

	// Marcar como lida.
	expect(t, agent.do("POST", "/api/v1/conversations/"+id+"/read", nil), 204)
	if agent.do("GET", "/api/v1/conversations/"+id, nil).body["unreadCount"].(float64) != 0 {
		t.Fatal("não lidas deveria zerar")
	}

	// Observação interna do contato.
	expect(t, agent.do("PATCH", "/api/v1/contacts/"+contact["id"].(string), map[string]string{"note": "Prefere manhã"}), 204)
	if agent.do("GET", "/api/v1/conversations/"+id, nil).body["contact"].(map[string]any)["note"] != "Prefere manhã" {
		t.Fatal("observação não salva")
	}

	// Após encerramento, nova mensagem abre nova conversa (pode ser reclassificada na Fase 3).
	_, _ = e.pool.Exec(context.Background(), `UPDATE conversations SET status = 'closed', closed_at = now()`)
	e.wa.Receive(inbound("ABC126", "5592998112040", "Rafael", "outra demanda"))
	if n := len(agent.do("GET", "/api/v1/conversations", nil).body["conversations"].([]any)); n != 2 {
		t.Fatalf("conversas após encerrar = %d, esperado 2", n)
	}

	expect(t, agent.do("GET", "/api/v1/conversations/nao-existe/messages", nil), 404)
}

func TestConcurrentIngestSameContact(t *testing.T) {
	e := setup(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "M" + string(rune('A'+i%5)) // 5 IDs distintos, cada um entregue 4 vezes
			e.wa.Receive(inbound(id, "5511988887777", "Bia", "oi"))
		}(i)
	}
	wg.Wait()
	var convs, msgs, unread int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*), coalesce(sum(unread_count),0) FROM conversations`).Scan(&convs, &unread)
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM messages`).Scan(&msgs)
	if convs != 1 || msgs != 5 || unread != 5 {
		t.Fatalf("conversas=%d mensagens=%d não lidas=%d; esperado 1/5/5", convs, msgs, unread)
	}
}

func TestWebSocketRequiresSessionAndOrigin(t *testing.T) {
	e := setup(t)
	e.createUser("ana@x.com", auth.RoleAgent)
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/v1/ws"
	_, resp, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {origin}},
	})
	if err == nil || resp == nil || resp.StatusCode != 401 {
		t.Fatalf("sem sessão deveria ser 401: %v", err)
	}

	c := e.client().mustLogin("ana@x.com")
	c.origin = "https://evil.example"
	u, _ := url.Parse(c.base)
	h := http.Header{"Origin": {c.origin}}
	for _, ck := range c.hc.Jar.Cookies(u) {
		h.Add("Cookie", ck.Name+"="+ck.Value)
	}
	_, resp, err = websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{HTTPHeader: h})
	if err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatalf("origem externa deveria ser 403: %v", err)
	}
}

// Regressão: o WebSocket não pode herdar o ReadTimeout/WriteTimeout do servidor HTTP.
func TestWebSocketSurvivesServerTimeouts(t *testing.T) {
	e := setup(t)
	e.srv.Close()
	srv := httptest.NewUnstartedServer(e.srv.Config.Handler)
	srv.Config.ReadTimeout = 500 * time.Millisecond
	srv.Config.WriteTimeout = 500 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)

	e.createUser("ana@x.com", auth.RoleAgent)
	c := e.client()
	c.base = srv.URL
	c.mustLogin("ana@x.com")
	ws := c.websocket(t)
	eventually(t, "ws", func() bool { return e.svcs.Hub.Count() == 1 })

	time.Sleep(1500 * time.Millisecond) // 3x os timeouts do servidor
	e.wa.Receive(inbound("LATE1", "5511977776666", "Caio", "ainda conectado?"))
	eventually(t, "evento após timeouts", func() bool { return ws.count("message.created") == 1 })
}

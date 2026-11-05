package httpapi

// Critérios de aceite da Fase 3: classificação, filas por equipe, atribuição atômica,
// visibilidade, envio idempotente, recibos, encerramento e etiquetas.

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp/fake"
)

type world struct {
	e                                 *env
	admin, ana, ana2, bia, caio, dani *client
	teams                             map[string]string // folha | notas | clientes → id
	segs                              map[string]string // chave do segmento → id
	users                             map[string]string
}

// setupWorld monta: equipes Folha (ana, ana2), Notas (bia) e Clientes novos (caio);
// dani sem equipe; segmentos padrão vinculados às equipes; WhatsApp conectado.
func setupWorld(t *testing.T) *world {
	t.Helper()
	e := setup(t)
	e.createUser("adm@x.com", auth.RoleAdmin)
	w := &world{e: e, teams: map[string]string{}, segs: map[string]string{}, users: map[string]string{}}
	w.admin = e.client().mustLogin("adm@x.com")

	for key, name := range map[string]string{"folha": "Folha", "notas": "Notas", "clientes": "Clientes novos"} {
		r := w.admin.do("POST", "/api/v1/teams", map[string]string{"name": name})
		expect(t, r, 201)
		w.teams[key] = r.body["id"].(string)
	}
	mk := func(email string, teams ...string) *client {
		var ids []string
		for _, k := range teams {
			ids = append(ids, w.teams[k])
		}
		u := e.createUser(email, auth.RoleAgent, ids...)
		w.users[strings.Split(email, "@")[0]] = u.ID
		return e.client().mustLogin(email)
	}
	w.ana, w.ana2 = mk("ana@x.com", "folha"), mk("ana2@x.com", "folha")
	w.bia, w.caio, w.dani = mk("bia@x.com", "notas"), mk("caio@x.com", "clientes"), mk("dani@x.com")

	segs := w.admin.do("GET", "/api/v1/segments", nil)
	expect(t, segs, 200)
	for _, s := range segs.body["segments"].([]any) {
		m := s.(map[string]any)
		w.segs[m["key"].(string)] = m["id"].(string)
	}
	for seg, team := range map[string]string{"folha": "folha", "notas": "notas", "clientes_novos": "clientes"} {
		expect(t, w.admin.do("PUT", "/api/v1/segments/"+w.segs[seg]+"/team", map[string]any{"teamId": w.teams[team]}), 200)
	}
	w.connect()
	return w
}

func (w *world) connect() {
	t := w.e.t
	expect(t, w.admin.do("POST", "/api/v1/whatsapp/connect", nil), 202)
	eventually(t, "aguardando QR", func() bool { return w.e.svcs.WhatsApp.Status().State == whatsapp.StatePendingQR })
	w.e.wa.CompletePairing()
	eventually(t, "conectado", func() bool { return w.e.svcs.WhatsApp.Connected() })
}

// phoneSeq gera telefones distintos; atômico porque os testes rodam em paralelo.
var phoneSeq atomic.Int64

func init() { phoneSeq.Store(5511900000000) }

// receive entrega uma mensagem de um cliente novo (telefone distinto por nome).
func (w *world) receive(name, text string) string {
	p := itoa(int(phoneSeq.Add(1)))
	w.e.wa.Receive(inbound("M-"+p, p, name, text))
	return p
}

func (w *world) receiveFrom(phone, name, text string) {
	w.e.wa.Receive(inbound("M-"+uuid.NewString(), phone, name, text))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func list(t *testing.T, c *client) []map[string]any {
	t.Helper()
	r := c.do("GET", "/api/v1/conversations", nil)
	expect(t, r, 200)
	var out []map[string]any
	for _, x := range r.body["conversations"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

// conv devolve a conversa visível ao cliente com o nome do contato, ou nil.
func conv(t *testing.T, c *client, name string) map[string]any {
	t.Helper()
	for _, m := range list(t, c) {
		if m["contact"].(map[string]any)["name"] == name {
			return m
		}
	}
	return nil
}

func mustConv(t *testing.T, c *client, name string) map[string]any {
	t.Helper()
	m := conv(t, c, name)
	if m == nil {
		t.Fatalf("conversa %q não visível", name)
	}
	return m
}

func (w *world) send(c *client, convID, text string) resp {
	return c.do("POST", "/api/v1/conversations/"+convID+"/messages", map[string]any{"id": uuid.NewString(), "text": text})
}

func visibleTo(t *testing.T, name string, who map[string]*client) []string {
	t.Helper()
	var out []string
	for n, c := range who {
		if conv(t, c, name) != nil {
			out = append(out, n)
		}
	}
	return out
}

// ---------------------------------------------------------------------------

func TestClassificationAndQueues(t *testing.T) {
	w := setupWorld(t)
	all := map[string]*client{"admin": w.admin, "ana": w.ana, "bia": w.bia, "caio": w.caio, "dani": w.dani}

	cases := []struct {
		name, text, segment, class string
		seenBy                     []string // quem enxerga (admin sempre)
	}{
		{"C Notas", "quero emitir uma nota", "notas", "keyword", []string{"admin", "bia"}},
		{"C Folha", "preciso de férias de funcionário", "folha", "keyword", []string{"admin", "ana"}},
		{"C Novo", "quero orçamento", "clientes_novos", "keyword", []string{"admin", "caio"}},
		{"C Contratar", "quero contratar", "clientes_novos", "keyword", []string{"admin", "caio"}},
		{"C Calcular", "preciso calcular a folha", "folha", "keyword", []string{"admin", "ana"}},
		{"C Bomdia", "bom dia", "geral", "none", []string{"admin", "ana", "bia", "caio", "dani"}},
		{"C Conflito", "quero orçamento da folha de pagamento", "geral", "conflict", []string{"admin", "ana", "bia", "caio", "dani"}},
	}
	for _, c := range cases {
		w.receive(c.name, c.text)
	}
	for _, c := range cases {
		got := mustConv(t, w.admin, c.name)
		if got["segment"] != c.segment || got["classification"] != c.class {
			t.Errorf("%q → segmento=%v classificação=%v, esperado %s/%s", c.text, got["segment"], got["classification"], c.segment, c.class)
		}
		seen := visibleTo(t, c.name, all)
		if len(seen) != len(c.seenBy) {
			t.Errorf("%q visível a %v, esperado %v", c.text, seen, c.seenBy)
			continue
		}
		for _, n := range c.seenBy {
			if conv(t, all[n], c.name) == nil {
				t.Errorf("%q deveria ser visível a %s", c.text, n)
			}
		}
	}

	// Nenhuma conversa duplicada: uma por contato.
	if n := len(list(t, w.admin)); n != len(cases) {
		t.Fatalf("conversas = %d, esperado %d", n, len(cases))
	}
	// Nome do segmento chega à interface.
	if got := mustConv(t, w.admin, "C Notas")["segmentName"]; got != "Emissão de notas" {
		t.Fatalf("segmentName = %v", got)
	}
}

func TestUnroutedSegmentIsVisibleToEveryone(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", auth.RoleAdmin)
	e.createUser("ana@x.com", auth.RoleAgent)
	ana := e.client().mustLogin("ana@x.com")
	e.wa.Receive(inbound("U1", "5511911110001", "Sem Equipe", "quero emitir nota"))
	got := mustConv(t, ana, "Sem Equipe")
	if got["segment"] != "notas" {
		t.Fatalf("segmento = %v", got["segment"])
	}
}

func TestLaterMessageClassifiesUnclaimedGeneralConversation(t *testing.T) {
	w := setupWorld(t)
	anaWS, biaWS := w.ana.websocket(t), w.bia.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() == 2 })

	phone := w.receive("Cliente Aos Poucos", "bom dia")
	if mustConv(t, w.ana, "Cliente Aos Poucos")["segment"] != "geral" {
		t.Fatal("deveria começar na fila geral")
	}
	w.receiveFrom(phone, "Cliente Aos Poucos", "quero emitir uma nota")

	if got := mustConv(t, w.bia, "Cliente Aos Poucos"); got["segment"] != "notas" {
		t.Fatalf("segmento = %v", got["segment"])
	}
	if conv(t, w.ana, "Cliente Aos Poucos") != nil {
		t.Fatal("ana (Folha) não deveria mais ver a conversa de Notas")
	}
	// Quem perdeu o acesso é avisado para tirar da lista; quem ganhou recebe a conversa.
	eventually(t, "ana notificada da remoção", func() bool { return anaWS.count("conversation.removed") == 1 })
	if biaWS.count("conversation.removed") != 0 {
		t.Fatal("bia não deveria receber remoção")
	}
	if n := len(list(t, w.admin)); n != 1 {
		t.Fatalf("conversas = %d, esperado 1 (sem cópias)", n)
	}
}

func TestActiveConversationKeepsQueue(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Cliente Fixo", "preciso das férias")
	id := mustConv(t, w.ana, "Cliente Fixo")["id"].(string)

	// Ainda sem responsável: nova frase de outro segmento não muda a fila.
	w.receiveFrom(phone, "Cliente Fixo", "e também quero emitir nota fiscal")
	if got := mustConv(t, w.ana, "Cliente Fixo"); got["segment"] != "folha" {
		t.Fatalf("segmento mudou sem responsável: %v", got["segment"])
	}

	// Em atendimento: idem, e o responsável continua o mesmo.
	expect(t, w.send(w.ana, id, "Olá, vou ajudar com as férias."), 201)
	w.receiveFrom(phone, "Cliente Fixo", "quero orçamento e emitir nota")
	got := mustConv(t, w.ana, "Cliente Fixo")
	if got["segment"] != "folha" || got["status"] != "in_progress" || got["assignee"].(map[string]any)["id"] != w.users["ana"] {
		t.Fatalf("conversa ativa mudou: %v", got)
	}
	if conv(t, w.bia, "Cliente Fixo") != nil {
		t.Fatal("bia não deveria ver conversa da Folha")
	}
	if n := len(list(t, w.admin)); n != 1 {
		t.Fatalf("conversas = %d", n)
	}
}

func TestConcurrentClaimOnlyOneWins(t *testing.T) {
	w := setupWorld(t)
	const rounds = 12
	ids := make([]string, rounds)
	for i := range ids {
		w.receive("Disputa "+itoa(i), "preciso das férias")
		ids[i] = mustConv(t, w.ana, "Disputa "+itoa(i))["id"].(string)
	}

	type result struct{ ana, ana2 resp }
	results := make([]result, rounds)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range ids {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i].ana = w.send(w.ana, ids[i], "resposta da ana")
		}(i)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i].ana2 = w.send(w.ana2, ids[i], "resposta da ana2")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, r := range results {
		a, b := r.ana.status, r.ana2.status
		if !((a == 201 && b == 409) || (a == 409 && b == 201)) {
			t.Fatalf("rodada %d: ana=%d ana2=%d (%v / %v), esperado exatamente um 201 e um 409", i, a, b, r.ana.body, r.ana2.body)
		}
		loser := r.ana
		if a == 201 {
			loser = r.ana2
		}
		if loser.code() != "conversation_taken" {
			t.Fatalf("rodada %d: perdedor recebeu %q", i, loser.code())
		}
	}

	ctx := context.Background()
	var active, assignments, outMsgs int
	_ = w.e.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_assignments WHERE ended_at IS NULL`).Scan(&active)
	_ = w.e.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_assignments`).Scan(&assignments)
	_ = w.e.pool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE direction = 'out'`).Scan(&outMsgs)
	if active != rounds || assignments != rounds || outMsgs != rounds {
		t.Fatalf("atribuições ativas=%d total=%d mensagens enviadas=%d; esperado %d de cada", active, assignments, outMsgs, rounds)
	}
	if n := len(w.e.wa.Sent()); n != rounds {
		t.Fatalf("envios ao WhatsApp = %d, esperado %d (nada de resposta dupla)", n, rounds)
	}
	// O responsável é quem enviou a mensagem.
	var mismatches int
	_ = w.e.pool.QueryRow(ctx, `
		SELECT count(*) FROM conversations c JOIN messages m ON m.conversation_id = c.id AND m.direction = 'out'
		 WHERE m.sender_user_id IS DISTINCT FROM c.assignee_id`).Scan(&mismatches)
	if mismatches != 0 {
		t.Fatalf("%d conversas com responsável diferente de quem respondeu", mismatches)
	}
}

func TestVisibilityIsEnforcedOnEveryEndpoint(t *testing.T) {
	w := setupWorld(t)
	w.receive("Da Folha", "quero as férias")
	conv0 := mustConv(t, w.ana, "Da Folha")
	id, contactID := conv0["id"].(string), conv0["contact"].(map[string]any)["id"].(string)

	// bia (Notas) e dani (sem equipe) não enxergam nada da Folha.
	for name, c := range map[string]*client{"bia": w.bia, "dani": w.dani} {
		if conv(t, c, "Da Folha") != nil {
			t.Fatalf("%s vê conversa da Folha na lista", name)
		}
		expect(t, c.do("GET", "/api/v1/conversations/"+id, nil), 404)
		expect(t, c.do("GET", "/api/v1/conversations/"+id+"/messages", nil), 404)
		expect(t, c.do("POST", "/api/v1/conversations/"+id+"/read", nil), 404)
		expect(t, w.send(c, id, "invasão"), 404)
		expect(t, c.do("POST", "/api/v1/conversations/"+id+"/close", nil), 404)
		expect(t, c.do("PATCH", "/api/v1/contacts/"+contactID, map[string]string{"note": "x"}), 404)
		tags := c.do("GET", "/api/v1/tags", nil)
		expect(t, tags, 200)
		tagID := tags.body["tags"].([]any)[0].(map[string]any)["id"].(string)
		expect(t, c.do("PUT", "/api/v1/conversations/"+id+"/tags/"+tagID, nil), 404)
	}
	if len(w.e.wa.Sent()) != 0 {
		t.Fatal("alguma resposta indevida chegou ao WhatsApp")
	}

	// Depois que ana assume: a equipe continua vendo, mas só ela responde e encerra.
	expect(t, w.send(w.ana, id, "Oi, aqui é a ana"), 201)
	if conv(t, w.ana2, "Da Folha") == nil {
		t.Fatal("colega da equipe deveria continuar vendo a conversa assumida")
	}
	r := w.send(w.ana2, id, "posso ajudar?")
	expect(t, r, 409, "conversation_taken")
	if !strings.Contains(r.body["error"].(map[string]any)["message"].(string), "ana") {
		t.Fatalf("mensagem deveria citar o responsável: %v", r.body)
	}
	expect(t, w.ana2.do("POST", "/api/v1/conversations/"+id+"/close", nil), 403, "forbidden")
	expect(t, w.admin.do("GET", "/api/v1/conversations/"+id, nil), 200)
}

func TestClaimedGeneralConversationBecomesPrivate(t *testing.T) {
	w := setupWorld(t)
	w.receive("Geral Privada", "bom dia")
	id := mustConv(t, w.dani, "Geral Privada")["id"].(string)
	anaWS, biaWS := w.ana.websocket(t), w.bia.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() == 2 })

	// dani (sem equipe) responde e assume; os demais deixam de ver.
	expect(t, w.send(w.dani, id, "Bom dia! Como posso ajudar?"), 201)
	for name, c := range map[string]*client{"ana": w.ana, "bia": w.bia, "caio": w.caio} {
		if conv(t, c, "Geral Privada") != nil {
			t.Fatalf("%s ainda vê conversa geral assumida por outra pessoa", name)
		}
		expect(t, c.do("GET", "/api/v1/conversations/"+id+"/messages", nil), 404)
	}
	eventually(t, "remoções", func() bool {
		return anaWS.count("conversation.removed") == 1 && biaWS.count("conversation.removed") == 1
	})
	mustConv(t, w.dani, "Geral Privada")
	mustConv(t, w.admin, "Geral Privada")
}

func TestRealtimeEventsOnlyReachAuthorizedUsers(t *testing.T) {
	w := setupWorld(t)
	anaWS, biaWS, daniWS := w.ana.websocket(t), w.bia.websocket(t), w.dani.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() == 3 })

	w.receive("Evento Folha", "férias de funcionário")
	eventually(t, "ana recebe", func() bool { return anaWS.count("message.created") == 1 && anaWS.count("conversation.updated") >= 1 })
	time.Sleep(150 * time.Millisecond)
	if biaWS.count("message.created") != 0 || biaWS.count("conversation.updated") != 0 ||
		daniWS.count("message.created") != 0 || daniWS.count("conversation.updated") != 0 {
		t.Fatalf("eventos vazaram: bia=%d/%d dani=%d/%d", biaWS.count("message.created"), biaWS.count("conversation.updated"),
			daniWS.count("message.created"), daniWS.count("conversation.updated"))
	}

	w.receive("Evento Geral", "bom dia")
	eventually(t, "todos recebem a geral", func() bool {
		return anaWS.count("message.created") == 2 && biaWS.count("message.created") == 1 && daniWS.count("message.created") == 1
	})
}

func TestSendNeedsWhatsAppAndDoesNotClaim(t *testing.T) {
	w := setupWorld(t)
	w.receive("Sem Conexão", "férias")
	id := mustConv(t, w.ana, "Sem Conexão")["id"].(string)

	w.e.wa.Drop(false)
	eventually(t, "erro", func() bool { return w.e.svcs.WhatsApp.Status().State == whatsapp.StateError })
	expect(t, w.send(w.ana, id, "olá"), 503, "whatsapp_unavailable")

	got := mustConv(t, w.ana2, "Sem Conexão")
	if got["assignee"] != nil || got["status"] != "new" {
		t.Fatalf("a tentativa sem conexão não deveria assumir a conversa: %v", got)
	}
	var msgs int
	_ = w.e.pool.QueryRow(context.Background(), `SELECT count(*) FROM messages WHERE direction = 'out'`).Scan(&msgs)
	if msgs != 0 {
		t.Fatalf("mensagens gravadas = %d", msgs)
	}
	// Outra pessoa da equipe ainda pode assumir depois de reconectar.
	w.e.wa.CompletePairing()
	eventually(t, "conectado", func() bool { return w.e.svcs.WhatsApp.Connected() })
	expect(t, w.send(w.ana2, id, "Oi, aqui é a ana2"), 201)
}

func TestSendValidationAndIdempotencyAndRetry(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Envio", "férias")
	id := mustConv(t, w.ana, "Cliente Envio")["id"].(string)

	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", map[string]any{"text": "   "}), 422)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", map[string]any{"text": strings.Repeat("a", 4001)}), 422)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", map[string]any{"text": "oi", "id": "não-é-uuid"}), 422)

	// Mesma requisição duas vezes (duplo clique): uma mensagem, um envio.
	msgID := uuid.NewString()
	body := map[string]any{"id": msgID, "text": "Segue o resumo das férias."}
	first := w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", body)
	expect(t, first, 201)
	if first.body["status"] != "sent" || first.body["authorName"] != "ana" || first.body["id"] != msgID {
		t.Fatalf("mensagem = %v", first.body)
	}
	again := w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", body)
	expect(t, again, 200)
	if n := len(w.e.wa.Sent()); n != 1 {
		t.Fatalf("envios = %d, esperado 1", n)
	}
	var count int
	_ = w.e.pool.QueryRow(context.Background(), `SELECT count(*) FROM messages WHERE direction = 'out'`).Scan(&count)
	if count != 1 {
		t.Fatalf("mensagens = %d", count)
	}
	if sent := w.e.wa.Sent()[0]; sent.Text != "Segue o resumo das férias." || !strings.HasSuffix(sent.ChatJID, "@s.whatsapp.net") {
		t.Fatalf("envio = %+v", sent)
	}

	// Colega não pode reaproveitar o id de outra pessoa.
	expect(t, w.ana2.do("POST", "/api/v1/conversations/"+id+"/messages", body), 409, "id_conflict")

	// Falha no WhatsApp: a mensagem fica registrada como "failed", sem apagar nada.
	w.e.wa.FailSends(fake.SimulateFailure)
	failID := uuid.NewString()
	failBody := map[string]any{"id": failID, "text": "Essa vai falhar"}
	r := w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", failBody)
	expect(t, r, 201)
	if r.body["status"] != "failed" {
		t.Fatalf("status = %v", r.body["status"])
	}
	msgs := w.ana.do("GET", "/api/v1/conversations/"+id+"/messages", nil).body["messages"].([]any)
	if last := msgs[len(msgs)-1].(map[string]any); last["id"] != failID || last["status"] != "failed" {
		t.Fatalf("a mensagem com falha deveria permanecer na conversa: %v", last)
	}

	// Tentar de novo com o mesmo id: reenvia sem duplicar.
	w.e.wa.FailSends(nil)
	retry := w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", failBody)
	expect(t, retry, 200)
	if retry.body["status"] != "sent" {
		t.Fatalf("após reenvio: %v", retry.body)
	}
	_ = w.e.pool.QueryRow(context.Background(), `SELECT count(*) FROM messages WHERE direction = 'out'`).Scan(&count)
	if count != 2 || len(w.e.wa.Sent()) != 2 {
		t.Fatalf("mensagens=%d envios=%d, esperado 2 e 2", count, len(w.e.wa.Sent()))
	}
}

func TestReceiptsAdvanceStatusAndNeverRegress(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Recibo", "férias")
	id := mustConv(t, w.ana, "Cliente Recibo")["id"].(string)
	anaWS := w.ana.websocket(t)
	biaWS := w.bia.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() == 2 })

	expect(t, w.send(w.ana, id, "Olá!"), 201)
	ext := w.e.wa.Sent()[0].MessageID
	lastOut := func() map[string]any {
		ms := w.ana.do("GET", "/api/v1/conversations/"+id+"/messages", nil).body["messages"].([]any)
		return ms[len(ms)-1].(map[string]any)
	}

	w.e.wa.Receipt(whatsapp.ReceiptDelivered, ext)
	if m := lastOut(); m["status"] != "delivered" || m["deliveredAt"] == nil || m["readAt"] != nil {
		t.Fatalf("após entrega: %v", m)
	}
	w.e.wa.Receipt(whatsapp.ReceiptRead, ext)
	m := lastOut()
	if m["status"] != "read" || m["readAt"] == nil || m["deliveredAt"] == nil {
		t.Fatalf("após leitura: %v", m)
	}
	// Recibo de entrega atrasado não faz o status retroceder.
	w.e.wa.Receipt(whatsapp.ReceiptDelivered, ext)
	if lastOut()["status"] != "read" {
		t.Fatal("status retrocedeu")
	}
	// O resumo da lista acompanha.
	if got := mustConv(t, w.ana, "Cliente Recibo")["lastMessage"].(map[string]any)["status"]; got != "read" {
		t.Fatalf("última mensagem na lista = %v", got)
	}
	eventually(t, "message.updated para a equipe", func() bool { return anaWS.count("message.updated") >= 2 })
	if biaWS.count("message.updated") != 0 {
		t.Fatal("recibo vazou para outra equipe")
	}
	// Recibos de mensagens desconhecidas ou recebidas são ignorados.
	w.e.wa.Receipt(whatsapp.ReceiptRead, "NAO-EXISTE")
}

func TestCloseAndNewDemandIsReclassified(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Cliente Recorrente", "preciso das férias")
	id := mustConv(t, w.ana, "Cliente Recorrente")["id"].(string)
	expect(t, w.send(w.ana, id, "Claro, vamos ver."), 201)

	expect(t, w.ana2.do("POST", "/api/v1/conversations/"+id+"/close", nil), 403)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/close", nil), 204)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/close", nil), 204) // idempotente
	// Encerrada some da lista de todos; o detalhe continua acessível (histórico).
	for name, c := range map[string]*client{"ana": w.ana, "ana2": w.ana2, "admin": w.admin} {
		if conv(t, c, "Cliente Recorrente") != nil {
			t.Fatalf("%s ainda vê a conversa encerrada na lista", name)
		}
	}
	if got := w.ana.do("GET", "/api/v1/conversations/"+id, nil); got.status != 200 || got.body["status"] != "closed" {
		t.Fatalf("detalhe da encerrada: %v", got.body)
	}
	expect(t, w.send(w.ana, id, "ainda aí?"), 409, "conversation_closed")

	var ended int
	_ = w.e.pool.QueryRow(context.Background(), `SELECT count(*) FROM conversation_assignments WHERE ended_at IS NOT NULL AND ended_reason = 'closed'`).Scan(&ended)
	if ended != 1 {
		t.Fatalf("atribuições encerradas = %d", ended)
	}

	// Nova demanda do mesmo cliente: nova conversa, classificada de novo, sem responsável.
	w.receiveFrom(phone, "Cliente Recorrente", "agora quero emitir uma nota")
	n := 0
	var fresh map[string]any
	for _, c := range list(t, w.admin) {
		if c["contact"].(map[string]any)["name"] == "Cliente Recorrente" {
			n++
			fresh = c
		}
	}
	if n != 1 || fresh["status"] != "new" || fresh["segment"] != "notas" || fresh["assignee"] != nil || fresh["id"] == id {
		t.Fatalf("conversas do cliente na lista = %d, nova = %v", n, fresh)
	}

	// Admin pode encerrar mesmo sem ser responsável.
	expect(t, w.admin.do("POST", "/api/v1/conversations/"+fresh["id"].(string)+"/close", nil), 204)

	var audits int
	_ = w.e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action IN ('conversation.claim','conversation.close')`).Scan(&audits)
	if audits != 3 { // claim + close (ana) + close (admin)
		t.Fatalf("auditoria = %d, esperado 3", audits)
	}
}

func TestTags(t *testing.T) {
	w := setupWorld(t)
	cat := w.ana.do("GET", "/api/v1/tags", nil)
	expect(t, cat, 200)
	tags := cat.body["tags"].([]any)
	if len(tags) != 5 {
		t.Fatalf("catálogo inicial = %d, esperado 5", len(tags))
	}
	ids := map[string]string{}
	for _, x := range tags {
		m := x.(map[string]any)
		ids[m["name"].(string)] = m["id"].(string)
	}
	for _, n := range []string{"PENDENTE", "URGENTE", "PGDAS", "CERTIFICADO"} {
		if ids[n] == "" {
			t.Fatalf("etiqueta %s ausente", n)
		}
	}

	w.receive("Com Etiqueta", "férias")
	id := mustConv(t, w.ana, "Com Etiqueta")["id"].(string)
	expect(t, w.ana.do("PUT", "/api/v1/conversations/"+id+"/tags/"+ids["URGENTE"], nil), 204)
	expect(t, w.ana.do("PUT", "/api/v1/conversations/"+id+"/tags/"+ids["URGENTE"], nil), 204) // idempotente
	expect(t, w.ana2.do("PUT", "/api/v1/conversations/"+id+"/tags/"+ids["PGDAS"], nil), 204)
	expect(t, w.ana.do("PUT", "/api/v1/conversations/"+id+"/tags/"+uuid.NewString(), nil), 404, "tag_not_found")
	got := mustConv(t, w.ana, "Com Etiqueta")
	if tg := got["tags"].([]any); len(tg) != 2 || tg[0] != "PGDAS" || tg[1] != "URGENTE" || got["segment"] != "folha" {
		t.Fatalf("etiquetas/segmento = %v / %v (etiqueta não pode mudar o segmento)", got["tags"], got["segment"])
	}
	expect(t, w.ana.do("DELETE", "/api/v1/conversations/"+id+"/tags/"+ids["URGENTE"], nil), 204)
	if tg := mustConv(t, w.ana, "Com Etiqueta")["tags"].([]any); len(tg) != 1 {
		t.Fatalf("etiquetas = %v", tg)
	}

	// Catálogo: só admin gerencia.
	expect(t, w.ana.do("POST", "/api/v1/tags", map[string]string{"name": "NOVA"}), 403)
	expect(t, w.ana.do("DELETE", "/api/v1/tags/"+ids["PGDAS"], nil), 403)
	created := w.admin.do("POST", "/api/v1/tags", map[string]string{"name": "  retorno   cliente "})
	expect(t, created, 201)
	if created.body["name"] != "RETORNO CLIENTE" {
		t.Fatalf("nome normalizado = %v", created.body["name"])
	}
	expect(t, w.admin.do("POST", "/api/v1/tags", map[string]string{"name": "retorno cliente"}), 422)
	expect(t, w.admin.do("PATCH", "/api/v1/tags/"+created.body["id"].(string), map[string]string{"name": "Retorno"}), 200)
	expect(t, w.admin.do("DELETE", "/api/v1/tags/"+created.body["id"].(string), nil), 204)
	expect(t, w.admin.do("DELETE", "/api/v1/tags/"+created.body["id"].(string), nil), 404)
}

func TestSegmentAdministration(t *testing.T) {
	w := setupWorld(t)

	// Atendente lê segmentos (sem palavras-chave) mas não altera nada.
	r := w.ana.do("GET", "/api/v1/segments", nil)
	expect(t, r, 200)
	if _, has := r.body["segments"].([]any)[0].(map[string]any)["keywords"]; has {
		t.Fatal("atendente não deveria receber as palavras-chave")
	}
	expect(t, w.ana.do("POST", "/api/v1/segments", map[string]string{"name": "X"}), 403)
	expect(t, w.ana.do("POST", "/api/v1/segments/classify", map[string]string{"text": "x"}), 403)
	expect(t, w.ana.do("PUT", "/api/v1/segments/"+w.segs["folha"]+"/team", map[string]any{"teamId": nil}), 403)

	// Admin vê as palavras-chave padrão e testa uma frase.
	r = w.admin.do("GET", "/api/v1/segments", nil)
	folha := func() map[string]any {
		for _, s := range w.admin.do("GET", "/api/v1/segments", nil).body["segments"].([]any) {
			if s.(map[string]any)["key"] == "folha" {
				return s.(map[string]any)
			}
		}
		return nil
	}
	if kws := folha()["keywords"].([]any); len(kws) < 5 {
		t.Fatalf("palavras-chave padrão da Folha = %d", len(kws))
	}
	test := w.admin.do("POST", "/api/v1/segments/classify", map[string]string{"text": "quero orçamento da folha"})
	expect(t, test, 200)
	if test.body["reason"] != "conflict" || len(test.body["matches"].([]any)) != 2 {
		t.Fatalf("teste de classificação = %v", test.body)
	}

	// Prioridade resolve o conflito.
	kw := w.admin.do("POST", "/api/v1/segments/"+w.segs["folha"]+"/keywords", map[string]any{"phrase": "Folha de Pagamento!", "priority": 0})
	expect(t, kw, 422) // já existe (normalizada igual)
	for _, k := range folha()["keywords"].([]any) {
		m := k.(map[string]any)
		if m["phrase"] == "folha" {
			expect(t, w.admin.do("PATCH", "/api/v1/segments/"+w.segs["folha"]+"/keywords/"+m["id"].(string), map[string]any{"priority": 10}), 204)
		}
	}
	test = w.admin.do("POST", "/api/v1/segments/classify", map[string]string{"text": "quero orçamento da folha"})
	if test.body["reason"] != "keyword" || test.body["segmentKey"] != "folha" {
		t.Fatalf("com prioridade 10: %v", test.body)
	}

	// Validações de palavra-chave.
	expect(t, w.admin.do("POST", "/api/v1/segments/"+w.segs["folha"]+"/keywords", map[string]any{"phrase": "de uma para"}), 422) // só palavras de ligação
	expect(t, w.admin.do("POST", "/api/v1/segments/"+w.segs["folha"]+"/keywords", map[string]any{"phrase": ""}), 422)
	expect(t, w.admin.do("POST", "/api/v1/segments/"+w.segs["folha"]+"/keywords", map[string]any{"phrase": "ponto eletrônico", "priority": 101}), 422)
	added := w.admin.do("POST", "/api/v1/segments/"+w.segs["folha"]+"/keywords", map[string]any{"phrase": "ponto eletrônico"})
	expect(t, added, 201)

	// Nova mensagem usa a configuração atual.
	w.receive("Ponto", "Preciso ajustar o ponto eletrônico")
	if mustConv(t, w.admin, "Ponto")["segment"] != "folha" {
		t.Fatal("palavra-chave recém-cadastrada deveria classificar")
	}
	expect(t, w.admin.do("DELETE", "/api/v1/segments/"+w.segs["folha"]+"/keywords/"+added.body["id"].(string), nil), 204)
	expect(t, w.admin.do("DELETE", "/api/v1/segments/"+w.segs["folha"]+"/keywords/"+added.body["id"].(string), nil), 404)

	// Segmento desativado não classifica.
	expect(t, w.admin.do("PATCH", "/api/v1/segments/"+w.segs["notas"], map[string]any{"isActive": false}), 200)
	w.receive("Sem Notas", "quero emitir nota")
	if got := mustConv(t, w.admin, "Sem Notas"); got["segment"] != "geral" || got["classification"] != "none" {
		t.Fatalf("segmento inativo classificou: %v", got)
	}
	expect(t, w.admin.do("PATCH", "/api/v1/segments/"+w.segs["notas"], map[string]any{"isActive": true}), 200)

	// Novo segmento com equipe; nome duplicado é recusado.
	created := w.admin.do("POST", "/api/v1/segments", map[string]any{"name": "Imposto de Renda", "teamId": w.teams["notas"]})
	expect(t, created, 201)
	if created.body["key"] != "imposto_de_renda" {
		t.Fatalf("chave = %v", created.body["key"])
	}
	expect(t, w.admin.do("POST", "/api/v1/segments", map[string]any{"name": "imposto de renda"}), 422)
	expect(t, w.admin.do("POST", "/api/v1/segments", map[string]any{"name": "Y", "teamId": uuid.NewString()}), 422)
	expect(t, w.admin.do("POST", "/api/v1/segments/"+created.body["id"].(string)+"/keywords", map[string]any{"phrase": "declaração anual"}), 201)
	w.receive("IR", "preciso da declaração anual")
	if mustConv(t, w.bia, "IR")["segment"] != "imposto_de_renda" {
		t.Fatal("novo segmento deveria classificar")
	}

	// Excluir equipe vinculada a segmento é recusado (não pode abrir a fila a todos).
	expect(t, w.admin.do("DELETE", "/api/v1/teams/"+w.teams["notas"], nil), 409, "team_in_use")

	// Excluir segmento devolve as conversas à fila geral (visível a todos).
	expect(t, w.admin.do("DELETE", "/api/v1/segments/"+created.body["id"].(string), nil), 204)
	if got := mustConv(t, w.dani, "IR"); got["segment"] != "geral" {
		t.Fatalf("após excluir o segmento: %v", got["segment"])
	}
}

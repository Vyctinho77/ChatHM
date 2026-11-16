package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/aiaccess"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// agentTokens liga todos os interruptores e cria um token de cada tipo (todos da pessoa administradora).
func agentTokens(t *testing.T) (*world, map[string]string) {
	t.Helper()
	w := setupWorld(t)
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "allowWrite": true, "allowSend": true, "allowAutomation": true, "allowAttend": true}), 200)
	mk := func(name string, provider string, scopes ...string) string {
		body := map[string]any{"name": name, "scopes": scopes}
		if provider != "" {
			body["provider"] = provider
		}
		r := w.admin.do("POST", "/api/v1/ai/tokens", body)
		expect(t, r, 201)
		return r.body["token"].(string)
	}
	return w, map[string]string{
		"read":     mk("leitura", "", "read"),
		"write":    mk("escrita", "", "read", "write"),
		"send":     mk("envio", "", "send"),
		"automate": mk("automatico", "claude", "automate"),
		"attend":   mk("atendente", "grok", "attend"),
	}
}

func accessOf(t *testing.T, w *world, tok string) map[string]map[string]any {
	t.Helper()
	out, isErr := w.tool(tok, "my_access", map[string]any{})
	if isErr {
		t.Fatalf("my_access: %v", out)
	}
	res := map[string]map[string]any{}
	for _, x := range out["capabilities"].([]any) {
		c := x.(map[string]any)
		res[c["capability"].(string)] = c
	}
	return res
}

// A lista de ferramentas é a MESMA para qualquer token e cobre todas as regras de acesso; my_access diz o que cada token pode.
func TestEveryTokenSeesTheSameToolsAndMyAccessExplains(t *testing.T) {
	w, toks := agentTokens(t)
	want := aiaccess.ToolCapabilities()
	var wantNames []string
	for n := range want {
		wantNames = append(wantNames, n)
	}
	sort.Strings(wantNames)
	for kind, tok := range toks {
		got := toolNames(w, tok)
		if len(got) != len(wantNames) {
			t.Fatalf("token %s vê %d ferramentas; esperado %d", kind, len(got), len(wantNames))
		}
		for _, n := range wantNames {
			if !got[n] {
				t.Fatalf("token %s sem a ferramenta %s", kind, n)
			}
		}
	}

	// my_access: cada token sabe o que pode, por que não pode e como liberar.
	r := accessOf(t, w, toks["read"])
	if r["read"]["available"] != true || r["write"]["available"] != false || r["attend"]["available"] != false || r["automate"]["available"] != false {
		t.Fatalf("token de leitura: %v", r)
	}
	if why, _ := r["write"]["why"].(string); why == "" || r["write"]["howToEnable"] == "" {
		t.Fatalf("negativa sem motivo ou caminho: %v", r["write"])
	}
	if accessOf(t, w, toks["write"])["write"]["available"] != true {
		t.Fatal("token de escrita deveria poder alterar")
	}
	if accessOf(t, w, toks["automate"])["automate"]["available"] != true || accessOf(t, w, toks["attend"])["attend"]["available"] != true {
		t.Fatal("tokens automate e attend deveriam poder usar o que têm")
	}
	// O administrador desliga o interruptor: o mesmo token passa a explicar o motivo (e continua vendo a ferramenta).
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "allowWrite": true, "allowSend": true, "allowAutomation": false, "allowAttend": true}), 200)
	if a := accessOf(t, w, toks["automate"])["automate"]; a["available"] != false || a["howToEnable"] == "" {
		t.Fatalf("automate com a chave mestra desligada: %v", a)
	}

	// Biblioteca de PDFs e diretrizes servem ao envio automático OU ao atendente: o atendente usa, o de leitura é recusado.
	if _, isErr := w.tool(toks["attend"], "list_documents", map[string]any{}); isErr {
		t.Fatal("o atendente autônomo precisa listar a biblioteca para anexar PDFs")
	}
	w.expectDenied(toks["read"], "list_documents", map[string]any{}, "escopo automate")
	w.expectDenied(toks["read"], "get_send_guidelines", map[string]any{"category": "das"}, "automate ou ao attend")
}

// Cada mensagem diz quem falou: cliente, pessoa do escritório (e se é a dona do token) ou IA (e se foi este agente).
func TestMessagesCarryStableAuthorIdentity(t *testing.T) {
	w, toks := agentTokens(t)
	phone := w.receive("Cliente Identidade", "Oi, tudo bem?")
	id := mustConv(t, w.dani, "Cliente Identidade")["id"].(string)
	expect(t, w.send(w.dani, id, "Oi! Sou a Dani."), 201) // outra pessoa do escritório (assume)
	var adminID string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT id::text FROM users WHERE email = 'adm@x.com'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	expect(t, w.dani.do("POST", "/api/v1/conversations/"+id+"/transfer", map[string]any{"userId": adminID}), 204)
	expect(t, w.send(w.admin, id, "Aqui é o administrador."), 201) // a pessoa dona dos tokens
	out, _ := w.tool(toks["attend"], "send_as_attendant", map[string]any{"conversation_id": id, "text": "Resposta da IA"})
	if out["ok"] != true {
		t.Fatalf("envio da IA: %v", out)
	}
	w.receiveFrom(phone, "Cliente Identidade", "Obrigado!")

	from := func(tok string) []map[string]any {
		out, isErr := w.tool(tok, "read_messages", map[string]any{"conversation_id": id})
		if isErr {
			t.Fatalf("read_messages: %v", out)
		}
		var list []map[string]any
		for _, x := range out["messages"].([]any) {
			list = append(list, x.(map[string]any)["from"].(map[string]any))
		}
		return list
	}
	// Visto pelo agente atendente (provedor grok): a IA da conversa é ELE.
	got := from(toks["attend"])
	if len(got) != 5 {
		t.Fatalf("mensagens = %d: %v", len(got), got)
	}
	types := []string{got[0]["type"].(string), got[1]["type"].(string), got[2]["type"].(string), got[3]["type"].(string), got[4]["type"].(string)}
	if types[0] != "customer" || types[1] != "staff" || types[2] != "staff" || types[3] != "ai" || types[4] != "customer" {
		t.Fatalf("tipos = %v", types)
	}
	if got[1]["isTokenOwner"] != false || got[1]["id"] == nil || got[1]["id"] == got[2]["id"] {
		t.Fatalf("Dani é outra pessoa, não a dona do token: %v", got[1])
	}
	if got[2]["isTokenOwner"] != true || got[2]["id"] != adminID {
		t.Fatalf("o administrador deveria ser o dono do token: %v", got[2])
	}
	if got[3]["isYou"] != true || got[3]["provider"] != "grok" || got[3]["mode"] != "attend" || got[3]["id"] != "ai:grok" {
		t.Fatalf("IA vista pelo próprio agente: %v", got[3])
	}
	if got[0]["id"] == nil || got[0]["id"] != got[4]["id"] {
		t.Fatalf("o cliente deveria ter a mesma identidade nas duas mensagens: %v %v", got[0], got[4])
	}
	// Outro agente da mesma pessoa (provedor diferente) vê a mesma IA como OUTRA.
	if ai := from(toks["automate"])[3]; ai["isYou"] != false || ai["id"] != "ai:grok" {
		t.Fatalf("IA vista por outro agente: %v", ai)
	}
	// A última mensagem da lista de conversas também traz quem falou.
	out2, _ := w.tool(toks["read"], "list_conversations", map[string]any{})
	last := out2["conversations"].([]any)[0].(map[string]any)["lastMessage"].(map[string]any)
	if last["from"].(map[string]any)["type"] != "customer" {
		t.Fatalf("lastMessage.from = %v", last["from"])
	}
}

// Respostas pendentes: o agente lê, avisa e marca; cada agente guarda o seu; resposta nova faz voltar; nunca responde ao cliente.
func TestPendingRepliesWorkflow(t *testing.T) {
	w, toks := agentTokens(t)
	phone := w.receive("Cliente Pendente", "Bom dia")
	id := mustConv(t, w.dani, "Cliente Pendente")["id"].(string)
	expect(t, w.send(w.dani, id, "Bom dia! Vou verificar."), 201) // a conversa passa a ser da Dani
	out, _ := w.tool(toks["attend"], "send_as_attendant", map[string]any{"conversation_id": id, "text": "Seu DAS vence dia 20."})
	if out["ok"] != true {
		t.Fatalf("envio da IA: %v", out)
	}
	// Antes de o cliente responder nada está pendente.
	if o, _ := w.tool(toks["attend"], "list_pending_replies", map[string]any{}); o["total"] != float64(0) {
		t.Fatalf("antes da resposta: %v", o)
	}
	w.receiveFrom(phone, "Cliente Pendente", "Obrigado, já paguei!")
	sentBefore := len(w.e.wa.Sent())

	o, isErr := w.tool(toks["attend"], "list_pending_replies", map[string]any{})
	if isErr || o["total"] != float64(1) {
		t.Fatalf("pendentes: %v", o)
	}
	item := o["conversations"].([]any)[0].(map[string]any)
	last := item["lastReply"].(map[string]any)
	if item["conversationId"] != id || item["pending"].(map[string]any)["count"] != float64(1) || last["text"] != "Obrigado, já paguei!" || last["from"].(map[string]any)["type"] != "customer" {
		t.Fatalf("item = %v", item)
	}
	if item["queue"] == nil || item["contact"] == nil || item["answeredAfter"] != nil {
		t.Fatalf("faltam fila/contato ou já aparece como respondida: %v", item)
	}

	// Quem não vê a conversa não a vê pendente (ana não é da Dani nem administradora).
	anaTok := w.mkScopedToken(w.ana, "ana", "read")
	if o, _ := w.tool(anaTok, "list_pending_replies", map[string]any{}); o["total"] != float64(0) {
		t.Fatalf("ana não deveria ver: %v", o)
	}

	// Marcar: dados inválidos viram erro por item; o válido é registrado.
	m, isErr := w.tool(toks["attend"], "mark_replies", map[string]any{"items": []map[string]any{
		{"conversation_id": id, "status": "notified", "note": "Avisei a pessoa."},
		{"conversation_id": id, "status": "feito"},
		{"conversation_id": "00000000-0000-0000-0000-000000000000", "status": "resolved"},
	}})
	if isErr || m["marked"] != float64(1) || m["ok"] != false {
		t.Fatalf("mark_replies: %v", m)
	}
	if isErr, _ := w.toolCall(toks["attend"], "mark_replies", map[string]any{"items": []map[string]any{}}); !isErr {
		t.Fatal("lista vazia deveria ser recusada")
	}
	if o, _ := w.tool(toks["attend"], "list_pending_replies", map[string]any{}); o["total"] != float64(0) {
		t.Fatalf("depois de avisar: %v", o)
	}
	h, _ := w.tool(toks["attend"], "list_pending_replies", map[string]any{"include_handled": true})
	handled := h["handled"].([]any)
	if len(handled) != 1 || handled[0].(map[string]any)["status"] != "notified" || handled[0].(map[string]any)["note"] != "Avisei a pessoa." {
		t.Fatalf("handled = %v", h["handled"])
	}
	// Cada agente guarda o seu: outro agente (provedor diferente) ainda vê a resposta pendente.
	if o, _ := w.tool(toks["automate"], "list_pending_replies", map[string]any{}); o["total"] != float64(1) {
		t.Fatalf("outro agente: %v", o)
	}

	// O cliente escreve de novo: volta para a lista (só a resposta nova), e uma pessoa que já respondeu aparece em answeredAfter.
	w.receiveFrom(phone, "Cliente Pendente", "Qual era o valor mesmo?")
	o, _ = w.tool(toks["attend"], "list_pending_replies", map[string]any{})
	if o["total"] != float64(1) || o["conversations"].([]any)[0].(map[string]any)["pending"].(map[string]any)["count"] != float64(1) {
		t.Fatalf("resposta nova: %v", o)
	}
	expect(t, w.send(w.dani, id, "R$ 150,00."), 201)
	o, _ = w.tool(toks["attend"], "list_pending_replies", map[string]any{})
	after := o["conversations"].([]any)[0].(map[string]any)["answeredAfter"].(map[string]any)
	if after["from"].(map[string]any)["type"] != "staff" {
		t.Fatalf("answeredAfter = %v", after)
	}
	w.tool(toks["attend"], "mark_replies", map[string]any{"items": []map[string]any{{"conversation_id": id, "status": "resolved"}}})
	if o, _ := w.tool(toks["attend"], "list_pending_replies", map[string]any{}); o["total"] != float64(0) {
		t.Fatalf("resolvida: %v", o)
	}
	// Só marcar e listar: a única coisa enviada ao cliente nesse período foi o que a Dani escreveu.
	if got := len(w.e.wa.Sent()) - sentBefore; got != 1 {
		t.Fatalf("mensagens enviadas ao cliente = %d (só a da Dani)", got)
	}
}

func TestWaitForRepliesReturnsAsSoonAsAReplyArrives(t *testing.T) {
	w, toks := agentTokens(t)
	phone := w.receive("Cliente Espera", "Oi")
	id := mustConv(t, w.dani, "Cliente Espera")["id"].(string)
	expect(t, w.send(w.dani, id, "Olá!"), 201)
	if out, _ := w.tool(toks["attend"], "send_as_attendant", map[string]any{"conversation_id": id, "text": "Seu DAS vence dia 20."}); out["ok"] != true {
		t.Fatalf("envio da IA: %v", out)
	}

	// Nada pendente: espera o prazo curto e volta vazio, avisando que acabou o tempo.
	start := time.Now()
	o, isErr := w.tool(toks["attend"], "wait_for_replies", map[string]any{"timeout_seconds": 1})
	if isErr || o["timedOut"] != true || o["total"] != float64(0) || time.Since(start) < time.Second {
		t.Fatalf("sem resposta: %v (%v)", o, time.Since(start))
	}

	// A resposta chega enquanto o agente espera: volta antes do prazo, sem timedOut.
	go func() {
		time.Sleep(1200 * time.Millisecond)
		w.receiveFrom(phone, "Cliente Espera", "Já paguei, obrigado!")
	}()
	start = time.Now()
	o, isErr = w.tool(toks["attend"], "wait_for_replies", map[string]any{"timeout_seconds": 20})
	if isErr || o["timedOut"] != false || o["total"] != float64(1) || time.Since(start) > 15*time.Second {
		t.Fatalf("com resposta: %v (%v)", o, time.Since(start))
	}
	// Já pendente: devolve na hora.
	start = time.Now()
	if o, _ = w.tool(toks["attend"], "wait_for_replies", map[string]any{"timeout_seconds": 20}); o["total"] != float64(1) || time.Since(start) > 2*time.Second {
		t.Fatalf("já pendente: %v", o)
	}
	// Depois da baixa, volta a esperar.
	w.tool(toks["attend"], "mark_replies", map[string]any{"items": []map[string]any{{"conversation_id": id, "status": "resolved"}}})
	if o, _ = w.tool(toks["attend"], "wait_for_replies", map[string]any{"timeout_seconds": 1}); o["timedOut"] != true {
		t.Fatalf("após baixa: %v", o)
	}
}

// Quem não tem endereço público consulta o ChatHM por HTTP simples (GET /mcp/replies), sem cliente MCP.
func TestRepliesOverPlainHTTP(t *testing.T) {
	w, toks := agentTokens(t)
	get := func(token, query string) (int, map[string]any) {
		req, _ := http.NewRequest("GET", w.e.srv.URL+"/mcp/replies"+query, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	if st, _ := get("", ""); st != 401 {
		t.Fatalf("sem token: %d", st)
	}
	if st, _ := get("chm_invalido", ""); st != 401 {
		t.Fatalf("token inválido: %d", st)
	}

	phone := w.receive("Cliente Http", "Oi")
	id := mustConv(t, w.dani, "Cliente Http")["id"].(string)
	expect(t, w.send(w.dani, id, "Olá!"), 201)
	if out, _ := w.tool(toks["attend"], "send_as_attendant", map[string]any{"conversation_id": id, "text": "Seu DAS vence dia 20."}); out["ok"] != true {
		t.Fatalf("envio da IA: %v", out)
	}
	start := time.Now()
	if st, o := get(toks["attend"], "?wait=1"); st != 200 || o["timedOut"] != true || o["total"] != float64(0) || time.Since(start) < time.Second {
		t.Fatalf("sem resposta: %d %v", st, o)
	}
	go func() {
		time.Sleep(1200 * time.Millisecond)
		w.receiveFrom(phone, "Cliente Http", "Já paguei, obrigado!")
	}()
	start = time.Now()
	st, o := get(toks["attend"], "?wait=40")
	if st != 200 || o["timedOut"] != false || o["total"] != float64(1) || time.Since(start) > 15*time.Second {
		t.Fatalf("com resposta: %d %v (%v)", st, o, time.Since(start))
	}
	item := o["conversations"].([]any)[0].(map[string]any)
	if item["conversationId"] != id || item["lastReply"].(map[string]any)["text"] != "Já paguei, obrigado!" {
		t.Fatalf("item: %v", item)
	}
	if o["aviso"] == nil {
		t.Fatalf("falta o aviso de conteúdo de terceiros: %v", o)
	}
}

// O WhatsApp costuma avisar a leitura só da última mensagem: as enviadas antes dela na conversa também passam a lidas; as depois, não.
func TestReadReceiptAlsoMarksEarlierMessagesAsRead(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Cadeia", "Oi")
	id := mustConv(t, w.ana, "Cliente Cadeia")["id"].(string)
	for _, txt := range []string{"um", "dois", "tres", "quatro"} {
		expect(t, w.send(w.ana, id, txt), 201)
	}
	sent := w.e.wa.Sent()
	statuses := func() map[string]string {
		out := map[string]string{}
		for _, x := range w.ana.do("GET", "/api/v1/conversations/"+id+"/messages", nil).body["messages"].([]any) {
			m := x.(map[string]any)
			if m["direction"] == "out" {
				out[m["text"].(string)], _ = m["status"].(string)
			}
		}
		return out
	}
	// Leitura da TERCEIRA: a primeira, a segunda e a terceira ficam lidas; a quarta continua como estava.
	w.e.wa.Receipt(whatsapp.ReceiptRead, sent[2].MessageID)
	eventually(t, "leitura em cadeia", func() bool { s := statuses(); return s["um"] == "read" && s["dois"] == "read" && s["tres"] == "read" })
	if s := statuses(); s["quatro"] == "read" || s["quatro"] == "delivered" {
		t.Fatalf("a mensagem seguinte não foi lida: %v", s)
	}
	// Entrega da quarta não mexe nas lidas (nunca retrocede).
	w.e.wa.Receipt(whatsapp.ReceiptDelivered, sent[3].MessageID)
	eventually(t, "entrega da última", func() bool { return statuses()["quatro"] == "delivered" })
	if s := statuses(); s["um"] != "read" || s["tres"] != "read" {
		t.Fatalf("status retrocedeu: %v", s)
	}
}

func agentToken(t *testing.T, w *world, name, provider string, scopes ...string) string {
	t.Helper()
	r := w.admin.do("POST", "/api/v1/ai/tokens", map[string]any{"name": name, "provider": provider, "scopes": scopes})
	expect(t, r, 201)
	return r.body["token"].(string)
}

// message_status: o agente vê se a mensagem chegou e se foi lida antes de reenviar; check_send diz o que bloquearia o envio.
func TestMessageStatusAndCheckSend(t *testing.T) {
	w, toks := agentTokens(t)
	phone := w.receive("Cliente Status", "Oi")
	_ = phone
	id := mustConv(t, w.dani, "Cliente Status")["id"].(string)
	expect(t, w.send(w.dani, id, "Olá!"), 201)
	tok := toks["attend"]

	// Antes de enviar: o envio como atendente passaria.
	chk, isErr := w.tool(tok, "check_send", map[string]any{"conversation_id": id})
	if isErr || chk["wouldSend"] != true || len(chk["blockers"].([]any)) != 0 || chk["contact"].(map[string]any)["lastCustomerMessageAt"] == nil {
		t.Fatalf("check_send attendant: %v", chk)
	}
	// Automático: a categoria não está liberada e a conversa está com a Dani, e as duas coisas aparecem juntas ou em sequência.
	chk, isErr = w.tool(toks["automate"], "check_send", map[string]any{"conversation_id": id, "mode": "automatic", "category": "aviso"})
	codes := map[string]bool{}
	for _, b := range chk["blockers"].([]any) {
		codes[b.(map[string]any)["code"].(string)] = true
	}
	if isErr || chk["wouldSend"] != false || len(codes) == 0 {
		t.Fatalf("check_send automatic deveria bloquear: %v", chk)
	}
	if isErr, msg := w.toolCall(toks["automate"], "check_send", map[string]any{"conversation_id": id, "mode": "automatic"}); !isErr || msg == "" {
		t.Fatalf("automatic sem category deve explicar o erro: %v %q", isErr, msg)
	}

	out, _ := w.tool(tok, "send_as_attendant", map[string]any{"conversation_id": id, "text": "Seu DAS vence dia 20."})
	if out["ok"] != true || out["messageId"] == nil {
		t.Fatalf("envio: %v", out)
	}
	msgID := out["messageId"].(string)
	ext := w.e.wa.Sent()[len(w.e.wa.Sent())-1].MessageID

	st, isErr := w.tool(tok, "message_status", map[string]any{"message_id": msgID})
	if isErr || st["arrived"] != false || st["read"] != false || st["andamento"] == nil || st["customerWroteAfter"] != false {
		t.Fatalf("recém-enviada: %v", st)
	}
	w.e.wa.Receipt(whatsapp.ReceiptDelivered, ext)
	eventually(t, "entregue", func() bool {
		st, _ = w.tool(tok, "message_status", map[string]any{"message_id": msgID})
		return st["status"] == "delivered"
	})
	if st["arrived"] != true || st["read"] != false || st["deliveredAt"] == nil {
		t.Fatalf("entregue: %v", st)
	}
	w.e.wa.Receipt(whatsapp.ReceiptRead, ext)
	eventually(t, "lida", func() bool {
		st, _ = w.tool(tok, "message_status", map[string]any{"message_id": msgID})
		return st["status"] == "read"
	})
	if st["read"] != true || st["readAt"] == nil {
		t.Fatalf("lida: %v", st)
	}
	// read_messages também traz os recibos.
	msgs, _ := w.tool(tok, "read_messages", map[string]any{"conversation_id": id})
	found := false
	for _, x := range msgs["messages"].([]any) {
		if m := x.(map[string]any); m["id"] == msgID && m["readAt"] != nil && m["deliveredAt"] != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("read_messages sem recibos: %v", msgs)
	}
	// Mensagem do cliente não tem entrega; id inventado e quem não vê a conversa recebem a mesma recusa.
	if isErr, _ := w.toolCall(tok, "message_status", map[string]any{"message_id": "00000000-0000-0000-0000-000000000000"}); !isErr {
		t.Fatal("id inexistente deveria ser recusado")
	}
	anaTok := w.mkScopedToken(w.ana, "ana", "read")
	if isErr, _ := w.toolCall(anaTok, "message_status", map[string]any{"message_id": msgID}); !isErr {
		t.Fatal("quem não vê a conversa não pode consultar")
	}
}

// Dois agentes da mesma pessoa e do mesmo provedor: cada um com o seu nome, a sua fila de pendentes e o seu "isYou".
func TestTwoAgentsOfTheSamePersonStaySeparate(t *testing.T) {
	w, _ := agentTokens(t)
	fin := agentToken(t, w, "HM Financeiro", "claude", "attend")
	chefao := agentToken(t, w, "Chefão", "claude", "attend")
	phone := w.receive("Cliente Dois", "Oi")
	id := mustConv(t, w.dani, "Cliente Dois")["id"].(string)
	expect(t, w.send(w.dani, id, "Olá!"), 201)
	if out, _ := w.tool(fin, "send_as_attendant", map[string]any{"conversation_id": id, "text": "Seu boleto vence dia 20."}); out["ok"] != true {
		t.Fatalf("envio: %v", out)
	}
	fromOf := func(tok string) map[string]any {
		msgs, _ := w.tool(tok, "read_messages", map[string]any{"conversation_id": id})
		for _, x := range msgs["messages"].([]any) {
			if f := x.(map[string]any)["from"].(map[string]any); f["type"] == "ai" {
				return f
			}
		}
		t.Fatal("sem mensagem de IA")
		return nil
	}
	if f := fromOf(fin); f["agent"] != "HM Financeiro" || f["isYou"] != true {
		t.Fatalf("para o próprio agente: %v", f)
	}
	if f := fromOf(chefao); f["agent"] != "HM Financeiro" || f["isYou"] != false {
		t.Fatalf("o outro agente da mesma pessoa não é você: %v", f)
	}
	// Quem enviou a mensagem aparece pelo nome na própria conversa para a equipe.
	var shown string
	for _, x := range w.dani.do("GET", "/api/v1/conversations/"+id+"/messages", nil).body["messages"].([]any) {
		if m := x.(map[string]any); m["automated"] == true {
			shown, _ = m["aiAgent"].(string)
		}
	}
	if shown != "HM Financeiro" {
		t.Fatalf("a equipe vê aiAgent = %q", shown)
	}
	// Respostas pendentes são por agente: marcar num não baixa no outro.
	w.receiveFrom(phone, "Cliente Dois", "Obrigado!")
	for name, tok := range map[string]string{"fin": fin, "chefao": chefao} {
		if o, _ := w.tool(tok, "list_pending_replies", map[string]any{}); o["total"] != float64(1) {
			t.Fatalf("%s deveria ver 1 pendente: %v", name, o)
		}
	}
	w.tool(fin, "mark_replies", map[string]any{"items": []map[string]any{{"conversation_id": id, "status": "resolved"}}})
	if o, _ := w.tool(fin, "list_pending_replies", map[string]any{}); o["total"] != float64(0) {
		t.Fatalf("fin deveria ter baixado: %v", o)
	}
	if o, _ := w.tool(chefao, "list_pending_replies", map[string]any{}); o["total"] != float64(1) {
		t.Fatalf("a baixa do fin não pode apagar a do Chefão: %v", o)
	}
}

// Contato ligado ao cadastro financeiro: CNPJ/CPF e id do Asaas, achados sem casar por telefone.
func TestContactReferencesFindAndLinkCharges(t *testing.T) {
	w, toks := agentTokens(t)
	// Para o agente financeiro ver o CNPJ, o administrador não mascara documentos.
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": false, "allowWrite": true, "allowSend": true, "allowAutomation": true, "allowAttend": true}), 200)
	w.receive("Cliente Financeiro", "Oi")
	conv := mustConv(t, w.admin, "Cliente Financeiro")
	cid := conv["contact"].(map[string]any)["id"].(string)

	// Pela tela (equipe): valida e guarda só os dígitos.
	expect(t, w.admin.do("PATCH", "/api/v1/contacts/"+cid, map[string]any{"taxId": "11.222.333/0001-82"}), 422) // dígito verificador errado
	expect(t, w.admin.do("PATCH", "/api/v1/contacts/"+cid, map[string]any{"asaasId": "000005401844"}), 422)     // sem cus_
	// Uma cobrança do Asaas sem cliente ligado (vinda da sincronização).
	if _, err := w.e.pool.Exec(t.Context(), `INSERT INTO billing_charges (provider, external_id, customer_ref, customer_name, description, amount_cents, due_date, status)
		VALUES ('asaas', 'pay_1', 'cus_000005401844', 'Financeiro Ltda', 'Honorários', 15000, current_date, 'open')`); err != nil {
		t.Fatal(err)
	}
	expect(t, w.admin.do("PATCH", "/api/v1/contacts/"+cid, map[string]any{"taxId": "11.222.333/0001-81", "asaasId": "cus_000005401844"}), 204)
	var linked *string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT contact_id::text FROM billing_charges WHERE external_id = 'pay_1'`).Scan(&linked); err != nil || linked == nil || *linked != cid {
		t.Fatalf("a cobrança do Asaas deveria ter sido ligada ao contato: %v %v", linked, err)
	}
	got := mustConv(t, w.admin, "Cliente Financeiro")["contact"].(map[string]any)
	if got["taxId"] != "11222333000181" || got["asaasId"] != "cus_000005401844" {
		t.Fatalf("a tela recebe os campos: %v", got)
	}

	// O agente acha o contato pelo CNPJ (com pontuação) ou pelo id do Asaas, e vê as referências.
	for _, args := range []map[string]any{{"tax_id": "11.222.333/0001-81"}, {"asaas_id": "cus_000005401844"}} {
		o, isErr := w.tool(toks["read"], "find_contact", args)
		if isErr || o["total"] != float64(1) {
			t.Fatalf("find_contact %v: %v", args, o)
		}
		c := o["contacts"].([]any)[0].(map[string]any)
		if c["id"] != cid || c["asaasId"] != "cus_000005401844" || c["document"] != "11.222.333/0001-81" {
			t.Fatalf("contato achado: %v", c)
		}
	}
	if o, _ := w.tool(toks["read"], "find_contact", map[string]any{"asaas_id": "cus_999999999999"}); o["total"] != float64(0) {
		t.Fatalf("id desconhecido: %v", o)
	}
	for _, bad := range []map[string]any{{}, {"tax_id": "123"}, {"tax_id": "11222333000182"}, {"asaas_id": "xyz"}, {"tax_id": "11222333000181", "asaas_id": "cus_000005401844"}} {
		if isErr, _ := w.toolCall(toks["read"], "find_contact", bad); !isErr {
			t.Fatalf("%v deveria ser recusado", bad)
		}
	}
	// A busca geral também acha pelo id do Asaas.
	if o, _ := w.tool(toks["read"], "search", map[string]any{"q": "cus_000005401844"}); len(o["contacts"].([]any)) != 1 {
		t.Fatalf("search pelo id do Asaas: %v", o)
	}

	// Quem não pode alterar não grava; quem pode, grava e apaga.
	if isErr, _ := w.toolCall(toks["read"], "set_contact_reference", map[string]any{"contact_id": cid, "tax_id": "52998224725"}); !isErr {
		t.Fatal("token só de leitura não grava referências")
	}
	o, isErr := w.tool(toks["write"], "set_contact_reference", map[string]any{"contact_id": cid, "tax_id": "529.982.247-25", "asaas_id": ""})
	if isErr || o["ok"] != true {
		t.Fatalf("set_contact_reference: %v", o)
	}
	if o, _ := w.tool(toks["read"], "find_contact", map[string]any{"tax_id": "52998224725"}); o["total"] != float64(1) {
		t.Fatalf("CPF novo: %v", o)
	}
	if o, _ := w.tool(toks["read"], "find_contact", map[string]any{"asaas_id": "cus_000005401844"}); o["total"] != float64(0) {
		t.Fatalf("asaas_id apagado não deveria achar: %v", o)
	}

	// Com documentos mascarados, o valor não aparece para o agente, mas ele ainda acha o contato pelo CNPJ que já conhece.
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "allowWrite": true, "allowSend": true, "allowAutomation": true, "allowAttend": true}), 200)
	o, _ = w.tool(toks["read"], "find_contact", map[string]any{"tax_id": "52998224725"})
	if o["total"] != float64(1) {
		t.Fatalf("achar por CNPJ continua funcionando: %v", o)
	}
	if c := o["contacts"].([]any)[0].(map[string]any); c["document"] == "529.982.247-25" {
		t.Fatalf("o documento deveria sair mascarado: %v", c)
	}
}

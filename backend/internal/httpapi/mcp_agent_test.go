package httpapi

import (
	"sort"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/aiaccess"
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

package httpapi

// Transferência de conversa, encerradas fora da lista e histórico do cliente que volta.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func (w *world) transfer(c *client, convID, userID string) resp {
	return c.do("POST", "/api/v1/conversations/"+convID+"/transfer", map[string]any{"userId": userID})
}

func (w *world) assignee(t *testing.T, convID string) any {
	t.Helper()
	return w.admin.do("GET", "/api/v1/conversations/"+convID, nil).body["assignee"]
}

func TestTransferBetweenPeople(t *testing.T) {
	w := setupWorld(t)
	anaWS, ana2WS, biaWS := w.ana.websocket(t), w.ana2.websocket(t), w.bia.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() == 3 })

	w.receive("Cliente Transfer", "férias")
	id := mustConv(t, w.ana, "Cliente Transfer")["id"].(string)

	// Sem responsável: quem vê a conversa pode passá-la a um colega (e até a outra equipe).
	expect(t, w.transfer(w.ana, id, w.users["ana2"]), 204)
	got := mustConv(t, w.ana2, "Cliente Transfer")
	if got["assignee"].(map[string]any)["id"] != w.users["ana2"] || got["status"] != "in_progress" {
		t.Fatalf("após transferir: %v", got)
	}
	eventually(t, "ana2 recebe a atualização", func() bool { return ana2WS.count("conversation.updated") >= 1 })

	// Agora só ana2 (ou o admin) transfere e responde; ana, da mesma equipe, não.
	expect(t, w.transfer(w.ana, id, w.users["ana"]), 409, "conversation_taken")
	expect(t, w.send(w.ana, id, "oi"), 409, "conversation_taken")
	expect(t, w.send(w.ana2, id, "Oi, agora é comigo."), 201)

	// Transferir para alguém de OUTRA equipe: a pessoa passa a ver e atender.
	if conv(t, w.bia, "Cliente Transfer") != nil {
		t.Fatal("bia (Notas) não deveria ver a conversa da Folha antes da transferência")
	}
	expect(t, w.transfer(w.ana2, id, w.users["bia"]), 204)
	if got := mustConv(t, w.bia, "Cliente Transfer"); got["assignee"].(map[string]any)["id"] != w.users["bia"] {
		t.Fatalf("bia deveria ser a responsável: %v", got)
	}
	eventually(t, "bia ganha a conversa", func() bool { return biaWS.count("conversation.updated") >= 1 })
	expect(t, w.send(w.bia, id, "Aqui é a bia, vou continuar."), 201)
	expect(t, w.send(w.ana2, id, "ainda posso?"), 409, "conversation_taken")

	// A equipe da Folha continua vendo (a conversa pertence ao segmento dela).
	if conv(t, w.ana, "Cliente Transfer") == nil {
		t.Fatal("a equipe do segmento deveria continuar vendo a conversa")
	}
	_ = anaWS

	// Admin pode transferir mesmo sem ser o responsável; transferir para quem já é o responsável não muda nada.
	expect(t, w.transfer(w.admin, id, w.users["ana"]), 204)
	expect(t, w.transfer(w.admin, id, w.users["ana"]), 204)

	// Histórico de atribuições: uma ativa, as demais encerradas por transferência.
	ctx := context.Background()
	var active, transferred int
	_ = w.e.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_assignments WHERE conversation_id = $1 AND ended_at IS NULL`, id).Scan(&active)
	_ = w.e.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_assignments WHERE conversation_id = $1 AND ended_reason = 'transferred'`, id).Scan(&transferred)
	if active != 1 || transferred != 2 {
		var total int
		_ = w.e.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_assignments WHERE conversation_id = $1`, id).Scan(&total)
		t.Fatalf("ativas=%d transferidas=%d total=%d", active, transferred, total)
	}
	var audits int
	_ = w.e.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'conversation.transfer'`).Scan(&audits)
	if audits != 3 {
		t.Fatalf("auditoria de transferências = %d, esperado 3 (a repetição não conta)", audits)
	}
}

func TestTransferRules(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Regras", "férias")
	id := mustConv(t, w.ana, "Cliente Regras")["id"].(string)

	// Quem não vê a conversa não transfere (404, sem revelar que existe).
	expect(t, w.transfer(w.bia, id, w.users["bia"]), 404)
	expect(t, w.transfer(w.dani, id, w.users["dani"]), 404)

	// Alvo inválido, inexistente ou desativado.
	expect(t, w.transfer(w.ana, id, "não-é-uuid"), 422)
	expect(t, w.transfer(w.ana, id, uuid.NewString()), 422)
	expect(t, w.admin.do("PATCH", "/api/v1/users/"+w.users["caio"], map[string]any{"isActive": false}), 200)
	expect(t, w.transfer(w.ana, id, w.users["caio"]), 422)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/transfer", map[string]any{}), 422)

	// Encerrada não se transfere.
	expect(t, w.send(w.ana, id, "Oi"), 201)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/close", nil), 204)
	expect(t, w.transfer(w.ana, id, w.users["ana2"]), 409, "conversation_closed")
}

func TestTransferFromGeneralQueueMakesItPrivate(t *testing.T) {
	w := setupWorld(t)
	w.receive("Geral Transfer", "bom dia")
	id := mustConv(t, w.dani, "Geral Transfer")["id"].(string)
	anaWS, biaWS := w.ana.websocket(t), w.bia.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() == 2 })

	// dani passa a conversa geral para caio: os demais deixam de vê-la, ao vivo.
	expect(t, w.transfer(w.dani, id, w.users["caio"]), 204)
	mustConv(t, w.caio, "Geral Transfer")
	for name, c := range map[string]*client{"ana": w.ana, "bia": w.bia, "dani": w.dani} {
		if conv(t, c, "Geral Transfer") != nil {
			t.Fatalf("%s ainda vê a conversa geral transferida a outra pessoa", name)
		}
	}
	eventually(t, "remoções", func() bool {
		return anaWS.count("conversation.removed") == 1 && biaWS.count("conversation.removed") == 1
	})
	mustConv(t, w.admin, "Geral Transfer")
}

func TestClosedConversationLeavesEveryListInRealtime(t *testing.T) {
	w := setupWorld(t)
	w.receive("Some Da Lista", "férias")
	id := mustConv(t, w.ana, "Some Da Lista")["id"].(string)
	expect(t, w.send(w.ana, id, "Vou cuidar disso."), 201)
	ana2WS := w.ana2.websocket(t)
	biaWS := w.bia.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() == 2 })

	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/close", nil), 204)
	eventually(t, "colega da equipe remove da lista", func() bool { return ana2WS.count("conversation.removed") == 1 })
	if biaWS.count("conversation.removed") != 0 || biaWS.count("conversation.updated") != 0 {
		t.Fatal("outra equipe não deveria receber eventos desta conversa")
	}
}

func TestReturningContactSeesOwnTeamHistoryOnly(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Cliente Volta", "preciso das férias")
	id1 := mustConv(t, w.ana, "Cliente Volta")["id"].(string)
	expect(t, w.send(w.ana, id1, "Resposta do primeiro atendimento."), 201)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id1+"/close", nil), 204)

	texts := func(c *client, convID string) []string {
		var out []string
		for _, m := range c.do("GET", "/api/v1/conversations/"+convID+"/messages", nil).body["messages"].([]any) {
			if s, ok := m.(map[string]any)["text"].(string); ok {
				out = append(out, s)
			}
		}
		return out
	}

	// Mesmo assunto: volta à fila da Folha; quem atende vê o histórico do atendimento anterior.
	w.receiveFrom(phone, "Cliente Volta", "quero tirar mais férias")
	id2 := mustConv(t, w.ana2, "Cliente Volta")["id"].(string)
	if id2 == id1 {
		t.Fatal("deveria ser uma nova conversa")
	}
	got := texts(w.ana2, id2)
	if len(got) != 3 || got[0] != "preciso das férias" || got[1] != "Resposta do primeiro atendimento." || got[2] != "quero tirar mais férias" {
		t.Fatalf("histórico = %v", got)
	}
	// O histórico vem com o id da conversa de origem (a interface marca "novo atendimento").
	ms := w.ana2.do("GET", "/api/v1/conversations/"+id2+"/messages", nil).body["messages"].([]any)
	if ms[0].(map[string]any)["conversationId"] != id1 || ms[2].(map[string]any)["conversationId"] != id2 {
		t.Fatalf("conversationId das mensagens: %v", ms)
	}

	// Outro assunto: vai para a fila de Notas; quem atende lá NÃO vê o atendimento anterior da Folha.
	w.receiveFrom(phone, "Cliente Volta", "e quero emitir uma nota")
	// (a conversa aberta continua a da Folha: nada muda de fila por palavras isoladas)
	if conv(t, w.bia, "Cliente Volta") != nil {
		t.Fatal("conversa ativa não deveria mudar de fila")
	}
	// Encerra a da Folha e o cliente pede nota: nova conversa, agora na fila de Notas.
	expect(t, w.send(w.ana2, id2, "ok"), 201)
	expect(t, w.ana2.do("POST", "/api/v1/conversations/"+id2+"/close", nil), 204)
	w.receiveFrom(phone, "Cliente Volta", "quero emitir uma nota fiscal")
	id3 := mustConv(t, w.bia, "Cliente Volta")["id"].(string)
	if got := texts(w.bia, id3); len(got) != 1 || got[0] != "quero emitir uma nota fiscal" {
		t.Fatalf("bia (Notas) não deveria ver o histórico da Folha: %v", got)
	}
	if conv(t, w.ana, "Cliente Volta") != nil {
		t.Fatal("ana (Folha) não deveria ver o pedido de Notas: continua fora da lista dela")
	}
	// Admin vê tudo.
	if got := texts(w.admin, id3); len(got) < 5 {
		t.Fatalf("admin deveria ver o histórico completo: %v", got)
	}
}

func TestPeopleListsTheWholeOffice(t *testing.T) {
	w := setupWorld(t)
	for name, c := range map[string]*client{"dani": w.dani, "ana": w.ana, "admin": w.admin} {
		r := c.do("GET", "/api/v1/people", nil)
		expect(t, r, 200)
		ps := r.body["people"].([]any)
		if len(ps) != 6 { // admin, ana, ana2, bia, caio, dani
			t.Fatalf("%s vê %d pessoas, esperado 6", name, len(ps))
		}
		for _, p := range ps {
			m := p.(map[string]any)
			for _, secret := range []string{"email", "password", "passwordHash", "mfaEnabled"} {
				if _, has := m[secret]; has {
					t.Fatalf("a lista de pessoas expôs %q", secret)
				}
			}
		}
	}
	// Equipes aparecem; desativados não.
	expect(t, w.admin.do("PATCH", "/api/v1/users/"+w.users["caio"], map[string]any{"isActive": false}), 200)
	ps := w.ana.do("GET", "/api/v1/people", nil).body["people"].([]any)
	if len(ps) != 5 {
		t.Fatalf("desativado deveria sair: %d", len(ps))
	}
	for _, p := range ps {
		m := p.(map[string]any)
		if m["name"] == "ana" && (len(m["teams"].([]any)) != 1 || m["teams"].([]any)[0] != "Folha") {
			t.Fatalf("equipes da ana = %v", m["teams"])
		}
	}
	expect(t, e2eAnon(w), 401)
}

func e2eAnon(w *world) resp { return w.e.client().do("GET", "/api/v1/people", nil) }

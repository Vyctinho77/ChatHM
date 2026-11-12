package httpapi

// Encerrar só a própria parte (ou para todos): responsável, participantes, administrador de fora e o cliente que volta.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

func (w *world) closeConv(c *client, id string, body any) resp {
	return c.do("POST", "/api/v1/conversations/"+id+"/close", body)
}

// attended cria uma conversa da fila de Folha, já atendida por ana.
func (w *world) attended(t *testing.T, name string) (id, phone string) {
	t.Helper()
	phone = w.receive(name, "folha de pagamento")
	id = mustConv(t, w.admin, name)["id"].(string)
	expect(t, w.send(w.ana, id, "Vou verificar"), 201)
	return id, phone
}

func (w *world) addParticipant(t *testing.T, by *client, convID, userID string) {
	t.Helper()
	expect(t, by.do("POST", "/api/v1/conversations/"+convID+"/participants", map[string]string{"userId": userID}), 204)
}

func (w *world) convState(t *testing.T, convID string) (status string, assignee any, participants []string) {
	t.Helper()
	r := w.admin.do("GET", "/api/v1/conversations/"+convID, nil)
	expect(t, r, 200)
	for _, p := range r.body["participants"].([]any) {
		participants = append(participants, p.(map[string]any)["id"].(string))
	}
	if a, ok := r.body["assignee"].(map[string]any); ok {
		assignee = a["id"]
	}
	return r.body["status"].(string), assignee, participants
}

func (w *world) auditMeta(t *testing.T, action, convID string) []map[string]any {
	t.Helper()
	rows, err := w.e.pool.Query(t.Context(), `SELECT metadata FROM audit_logs WHERE action = $1 AND target_id = $2 ORDER BY id`, action, convID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		_ = json.Unmarshal(raw, &m)
		out = append(out, m)
	}
	return out
}

func idsOf(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestCloseForEveryoneIsUnchangedAndWarnsTheOthers(t *testing.T) {
	w := setupWorld(t)
	ana, bia := w.users["ana"], w.users["bia"]

	// Sem corpo (como antes) o responsável encerra para todos.
	alone, _ := w.attended(t, "Cliente Sozinho")
	expect(t, w.closeConv(w.ana, alone, nil), 204)
	if status, _, _ := w.convState(t, alone); status != "closed" {
		t.Fatalf("status = %s, want closed", status)
	}

	id, phone := w.attended(t, "Cliente Com Equipe")
	w.addParticipant(t, w.ana, id, bia)
	anaWS, biaWS, adminWS := w.ana.websocket(t), w.bia.websocket(t), w.admin.websocket(t)
	eventually(t, "sockets", func() bool { return w.e.svcs.Hub.Count() == 3 })

	// Participante não encerra para todos; quem nem está no atendimento e não supervisiona também não.
	expect(t, w.closeConv(w.bia, id, map[string]string{"scope": "all"}), 403)
	expect(t, w.closeConv(w.ana2, id, map[string]string{"scope": "all"}), 403)

	// Administrador de fora encerra para todos; quem estava no atendimento é avisado e fica registrado.
	expect(t, w.closeConv(w.admin, id, map[string]string{"scope": "all"}), 204)
	if status, _, _ := w.convState(t, id); status != "closed" {
		t.Fatal("conversation not closed")
	}
	eventually(t, "notices", func() bool { return anaWS.count("conversation.notice") == 1 && biaWS.count("conversation.notice") == 1 })
	for _, ws := range []*wsClient{anaWS, biaWS} {
		ev := ws.last("conversation.notice")
		// O cliente sem nome salvo aparece pelo telefone formatado, como no cabeçalho da conversa.
		if ev["kind"] != "closed_for_all" || ev["id"] != id || ev["contact"] != conversations.FormatPhone(phone) || ev["by"] == "" {
			t.Fatalf("notice wrong: %v", ev)
		}
	}
	if adminWS.count("conversation.notice") != 0 {
		t.Fatal("the person who closed was warned about her own action")
	}
	metas := w.auditMeta(t, "conversation.close", id)
	if len(metas) != 1 || metas[0]["scope"] != "all" {
		t.Fatalf("close audit wrong: %v", metas)
	}
	affected := idsOf(metas[0]["affected"])
	if len(affected) != 2 || !(strings.Contains(strings.Join(affected, ","), ana) && strings.Contains(strings.Join(affected, ","), bia)) {
		t.Fatalf("audit does not list who was affected: %v", affected)
	}
	// Repetir é inofensivo.
	expect(t, w.closeConv(w.admin, id, map[string]string{"scope": "all"}), 204)
}

func TestCloseMyPartAsParticipantKeepsTheAtendimentoOpen(t *testing.T) {
	w := setupWorld(t)
	ana, bia := w.users["ana"], w.users["bia"]
	id, _ := w.attended(t, "Cliente Participante")
	w.addParticipant(t, w.ana, id, bia) // bia é da equipe de Notas: só vê a conversa como participante
	if conv(t, w.bia, "Cliente Participante") == nil {
		t.Fatal("participant cannot see the conversation")
	}

	expect(t, w.closeConv(w.bia, id, map[string]string{"scope": "me"}), 204)
	status, assignee, participants := w.convState(t, id)
	if status != "in_progress" || assignee != ana || len(participants) != 0 {
		t.Fatalf("conversation changed for the others: %s %v %v", status, assignee, participants)
	}
	if conv(t, w.bia, "Cliente Participante") != nil {
		t.Fatal("conversation still listed for the person who left")
	}
	expect(t, w.bia.do("GET", "/api/v1/conversations/"+id, nil), 404)
	if conv(t, w.ana, "Cliente Participante") == nil {
		t.Fatal("conversation vanished for the person still attending")
	}
	if metas := w.auditMeta(t, "conversation.leave", id); len(metas) != 1 || metas[0]["role"] != "participant" {
		t.Fatalf("leave audit wrong: %v", metas)
	}

	// Quem não está no atendimento não tem parte para encerrar (administrador de fora usa "para todos").
	expect(t, w.closeConv(w.ana2, id, map[string]string{"scope": "me"}), 403)
	r := w.closeConv(w.admin, id, map[string]string{"scope": "me"})
	expect(t, r, 403, "not_involved")
	expect(t, w.closeConv(w.ana, id, map[string]string{"scope": "talvez"}), 422)
	expect(t, w.closeConv(w.ana, id, map[string]string{"scope": "me", "passTo": "nao-e-uuid"}), 422)
}

func TestCloseMyPartAsAssigneeHandsOverToTheChosenPerson(t *testing.T) {
	w := setupWorld(t)
	ana, bia, caio := w.users["ana"], w.users["bia"], w.users["caio"]
	id, _ := w.attended(t, "Cliente Passagem")
	w.addParticipant(t, w.ana, id, bia)
	w.addParticipant(t, w.ana, id, caio)
	expect(t, w.ana.do("PUT", "/api/v1/conversations/"+id+"/reminder", map[string]string{"text": "Cliente espera a guia", "color": "sky"}), 204)
	caioWS := w.caio.websocket(t)
	eventually(t, "socket", func() bool { return w.e.svcs.Hub.Count() == 1 })

	// Pessoa que não participa não pode receber o atendimento.
	expect(t, w.closeConv(w.ana, id, map[string]any{"scope": "me", "passTo": w.users["dani"]}), 422)
	if _, assignee, parts := w.convState(t, id); assignee != ana || len(parts) != 2 {
		t.Fatal("a rejected handover changed the conversation")
	}

	expect(t, w.closeConv(w.ana, id, map[string]any{"scope": "me", "passTo": caio, "shareReminder": true}), 204)
	status, assignee, participants := w.convState(t, id)
	if status != "in_progress" || assignee != caio || len(participants) != 1 || participants[0] != bia {
		t.Fatalf("handover wrong: %s %v %v", status, assignee, participants)
	}
	// O histórico de responsáveis: a de ana termina como "transferida" e a de caio começa já com o recado.
	var endedReason, note *string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT ended_reason FROM conversation_assignments WHERE conversation_id = $1 AND user_id = $2`, id, ana).Scan(&endedReason); err != nil || endedReason == nil || *endedReason != "transferred" {
		t.Fatalf("previous assignment not ended as transferred: %v %v", endedReason, err)
	}
	if err := w.e.pool.QueryRow(t.Context(), `SELECT handover_note FROM conversation_assignments WHERE conversation_id = $1 AND user_id = $2 AND ended_at IS NULL`, id, caio).Scan(&note); err != nil || note == nil || *note != "Cliente espera a guia" {
		t.Fatalf("handover note missing: %v %v", note, err)
	}
	if got := personalReminders(t, w.caio); len(got) != 1 || got[0].(map[string]any)["text"] != "Cliente espera a guia" {
		t.Fatalf("reminder did not go with the handover: %v", got)
	}
	eventually(t, "notice", func() bool { return caioWS.count("conversation.notice") == 1 })
	if ev := caioWS.last("conversation.notice"); ev["kind"] != "handed_over" || ev["id"] != id {
		t.Fatalf("new responsible not told: %v", ev)
	}
	if metas := w.auditMeta(t, "conversation.leave", id); len(metas) != 1 || metas[0]["role"] != "assignee" || metas[0]["handoverTo"] != caio || metas[0]["reminderShared"] != true {
		t.Fatalf("leave audit wrong: %v", metas)
	}
	// Sem escolher, passa à participante mais antiga e o lembrete continua só de quem saiu.
	id2, _ := w.attended(t, "Cliente Padrão")
	w.addParticipant(t, w.ana, id2, bia)
	w.addParticipant(t, w.ana, id2, caio)
	expect(t, w.ana.do("PUT", "/api/v1/conversations/"+id2+"/reminder", map[string]string{"text": "Só meu", "color": "amber"}), 204)
	expect(t, w.closeConv(w.ana, id2, map[string]string{"scope": "me"}), 204)
	if _, assignee, parts := w.convState(t, id2); assignee != bia || len(parts) != 1 || parts[0] != caio {
		t.Fatalf("default handover wrong: %v %v", assignee, parts)
	}
	for _, c := range []*client{w.bia, w.caio} {
		for _, r := range personalReminders(t, c) {
			if r.(map[string]any)["conversationId"] == id2 {
				t.Fatal("reminder shared without consent")
			}
		}
	}
}

func TestCloseMyPartAloneClosesTheAtendimentoAndTheLastOneToLeaveCloses(t *testing.T) {
	w := setupWorld(t)
	bia := w.users["bia"]

	// Única pessoa: encerrar a própria parte é encerrar o atendimento; o cliente que volta abre outro.
	id, phone := w.attended(t, "Cliente Só Eu")
	expect(t, w.closeConv(w.ana, id, map[string]string{"scope": "me"}), 204)
	if status, _, _ := w.convState(t, id); status != "closed" {
		t.Fatal("sole person closing her part should close the atendimento")
	}
	var reason *string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT ended_reason FROM conversation_assignments WHERE conversation_id = $1`, id).Scan(&reason); err != nil || reason == nil || *reason != "closed" {
		t.Fatalf("assignment not closed: %v %v", reason, err)
	}
	if metas := w.auditMeta(t, "conversation.close", id); len(metas) != 1 || metas[0]["scope"] != "me" || metas[0]["last"] != true {
		t.Fatalf("close audit wrong: %v", metas)
	}
	w.receiveFrom(phone, "Cliente Só Eu", "folha de pagamento de novo")
	again := mustConv(t, w.admin, "Cliente Só Eu")
	if again["id"] == id || again["assignee"] != nil {
		t.Fatalf("returning customer should open a new, unassigned conversation: %v", again)
	}

	// Em dupla: quem sai primeiro passa o atendimento; a última a sair o encerra.
	pair, _ := w.attended(t, "Cliente Em Dupla")
	w.addParticipant(t, w.ana, pair, bia)
	expect(t, w.closeConv(w.ana, pair, map[string]string{"scope": "me"}), 204)
	if status, assignee, parts := w.convState(t, pair); status != "in_progress" || assignee != bia || len(parts) != 0 {
		t.Fatalf("after the first leaves: %s %v %v", status, assignee, parts)
	}
	expect(t, w.closeConv(w.bia, pair, map[string]string{"scope": "me"}), 204)
	if status, _, _ := w.convState(t, pair); status != "closed" {
		t.Fatal("the last person to leave should close the atendimento")
	}
}

func TestCustomerWritingAfterSomeoneLeftStaysWithThoseWhoRemain(t *testing.T) {
	w := setupWorld(t)
	bia := w.users["bia"]
	id, phone := w.attended(t, "Cliente Continua")
	w.addParticipant(t, w.ana, id, bia)
	expect(t, w.closeConv(w.ana, id, map[string]string{"scope": "me"}), 204)

	w.receiveFrom(phone, "Cliente Continua", "mais uma dúvida")
	got := mustConv(t, w.bia, "Cliente Continua")
	if got["id"] != id || got["unreadCount"] == float64(0) {
		t.Fatalf("customer message should reach the same conversation, with the remaining person: %v", got)
	}
	if a, _ := got["assignee"].(map[string]any); a["id"] != bia {
		t.Fatalf("remaining person is not the responsible: %v", got["assignee"])
	}
}

func TestParticipantLeavingAQueueConversationNeverClosesIt(t *testing.T) {
	w := setupWorld(t)
	bia := w.users["bia"]
	w.receive("Cliente Na Fila", "folha de pagamento")
	id := mustConv(t, w.admin, "Cliente Na Fila")["id"].(string)
	w.addParticipant(t, w.admin, id, bia) // fila sem responsável, com uma participante

	expect(t, w.closeConv(w.bia, id, map[string]string{"scope": "me"}), 204)
	status, assignee, parts := w.convState(t, id)
	if status != "new" || assignee != nil || len(parts) != 0 {
		t.Fatalf("queue conversation changed: %s %v %v", status, assignee, parts)
	}
	if conv(t, w.ana, "Cliente Na Fila") == nil {
		t.Fatal("queue conversation disappeared for the team")
	}
	if len(w.e.wa.Sent()) != 0 {
		t.Fatal("closing sent a WhatsApp message")
	}
}

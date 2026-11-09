package httpapi

import "testing"

func (w *world) addPart(c *client, convID, userID string) resp {
	return c.do("POST", "/api/v1/conversations/"+convID+"/participants", map[string]any{"userId": userID})
}

func TestParticipantsJoinTheSameAttendance(t *testing.T) {
	w := setupWorld(t)
	biaWS := w.bia.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() >= 1 })

	w.receive("Cliente Folha e Nota", "preciso da folha de pagamento")
	id := mustConv(t, w.ana, "Cliente Folha e Nota")["id"].(string)
	expect(t, w.send(w.ana, id, "Já te atendo."), 201) // ana assume
	if conv(t, w.bia, "Cliente Folha e Nota") != nil {
		t.Fatal("bia (Notas) não deveria ver a fila Folha")
	}

	// Quem não está no atendimento (ana2, mesma equipe) não inclui ninguém; quem não vê a conversa nem a encontra.
	expect(t, w.addPart(w.ana2, id, w.users["bia"]), 409, "conversation_taken")
	expect(t, w.addPart(w.caio, id, w.users["bia"]), 404)
	// O responsável inclui bia: ela passa a ver, em tempo real, e responde sem tirar a ana.
	expect(t, w.addPart(w.ana, id, w.users["bia"]), 204)
	eventually(t, "bia recebe a conversa", func() bool { return biaWS.count("conversation.updated") >= 1 })
	got := mustConv(t, w.bia, "Cliente Folha e Nota")
	parts, _ := got["participants"].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["id"] != w.users["bia"] || got["assignee"].(map[string]any)["id"] != w.users["ana"] {
		t.Fatalf("participantes = %v, responsável = %v", got["participants"], got["assignee"])
	}
	expect(t, w.send(w.bia, id, "Sobre a nota fiscal: já emiti."), 201)
	if a := w.assignee(t, id).(map[string]any)["id"]; a != w.users["ana"] {
		t.Fatalf("responsável mudou para %v", a)
	}
	// Quem participa também inclui outras pessoas e pode sair; o responsável não se inclui como participante.
	expect(t, w.addPart(w.bia, id, w.users["dani"]), 204)
	expect(t, w.addPart(w.ana, id, w.users["ana"]), 422)
	expect(t, w.admin.do("DELETE", "/api/v1/conversations/"+id+"/participants/"+w.users["dani"], nil), 204)
	expect(t, w.admin.do("DELETE", "/api/v1/conversations/"+id+"/participants/"+w.users["bia"], nil), 204)
	eventually(t, "bia perde a conversa", func() bool { return biaWS.count("conversation.removed") >= 1 })
	if conv(t, w.bia, "Cliente Folha e Nota") != nil {
		t.Fatal("bia ainda vê a conversa depois de removida")
	}
	expect(t, w.send(w.bia, id, "ainda posso?"), 404)
}

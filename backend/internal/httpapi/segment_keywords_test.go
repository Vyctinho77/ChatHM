package httpapi

import (
	"strings"
	"testing"
)

// segmentOf devolve o segmento (como a tela o recebe) visto por quem faz a chamada.
func segmentOf(t *testing.T, c *client, key string) map[string]any {
	t.Helper()
	r := c.do("GET", "/api/v1/segments", nil)
	expect(t, r, 200)
	for _, x := range r.body["segments"].([]any) {
		if m := x.(map[string]any); m["key"] == key {
			return m
		}
	}
	t.Fatalf("segmento %q não encontrado", key)
	return nil
}

func phrasesOf(seg map[string]any) []string {
	var out []string
	for _, k := range seg["keywords"].([]any) {
		out = append(out, k.(map[string]any)["phrase"].(string))
	}
	return out
}

func hasPhrase(list []string, want string) bool {
	for _, p := range list {
		if strings.EqualFold(p, want) {
			return true
		}
	}
	return false
}

func participantIDs(c map[string]any) []string {
	var out []string
	parts, _ := c["participants"].([]any)
	for _, p := range parts {
		out = append(out, p.(map[string]any)["id"].(string))
	}
	return out
}

// Cada equipe mexe nas palavras-chave do próprio segmento; o administrador, em todas. Quem não é da equipe não vê nem mexe.
func TestTeamsManageTheirOwnKeywords(t *testing.T) {
	w := setupWorld(t)
	notas, folha := w.segs["notas"], w.segs["folha"]

	// Leitura: bia (Notas) vê as palavras do seu segmento e só delas.
	mine, other := segmentOf(t, w.bia, "notas"), segmentOf(t, w.bia, "folha")
	if mine["canEdit"] != true || len(phrasesOf(mine)) == 0 {
		t.Fatalf("bia deveria editar Notas: %v", mine)
	}
	if other["canEdit"] != false || other["keywords"] != nil {
		t.Fatalf("bia não deveria ver nem editar Folha: %v", other)
	}
	if segmentOf(t, w.dani, "notas")["canEdit"] != false { // sem equipe
		t.Fatal("pessoa sem equipe não edita")
	}
	if segmentOf(t, w.admin, "folha")["canEdit"] != true {
		t.Fatal("administrador edita todos")
	}

	// Cadastro pela equipe: a prioridade é sempre a padrão (50), mesmo que peça 100.
	r := w.bia.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": "Nota de serviço", "priority": 100})
	expect(t, r, 201)
	if r.body["priority"] != float64(50) {
		t.Fatalf("prioridade = %v, esperado 50", r.body["priority"])
	}
	kid := r.body["id"].(string)
	expect(t, w.bia.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": "nota de serviço"}), 422) // duplicada
	// Outra equipe, e quem não é de nenhuma, não mexe.
	expect(t, w.bia.do("POST", "/api/v1/segments/"+folha+"/keywords", map[string]any{"phrase": "ponto"}), 403)
	expect(t, w.ana.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": "ponto"}), 403)
	expect(t, w.dani.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": "ponto"}), 403)
	expect(t, w.ana.do("DELETE", "/api/v1/segments/"+notas+"/keywords/"+kid, nil), 403)
	expect(t, w.ana.do("GET", "/api/v1/segments/"+notas+"/suggestions", nil), 403)
	// Prioridade e estrutura continuam do administrador.
	expect(t, w.bia.do("PATCH", "/api/v1/segments/"+notas+"/keywords/"+kid, map[string]any{"priority": 90}), 403)
	expect(t, w.bia.do("PATCH", "/api/v1/segments/"+notas, map[string]any{"autoInclude": false}), 403)
	expect(t, w.admin.do("PATCH", "/api/v1/segments/"+notas+"/keywords/"+kid, map[string]any{"priority": 70}), 204)
	expect(t, w.bia.do("DELETE", "/api/v1/segments/"+notas+"/keywords/"+kid, nil), 204)
	expect(t, w.bia.do("DELETE", "/api/v1/segments/"+notas+"/keywords/"+kid, nil), 404)

	// Curinga: só no fim da palavra e com 3+ letras.
	expect(t, w.bia.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": "emit* nota"}), 201)
	for _, bad := range []string{"a*", "*nota", "no*ta", "em*"} {
		expect(t, w.bia.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": bad}), 422)
	}
	// Qualquer pessoa testa uma mensagem contra as palavras atuais.
	r = w.ana.do("POST", "/api/v1/segments/classify", map[string]any{"text": "por favor emita a nota"})
	expect(t, r, 200)
	if r.body["segmentKey"] != "notas" || r.body["reason"] != "keyword" {
		t.Fatalf("classify = %v", r.body)
	}
}

// Sugestões: só as que o segmento ainda não tem; aceitas em lote entram com a prioridade sugerida; palavras repetidas são puladas.
func TestKeywordSuggestionsAndBulk(t *testing.T) {
	w := setupWorld(t)
	notas := w.segs["notas"]

	r := w.bia.do("GET", "/api/v1/segments/"+notas+"/suggestions", nil)
	expect(t, r, 200)
	prio := map[string]float64{}
	for _, x := range r.body["suggestions"].([]any) {
		m := x.(map[string]any)
		prio[strings.ToLower(m["phrase"].(string))] = m["priority"].(float64)
	}
	if _, ok := prio["nfe"]; ok { // já cadastrada nos exemplos
		t.Fatal("sugeriu palavra que o segmento já tem")
	}
	if prio["danfe"] != 60 || prio["nota"] != 40 {
		t.Fatalf("prioridades sugeridas: danfe=%v nota=%v", prio["danfe"], prio["nota"])
	}

	r = w.bia.do("POST", "/api/v1/segments/"+notas+"/keywords/bulk", map[string]any{"phrases": []string{"danfe", "nota", "nota fiscal", "  ", "meu termo novo"}})
	expect(t, r, 200)
	added := map[string]float64{}
	for _, x := range r.body["added"].([]any) {
		m := x.(map[string]any)
		added[m["phrase"].(string)] = m["priority"].(float64)
	}
	if added["danfe"] != 60 || added["nota"] != 40 || added["meu termo novo"] != 50 || len(added) != 3 {
		t.Fatalf("adicionadas = %v", added)
	}
	if skipped := r.body["skipped"].([]any); len(skipped) != 2 { // "nota fiscal" repetida e a linha em branco
		t.Fatalf("puladas = %v", skipped)
	}
	// Depois de aceitas, deixam de ser sugeridas.
	r = w.bia.do("GET", "/api/v1/segments/"+notas+"/suggestions", nil)
	for _, x := range r.body["suggestions"].([]any) {
		if p := strings.ToLower(x.(map[string]any)["phrase"].(string)); p == "danfe" || p == "nota" {
			t.Fatalf("%q continua sugerida", p)
		}
	}
	expect(t, w.bia.do("POST", "/api/v1/segments/"+notas+"/keywords/bulk", map[string]any{"phrases": []string{}}), 422)
}

// Palavra repetida em outro segmento (mesma prioridade) avisa quem cadastrou; o limite por segmento é respeitado.
func TestKeywordSharedWarningAndLimit(t *testing.T) {
	w := setupWorld(t)
	notas := w.segs["notas"]
	// "férias" já é da Folha com prioridade 0: o administrador cadastra em Notas também com 0.
	r := w.admin.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": "férias", "priority": 0})
	expect(t, r, 201)
	shared, _ := r.body["sharedWith"].([]any)
	if len(shared) != 1 || shared[0] != "Folha de pagamento" {
		t.Fatalf("sharedWith = %v", r.body["sharedWith"])
	}
	if _, err := w.e.pool.Exec(t.Context(), `
		INSERT INTO segment_keywords (segment_id, phrase, normalized) SELECT $1, 'x' || g, 'x' || g FROM generate_series(1, 400) g ON CONFLICT DO NOTHING`, notas); err != nil {
		t.Fatal(err)
	}
	expect(t, w.admin.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": "mais uma", "priority": 50}), 422)
}

// A palavra que a equipe cadastra muda o roteamento de verdade: antes "faz essa nota" ia para a fila geral.
func TestTeamKeywordChangesRouting(t *testing.T) {
	w := setupWorld(t)
	notas := w.segs["notas"]

	w.receive("Cliente Antes", "faz essa nota pra mim")
	if c := mustConv(t, w.bia, "Cliente Antes"); c["segment"] != "geral" {
		t.Fatalf("antes: fila = %v", c["segment"])
	}
	expect(t, w.bia.do("POST", "/api/v1/segments/"+notas+"/keywords", map[string]any{"phrase": "nota"}), 201)
	w.receive("Cliente Depois", "faz essa nota pra mim")
	if c := mustConv(t, w.bia, "Cliente Depois"); c["segment"] != "notas" {
		t.Fatalf("depois: fila = %v", c["segment"])
	}
	if conv(t, w.ana, "Cliente Depois") != nil {
		t.Fatal("Folha não deveria ver a fila de Notas")
	}
}

// Conversa já em atendimento em que o cliente passa a falar de outro assunto: a equipe do assunto entra como participante e a
// conversa aparece em "Minhas" dela; o responsável e a fila continuam os mesmos.
func TestTopicTeamIsCalledIntoOngoingConversation(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Cliente Meio", "bom dia")
	id := mustConv(t, w.dani, "Cliente Meio")["id"].(string)
	expect(t, w.send(w.dani, id, "Bom dia! Como posso ajudar?"), 201) // dani (sem equipe) assume
	if conv(t, w.bia, "Cliente Meio") != nil {
		t.Fatal("bia não deveria ver a conversa antes do assunto aparecer")
	}

	w.receiveFrom(phone, "Cliente Meio", "preciso emitir uma nota fiscal")
	got := mustConv(t, w.bia, "Cliente Meio")
	if parts := participantIDs(got); len(parts) != 1 || parts[0] != w.users["bia"] {
		t.Fatalf("participantes = %v", got["participants"])
	}
	if got["assignee"].(map[string]any)["id"] != w.users["dani"] || got["segment"] != "geral" {
		t.Fatalf("a conversa trocou de dono ou de fila: %v / %v", got["assignee"], got["segment"])
	}
	if conv(t, w.ana, "Cliente Meio") != nil {
		t.Fatal("Folha não deveria ver a conversa (assunto de Notas)")
	}

	// Cada assunto chama a equipe uma vez: citar de novo não repete, e quem foi removido à mão não volta.
	w.receiveFrom(phone, "Cliente Meio", "a nota fiscal é de serviço")
	expect(t, w.admin.do("DELETE", "/api/v1/conversations/"+id+"/participants/"+w.users["bia"], nil), 204)
	w.receiveFrom(phone, "Cliente Meio", "e a nota fiscal de ontem?")
	if conv(t, w.bia, "Cliente Meio") != nil {
		t.Fatal("bia voltou depois de ser removida à mão")
	}
	var calls int
	if err := w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action = 'conversation.team_called' AND target_id = $1::text`, id).Scan(&calls); err != nil || calls != 1 {
		t.Fatalf("chamadas auditadas = %d, %v", calls, err)
	}

	// Outro assunto chama a outra equipe (as duas pessoas de Folha), sem tirar ninguém.
	w.receiveFrom(phone, "Cliente Meio", "agora sobre a folha de pagamento")
	for _, c := range []*client{w.ana, w.ana2} {
		if conv(t, c, "Cliente Meio") == nil {
			t.Fatal("a equipe Folha deveria ver a conversa")
		}
	}
	if a := w.assignee(t, id).(map[string]any)["id"]; a != w.users["dani"] {
		t.Fatalf("responsável mudou para %v", a)
	}
}

// O segmento pode desligar a chamada automática; conversa encerrada e assunto que já é o da fila atual não chamam ninguém.
func TestTopicTeamCallCanBeDisabledAndSkipsOwnQueue(t *testing.T) {
	w := setupWorld(t)
	notas := w.segs["notas"]
	expect(t, w.admin.do("PATCH", "/api/v1/segments/"+notas, map[string]any{"autoInclude": false}), 200)

	phone := w.receive("Cliente Sem Chamada", "oi")
	id := mustConv(t, w.dani, "Cliente Sem Chamada")["id"].(string)
	expect(t, w.send(w.dani, id, "Olá!"), 201)
	w.receiveFrom(phone, "Cliente Sem Chamada", "preciso emitir nota fiscal")
	if conv(t, w.bia, "Cliente Sem Chamada") != nil {
		t.Fatal("chamou a equipe mesmo com a chamada automática desligada")
	}

	// Religada, passa a chamar.
	expect(t, w.admin.do("PATCH", "/api/v1/segments/"+notas, map[string]any{"autoInclude": true}), 200)
	w.receiveFrom(phone, "Cliente Sem Chamada", "a nota fiscal é urgente")
	if conv(t, w.bia, "Cliente Sem Chamada") == nil {
		t.Fatal("não chamou a equipe depois de religar")
	}

	// A conversa que já está na fila do assunto não chama a própria equipe como participante.
	p2 := w.receive("Cliente Na Fila", "emitir nota fiscal")
	id2 := mustConv(t, w.bia, "Cliente Na Fila")["id"].(string)
	expect(t, w.send(w.bia, id2, "Já emito."), 201)
	w.receiveFrom(p2, "Cliente Na Fila", "a nota fiscal é de R$ 100")
	if parts := participantIDs(mustConv(t, w.bia, "Cliente Na Fila")); len(parts) != 0 {
		t.Fatalf("participantes = %v", parts)
	}

	// Encerrada: nada acontece (uma nova demanda abre outra conversa e é classificada de novo).
	expect(t, w.admin.do("POST", "/api/v1/conversations/"+id+"/close", nil), 204)
	var before int
	_ = w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM conversation_participants WHERE conversation_id = $1`, id).Scan(&before)
	w.receiveFrom(phone, "Cliente Sem Chamada", "folha de pagamento")
	var after int
	_ = w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM conversation_participants WHERE conversation_id = $1`, id).Scan(&after)
	if after != before {
		t.Fatalf("conversa encerrada ganhou participantes: %d → %d", before, after)
	}
}

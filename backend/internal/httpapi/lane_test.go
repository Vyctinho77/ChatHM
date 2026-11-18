package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// laneWorld: IA ligada, um token do atendente autônomo preso à raia "certificado" (presa à fila Folha), conversa de cliente na fila.
type laneEnv struct {
	w       *world
	token   string
	tokenID string
	convID  string
	contact string
	phone   string
}

func laneSetup(t *testing.T, mode string, tweak func(in map[string]any)) laneEnv {
	t.Helper()
	w, toks := agentTokens(t)
	var tokenID string
	for _, x := range w.admin.do("GET", "/api/v1/ai/tokens", nil).body["tokens"].([]any) {
		if x.(map[string]any)["name"] == "atendente" {
			tokenID = x.(map[string]any)["id"].(string)
		}
	}
	if tokenID == "" {
		t.Fatal("token do atendente não encontrado")
	}
	in := map[string]any{"label": "Certificado digital", "enabled": true, "mode": mode, "canInitiate": false, "segmentId": w.segs["folha"], "handoffSegmentId": "",
		"maxTurns": 8, "maxConsecutive": 2, "contactDaily": 6, "humanQuietMinutes": 60, "staleHours": 24, "requireSources": true,
		"allowedDomains": []string{}, "guidelines": "Oriente o cliente."}
	if tweak != nil {
		tweak(in)
	}
	expect(t, w.admin.do("PUT", "/api/v1/automation/lanes/certificado", in), 200)
	expect(t, w.admin.do("PUT", "/api/v1/ai/tokens/"+tokenID+"/lane", map[string]any{"laneKey": "certificado"}), 204)
	p := w.receive("Cliente Raia", "folha de pagamento: preciso de ajuda com o certificado digital")
	c := mustConv(t, w.admin, "Cliente Raia")
	return laneEnv{w: w, token: toks["attend"], tokenID: tokenID, convID: c["id"].(string), contact: c["contact"].(map[string]any)["id"].(string), phone: p}
}

func (e laneEnv) send(text string) map[string]any {
	e.w.e.t.Helper()
	o, isErr := e.w.tool(e.token, "send_as_attendant", map[string]any{"conversation_id": e.convID, "text": text})
	if isErr {
		e.w.e.t.Fatalf("send_as_attendant: %v", o)
	}
	return o
}

func (e laneEnv) count(q string, args ...any) int { return scalar[int](e.w.e.t, e.w, q, args...) }

const goodReply = "Para emitir o certificado digital é preciso agendar uma validação por vídeo. Posso explicar o passo a passo?"

func TestLaneIsOffUntilEnabledAndShadowSendsNothing(t *testing.T) {
	e := laneSetup(t, "shadow", func(in map[string]any) { in["enabled"] = false })
	o := e.send(goodReply)
	if o["recusado"] != true || o["codigo"] != "raia_desligada" {
		t.Fatalf("raia desligada deveria recusar: %v", o)
	}
	// Em sombra: vira rascunho e NADA vai ao cliente.
	expect(t, e.w.admin.do("PUT", "/api/v1/automation/lanes/certificado", map[string]any{"label": "Certificado digital", "enabled": true, "mode": "shadow",
		"segmentId": e.w.segs["folha"], "maxTurns": 8, "maxConsecutive": 2, "contactDaily": 6, "humanQuietMinutes": 60, "allowedDomains": []string{}}), 200)
	sentBefore := len(e.w.e.wa.Sent())
	o = e.send(goodReply)
	if o["status"] != "sombra" || o["ok"] != false {
		t.Fatalf("em sombra: %v", o)
	}
	if len(e.w.e.wa.Sent()) != sentBefore || e.count(`SELECT count(*) FROM messages WHERE payload->>'aiMode' = 'attend'`) != 0 {
		t.Fatal("em sombra nada pode ser enviado")
	}
	drafts := e.w.admin.do("GET", "/api/v1/automation/lane-drafts?lane=certificado", nil).body["drafts"].([]any)
	if len(drafts) != 1 || drafts[0].(map[string]any)["body"].(string) != goodReply {
		t.Fatalf("rascunhos: %v", drafts)
	}
}

func TestLaneLiveSendsAsWrittenAndRulesOfTheConversation(t *testing.T) {
	e := laneSetup(t, "live", nil)
	o := e.send(goodReply)
	if o["ok"] != true || o["status"] != "enviado" {
		t.Fatalf("envio ao vivo: %v", o)
	}
	m := lastSent(t, e.w)
	if !strings.HasSuffix(m.Text, goodReply) || strings.Contains(m.Text, "virtual") {
		t.Fatalf("a mensagem sai como a IA escreveu, só com a assinatura do atendimento (sem aviso de IA): %q", m.Text)
	}
	if lane := scalar[string](t, e.w, `SELECT payload->>'aiLane' FROM messages WHERE payload->>'aiMode' = 'attend' ORDER BY created_at DESC LIMIT 1`); lane != "certificado" {
		t.Fatalf("a mensagem leva a raia: %q", lane)
	}
	e.w.receiveFrom(e.phone, "Cliente Raia", "ok, pode explicar")
	o = e.send("Claro: primeiro escolha o tipo de certificado.")
	if o["ok"] != true {
		t.Fatalf("segunda mensagem: %v", o)
	}
	// Duas mensagens seguidas sem resposta do cliente: a terceira espera.
	e.send("Quando quiser, é só dizer o dia.")
	o = e.send("Estou à disposição.")
	if o["recusado"] != true || o["codigo"] != "aguarde_cliente" {
		t.Fatalf("a terceira seguida deveria esperar o cliente: %v", o)
	}
}

func TestLaneTextRulesOnSendAreFew(t *testing.T) {
	e := laneSetup(t, "live", nil)
	// Só credenciais são barradas; valores, números, e-mails e links ficam livres (lista de domínios vazia).
	o := e.send("Me envie a senha do certificado, por favor")
	if o["recusado"] != true || o["codigo"] != "credencial" {
		t.Fatalf("senha: %v", o)
	}
	if e.count(`SELECT count(*) FROM messages WHERE payload->>'aiMode' = 'attend'`) != 0 {
		t.Fatal("a mensagem recusada não pode ter saído")
	}
	o = e.send("O certificado custa R$ 150 por ano. Mais informações em https://gov.br/certificado ou contato@exemplo.com.br")
	if o["ok"] != true {
		t.Fatalf("valor, link e e-mail são livres: %v", o)
	}
	// Documento da biblioteca não vale na raia.
	o, _ = e.w.tool(e.token, "send_as_attendant", map[string]any{"conversation_id": e.convID, "text": goodReply, "document_id": "11111111-1111-4111-8111-111111111111"})
	if o["codigo"] != "documento" {
		t.Fatalf("documento na raia: %v", o)
	}
}

func TestLaneHumanTakingOverSilencesTheAI(t *testing.T) {
	e := laneSetup(t, "live", nil)
	if o := e.send(goodReply); o["ok"] != true {
		t.Fatalf("antes: %v", o)
	}
	// Uma pessoa responde na conversa pelo app (ela assume).
	e.w.receiveFrom(e.phone, "Cliente Raia", "tenho outra dúvida")
	expect(t, e.w.send(e.w.ana, e.convID, "Oi! Aqui é a Ana, vou te ajudar."), 201)
	o := e.send("Posso ajudar também?")
	if o["recusado"] != true || o["codigo"] != "humano_ativo" {
		t.Fatalf("com uma pessoa na conversa a IA não escreve: %v", o)
	}
}

func TestLaneClientMediaGoesToAHumanAndTheAIStops(t *testing.T) {
	e := laneSetup(t, "live", nil)
	img := testPNG(32, 32)
	m := inbound("FOTO-1", e.phone, "Cliente Raia", "")
	m.Kind = whatsapp.KindImage
	m.Payload = map[string]any{"mimeType": "image/png"}
	m.Download = func(context.Context) ([]byte, error) { return img, nil }
	e.w.e.wa.Receive(m)
	eventually(t, "foto recebida", func() bool { return e.count(`SELECT count(*) FROM messages WHERE external_id = 'FOTO-1'`) == 1 })

	sent := len(e.w.e.wa.Sent())
	o := e.send(goodReply)
	if o["recusado"] != true || o["codigo"] != "passada_para_humano" {
		t.Fatalf("foto do cliente passa para um humano: %v", o)
	}
	if len(e.w.e.wa.Sent()) != sent {
		t.Fatal("a IA não pode ter enviado nada")
	}
	if e.count(`SELECT count(*) FROM ai_handoffs WHERE conversation_id = $1 AND reason = 'documento' AND source = 'server'`, e.convID) != 1 {
		t.Fatal("a passagem deveria estar registrada")
	}
	if e.count(`SELECT count(*) FROM conversation_tags ct JOIN tags t ON t.id = ct.tag_id WHERE ct.conversation_id = $1 AND t.name = 'Precisa de humano'`, e.convID) != 1 {
		t.Fatal("a conversa deveria ter a etiqueta")
	}
	if note := scalar[string](t, e.w, `SELECT note FROM contacts WHERE id = $1`, e.contact); !strings.Contains(note, "IA pediu ajuda") || !strings.Contains(note, "documento") {
		t.Fatalf("a nota do contato: %q", note)
	}
	// Daí em diante a IA fica em silêncio, mesmo com texto perfeito.
	if o := e.send("Pode me mandar mais detalhes?"); o["codigo"] != "passada_para_humano" {
		t.Fatalf("silêncio depois da passagem: %v", o)
	}
	// O administrador vê a passagem.
	h := e.w.admin.do("GET", "/api/v1/automation/handoffs?open=1", nil).body["handoffs"].([]any)
	if len(h) != 1 || h[0].(map[string]any)["reason"] != "documento" {
		t.Fatalf("lista de passagens: %v", h)
	}
	// Uma pessoa responde: a passagem fica "respondida".
	expect(t, e.w.send(e.w.ana, e.convID, "Oi, aqui é a Ana, recebi a foto."), 201)
	h = e.w.admin.do("GET", "/api/v1/automation/handoffs", nil).body["handoffs"].([]any)
	if h[0].(map[string]any)["answered"] != true {
		t.Fatalf("a passagem deveria estar respondida: %v", h[0])
	}
}

func TestLaneRequestHumanTool(t *testing.T) {
	e := laneSetup(t, "live", func(in map[string]any) {})
	o, isErr := e.w.tool(e.token, "request_human", map[string]any{"conversation_id": e.convID, "reason": "Preco", "summary": "Cliente quer saber o valor do certificado e a validade."})
	if isErr || o["ok"] != true || o["jaEstavaPassada"] != false {
		t.Fatalf("request_human: %v", o)
	}
	// Repetir não duplica.
	o, _ = e.w.tool(e.token, "request_human", map[string]any{"conversation_id": e.convID, "reason": "preco", "summary": "de novo"})
	if o["jaEstavaPassada"] != true || e.count(`SELECT count(*) FROM ai_handoffs WHERE conversation_id = $1`, e.convID) != 1 {
		t.Fatalf("repetir não duplica: %v", o)
	}
	if note := scalar[string](t, e.w, `SELECT note FROM contacts WHERE id = $1`, e.contact); !strings.Contains(note, "preco") || !strings.Contains(note, "valor do certificado") {
		t.Fatalf("nota: %q", note)
	}
	if o := e.send("Posso ajudar mais?"); o["codigo"] != "passada_para_humano" {
		t.Fatalf("depois de pedir humano a IA fica em silêncio: %v", o)
	}
	// Conversa que o token não vê é recusada.
	if isErr, _ := e.w.toolCall(e.token, "request_human", map[string]any{"conversation_id": "22222222-2222-4222-8222-222222222222", "reason": "outro", "summary": "x"}); !isErr {
		t.Fatal("conversa inexistente deveria falhar")
	}
	// Só o atendente autônomo tem a ferramenta.
	e.w.expectDenied(e.w.mkToken(e.w.admin, "so-leitura"), "request_human", map[string]any{"conversation_id": e.convID, "reason": "outro", "summary": "x"})
}

func TestLaneBudgetTurnsGoToAHumanAndQueueScope(t *testing.T) {
	e := laneSetup(t, "live", func(in map[string]any) { in["maxTurns"] = 2; in["maxConsecutive"] = 5 })
	e.send("Olá! Sobre o certificado, qual tipo você precisa, A1 ou A3?")
	e.send("Pode me contar mais sobre o seu caso?")
	if o := e.send("E qual é a sua dúvida principal?"); o["codigo"] != "passada_para_humano" {
		t.Fatalf("estourou as rodadas: %v", o)
	}
	if e.count(`SELECT count(*) FROM ai_handoffs WHERE reason = 'turnos'`) != 1 {
		t.Fatal("a passagem por rodadas deveria estar registrada")
	}
	// Outra conversa, de outra fila, está fora da raia.
	e.w.receive("Cliente Fiscal", "nota fiscal de serviço, preciso emitir")
	other := mustConv(t, e.w.admin, "Cliente Fiscal")["id"].(string)
	o, _ := e.w.tool(e.token, "send_as_attendant", map[string]any{"conversation_id": other, "text": goodReply})
	if o["codigo"] != "fora_da_raia" {
		t.Fatalf("conversa de outra fila: %v", o)
	}
}

func TestLaneInitiateOnlyWhereAllowedAndCheckSend(t *testing.T) {
	e := laneSetup(t, "live", func(in map[string]any) { in["segmentId"] = "" })
	// Fecha a conversa: sem conversa aberta, a raia que só responde recusa; com can_initiate abre uma nova.
	if _, err := e.w.e.pool.Exec(context.Background(), `UPDATE conversations SET status = 'closed', closed_at = now() WHERE id = $1`, e.convID); err != nil {
		t.Fatal(err)
	}
	o, _ := e.w.tool(e.token, "send_as_attendant", map[string]any{"contact_id": e.contact, "text": goodReply})
	if o["codigo"] != "so_responde" {
		t.Fatalf("raia que só responde: %v", o)
	}
	expect(t, e.w.admin.do("PUT", "/api/v1/automation/lanes/certificado", map[string]any{"label": "Cobrança", "enabled": true, "mode": "live", "canInitiate": true,
		"maxTurns": 8, "maxConsecutive": 1, "contactDaily": 2, "humanQuietMinutes": 120, "allowedDomains": []string{"asaas.com"}}), 200)
	o, _ = e.w.tool(e.token, "send_as_attendant", map[string]any{"contact_id": e.contact, "text": "Oi! O boleto de R$ 350,00 vence dia 25/11: https://www.asaas.com/i/abc123"})
	if o["ok"] != true {
		t.Fatalf("raia de cobrança inicia e aceita valor e link do Asaas: %v", o)
	}
	// Link de outro domínio continua barrado.
	e.w.receiveFrom(e.phone, "Cliente Raia", "ok, vou ver")
	o, _ = e.w.tool(e.token, "send_as_attendant", map[string]any{"contact_id": e.contact, "text": "Pague em https://pagamento-falso.com/x"})
	if o["codigo"] != "link" {
		t.Fatalf("link de outro domínio: %v", o)
	}
	// check_send conhece as regras da raia e confere o texto.
	c, isErr := e.w.tool(e.token, "check_send", map[string]any{"contact_id": e.contact, "text": "Pague em https://pagamento-falso.com/x"})
	if isErr || c["wouldSend"] != false || c["lane"] != "certificado" {
		t.Fatalf("check_send com link ruim: %v", c)
	}
	c, _ = e.w.tool(e.token, "check_send", map[string]any{"contact_id": e.contact, "text": "Obrigado pelo retorno!"})
	if c["wouldSend"] != true {
		t.Fatalf("check_send com texto bom: %v", c)
	}
}

func TestLaneTokenBindingAndAdminAPI(t *testing.T) {
	w, toks := agentTokens(t)
	_ = toks
	var readID, attendID string
	for _, x := range w.admin.do("GET", "/api/v1/ai/tokens", nil).body["tokens"].([]any) {
		m := x.(map[string]any)
		switch m["name"] {
		case "leitura":
			readID = m["id"].(string)
		case "atendente":
			attendID = m["id"].(string)
		}
	}
	// Só tokens do atendente autônomo; só administrador; raia precisa existir.
	expect(t, w.admin.do("PUT", "/api/v1/ai/tokens/"+readID+"/lane", map[string]any{"laneKey": "certificado"}), 422)
	expect(t, w.admin.do("PUT", "/api/v1/ai/tokens/"+attendID+"/lane", map[string]any{"laneKey": "nao_existe"}), 422)
	expect(t, w.ana.do("PUT", "/api/v1/ai/tokens/"+attendID+"/lane", map[string]any{"laneKey": "certificado"}), 403)
	expect(t, w.admin.do("PUT", "/api/v1/ai/tokens/"+attendID+"/lane", map[string]any{"laneKey": "certificado"}), 204)
	var lane string
	for _, x := range w.admin.do("GET", "/api/v1/ai/tokens", nil).body["tokens"].([]any) {
		if m := x.(map[string]any); m["name"] == "atendente" {
			lane, _ = m["lane"].(string)
		}
	}
	if lane != "certificado" {
		t.Fatalf("o token deveria mostrar a raia: %q", lane)
	}
	// my_access mostra a raia ao agente.
	acc, _ := w.tool(toks["attend"], "my_access", map[string]any{})
	if l, _ := acc["lane"].(map[string]any); l["key"] != "certificado" || l["mode"] != "shadow" || l["enabled"] != false {
		t.Fatalf("my_access da raia: %v", acc["lane"])
	}
	// Soltar volta ao atendente livre.
	expect(t, w.admin.do("PUT", "/api/v1/ai/tokens/"+attendID+"/lane", map[string]any{"laneKey": ""}), 204)
	if acc, _ := w.tool(toks["attend"], "my_access", map[string]any{}); acc["lane"] != nil {
		t.Fatalf("sem raia: %v", acc["lane"])
	}

	// API de raias: as duas raias de fábrica nascem desligadas e em sombra; só administrador edita; valida os campos.
	lanes := w.admin.do("GET", "/api/v1/automation/lanes", nil).body["lanes"].([]any)
	if len(lanes) != 2 {
		t.Fatalf("raias de fábrica: %v", lanes)
	}
	for _, x := range lanes {
		m := x.(map[string]any)
		if m["enabled"] != false || m["mode"] != "shadow" {
			t.Fatalf("raia de fábrica deve nascer desligada e em sombra: %v", m)
		}
	}
	expect(t, w.ana.do("GET", "/api/v1/automation/lanes", nil), 403)
	bad := map[string]any{"label": "", "enabled": true, "mode": "voando", "maxTurns": 0, "maxConsecutive": 99, "contactDaily": 6, "humanQuietMinutes": 60,
		"allowedDomains": []string{"http://x.com/y"}}
	r := w.admin.do("PUT", "/api/v1/automation/lanes/certificado", bad)
	expect(t, r, 422)
	expect(t, w.admin.do("PUT", "/api/v1/automation/lanes/nao_existe", bad), 404)
}

func TestPlainAttendTokenIsUnchangedByLanes(t *testing.T) {
	w, toks := agentTokens(t)
	w.receive("Cliente Livre", "oi, tudo bem")
	cid := mustConv(t, w.admin, "Cliente Livre")["id"].(string)
	// Sem raia, o atendente continua livre como antes (texto com número e sem apresentação).
	o, _ := w.tool(toks["attend"], "send_as_attendant", map[string]any{"conversation_id": cid, "text": "Seu protocolo é 20261118334455, ok?"})
	if o["ok"] != true || strings.Contains(lastSent(t, w).Text, "assistente virtual") {
		t.Fatalf("sem raia nada muda: %v", o)
	}
}

// Conversas paradas: o cliente escreveu e ninguém da equipe iniciou o atendimento.
func TestLaneListUnattended(t *testing.T) {
	e := laneSetup(t, "live", nil)
	list := func(args map[string]any) []any {
		o, isErr := e.w.tool(e.token, "list_unattended", args)
		if isErr {
			t.Fatalf("list_unattended: %v", o)
		}
		return o["conversations"].([]any)
	}
	// Recém-chegada: ainda não é "parada".
	if got := list(nil); len(got) != 0 {
		t.Fatalf("conversa de agora não está parada: %v", got)
	}
	// Envelhece a mensagem do cliente: passa a esperar há 30 h (o padrão da raia é 24 h).
	if _, err := e.w.e.pool.Exec(context.Background(), `UPDATE messages SET sent_at = now() - interval '30 hours' WHERE conversation_id = $1`, e.convID); err != nil {
		t.Fatal(err)
	}
	got := list(nil)
	if len(got) != 1 || got[0].(map[string]any)["conversationId"] != e.convID || got[0].(map[string]any)["waitedHours"].(float64) < 29 {
		t.Fatalf("conversa parada: %v", got)
	}
	if last, _ := got[0].(map[string]any)["lastMessage"].(map[string]any); last == nil {
		t.Fatalf("deveria trazer a última mensagem do cliente: %v", got[0])
	}
	if len(list(map[string]any{"min_hours": 48})) != 0 {
		t.Fatal("com min_hours 48 ainda não está parada")
	}
	// Outra fila fica fora (a raia é da fila Folha).
	e.w.receive("Cliente Fiscal", "nota fiscal de serviço, preciso emitir")
	if _, err := e.w.e.pool.Exec(context.Background(), `UPDATE messages SET sent_at = now() - interval '30 hours' WHERE conversation_id = $1`,
		mustConv(t, e.w.admin, "Cliente Fiscal")["id"].(string)); err != nil {
		t.Fatal(err)
	}
	if got := list(nil); len(got) != 1 {
		t.Fatalf("só a fila da raia: %v", got)
	}
	// A IA responde: deixa de estar parada.
	if o := e.send(goodReply); o["ok"] != true {
		t.Fatalf("responder à conversa parada: %v", o)
	}
	if got := list(nil); len(got) != 0 {
		t.Fatalf("respondida, não está mais parada: %v", got)
	}
	// Uma pessoa que já falou na conversa a tira da lista, mesmo que o cliente tenha escrito de novo e esperado.
	e.w.receiveFrom(e.phone, "Cliente Raia", "alguém aí?")
	expect(t, e.w.send(e.w.ana, e.convID, "Oi, aqui é a Ana."), 201)
	e.w.receiveFrom(e.phone, "Cliente Raia", "ainda preciso de ajuda")
	if _, err := e.w.e.pool.Exec(context.Background(), `UPDATE messages SET sent_at = now() - interval '30 hours' WHERE conversation_id = $1 AND direction = 'in'`, e.convID); err != nil {
		t.Fatal(err)
	}
	if got := list(nil); len(got) != 0 {
		t.Fatalf("já houve atendimento humano: %v", got)
	}
}

// Qualquer IA externa (Grok, Claude, ChatGPT, Gemini) usa a mesma raia pelo mesmo MCP.
func TestLaneWorksForEveryProvider(t *testing.T) {
	e := laneSetup(t, "live", func(in map[string]any) { in["maxConsecutive"] = 10; in["maxTurns"] = 20; in["contactDaily"] = 20 })
	for _, prov := range []string{"claude", "chatgpt", "grok", "gemini", "other"} {
		r := e.w.admin.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "ia-" + prov, "scopes": []string{"attend"}, "provider": prov})
		expect(t, r, 201)
		tok := r.body["token"].(string)
		var id string
		for _, x := range e.w.admin.do("GET", "/api/v1/ai/tokens", nil).body["tokens"].([]any) {
			if m := x.(map[string]any); m["name"] == "ia-"+prov {
				id = m["id"].(string)
			}
		}
		expect(t, e.w.admin.do("PUT", "/api/v1/ai/tokens/"+id+"/lane", map[string]any{"laneKey": "certificado"}), 204)
		if acc, _ := e.w.tool(tok, "my_access", map[string]any{}); acc["lane"] == nil {
			t.Fatalf("%s: o token deveria estar na raia", prov)
		}
		o, isErr := e.w.tool(tok, "send_as_attendant", map[string]any{"conversation_id": e.convID, "text": "Olá! Aqui é " + prov + ", posso ajudar com o certificado?"})
		if isErr || o["ok"] != true {
			t.Fatalf("%s: %v", prov, o)
		}
		if got := scalar[string](t, e.w, `SELECT payload->>'aiProvider' FROM messages WHERE payload->>'aiMode' = 'attend' ORDER BY created_at DESC LIMIT 1`); got != prov {
			t.Fatalf("a mensagem deveria levar o provedor %s: %q", prov, got)
		}
	}
}

// Informação de fora (regra do governo) só com as fontes consultadas, que o cliente vê no fim da mensagem.
func TestLaneOutsideInformationNeedsSources(t *testing.T) {
	e := laneSetup(t, "live", nil)
	answer := "Pela nova instrução normativa da Receita Federal, o certificado e-CNPJ A1 passou a ter validade de 1 ano."
	o := e.send(answer)
	if o["recusado"] != true || o["codigo"] != "fontes" {
		t.Fatalf("sem fontes: %v", o)
	}
	if e.count(`SELECT count(*) FROM messages WHERE payload->>'aiMode' = 'attend'`) != 0 {
		t.Fatal("nada pode ter saído")
	}
	// Fonte inválida (não https).
	o, _ = e.w.tool(e.token, "send_as_attendant", map[string]any{"conversation_id": e.convID, "text": answer,
		"sources": []map[string]any{{"title": "Receita", "url": "http://gov.br/x"}}})
	if o["codigo"] != "fontes" {
		t.Fatalf("fonte inválida: %v", o)
	}
	// check_send conhece a regra.
	c, _ := e.w.tool(e.token, "check_send", map[string]any{"conversation_id": e.convID, "text": answer})
	if c["wouldSend"] != false {
		t.Fatalf("check_send sem fonte: %v", c)
	}
	c, _ = e.w.tool(e.token, "check_send", map[string]any{"conversation_id": e.convID, "text": answer, "sources": []map[string]any{{"title": "Receita", "url": "https://www.gov.br/receitafederal"}}})
	if c["wouldSend"] != true {
		t.Fatalf("check_send com fonte: %v", c)
	}
	o, _ = e.w.tool(e.token, "send_as_attendant", map[string]any{"conversation_id": e.convID, "text": answer,
		"sources": []map[string]any{{"title": "Receita Federal", "url": "https://www.gov.br/receitafederal/pt-br"}, {"url": "https://www.gov.br/iti"}}})
	if o["ok"] != true {
		t.Fatalf("com fontes: %v", o)
	}
	txt := lastSent(t, e.w).Text
	if !strings.Contains(txt, answer) || !strings.Contains(txt, "Fontes consultadas:") || !strings.Contains(txt, "• Receita Federal: https://www.gov.br/receitafederal/pt-br") || !strings.Contains(txt, "• gov.br: https://www.gov.br/iti") {
		t.Fatalf("a mensagem deveria trazer as fontes: %q", txt)
	}
}

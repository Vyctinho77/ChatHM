package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

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

func (e laneEnv) pending(args map[string]any) map[string]any {
	e.w.e.t.Helper()
	o, isErr := e.w.tool(e.token, "list_pending_replies", args)
	if isErr {
		e.w.e.t.Fatalf("list_pending_replies: %v", o)
	}
	return o
}

func (e laneEnv) pendingIDs() map[string]string { // conversationId -> reason
	out := map[string]string{}
	for _, x := range e.pending(nil)["conversations"].([]any) {
		m := x.(map[string]any)
		out[m["conversationId"].(string)], _ = m["reason"].(string)
	}
	return out
}

func (e laneEnv) age(convID string) {
	e.w.e.t.Helper()
	if _, err := e.w.e.pool.Exec(context.Background(), `UPDATE messages SET sent_at = now() - interval '30 hours' WHERE conversation_id = $1`, convID); err != nil {
		e.w.e.t.Fatal(err)
	}
}

// Acordar a IA: o cliente respondeu, ou a conversa ficou parada sem atendimento. Só o que é da raia e ainda não tem pessoa.
func TestLaneWakesTheAIOnCustomerRepliesOnlyInsideTheLane(t *testing.T) {
	e := laneSetup(t, "live", func(in map[string]any) { in["maxConsecutive"] = 5 })
	if got := e.pendingIDs(); len(got) != 0 {
		t.Fatalf("nada para a IA ainda: %v", got)
	}
	// A IA responde e o cliente responde de volta: acorda a IA (reason resposta).
	e.send(goodReply)
	e.w.receiveFrom(e.phone, "Cliente Raia", "ok, pode explicar melhor?")
	eventually(t, "resposta acorda a IA", func() bool { return e.pendingIDs()[e.convID] == "resposta" })
	o := e.pending(nil)
	if o["lane"] != "certificado" || !strings.Contains(o["howToUse"].(string), "request_human") {
		t.Fatalf("as instruções da raia: %v", o["howToUse"])
	}
	// Dar baixa tira da lista; nova resposta do cliente volta.
	if r, _ := e.w.tool(e.token, "mark_replies", map[string]any{"items": []map[string]any{{"conversation_id": e.convID, "status": "resolved"}}}); r["ok"] != true {
		t.Fatalf("mark_replies: %v", r)
	}
	if got := e.pendingIDs(); len(got) != 0 {
		t.Fatalf("resolvida: %v", got)
	}
	// Uma pessoa assume: o cliente escreve de novo e a IA NÃO é acordada.
	expect(t, e.w.send(e.w.ana, e.convID, "Oi, aqui é a Ana."), 201)
	e.w.receiveFrom(e.phone, "Cliente Raia", "obrigado, e o prazo?")
	eventually(t, "mensagem chegou", func() bool {
		return e.count(`SELECT count(*) FROM messages WHERE conversation_id = $1 AND direction = 'in'`, e.convID) >= 3
	})
	if got := e.pendingIDs(); len(got) != 0 {
		t.Fatalf("com uma pessoa na conversa a IA não é acordada: %v", got)
	}
	// Raia desligada não acorda ninguém, mesmo com a mesma conversa.
	expect(t, e.w.admin.do("PUT", "/api/v1/automation/lanes/certificado", map[string]any{"label": "Certificado digital", "enabled": false, "mode": "live", "segmentId": e.w.segs["folha"],
		"maxTurns": 8, "maxConsecutive": 5, "contactDaily": 6, "humanQuietMinutes": 60, "allowedDomains": []string{}}), 200)
	if got := e.pendingIDs(); len(got) != 0 {
		t.Fatalf("raia desligada: %v", got)
	}
}

func TestLaneWakesTheAIForStaleConversationsOnce(t *testing.T) {
	e := laneSetup(t, "live", nil)
	// Recém-chegada: não está parada.
	if got := e.pendingIDs(); len(got) != 0 {
		t.Fatalf("conversa de agora: %v", got)
	}
	e.age(e.convID)
	if got := e.pendingIDs(); got[e.convID] != "parada" {
		t.Fatalf("conversa parada há 30 h deveria acordar a IA: %v", got)
	}
	// Outra fila não é da raia.
	e.w.receive("Cliente Fiscal", "nota fiscal de serviço, preciso emitir")
	e.age(mustConv(t, e.w.admin, "Cliente Fiscal")["id"].(string))
	if got := e.pendingIDs(); len(got) != 1 {
		t.Fatalf("só a fila da raia: %v", got)
	}
	// O cursor "since" (do listener) deixa de fora o que ficou parado antes dele; o que ficou parado depois entra.
	later := e.pending(map[string]any{"since": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	if later["total"].(float64) != 0 {
		t.Fatalf("since no futuro: %v", later["total"])
	}
	earlier := e.pending(map[string]any{"since": time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)})
	if earlier["total"].(float64) != 1 {
		t.Fatalf("since antigo: %v", earlier["total"])
	}
	// wait_for_replies devolve na hora, sem esperar.
	start := time.Now()
	w, _ := e.w.tool(e.token, "wait_for_replies", map[string]any{"timeout_seconds": 20})
	if w["timedOut"] != false || w["total"].(float64) != 1 || time.Since(start) > 10*time.Second {
		t.Fatalf("wait_for_replies: %v (%v)", w["total"], time.Since(start))
	}
	// Dar baixa: não volta enquanto o cliente não escrever de novo.
	if r, _ := e.w.tool(e.token, "mark_replies", map[string]any{"items": []map[string]any{{"conversation_id": e.convID, "status": "resolved"}}}); r["ok"] != true {
		t.Fatalf("mark_replies: %v", r)
	}
	if got := e.pendingIDs(); len(got) != 0 {
		t.Fatalf("depois da baixa: %v", got)
	}
	// Passada para uma pessoa (a IA pediu ajuda): fora da lista, mesmo com baixa antiga.
	e.w.receiveFrom(e.phone, "Cliente Raia", "alguém?")
	e.age(e.convID)
	if got := e.pendingIDs(); got[e.convID] != "parada" {
		t.Fatalf("o cliente escreveu de novo e a conversa voltou: %v", got)
	}
	if o, _ := e.w.tool(e.token, "request_human", map[string]any{"conversation_id": e.convID, "reason": "duvida", "summary": "Cliente sem resposta."}); o["ok"] != true {
		t.Fatalf("request_human: %v", o)
	}
	if got := e.pendingIDs(); len(got) != 0 {
		t.Fatalf("passada para uma pessoa não acorda a IA: %v", got)
	}
}

// O script scripts/chathm-listen.mjs chama GET /mcp/replies: com um token de raia, a conversa parada também chega por ali,
// uma vez só (o cursor "since" avança), e some quando uma pessoa assume.
func TestLaneWakeOverPlainHTTPForTheListener(t *testing.T) {
	e := laneSetup(t, "live", nil)
	e.age(e.convID)
	get := func(query string) map[string]any {
		req, _ := http.NewRequest("GET", e.w.e.srv.URL+"/mcp/replies"+query, nil)
		req.Header.Set("Authorization", "Bearer "+e.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("status %d", res.StatusCode)
		}
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return out
	}
	out := get("?wait=0&since=" + url.QueryEscape(time.Now().Add(-72*time.Hour).UTC().Format(time.RFC3339)))
	cs, _ := out["conversations"].([]any)
	if len(cs) != 1 || cs[0].(map[string]any)["reason"] != "parada" || out["lane"] != "certificado" {
		t.Fatalf("conversa parada por HTTP: %v", out)
	}
	// O cursor que o listener guarda é o "lastAt" do item: com ele, não volta.
	cursor := cs[0].(map[string]any)["pending"].(map[string]any)["lastAt"].(string)
	if again := get("?wait=0&since=" + url.QueryEscape(cursor)); again["total"].(float64) != 0 {
		t.Fatalf("depois do cursor não volta: %v", again["total"])
	}
	// Uma pessoa assume: nem com o cursor antigo.
	expect(t, e.w.send(e.w.ana, e.convID, "Oi, aqui é a Ana."), 201)
	if gone := get("?wait=0&since=" + url.QueryEscape(time.Now().Add(-72*time.Hour).UTC().Format(time.RFC3339))); gone["total"].(float64) != 0 {
		t.Fatalf("com uma pessoa na conversa: %v", gone["total"])
	}
}

func (e laneEnv) drafts(query string) []map[string]any {
	e.w.e.t.Helper()
	out := []map[string]any{}
	for _, x := range e.w.admin.do("GET", "/api/v1/automation/lane-drafts"+query, nil).body["drafts"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

// Rascunhos da sombra: quem revisa envia (como o agente que escreveu), edita e envia, ou descarta.
func TestLaneDraftReviewApproveEditDiscard(t *testing.T) {
	e := laneSetup(t, "shadow", nil)
	for i := 0; i < 3; i++ {
		if o := e.send(goodReply + " " + strings.Repeat("x", i)); o["status"] != "sombra" {
			t.Fatalf("em sombra: %v", o)
		}
	}
	pending := e.drafts("?pending=1")
	if len(pending) != 3 || pending[0]["agent"] != "atendente" || pending[0]["provider"] != "grok" || pending[0]["outcome"] != nil && pending[0]["outcome"] != "" {
		t.Fatalf("pendentes: %v", pending)
	}
	byBody := map[string]string{}
	for _, d := range pending {
		byBody[strings.TrimSpace(d["body"].(string))] = d["id"].(string)
	}
	plain, edited, trash := byBody[goodReply+" "], byBody[goodReply+" x"], byBody[goodReply+" xx"]
	if plain == "" {
		plain = byBody[strings.TrimSpace(goodReply)]
	}
	if plain == "" || edited == "" || trash == "" {
		t.Fatalf("ids: %v", byBody)
	}
	sent := len(e.w.e.wa.Sent())

	// Aprovar como está: sai como o agente que escreveu (Grok), com o texto do rascunho.
	r := e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+plain+"/approve", map[string]any{})
	expect(t, r, 200)
	if r.body["status"] != "enviado" || len(e.w.e.wa.Sent()) != sent+1 || !strings.HasSuffix(lastSent(t, e.w).Text, strings.TrimSpace(goodReply)) {
		t.Fatalf("aprovado: %v / %q", r.body, lastSent(t, e.w).Text)
	}
	if got := scalar[string](t, e.w, `SELECT payload->>'aiProvider' FROM messages WHERE id = $1`, r.body["messageId"]); got != "grok" {
		t.Fatalf("a mensagem leva o provedor do agente: %q", got)
	}
	if got := scalar[string](t, e.w, `SELECT payload->>'aiLane' FROM messages WHERE id = $1`, r.body["messageId"]); got != "certificado" {
		t.Fatalf("a mensagem leva a raia: %q", got)
	}
	// Não envia duas vezes.
	expect(t, e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+plain+"/approve", map[string]any{}), 409)
	expect(t, e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+plain+"/discard", nil), 409)
	if len(e.w.e.wa.Sent()) != sent+1 {
		t.Fatal("aprovar de novo não pode reenviar")
	}

	// Editar e enviar: sai o texto editado.
	r = e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+edited+"/approve", map[string]any{"text": "Texto revisado pela Ana: agende a validação por vídeo."})
	expect(t, r, 200)
	if !strings.HasSuffix(lastSent(t, e.w).Text, "Texto revisado pela Ana: agende a validação por vídeo.") {
		t.Fatalf("editado: %q", lastSent(t, e.w).Text)
	}
	if got := scalar[string](t, e.w, `SELECT outcome || ':' || final_body FROM ai_lane_drafts WHERE id = $1`, edited); got != "edited:Texto revisado pela Ana: agende a validação por vídeo." {
		t.Fatalf("registro do editado: %q", got)
	}

	// Descartar: nada sai.
	sent = len(e.w.e.wa.Sent())
	expect(t, e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+trash+"/discard", nil), 204)
	if len(e.w.e.wa.Sent()) != sent {
		t.Fatal("descartar não envia")
	}
	if got := e.drafts("?pending=1"); len(got) != 0 {
		t.Fatalf("nada pendente: %v", got)
	}
	all := e.drafts("")
	outcomes := map[string]int{}
	for _, d := range all {
		o, _ := d["outcome"].(string)
		outcomes[o]++
	}
	if outcomes["approved"] != 1 || outcomes["edited"] != 1 || outcomes["discarded"] != 1 {
		t.Fatalf("histórico: %v", outcomes)
	}
	// Só administrador; id desconhecido.
	expect(t, e.w.ana.do("POST", "/api/v1/automation/lane-drafts/"+plain+"/approve", map[string]any{}), 403)
	expect(t, e.w.admin.do("POST", "/api/v1/automation/lane-drafts/22222222-2222-4222-8222-222222222222/approve", map[string]any{}), 404)
}

// Entre o rascunho e a aprovação o mundo muda: uma pessoa assumiu, a conversa foi passada para a equipe ou o envio está pausado.
func TestLaneDraftApprovalRechecksTheConversation(t *testing.T) {
	e := laneSetup(t, "shadow", nil)
	e.send(goodReply)
	e.send(goodReply + " de novo")
	ds := e.drafts("?pending=1")
	if len(ds) != 2 {
		t.Fatalf("rascunhos: %v", ds)
	}
	// Uma pessoa assume a conversa.
	expect(t, e.w.send(e.w.ana, e.convID, "Oi, aqui é a Ana."), 201)
	sent := len(e.w.e.wa.Sent())
	r := e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+ds[0]["id"].(string)+"/approve", map[string]any{})
	expect(t, r, 422)
	if code := e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+ds[1]["id"].(string)+"/approve", map[string]any{}).body["error"].(map[string]any)["code"]; code != "humano_ativo" {
		t.Fatalf("com uma pessoa na conversa: %v", code)
	}
	if len(e.w.e.wa.Sent()) != sent {
		t.Fatal("nada pode ter saído")
	}
	// O rascunho continua pendente (não foi gasto) e pode ser descartado.
	if got := e.drafts("?pending=1"); len(got) != 2 {
		t.Fatalf("continuam pendentes: %v", got)
	}
	expect(t, e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+ds[0]["id"].(string)+"/discard", nil), 204)
	// Envio pausado pelo administrador também barra a aprovação.
	expect(t, e.w.admin.do("POST", "/api/v1/automation/pause", map[string]any{"paused": true, "reason": "teste"}), 204)
	r = e.w.admin.do("POST", "/api/v1/automation/lane-drafts/"+ds[1]["id"].(string)+"/approve", map[string]any{})
	expect(t, r, 422)
}

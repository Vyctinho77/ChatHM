package automation

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)

func baseLane() Lane {
	return Lane{Key: "certificado", Label: "Certificado digital", Enabled: true, Mode: LaneLive, MaxTurns: 8, MaxConsecutive: 2, ContactDaily: 6,
		HumanQuietMinutes: 60}
}

func baseFacts() laneFacts {
	return laneFacts{ConvID: "c1", ContactID: "k1", Status: "new", Now: t0}
}

func code(d laneDecision) string {
	switch {
	case d.Refusal != nil:
		return d.Refusal.Code
	case d.Handoff != nil:
		return "handoff:" + d.Handoff.Reason
	}
	return "ok"
}

func TestDecideLaneConversationWalls(t *testing.T) {
	l := baseLane()
	ok := "Para emitir o certificado, você precisa agendar a validação por vídeo. Quer que eu explique o passo a passo?"
	cases := []struct {
		name  string
		lane  func(*Lane)
		facts func(*laneFacts)
		want  string
	}{
		{"conversa normal passa", nil, nil, "ok"},
		{"raia desligada", func(l *Lane) { l.Enabled = false }, nil, "raia_desligada"},
		{"conversa encerrada", nil, func(f *laneFacts) { f.Status = "closed" }, "encerrada"},
		{"fora da fila da raia", func(l *Lane) { l.SegmentID = "q-cert" }, func(f *laneFacts) { f.SegmentID = "q-fiscal" }, "fora_da_raia"},
		{"na fila da raia", func(l *Lane) { l.SegmentID = "q-cert" }, func(f *laneFacts) { f.SegmentID = "q-cert" }, "ok"},
		{"atendente humano com a conversa", nil, func(f *laneFacts) { f.Assignee = "u1" }, "humano_ativo"},
		{"humano respondeu há pouco", nil, func(f *laneFacts) { x := t0.Add(-20 * time.Minute); f.LastHumanOut = &x }, "humano_ativo"},
		{"humano respondeu faz tempo", nil, func(f *laneFacts) { x := t0.Add(-3 * time.Hour); f.LastHumanOut = &x }, "ok"},
		{"já passada para humano", nil, func(f *laneFacts) { f.OpenHandoff = true }, "passada_para_humano"},
		{"conversa adiada", nil, func(f *laneFacts) { f.Snoozed = true }, "adiada"},
		{"limite de rodadas passa para humano", nil, func(f *laneFacts) { f.Turns = 8 }, "handoff:turnos"},
		{"duas seguidas sem resposta do cliente", nil, func(f *laneFacts) { f.Consecutive = 2 }, "aguarde_cliente"},
		{"limite diário por cliente", nil, func(f *laneFacts) { f.ContactToday = 6 }, "limite_contato"},
	}
	for _, c := range cases {
		lane, facts := l, baseFacts()
		if c.lane != nil {
			c.lane(&lane)
		}
		if c.facts != nil {
			c.facts(&facts)
		}
		if got := code(decideLane(lane, facts, ok)); got != c.want {
			t.Errorf("%s: %s, esperava %s", c.name, got, c.want)
		}
	}
}

func TestDecideLaneInboundGoesToHuman(t *testing.T) {
	l := baseLane()
	in := func(kind, body string) laneFacts {
		f := baseFacts()
		f.Inbound = []inboundMsg{{Kind: kind, Body: body, At: t0}}
		return f
	}
	cases := []struct {
		name string
		f    laneFacts
		want string
	}{
		{"pergunta simples", in("text", "Bom dia, quero saber como renovar meu certificado"), "ok"},
		{"áudio passa (transcrição)", in("audio", ""), "ok"},
		{"foto de documento", in("image", ""), "handoff:documento"},
		{"documento (PDF)", in("document", ""), "handoff:documento"},
		{"CPF digitado não trava", in("text", "meu cpf é 123.456.789-01"), "ok"},
		{"número longo não trava", in("text", "o protocolo é 20261118334455"), "ok"},
		{"fala de senha não trava", in("text", "qual a senha do certificado?"), "ok"},
		{"pede atendente", in("text", "quero falar com um atendente"), "handoff:pediu_humano"},
		{"reclamação", in("text", "isso é um absurdo, vou no procon"), "handoff:pediu_humano"},
		{"localização não trava", in("location", ""), "ok"},
	}
	for _, c := range cases {
		if got := code(decideLane(l, c.f, "Claro, posso ajudar.")); got != c.want {
			t.Errorf("%s: %s, esperava %s", c.name, got, c.want)
		}
	}
	// Muitas mensagens seguidas do cliente não travam a IA.
	f := in("text", "oi")
	if got := code(decideLane(l, f, "Olá!")); got != "ok" {
		t.Errorf("várias mensagens: %s", got)
	}
}

func TestTextRefusalRules(t *testing.T) {
	free := baseLane()
	cobr := baseLane()
	cobr.AllowedDomains = []string{"asaas.com"}
	cases := []struct {
		name string
		lane Lane
		text string
		want string
	}{
		{"texto comum", free, "Para o e-CNPJ A1 você precisa de uma validação por videoconferência.", ""},
		{"valores e números livres", free, "O certificado custa R$ 150 e o protocolo é 123 4567 890.", ""},
		{"CPF, e-mail e pressão livres", free, "Confirme o CPF 123.456.789-01 ou escreva para contato@exemplo.com.br, senão protestamos.", ""},
		{"lista vazia = qualquer link", free, "Veja em https://exemplo.com.br/certificado", ""},
		{"pede senha", free, "Me envie a senha do certificado para eu testar.", "credencial"},
		{"pede código", free, "Qual o código de verificação que chegou?", "credencial"},
		{"pede chave pix", free, "Me passe a chave pix do cliente.", "credencial"},
		{"caracteres invisíveis", free, "Olá\u200b tudo bem", "content"},
		{"cobrança com o link do Asaas", cobr, "O boleto de R$ 350,00 vence em 25/11/2026: https://www.asaas.com/i/abc123xyz", ""},
		{"cobrança com link de outro domínio", cobr, "Pague aqui https://pagamento-falso.com/x", "link"},
		{"domínio sem http fora da lista", cobr, "Acesse gov.br/pagar para ver", "link"},
	}
	for _, c := range cases {
		r := textRefusal(c.lane, c.text)
		got := ""
		if r != nil {
			got = r.Code
		}
		if got != c.want {
			t.Errorf("%s: %q, esperava %q (%v)", c.name, got, c.want, r)
		}
	}
}

func TestNoAIDisclosurePrefix(t *testing.T) {
	d := decideLane(baseLane(), baseFacts(), "Posso ajudar com o certificado.")
	if d.Refusal != nil || d.Text != "Posso ajudar com o certificado." {
		t.Fatalf("o texto da IA sai como ela escreveu, sem apresentação: %+v", d)
	}
}

func TestOrderOfWallsConversationBeforeContent(t *testing.T) {
	// Um texto ruim numa conversa que já é de um humano deve ser recusado pelo humano ativo (a regra da conversa vem antes).
	l := baseLane()
	f := baseFacts()
	f.Assignee = "u1"
	if got := code(decideLane(l, f, "Me mande a senha")); got != "humano_ativo" {
		t.Fatalf("humano ativo vem primeiro: %s", got)
	}
	// E um cliente mandando documento passa para humano antes de se discutir o texto da IA.
	f = baseFacts()
	f.Inbound = []inboundMsg{{Kind: "image"}}
	if got := code(decideLane(l, f, "Me mande a senha")); got != "handoff:documento" {
		t.Fatalf("a passagem para humano vem antes do texto: %s", got)
	}
}

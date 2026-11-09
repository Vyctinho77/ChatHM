package automation

import (
	"strings"
	"testing"
)

var testFields = map[string]string{"pix_escritorio": "chave@escritorio", "razao_social": "Escritório Exemplo"}

func TestValidateTextAcceptsNormalNotices(t *testing.T) {
	for _, ok := range []string{
		"Olá, {nome}! A guia do DAS de 10/2026 já está disponível, com vencimento em 20/11. Qualquer dúvida, é só avisar.",
		"Oi {nome}, os honorários de outubro (R$ 1.234,56) vencem dia 10/11. Para pagar use a chave: {pix_escritorio}.",
		"Bom dia! Lembrete: faltam os extratos de 10/2026 para fechar a folha. Podem enviar até sexta (14/11).",
		"Valor total de R$ 12.345.678,90 referente à competência 09/2026.", // valores grandes não são "número longo"
		"Aviso: o escritório fecha dia 20/11 (feriado) e volta no dia 21.",
	} {
		if _, ref := ValidateText(ok, nil, testFields); ref != nil {
			t.Errorf("deveria aceitar %q: %v", ok, ref)
		}
	}
}

func TestValidateTextRefusesWhatTheAIMustNeverTypeItself(t *testing.T) {
	long := strings.Repeat("a", maxTextRunes+1)
	cases := map[string]string{
		"vazio":                  "   ",
		"longo demais":           long,
		"negrito":                "Olá *cliente*, tudo bem?",
		"código":                 "Use `isto` para pagar",
		"riscado":                "Valor ~antigo~ corrigido",
		"itálico":                "Olá _cliente_, tudo bem?",
		"link":                   "Pague em https://pagar.exemplo.com/abc",
		"link sem esquema":       "Veja www.exemplo.com.br/boleto",
		"domínio solto":          "Acesse pagamento.com hoje",
		"e-mail":                 "Responda para financeiro@escritorio.com.br",
		"telefone":               "Ligue para (92) 99999-8888",
		"telefone colado":        "Chame no 92999998888",
		"cpf":                    "Seu CPF 123.456.789-01 está regular",
		"cnpj":                   "CNPJ 12.345.678/0001-90",
		"chave pix digitada":     "Pague na chave 12345678901234",
		"uuid (chave aleatória)": "Chave pix 123e4567-e89b-12d3-a456-426614174000",
		"variável inexistente":   "Pague em {chave_pix}",
		"invisível":              "Olá​ cliente",
		"inversão de leitura":    "Valor ‮100",
		"linhas demais":          strings.Repeat("a\n", 12),
	}
	for name, text := range cases {
		if _, ref := ValidateText(text, nil, testFields); ref == nil || ref.Code != "content" {
			t.Errorf("%s: deveria recusar %q (recusa=%v)", name, text, ref)
		}
	}
}

func TestValidateTextAllowedDomainsAndPressureFlags(t *testing.T) {
	if _, ref := ValidateText("Baixe a guia em www.gov.br/receitafederal", []string{"gov.br"}, nil); ref != nil {
		t.Errorf("domínio liberado deveria passar: %v", ref)
	}
	if _, ref := ValidateText("Baixe em pagamento.evil-gov.br/x", []string{"gov.br"}, nil); ref == nil {
		t.Error("domínio parecido (evil-gov.br) não é gov.br")
	}
	if _, ref := ValidateText("Baixe em gov.br.evil.com/x", []string{"gov.br"}, nil); ref == nil {
		t.Error("gov.br no começo do host não libera evil.com")
	}
	flags, ref := ValidateText("Se não pagar, vamos levar a protesto e negativar seu nome no SPC.", nil, nil)
	if ref != nil {
		t.Fatalf("termos de pressão avisam, não bloqueiam: %v", ref)
	}
	got := strings.Join(flags, ",")
	for _, want := range []string{"pressao:protesto", "pressao:negativ", "pressao:spc"} {
		if !strings.Contains(got, want) {
			t.Errorf("faltou o aviso %s em %v", want, flags)
		}
	}
	if flags, _ := ValidateText("Segue a guia do mês, com vencimento dia 20.", nil, nil); len(flags) != 0 {
		t.Errorf("texto cordial não deveria ter aviso: %v", flags)
	}
}

func TestRenderSubstitutesFixedFieldsAndAddsOptOutFooter(t *testing.T) {
	out := Render("Olá, {nome}! Pague na chave {pix_escritorio}.", "Maria Souza", testFields)
	if !strings.HasPrefix(out, "Olá, Maria! Pague na chave chave@escritorio.") || !strings.HasSuffix(out, optOutFooter) {
		t.Fatalf("render = %q", out)
	}
	if got := Render("Oi {nome}", "+5592988880000", nil); !strings.HasPrefix(got, "Oi cliente") {
		t.Fatalf("sem nome cadastrado deveria virar 'cliente': %q", got)
	}
	if got := Render("Oi {nome}", "", nil); !strings.HasPrefix(got, "Oi cliente") {
		t.Fatalf("nome vazio: %q", got)
	}
}

func TestAuthorIsNeverAPersonsName(t *testing.T) {
	if Author("") != "Assistente do escritório" || Author("  ") != "Assistente do escritório" {
		t.Fatal("sem assinatura configurada usa o padrão")
	}
	if got := Author("*Escritório   Exemplo*"); got != "Escritório Exemplo" {
		t.Fatalf("assinatura limpa = %q", got)
	}
}

func TestCleanFieldsAndDomains(t *testing.T) {
	if f, err := CleanFields(map[string]string{"pix_escritorio": " 12.345.678/0001-90 "}); err != nil || f["pix_escritorio"] != "12.345.678/0001-90" {
		t.Fatalf("campo válido: %v %v", f, err)
	}
	for _, bad := range []map[string]string{
		{"Pix": "x"}, {"nome": "x"}, {"a": "x"}, {"pix_escritorio": ""}, {"pix_escritorio": "a\nb"}, {"pix_escritorio": "*x*"}, {"pix_escritorio": strings.Repeat("x", 201)},
	} {
		if _, err := CleanFields(bad); err == nil {
			t.Errorf("deveria recusar o campo %v", bad)
		}
	}
	d, err := CleanDomains([]string{"GOV.BR", "https://receita.fazenda.gov.br/", "gov.br", ""})
	if err != nil || len(d) != 2 || d[0] != "gov.br" || d[1] != "receita.fazenda.gov.br" {
		t.Fatalf("domínios = %v %v", d, err)
	}
	for _, bad := range []string{"sem-ponto", "a b.com", "http://x", "-x.com"} {
		if _, err := CleanDomains([]string{bad}); err == nil && bad != "http://x" {
			t.Errorf("deveria recusar o domínio %q", bad)
		}
	}
}

func TestCleanDomainsEmptyIsNotNil(t *testing.T) {
	got, err := CleanDomains(nil)
	if err != nil || got == nil {
		t.Fatalf("got %v, %v", got, err)
	}
}

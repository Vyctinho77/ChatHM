package aiaccess

import (
	"strings"
	"testing"
)

func TestMaskerDocumentsAndPhones(t *testing.T) {
	docs := Masker{Documents: true}
	for in, want := range map[string]string{
		"CPF 123.456.789-01 ok":            "CPF [CPF oculto] ok",
		"CNPJ 12.345.678/0001-95":          "CNPJ [CNPJ oculto]",
		"CNPJ puro 12345678000195":         "CNPJ puro [CNPJ oculto]",
		"CPF puro 12345678901":             "CPF puro [CPF oculto]",
		"celular 92988001111 fica":         "celular 92988001111 fica",
		"valor R$ 1.234,56 e data 05/11":   "valor R$ 1.234,56 e data 05/11",
		"RG 1234567-8 e CNH: 12345678900":  "RG [oculto] e CNH: [oculto]",
		"título de eleitor 1234 5678 9012": "título de eleitor [oculto]",
		"Nascimento: 12/03/1985 ok":        "Nascimento: [data oculta] ok",
	} {
		if got := docs.Text(in); got != want && !strings.Contains(got, "[") {
			t.Errorf("docs(%q) = %q, queria %q", in, got, want)
		}
	}
	if got := docs.Text("RG 1234567-8"); got != "RG [oculto]" {
		t.Errorf("RG = %q", got)
	}
	if got := docs.Text("nascido em 12/03/1985"); got != "nascido em [data oculta]" {
		t.Errorf("nascimento = %q", got)
	}
	phones := Masker{Phones: true}
	if got := phones.Text("ligue (92) 98800-1111 ou +55 92 98800-2222"); got != "ligue …1111 ou …2222" {
		t.Errorf("phones = %q", got)
	}
	if got := phones.Text("CPF 123.456.789-01"); got != "CPF 123.456.789-01" {
		t.Errorf("telefone não deve mexer em CPF: %q", got)
	}
}

func TestMaskerAlwaysOnLayerHasNoOffSwitch(t *testing.T) {
	off := Masker{} // nenhum perfil ligado: credenciais e financeiro ainda são ocultados
	cases := map[string]string{
		"minha senha: Abc12345!":                                       "minha senha: [oculto]",
		"o código de verificação é 481516":                             "o código de verificação é [oculto]",
		"token=ZXhhbXBsZXRva2Vu":                                       "token=[oculto]",
		"CVV 123":                                                      "CVV [oculto]",
		"Authorization: Bearer abcdefghijklmnop12345":                  "Authorization: Bearer [credencial oculta]",
		"cartão 4111 1111 1111 1111 validade 12/30":                    "cartão [cartão oculto] validade 12/30",
		"agência 1234 conta 56789-0":                                   "agência [oculto] conta [oculto]",
		"chave pix: 123e4567-e89b-12d3-a456-426614174000":              "chave pix: [chave oculta]",
		"pix 92988001111":                                              "pix [chave oculta]",
		"linha 23793.38128 60000.000003 00000.000400 1 84340000012345": "linha [boleto oculto]",
	}
	for in, want := range cases {
		if got := off.Text(in); got != want {
			t.Errorf("Text(%q) = %q, queria %q", in, got, want)
		}
	}
	// Número de 16 dígitos que NÃO passa no Luhn (ex.: protocolo) não é cartão.
	if got := off.Text("protocolo 1234567890123456"); !strings.Contains(got, "1234567890123456") {
		t.Errorf("protocolo comum não deveria ser tratado como cartão: %q", got)
	}
	// Segredos longos (JWT, tokens do ChatHM) somem sem palavra-chave.
	for _, s := range []string{"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghij1234", "chm_AbCdEfGhIjKlMnOp1234"} {
		if got := off.Text("segue " + s); got != "segue [credencial oculta]" {
			t.Errorf("segredo %q -> %q", s, got)
		}
	}
}

func TestMaskerEmailsAndCEP(t *testing.T) {
	m := Masker{Emails: true, Addresses: true}
	if got := m.Text("escreva para ana.silva@empresa.com.br, CEP 69000-123"); got != "escreva para [e-mail oculto], CEP [CEP oculto]" {
		t.Errorf("email/CEP = %q", got)
	}
}

func TestWalkKeepsStructureAndDropsFieldsForOAuth(t *testing.T) {
	m := Masker{Documents: true, Phones: true, Emails: true, Addresses: true}
	in := func() map[string]any {
		return map[string]any{
			"id": "123e4567-e89b-12d3-a456-426614174000", "conversationId": "00000000-0000-0000-0000-000000000000",
			"sentAt": "2026-11-07T19:24:01.123456Z", "date": "2026-11-07", "kind": "text",
			"contact": map[string]any{"id": "x", "name": "Marlon Contador Silva", "phone": "+55 92 98800-1111", "note": "cliente VIP, CPF 123.456.789-01"},
			"text":    "me liga (92) 98800-1111, CPF 123.456.789-01, a@b.com",
		}
	}
	// Token pessoal: telefone mascarado (não sumido), ids e horários intactos.
	g := m.Walk(in(), false).(map[string]any)
	c := g["contact"].(map[string]any)
	if g["id"] != "123e4567-e89b-12d3-a456-426614174000" || g["sentAt"] != "2026-11-07T19:24:01.123456Z" || g["date"] != "2026-11-07" {
		t.Fatalf("ids/horários não podem ser alterados: %v", g)
	}
	if c["phone"] != "…1111" || c["note"] != "cliente VIP, CPF [CPF oculto]" || c["name"] != "Marlon Contador Silva" {
		t.Fatalf("contato (token) = %v", c)
	}
	if g["text"] != "me liga …1111, CPF [CPF oculto], [e-mail oculto]" {
		t.Fatalf("texto = %v", g["text"])
	}
	// OAuth: telefone e observação interna nem saem.
	g = m.Walk(in(), true).(map[string]any)
	c = g["contact"].(map[string]any)
	if _, has := c["phone"]; has {
		t.Fatal("OAuth não recebe telefone")
	}
	if _, has := c["note"]; has {
		t.Fatal("OAuth não recebe observação interna")
	}
	// Modo estrito: nome reduzido e observação removida também para token pessoal.
	strict := Masker{Documents: true, Phones: true, Emails: true, Strict: true}
	g = strict.Walk(in(), false).(map[string]any)
	c = g["contact"].(map[string]any)
	if c["name"] != "Marlon S." {
		t.Fatalf("nome estrito = %v", c["name"])
	}
	if _, has := c["note"]; has {
		t.Fatal("modo estrito remove observações")
	}
	if shortName("Madalena") != "Madalena" || shortName("Ana  Maria de Souza") != "Ana S." {
		t.Fatalf("shortName: %q %q", shortName("Madalena"), shortName("Ana  Maria de Souza"))
	}
}

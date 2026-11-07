package aiaccess

import "testing"

func TestMasker(t *testing.T) {
	docs := Masker{Documents: true}
	for in, want := range map[string]string{
		"CPF 123.456.789-01 ok":          "CPF [CPF oculto] ok",
		"CNPJ 12.345.678/0001-95":        "CNPJ [CNPJ oculto]",
		"CNPJ puro 12345678000195":       "CNPJ puro [CNPJ oculto]",
		"CPF puro 12345678901":           "CPF puro [CPF oculto]",
		"celular 92988001111 fica":       "celular 92988001111 fica",
		"valor R$ 1.234,56 e data 05/11": "valor R$ 1.234,56 e data 05/11",
	} {
		if got := docs.Text(in); got != want {
			t.Errorf("docs(%q) = %q, queria %q", in, got, want)
		}
	}
	phones := Masker{Phones: true}
	if got := phones.Text("ligue (92) 98800-1111 ou +55 92 98800-2222"); got != "ligue …1111 ou …2222" {
		t.Errorf("phones = %q", got)
	}
	if got := phones.Text("CPF 123.456.789-01"); got != "CPF 123.456.789-01" {
		t.Errorf("telefone não deve mexer em CPF: %q", got)
	}
	// Walk percorre estruturas aninhadas.
	v := docs.Walk(map[string]any{"a": []any{"CPF 123.456.789-01", map[string]any{"b": "ok"}}, "n": 3.0})
	if got := v.(map[string]any)["a"].([]any)[0]; got != "CPF [CPF oculto]" {
		t.Errorf("walk = %v", got)
	}
}

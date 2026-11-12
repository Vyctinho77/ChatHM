package segments

import "testing"

func TestNormalizeKeywordKeepsWildcardPrefix(t *testing.T) {
	cases := map[string]string{
		"emit* nota":      "emit* nota",
		"EMISS* de Notas": "emiss* nota",
		"Férias":          "feria",
		"cancel*":         "cancel*",
		"nota fiscal":     "nota fiscal",
		"13º salário":     "13o salario",
		"ab*":             "ab",      // curto demais: sem curinga (a validação recusa antes de gravar)
		"folhas*":         "folhas*", // o prefixo vale como foi escrito
	}
	for in, want := range cases {
		if got := NormalizeKeyword(in); got != want {
			t.Errorf("NormalizeKeyword(%q) = %q; esperado %q", in, got, want)
		}
	}
}

func TestValidWildcards(t *testing.T) {
	for _, ok := range []string{"nota fiscal", "emit* nota", "cancel*", "folha de pagamento", "13º salário"} {
		if !ValidWildcards(ok) {
			t.Errorf("ValidWildcards(%q) = false", ok)
		}
	}
	for _, bad := range []string{"a*", "ab*", "*nota", "no*ta", "emit*r", "**", "nota *"} {
		if ValidWildcards(bad) {
			t.Errorf("ValidWildcards(%q) = true", bad)
		}
	}
}

func TestWildcardMatchesVerbFormsAndStaysWholeWordOtherwise(t *testing.T) {
	kws := []Keyword{
		{SegmentID: "n", SegmentKey: "notas", Phrase: "emit* nota", Normalized: NormalizeKeyword("emit* nota"), Priority: 50},
		{SegmentID: "f", SegmentKey: "folha", Phrase: "folha", Normalized: NormalizeKeyword("folha"), Priority: 50},
	}
	for _, text := range []string{"Preciso emitir uma nota", "emita a nota por favor", "Vou emitindo nota todo mês", "já emitiu nota?", "emitido nota"} {
		if r := Classify(text, kws); r.SegmentKey != "notas" || r.Reason != ReasonKeyword {
			t.Errorf("%q → %v/%v; esperado notas", text, r.SegmentKey, r.Reason)
		}
	}
	// O prefixo precisa ser o começo da palavra e as palavras precisam estar juntas e em ordem.
	for _, text := range []string{"remitir nota", "emitir folha", "nota emitir", "emi nota"} {
		if r := Classify(text, kws); r.SegmentKey == "notas" {
			t.Errorf("%q não deveria combinar com 'emit* nota'", text)
		}
	}
	// Sem curinga, palavra inteira: "folha" não combina com "folhagem".
	if r := Classify("a folhagem do jardim", kws); r.Reason != ReasonNone {
		t.Errorf("folhagem combinou: %v", r)
	}
}

func TestSuggestionsFollowTheSegmentNameAndSkipExisting(t *testing.T) {
	names := func(name, key string, have map[string]bool) map[string]int {
		out := map[string]int{}
		for _, s := range suggestFor(name, key, have) {
			out[s.Phrase] = s.Priority
		}
		return out
	}
	fiscal := names("Fiscal", "notas", nil)
	for _, want := range []string{"nota fiscal", "danfe", "pgdas"} { // "Fiscal" recebe notas e impostos
		if _, ok := fiscal[want]; !ok {
			t.Errorf("Fiscal sem a sugestão %q", want)
		}
	}
	if fiscal["danfe"] != 60 || fiscal["nota"] != 40 {
		t.Errorf("prioridades: danfe=%d nota=%d", fiscal["danfe"], fiscal["nota"])
	}
	folha := names("DP", "dp", nil)
	if _, ok := folha["holerite"]; !ok {
		t.Error("DP deveria receber as sugestões de folha")
	}
	if _, ok := folha["danfe"]; ok {
		t.Error("DP não deveria receber sugestões de notas")
	}
	// Sem relação com os pacotes: nada a sugerir. O que já existe não volta.
	if got := names("Financeiro", "financeiro", nil); len(got) != 0 {
		t.Errorf("Financeiro recebeu %v", got)
	}
	skip := names("Notas", "notas", map[string]bool{NormalizeKeyword("nota fiscal"): true, NormalizeKeyword("emit* nota"): true})
	if _, ok := skip["nota fiscal"]; ok {
		t.Error("repetiu 'nota fiscal'")
	}
	if _, ok := skip["emit* nota"]; ok {
		t.Error("repetiu 'emit* nota'")
	}
}

func TestEverySuggestionIsValid(t *testing.T) {
	seen := map[string]string{}
	for _, p := range packs {
		for _, list := range [][]string{p.generic, p.precise} {
			for _, ph := range list {
				if !ValidWildcards(ph) || NormalizeKeyword(ph) == "" {
					t.Errorf("sugestão inválida %q (%s)", ph, p.label)
				}
				if prev, dup := seen[NormalizeKeyword(ph)]; dup && prev == p.label {
					t.Errorf("repetida no mesmo pacote: %q", ph)
				}
				seen[NormalizeKeyword(ph)] = p.label
			}
		}
	}
	if got := packsFor("Fiscal", "notas"); len(got) < 2 {
		t.Errorf("packsFor(Fiscal) = %d pacotes", len(got))
	}
}

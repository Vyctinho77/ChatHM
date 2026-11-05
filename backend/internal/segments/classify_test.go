package segments

import (
	"reflect"
	"testing"
)

func kw(seg, key, phrase string, prio int) Keyword {
	return Keyword{SegmentID: seg, SegmentKey: key, Phrase: phrase, Normalized: Normalize(phrase), Priority: prio}
}

// defaultKeywords monta as palavras-chave padrão como o serviço faz.
func defaultKeywords() []Keyword {
	var out []Keyword
	for _, d := range Defaults {
		for _, p := range d.Keywords {
			out = append(out, kw("id-"+d.Key, d.Key, p, 0))
		}
	}
	return out
}

func TestTokens(t *testing.T) {
	cases := map[string][]string{
		"Quero EMITIR uma Nota!":           {"quero", "emitir", "nota"},
		"Preciso de FÉRIAS de funcionário": {"preciso", "feria", "funcionario"},
		"Emissões de notas fiscais":        {"emissao", "nota", "fiscal"},
		"13º salário, por favor":           {"13o", "salario", "favor"},
		"  espaços   e\tquebras\n":         {"espaco", "quebra"},
		"":                                 nil,
		"de a o":                           nil,
		"nfs-e":                            {"nfs"}, // "e" é palavra de ligação
		"processo (2026)":                  {"processo", "2026"},
		"classe":                           {"classe"}, // "ss" final não é plural
		"mais":                             {"mai"},    // consistente nos dois lados da comparação
	}
	for in, want := range cases {
		if got := Tokens(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Tokens(%q) = %v, esperado %v", in, got, want)
		}
	}
}

// Critérios de aceite da Fase 3 (blueprint).
func TestAcceptanceExamples(t *testing.T) {
	kws := defaultKeywords()
	cases := []struct {
		text    string
		want    string // segmento ou "" (fila geral)
		reason  Reason
		comment string
	}{
		{"quero emitir uma nota", "notas", ReasonKeyword, "exemplo do blueprint"},
		{"preciso calcular a folha", "folha", ReasonKeyword, "exemplo do blueprint"},
		{"quero contratar", "clientes_novos", ReasonKeyword, "exemplo do blueprint"},
		{"preciso de férias de funcionário", "folha", ReasonKeyword, "critério de aceite"},
		{"quero orçamento", "clientes_novos", ReasonKeyword, "critério de aceite"},
		{"bom dia", "", ReasonNone, "sem correspondência"},
		{"Bom dia, tudo bem?", "", ReasonNone, "sem correspondência"},
		{"quero orçamento da folha de pagamento", "", ReasonConflict, "dois segmentos empatam"},
		{"EMITIR NOTA FISCAL URGENTE", "notas", ReasonKeyword, "caixa e acentos"},
		{"preciso emitir as notas do mês", "notas", ReasonKeyword, "plural e palavras de ligação"},
		{"o folheto ficou pronto", "", ReasonNone, "palavra inteira: folheto ≠ folha"},
		{"minha folhagem secou", "", ReasonNone, "palavra inteira: folhagem ≠ folha"},
		{"nota", "", ReasonNone, "palavra isolada genérica não classifica"},
		{"preciso das notas fiscais de outubro", "notas", ReasonKeyword, "plural -ais"},
		{"quero um orçamento para abrir uma empresa", "clientes_novos", ReasonKeyword, "palavras de ligação"},
	}
	for _, c := range cases {
		got := Classify(c.text, kws)
		if got.SegmentKey != c.want || got.Reason != c.reason {
			t.Errorf("%q (%s): segmento=%q motivo=%s, esperado %q/%s (matches=%v)",
				c.text, c.comment, got.SegmentKey, got.Reason, c.want, c.reason, got.Matches)
		}
	}
}

func TestPriorityBreaksConflict(t *testing.T) {
	kws := []Keyword{
		kw("A", "folha", "folha", 0),
		kw("B", "clientes", "orçamento", 0),
	}
	if r := Classify("orçamento da folha", kws); r.Reason != ReasonConflict || r.SegmentID != "" {
		t.Fatalf("esperava conflito: %+v", r)
	}
	kws = append(kws, kw("B", "clientes", "orçamento", 0)) // duplicata inofensiva
	kws[1].Priority = 10
	r := Classify("orçamento da folha", kws)
	if r.Reason != ReasonKeyword || r.SegmentKey != "clientes" {
		t.Fatalf("prioridade maior deveria vencer: %+v", r)
	}
	if len(r.Matches) < 2 || r.Matches[0].Priority != 10 {
		t.Fatalf("matches deveriam vir por prioridade: %+v", r.Matches)
	}
}

func TestSamePriorityInsideOneSegmentIsNotConflict(t *testing.T) {
	kws := []Keyword{kw("A", "notas", "emitir nota", 0), kw("A", "notas", "nota fiscal", 0)}
	if r := Classify("emitir nota fiscal", kws); r.Reason != ReasonKeyword || r.SegmentKey != "notas" {
		t.Fatalf("várias palavras do mesmo segmento não são conflito: %+v", r)
	}
}

func TestPhraseNeedsWordsInOrderAndTogether(t *testing.T) {
	kws := []Keyword{kw("A", "notas", "emitir nota", 0)}
	for text, want := range map[string]bool{
		"emitir nota":                 true,
		"vou emitir uma nota hoje":    true,
		"nota para emitir":            false, // fora de ordem
		"emitir a minha segunda nota": false, // palavra no meio ("segunda")
	} {
		if got := Classify(text, kws).SegmentKey == "notas"; got != want {
			t.Errorf("%q: combinou=%v, esperado %v", text, got, want)
		}
	}
}

func TestEmptyInputs(t *testing.T) {
	if r := Classify("", defaultKeywords()); r.Reason != ReasonNone || r.Matches == nil {
		t.Fatalf("texto vazio: %+v", r)
	}
	if r := Classify("quero emitir nota", nil); r.Reason != ReasonNone {
		t.Fatalf("sem palavras-chave: %+v", r)
	}
	// palavra-chave inválida (vazia) é ignorada, nunca combina com tudo.
	if r := Classify("qualquer coisa", []Keyword{{SegmentID: "A", Normalized: ""}}); r.Reason != ReasonNone {
		t.Fatalf("palavra-chave vazia combinou: %+v", r)
	}
}

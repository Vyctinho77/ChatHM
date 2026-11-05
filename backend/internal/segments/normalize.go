// Package segments contém o motor de classificação e o cadastro de segmentos (filas).
package segments

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// stopwords são palavras de ligação ignoradas na comparação (nos textos e nas
// palavras-chave), para que "emitir uma nota" combine com a palavra-chave "emitir nota".
var stopwords = map[string]struct{}{
	"a": {}, "o": {}, "as": {}, "os": {}, "um": {}, "uma": {}, "uns": {}, "umas": {},
	"de": {}, "do": {}, "da": {}, "dos": {}, "das": {}, "no": {}, "na": {}, "nos": {}, "nas": {},
	"em": {}, "ao": {}, "aos": {}, "para": {}, "pra": {}, "pro": {}, "por": {}, "com": {},
	"e": {}, "que": {}, "meu": {}, "minha": {}, "meus": {}, "minhas": {}, "seu": {}, "sua": {},
	"seus": {}, "suas": {}, "me": {}, "te": {}, "se": {},
}

// Tokens normaliza um texto em palavras comparáveis:
//
//  1. minúsculas e remoção de acentos ("Férias" → "ferias"; "13º" → "13o");
//  2. qualquer pontuação separa palavras;
//  3. palavras de ligação são descartadas ("de", "uma", "para"…);
//  4. plural simples vira singular ("notas" → "nota", "emissões" → "emissao"),
//     aplicado igualmente às palavras-chave, então a comparação é sempre consistente.
//
// A comparação posterior é por palavra inteira: "folha" não combina com "folhagem".
func Tokens(s string) []string {
	decomposed := norm.NFKD.String(strings.ToLower(s))
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r): // acentos soltos (após a decomposição)
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	var out []string
	for _, w := range strings.Fields(b.String()) {
		if _, skip := stopwords[w]; skip {
			continue
		}
		out = append(out, singular(w))
	}
	return out
}

// Normalize devolve os tokens unidos por espaço (forma gravada no banco).
func Normalize(s string) string { return strings.Join(Tokens(s), " ") }

// singular reduz os plurais mais comuns do português à forma singular. Não é um
// stemmer completo: o objetivo é só que "nota/notas", "fiscal/fiscais", "valor/valores"
// e "emissão/emissões" se comparem como iguais.
func singular(w string) string {
	n := len(w)
	has := func(suffix string) bool { return n > 4 && strings.HasSuffix(w, suffix) }
	switch {
	case has("oes"), has("aes"): // emissões, alemães
		return w[:n-3] + "ao"
	case has("ais"): // fiscais, mensais
		return w[:n-3] + "al"
	case has("eis"): // papéis
		return w[:n-3] + "el"
	case has("ns"): // imagens
		return w[:n-2] + "m"
	case has("res"), has("zes"): // valores, vezes
		return w[:n-2]
	case n > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:n-1]
	}
	return w
}

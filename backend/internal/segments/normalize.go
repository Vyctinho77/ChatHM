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
func Tokens(s string) []string { return tokenize(s, true) }

// tokenize é Tokens com a opção de não reduzir o plural (usada nos prefixos com curinga, que valem como foram escritos).
func tokenize(s string, plural bool) []string {
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
		if plural {
			w = singular(w)
		}
		out = append(out, w)
	}
	return out
}

// Normalize devolve os tokens unidos por espaço (forma gravada no banco).
func Normalize(s string) string { return strings.Join(Tokens(s), " ") }

// MinWildcard é o mínimo de letras antes do "*" (com menos, o curinga combinaria com palavras demais).
const MinWildcard = 3

// NormalizeKeyword normaliza uma palavra-chave cadastrada. Igual a Normalize, mas uma palavra terminada em "*"
// vira um prefixo: "emit*" combina com emitir, emita, emitiu, emitido e emissão não (outro radical: use "emiss*").
// O prefixo vale como foi escrito (sem acento, caixa ou plural). Palavras com curinga curto demais ficam sem ele;
// ValidPhrase recusa esse caso antes de gravar.
func NormalizeKeyword(s string) string {
	var out []string
	for _, raw := range strings.Fields(s) {
		if strings.HasSuffix(raw, "*") {
			if t := tokenize(raw, false); len(t) == 1 && len([]rune(t[0])) >= MinWildcard {
				out = append(out, t[0]+"*")
				continue
			}
		}
		out = append(out, Tokens(raw)...)
	}
	return strings.Join(out, " ")
}

// ValidWildcards confere o uso do "*": só no fim de uma palavra e com letras suficientes antes dele.
func ValidWildcards(phrase string) bool {
	for _, w := range strings.Fields(phrase) {
		i := strings.Index(w, "*")
		if i < 0 {
			continue
		}
		if i != len(w)-1 || len([]rune(strings.TrimSuffix(w, "*"))) < MinWildcard {
			return false
		}
		if t := tokenize(w, false); len(t) != 1 || len([]rune(t[0])) < MinWildcard {
			return false
		}
	}
	return true
}

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

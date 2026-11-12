package segments

import (
	"sort"
	"strings"
)

// Keyword é uma palavra-chave carregada para classificação.
type Keyword struct {
	SegmentID  string
	SegmentKey string
	Phrase     string // como o administrador cadastrou
	Normalized string // tokens normalizados, separados por espaço
	Priority   int    // maior vence
}

// Reason explica o resultado da classificação.
type Reason string

const (
	ReasonKeyword  Reason = "keyword"  // exatamente um segmento venceu
	ReasonConflict Reason = "conflict" // empate entre segmentos → fila geral
	ReasonNone     Reason = "none"     // nenhuma palavra-chave → fila geral
)

// Match é uma palavra-chave que apareceu no texto.
type Match struct {
	SegmentID  string `json:"segmentId"`
	SegmentKey string `json:"segmentKey"`
	Phrase     string `json:"phrase"`
	Priority   int    `json:"priority"`
}

// Result: SegmentID vazio significa fila geral.
type Result struct {
	SegmentID  string  `json:"segmentId,omitempty"`
	SegmentKey string  `json:"segmentKey,omitempty"`
	Reason     Reason  `json:"reason"`
	Matches    []Match `json:"matches"`
}

// Classify é o motor determinístico de classificação (sem IA):
//
//   - uma palavra-chave combina quando todas as suas palavras aparecem juntas e em
//     ordem no texto (depois da normalização); uma palavra terminada em "*" é um prefixo
//     ("emit*" combina com emitir, emita, emitiu);
//   - a pontuação de cada segmento é a maior prioridade entre as suas palavras que combinaram;
//   - se um único segmento tem a maior pontuação, ele vence; se mais de um empata,
//     é conflito e a conversa vai para a fila geral; sem nenhuma combinação, também.
func Classify(text string, keywords []Keyword) Result {
	tokens := Tokens(text)
	res := Result{Reason: ReasonNone, Matches: []Match{}}
	if len(tokens) == 0 {
		return res
	}

	best := map[string]int{} // segmento → maior prioridade
	keys := map[string]string{}
	for _, k := range keywords {
		kt := splitFields(k.Normalized)
		if len(kt) == 0 || !containsSeq(tokens, kt) {
			continue
		}
		res.Matches = append(res.Matches, Match{SegmentID: k.SegmentID, SegmentKey: k.SegmentKey, Phrase: k.Phrase, Priority: k.Priority})
		if p, ok := best[k.SegmentID]; !ok || k.Priority > p {
			best[k.SegmentID] = k.Priority
		}
		keys[k.SegmentID] = k.SegmentKey
	}
	if len(best) == 0 {
		return res
	}
	sort.SliceStable(res.Matches, func(i, j int) bool { return res.Matches[i].Priority > res.Matches[j].Priority })

	top, winners := -1, []string{}
	for id, p := range best {
		switch {
		case p > top:
			top, winners = p, []string{id}
		case p == top:
			winners = append(winners, id)
		}
	}
	if len(winners) == 1 {
		res.SegmentID, res.SegmentKey, res.Reason = winners[0], keys[winners[0]], ReasonKeyword
		return res
	}
	res.Reason = ReasonConflict
	return res
}

func splitFields(s string) []string {
	var out []string
	start := -1
	for i, r := range s {
		if r == ' ' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

func containsSeq(haystack, needle []string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		ok := true
		for j, w := range needle {
			if !tokenMatches(haystack[i+j], w) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// tokenMatches: igualdade de palavra inteira, ou prefixo quando a palavra-chave termina em "*".
func tokenMatches(have, want string) bool {
	if n := len(want); n > 1 && want[n-1] == '*' {
		return strings.HasPrefix(have, want[:n-1])
	}
	return have == want
}

package segments

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// Sugestões de palavras-chave: vocabulário comum de escritório de contabilidade, oferecido em cada segmento cujo nome indica o
// assunto (folha, notas/fiscal, impostos, certificado, clientes novos). A equipe aceita com um clique o que fizer sentido.
//
// Prioridades: palavra solta e genérica (nota, folha, imposto) vale 40; expressão específica ou termo técnico (nota fiscal,
// holerite, fgts) vale 60. Assim, quando duas equipes combinam na mesma mensagem, o termo técnico decide em vez de empatar.
// As palavras cadastradas à mão pela equipe entram com 50.

// Suggestion é uma palavra-chave sugerida.
type Suggestion struct {
	Phrase   string `json:"phrase"`
	Priority int    `json:"priority"`
	Pack     string `json:"pack"` // assunto de origem, para a tela agrupar
}

type pack struct {
	label string
	// match: palavras do nome/chave do segmento que ativam o pacote (aceita curinga: "trabalhist*").
	match   []string
	generic []string // prioridade 40
	precise []string // prioridade 60
}

var packs = []pack{
	{
		label:   "Emissão de notas",
		match:   []string{"nota", "fiscal", "nf", "nfe", "nfse", "emiss*"},
		generic: []string{"nota", "fiscal", "xml"},
		precise: []string{
			"nota fiscal", "nf", "nfe", "nfse", "nfs", "danfe", "emit* nota", "emiss* nota", "cancel* nota", "gerar nota",
			"segunda via de nota", "nota de serviço", "nota de devolução", "carta de correção", "cupom fiscal", "portal da nota",
		},
	},
	{
		label:   "Folha de pagamento",
		match:   []string{"folha", "dp", "pessoal", "rh", "trabalhist*", "salario", "funcionario"},
		generic: []string{"folha", "salário", "funcionário", "inss"},
		precise: []string{
			"folha de pagamento", "holerite", "contracheque", "contra cheque", "férias", "13º salário", "décimo terceiro",
			"rescisão", "admissão", "admissional", "demissão", "demissional", "fgts", "esocial", "vale transporte",
			"vale refeição", "vale alimentação", "pró-labore", "atestado", "afastamento", "licença maternidade",
			"horas extras", "aviso prévio", "banco de horas",
		},
	},
	{
		label:   "Impostos e guias",
		match:   []string{"imposto", "tribut*", "das", "guia", "pgdas", "simples", "darf", "fiscal", "contabil*"},
		generic: []string{"imposto", "guia", "declaração", "regularização", "mei"},
		precise: []string{
			"pgdas", "simples nacional", "darf", "dctf", "dasn", "parcelamento", "irpf", "irpj", "csll", "cofins", "icms", "iss",
			"ecf", "malha fina", "certidão negativa", "cnd", "restituição",
		},
	},
	{
		label:   "Certificado digital",
		match:   []string{"certificado", "digital"},
		generic: []string{"certificado", "token"},
		precise: []string{"certificado digital", "renov* certificado", "a1", "a3", "ecpf", "ecnpj"},
	},
	{
		label:   "Clientes novos",
		match:   []string{"novo", "cliente", "comercial", "venda", "orcamento", "abertura", "legalizacao"},
		generic: []string{"honorários"},
		precise: []string{
			"orçamento", "orçar", "proposta", "contratar", "quanto custa", "novo cliente", "ser cliente", "abrir empresa",
			"abrir cnpj", "abrir mei", "abertura de empresa", "trocar de contador", "mudar de contador", "migrar contabilidade",
		},
	},
}

// packsFor escolhe os pacotes pelo nome e pela chave do segmento ("Fiscal" → notas e impostos; "DP" → folha).
func packsFor(name, key string) []pack {
	have := Tokens(name + " " + strings.ReplaceAll(key, "_", " "))
	var out []pack
	for _, p := range packs {
		hit := false
	scan:
		for _, m := range p.match {
			mk := NormalizeKeyword(m)
			for _, h := range have {
				if tokenMatches(h, mk) {
					hit = true
					break scan
				}
			}
		}
		if hit {
			out = append(out, p)
		}
	}
	return out
}

// Suggestions devolve as palavras sugeridas para o segmento que ainda não estão cadastradas nele.
func (s *Service) Suggestions(ctx context.Context, segmentID string) ([]Suggestion, error) {
	if !httpx.ValidUUID(segmentID) {
		return nil, ErrNotFound
	}
	var name, key string
	if err := s.db.QueryRow(ctx, `SELECT name, key FROM segments WHERE id = $1`, segmentID).Scan(&name, &key); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT normalized FROM segment_keywords WHERE segment_id = $1`, segmentID)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			have[n] = true
		}
	}
	rows.Close()
	return suggestFor(name, key, have), nil
}

func suggestFor(name, key string, have map[string]bool) []Suggestion {
	out := []Suggestion{}
	seen := map[string]bool{}
	for _, p := range packsFor(name, key) {
		add := func(list []string, prio int) {
			for _, ph := range list {
				n := NormalizeKeyword(ph)
				if n == "" || have[n] || seen[n] {
					continue
				}
				seen[n] = true
				out = append(out, Suggestion{Phrase: ph, Priority: prio, Pack: p.label})
			}
		}
		add(p.precise, 60)
		add(p.generic, 40)
	}
	return out
}

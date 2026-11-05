package segments

// DefaultSegment descreve um segmento inicial do blueprint e suas palavras-chave.
type DefaultSegment struct {
	Key      string
	Name     string
	Keywords []string
}

// Defaults são os segmentos iniciais (regra 1 do blueprint). Os administradores podem
// editar tudo na tela Filas / Segmentos; estas listas são apenas o ponto de partida.
var Defaults = []DefaultSegment{
	{
		Key: "clientes_novos", Name: "Clientes novos",
		Keywords: []string{
			"orçamento", "orçar", "proposta", "contratar", "contratação", "novo cliente", "ser cliente",
			"abrir empresa", "abrir cnpj", "abrir mei", "abertura de empresa", "quanto custa",
			"honorários", "trocar de contador", "mudar de contador", "migrar contabilidade",
		},
	},
	{
		Key: "folha", Name: "Folha de pagamento",
		Keywords: []string{
			"folha", "folha de pagamento", "férias", "13º salário", "décimo terceiro", "rescisão",
			"admissão", "demissão", "holerite", "contracheque", "contra cheque", "esocial", "fgts",
			"vale transporte",
		},
	},
	{
		Key: "notas", Name: "Emissão de notas",
		Keywords: []string{
			"emitir nota", "emissão de nota", "nota fiscal", "nfe", "nfse", "cancelar nota",
			"segunda via de nota", "gerar nota",
		},
	},
}

// DefaultTags são as etiquetas iniciais do catálogo.
var DefaultTags = []string{"PENDENTE", "URGENTE", "PGDAS", "CERTIFICADO", "ACOMPANHAMENTO"}

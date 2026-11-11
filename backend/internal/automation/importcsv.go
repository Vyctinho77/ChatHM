package automation

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// ParseChargesCSV lê a planilha de cobranças (CSV com cabeçalho, separado por vírgula ou ponto e vírgula).
// Colunas (sem distinção de maiúsculas e acentos): codigo, telefone, cliente, descricao, valor, vencimento, status, link.
// Linhas com problema vêm em errs e não impedem as demais.
func ParseChargesCSV(r io.Reader) (rows []ImportRow, errs []string, err error) {
	raw, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return nil, nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	first, _, _ := bytes.Cut(raw, []byte("\n"))
	cr := csv.NewReader(bytes.NewReader(raw))
	cr.Comma = ','
	if bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	records, err := cr.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("planilha ilegível: %w", err)
	}
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("a planilha precisa de um cabeçalho e ao menos uma linha")
	}
	col := map[string]int{}
	for i, h := range records[0] {
		if k := csvColumn(h); k != "" {
			if _, dup := col[k]; !dup {
				col[k] = i
			}
		}
	}
	for _, need := range []string{"id", "valor", "vencimento"} {
		if _, ok := col[need]; !ok {
			return nil, nil, fmt.Errorf("falta a coluna %q no cabeçalho", map[string]string{"id": "codigo", "valor": "valor", "vencimento": "vencimento"}[need])
		}
	}
	get := func(rec []string, key string) string {
		if i, ok := col[key]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	for n, rec := range records[1:] {
		line := n + 2
		if len(strings.Join(rec, "")) == 0 {
			continue
		}
		row := ImportRow{Line: line, ExternalID: get(rec, "id"), Phone: get(rec, "telefone"), Name: get(rec, "cliente"), Description: get(rec, "descricao"), URL: get(rec, "link"), ContactID: get(rec, "contato")}
		var ok bool
		if row.AmountCents, ok = ParseMoney(get(rec, "valor")); !ok {
			errs = append(errs, fmt.Sprintf("linha %d: valor inválido", line))
			continue
		}
		if row.DueDate, ok = ParseDate(get(rec, "vencimento")); !ok {
			errs = append(errs, fmt.Sprintf("linha %d: vencimento inválido (use DD/MM/AAAA)", line))
			continue
		}
		if row.Status, ok = ParseStatus(get(rec, "status")); !ok {
			errs = append(errs, fmt.Sprintf("linha %d: situação desconhecida", line))
			continue
		}
		rows = append(rows, row)
	}
	return rows, errs, nil
}

// csvColumn normaliza o título de uma coluna para a chave interna.
func csvColumn(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case strings.ContainsRune("áàâãä", r):
			b.WriteRune('a')
		case strings.ContainsRune("éèêë", r):
			b.WriteRune('e')
		case strings.ContainsRune("íìîï", r):
			b.WriteRune('i')
		case strings.ContainsRune("óòôõö", r):
			b.WriteRune('o')
		case strings.ContainsRune("úùûü", r):
			b.WriteRune('u')
		case r == 'ç':
			b.WriteRune('c')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	switch b.String() {
	case "id", "codigo", "externalid", "cobranca":
		return "id"
	case "telefone", "celular", "whatsapp", "phone":
		return "telefone"
	case "cliente", "nome", "name":
		return "cliente"
	case "descricao", "description":
		return "descricao"
	case "valor", "amount":
		return "valor"
	case "vencimento", "datavencimento", "due", "duedate":
		return "vencimento"
	case "status", "situacao":
		return "status"
	case "link", "url", "linkpagamento", "linkdepagamento":
		return "link"
	case "contato", "contactid":
		return "contato"
	}
	return ""
}

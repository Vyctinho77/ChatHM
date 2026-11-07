package aiaccess

import (
	"regexp"
	"strings"
)

var (
	// CPF/CNPJ formatados (ou CNPJ só com dígitos). CPF só com dígitos é ambíguo com telefone e só é ocultado se não parecer celular.
	reCNPJ     = regexp.MustCompile(`\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b`)
	reCPFFmt   = regexp.MustCompile(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b`)
	reDigits11 = regexp.MustCompile(`\b\d{11}\b`)
	reMobile   = regexp.MustCompile(`^[1-9][1-9]9\d{8}$`)
	rePhone    = regexp.MustCompile(`(\+?\(?\d[\d\s()-]{7,}\d)`)
)

// Masker oculta dados pessoais nas respostas conforme a configuração do escritório.
type Masker struct{ Documents, Phones bool }

func (m Masker) Text(s string) string {
	if s == "" {
		return s
	}
	if m.Documents {
		s = reCNPJ.ReplaceAllStringFunc(s, func(x string) string {
			if len(onlyDigits(x)) != 14 {
				return x
			}
			return "[CNPJ oculto]"
		})
		s = reCPFFmt.ReplaceAllString(s, "[CPF oculto]")
		s = reDigits11.ReplaceAllStringFunc(s, func(x string) string {
			if reMobile.MatchString(x) {
				return x // parece celular
			}
			return "[CPF oculto]"
		})
	}
	if m.Phones {
		s = rePhone.ReplaceAllStringFunc(s, maskPhone)
	}
	return s
}

func maskPhone(x string) string {
	d := onlyDigits(x)
	if len(d) < 8 || len(d) > 13 || strings.Contains(x, "[") {
		return x
	}
	return "…" + d[len(d)-4:]
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Walk aplica o mascaramento em todas as strings de uma estrutura JSON genérica.
func (m Masker) Walk(v any) any {
	switch x := v.(type) {
	case string:
		return m.Text(x)
	case []any:
		for i := range x {
			x[i] = m.Walk(x[i])
		}
		return x
	case map[string]any:
		for k, val := range x {
			x[k] = m.Walk(val)
		}
		return x
	}
	return v
}

package conversations

import (
	"regexp"
	"strings"

	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// NormalizeTaxID aceita CPF ou CNPJ com ou sem pontuação e devolve só os dígitos, conferindo os dígitos verificadores
// (uma tecla trocada não passa). Vazio apaga o campo.
func NormalizeTaxID(s string) (string, error) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '/' || r == ' ':
		default:
			return "", httpx.FieldErrors{"taxId": "Use só números (ou a pontuação normal de CPF/CNPJ)."}
		}
	}
	d := b.String()
	switch len(d) {
	case 0:
		return "", nil
	case 11:
		if validCPF(d) {
			return d, nil
		}
		return "", httpx.FieldErrors{"taxId": "CPF inválido: confira os números."}
	case 14:
		if validCNPJ(d) {
			return d, nil
		}
		return "", httpx.FieldErrors{"taxId": "CNPJ inválido: confira os números."}
	}
	return "", httpx.FieldErrors{"taxId": "Informe um CPF (11 números) ou um CNPJ (14 números)."}
}

// FormatTaxID escreve o CPF (000.000.000-00) ou o CNPJ (00.000.000/0000-00) com a pontuação normal; outro valor volta como veio.
func FormatTaxID(d string) string {
	switch len(d) {
	case 11:
		return d[0:3] + "." + d[3:6] + "." + d[6:9] + "-" + d[9:]
	case 14:
		return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:]
	}
	return d
}

var asaasID = regexp.MustCompile(`^cus_[A-Za-z0-9]{6,40}$`)

// NormalizeAsaasID confere o id do cliente no Asaas (cus_...). Vazio apaga o campo.
func NormalizeAsaasID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !asaasID.MatchString(s) {
		return "", httpx.FieldErrors{"asaasId": "O id do cliente no Asaas começa com cus_ (ex.: cus_000005401844)."}
	}
	return s, nil
}

func allSame(d string) bool {
	for i := 1; i < len(d); i++ {
		if d[i] != d[0] {
			return false
		}
	}
	return true
}

func validCPF(d string) bool {
	if allSame(d) {
		return false
	}
	for n := 9; n <= 10; n++ {
		sum := 0
		for i := 0; i < n; i++ {
			sum += int(d[i]-'0') * (n + 1 - i)
		}
		dv := (sum * 10) % 11
		if dv == 10 {
			dv = 0
		}
		if dv != int(d[n]-'0') {
			return false
		}
	}
	return true
}

func validCNPJ(d string) bool {
	if allSame(d) {
		return false
	}
	w1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	w2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	check := func(w []int, n int) bool {
		sum := 0
		for i := 0; i < n; i++ {
			sum += int(d[i]-'0') * w[i]
		}
		dv := sum % 11
		if dv < 2 {
			dv = 0
		} else {
			dv = 11 - dv
		}
		return dv == int(d[n]-'0')
	}
	return check(w1, 12) && check(w2, 13)
}

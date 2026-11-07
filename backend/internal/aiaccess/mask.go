package aiaccess

import (
	"regexp"
	"strings"
	"unicode"
)

// Privacidade das respostas do MCP. Três camadas:
//
//  1. Sempre ligadas, para qualquer cliente de IA e sem opção de desligar: credenciais (senhas, códigos, tokens, chaves),
//     dados financeiros (cartão, agência/conta, chave PIX, boleto). Um assistente nunca precisa disso para ajudar no atendimento.
//  2. Pelo perfil do cliente: documentos (CPF, CNPJ, RG, CNH...), e-mail, telefone, CEP. Clientes OAuth (ChatGPT, claude.ai)
//     recebem SEMPRE tudo ocultado; tokens pessoais seguem as opções do administrador.
//  3. Modo estrito (administrador): nomes reduzidos a "Nome S." e observações internas removidas.
//
// O que um assistente precisa para funcionar continua aparecendo: ids, nomes, o texto das mensagens (já filtrado),
// horários, filas, etiquetas e o andamento do atendimento.

var (
	reCNPJ     = regexp.MustCompile(`\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b`)
	reCPFFmt   = regexp.MustCompile(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b`)
	reDigits11 = regexp.MustCompile(`\b\d{11}\b`)
	reMobile   = regexp.MustCompile(`^[1-9][1-9]9\d{8}$`)
	rePhone    = regexp.MustCompile(`(\+?\(?\d[\d\s()-]{7,}\d)`)
	reEmail    = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reCEP      = regexp.MustCompile(`\b\d{5}-\d{3}\b`)

	// Credenciais: palavra-chave + valor.
	reSecretWord = regexp.MustCompile(`(?i)\b(senha|password|pwd|passcode|token|segredo|c[oó]digo(?: de (?:verifica[cç][aã]o|acesso|seguran[cç]a|confirma[cç][aã]o))?|pin|cvv|cvc|otp|api[ _-]?key|chave de acesso)\b((?:\s+(?:d[oae]s?|para|no|na|em)\s+[\w.\-]+){0,3}\s*(?:[:=\-]|(?:é|foi|e)\s)?\s*)([^\s,;]{3,})`)
	reBearer     = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=\-]{12,}`)
	reLongSecret = regexp.MustCompile(`\b(?:eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{5,}|chm_[A-Za-z0-9_\-]{10,}|sk-[A-Za-z0-9_\-]{16,}|[A-Fa-f0-9]{32,}|[A-Za-z0-9+/_\-]{40,}={0,2})\b`)

	// Financeiro.
	reBoleto = regexp.MustCompile(`\b(?:\d[\s.\-]?){44,48}\b`)
	reCard   = regexp.MustCompile(`\b\d(?:[ \-]?\d){12,18}\b`)
	reBank   = regexp.MustCompile(`(?i)\b(ag[eê]ncia|ag\.|conta(?: corrente| poupan[cç]a)?|c/c|c\.c\.)(\s*(?:n[ºo°.]|:|-)?\s*)(\d[\d.\-\s]{2,14}\d)`)
	rePix    = regexp.MustCompile(`(?i)\b(chave\s+pix|pix)(\s*[:=\-]?\s*)(\+?\d[\d.\-/\s]{7,}\d|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[^\s@]+@\S+)`)

	// Documentos com palavra-chave (não têm formato único).
	reKeyDoc = regexp.MustCompile(`(?i)\b(rg|cnh|pis|pasep|nis|nit|t[ií]tulo(?: de eleitor)?|renavam|passaporte|identidade)\b(\s*(?:n[ºo°.]|:|-)?\s*)([A-Z0-9][A-Z0-9.\-/]{3,20}(?:\s\d{3,5}){0,2})`)
	reBirth  = regexp.MustCompile(`(?i)\b(nascimento|nasc\.?|data de nascimento|nascido em|nascida em)(\s*[:\-]?\s*)\d{1,2}[/.\-]\d{1,2}[/.\-]\d{2,4}`)
)

// Masker oculta dados pessoais. As camadas 1 (credenciais e financeiro) valem sempre.
type Masker struct {
	Documents bool // CPF, CNPJ, RG, CNH, PIS, título, Renavam, passaporte, data de nascimento
	Phones    bool
	Emails    bool
	Addresses bool // CEP
	Strict    bool // nomes reduzidos; observações internas removidas
}

// Text aplica as regras a um texto livre.
func (m Masker) Text(s string) string {
	if s == "" {
		return s
	}
	// 1) Sempre: credenciais.
	s = reBearer.ReplaceAllString(s, "$1 [credencial oculta]")
	s = reSecretWord.ReplaceAllString(s, "$1$2[oculto]")
	s = reLongSecret.ReplaceAllString(s, "[credencial oculta]")
	// 1) Sempre: financeiro. Boleto antes do cartão e dos documentos (são sequências longas de dígitos).
	s = reBoleto.ReplaceAllStringFunc(s, func(x string) string {
		if n := len(onlyDigits(x)); n >= 44 && n <= 48 {
			return "[boleto oculto]"
		}
		return x
	})
	s = reCard.ReplaceAllStringFunc(s, func(x string) string {
		if d := onlyDigits(x); len(d) >= 13 && len(d) <= 19 && luhn(d) {
			return "[cartão oculto]"
		}
		return x
	})
	s = reBank.ReplaceAllString(s, "$1$2[oculto]")
	s = rePix.ReplaceAllString(s, "$1$2[chave oculta]")

	// 2) Por perfil.
	if m.Documents {
		s = reKeyDoc.ReplaceAllString(s, "$1$2[oculto]")
		s = reBirth.ReplaceAllString(s, "$1$2[data oculta]")
		s = reCNPJ.ReplaceAllStringFunc(s, func(x string) string {
			if len(onlyDigits(x)) != 14 {
				return x
			}
			return "[CNPJ oculto]"
		})
		s = reCPFFmt.ReplaceAllString(s, "[CPF oculto]")
		s = reDigits11.ReplaceAllStringFunc(s, func(x string) string {
			if reMobile.MatchString(x) {
				return x // parece celular: tratado em Phones
			}
			return "[CPF oculto]"
		})
	}
	if m.Emails {
		s = reEmail.ReplaceAllString(s, "[e-mail oculto]")
	}
	if m.Addresses {
		s = reCEP.ReplaceAllString(s, "[CEP oculto]")
	}
	if m.Phones {
		s = rePhone.ReplaceAllStringFunc(s, maskPhone)
	}
	return s
}

func luhn(d string) bool {
	sum, alt := 0, false
	for i := len(d) - 1; i >= 0; i-- {
		n := int(d[i] - '0')
		if alt {
			if n *= 2; n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
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

// shortName: "Marlon Contador Silva" -> "Marlon S."; nomes de uma palavra ficam como estão.
func shortName(s string) string {
	f := strings.Fields(s)
	if len(f) < 2 {
		return s
	}
	last := []rune(f[len(f)-1])
	if len(last) == 0 || !unicode.IsLetter(last[0]) {
		return s
	}
	return f[0] + " " + string(unicode.ToUpper(last[0])) + "."
}

// Campos que carregam identificadores/horários/enums: nunca passam pelo filtro de texto (um id ou data não é dado pessoal
// e o filtro de telefone poderia estragá-los).
func structural(key string) bool {
	switch key {
	case "id", "date", "time", "until", "kind", "direction", "status", "key", "tag", "tags", "queue", "scopes", "aviso":
		return true
	}
	return strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "ID") || strings.HasSuffix(key, "At") || strings.HasSuffix(key, "Minutes")
}

// Campos removidos por completo: observações internas no modo estrito; telefone e observação para clientes OAuth.
func (m Masker) drop(key string, oauth bool) bool {
	switch key {
	case "note":
		return m.Strict || oauth
	case "phone":
		return oauth
	}
	return false
}

// Campos que guardam nome de pessoa (reduzidos no modo estrito).
func isPersonName(key string) bool {
	switch key {
	case "contact", "authorName", "author", "owner", "assignee":
		return true
	}
	return false
}

// Walk aplica o filtro a todas as strings de uma estrutura JSON genérica. oauth=true remove também telefone e observação.
func (m Masker) Walk(v any, oauth bool) any { return m.walk(v, "", oauth) }

func (m Masker) walk(v any, key string, oauth bool) any {
	switch x := v.(type) {
	case string:
		if structural(key) {
			return x
		}
		out := m.Text(x)
		if m.Strict && isPersonName(key) {
			out = shortName(out)
		}
		return out
	case []any:
		for i := range x {
			x[i] = m.walk(x[i], key, oauth)
		}
		return x
	case map[string]any:
		for k, val := range x {
			if m.drop(k, oauth) {
				delete(x, k)
				continue
			}
			if m.Strict && k == "name" && key == "contact" {
				if s, ok := val.(string); ok {
					x[k] = shortName(m.Text(s))
					continue
				}
			}
			x[k] = m.walk(val, k, oauth)
		}
		return x
	}
	return v
}

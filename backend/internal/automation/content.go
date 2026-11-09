package automation

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Refusal: o servidor recusou o envio. Code é estável (a IA e os testes dependem dele); Message explica em português.
type Refusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// ExistingID: quando é repetição, o envio que já existe.
	ExistingID string `json:"existingId,omitempty"`
}

func (r *Refusal) Error() string { return r.Message }

func refuse(code, format string, args ...any) *Refusal {
	return &Refusal{Code: code, Message: fmt.Sprintf(format, args...)}
}

const (
	maxTextRunes   = 900 // sobra espaço para o rodapé de opt-out dentro do limite de 1200 do banco
	optOutFooter   = "_Mensagem automática. Para não receber estes avisos, responda PARAR._"
	defaultSigText = "Assistente do escritório"
)

var (
	reURL     = regexp.MustCompile(`(?i)(?:\b(?:https?://|www\.)[^\s]+|\b[a-z0-9][a-z0-9-]*(?:\.[a-z0-9-]+)*\.(?:com|net|org|br|gov|io|app|dev|co|me|link|xyz|info|ly)\b[^\s]*)`)
	reEmail   = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	reDigits  = regexp.MustCompile(`\d(?:[\s.\-()]?\d){8,}`)
	reDoc     = regexp.MustCompile(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b|\b\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2}\b`)
	reUUID    = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	reUnder   = regexp.MustCompile(`(^|\s)_[^_\s][^_]*_($|\s|[.,;:!?])`)
	rePlace   = regexp.MustCompile(`\{([^{}]*)\}`)
	fieldKey  = regexp.MustCompile(`^[a-z][a-z_]{1,29}$`)
	dedupeKey = regexp.MustCompile(`^[A-Za-z0-9_.:/ \-]{1,80}$`)
)

// Termos de pressão: cobrança não pode expor nem constranger o cliente (CDC art. 42). Marcam o envio com um aviso para quem aprova.
var pressureTerms = []string{
	"protesto", "protestar", "negativ", "serasa", "spc", "cartório", "cartorio", "judicial", "justiça", "justica",
	"processo", "advogado", "ação de cobrança", "acao de cobranca", "restrição no nome", "restricao no nome",
	"bloquear", "bloqueio do cnpj", "última chance", "ultima chance", "ultimato", "vamos tomar", "medidas legais",
}

// ValidateText confere o texto que a IA escreveu ANTES de qualquer substituição de variáveis. A IA não tem como digitar
// dados de pagamento, contatos ou links soltos: tudo isso só entra por campos fixos do escritório ({pix_escritorio}) ou em PDF.
// Devolve avisos (não bloqueiam; quem aprova vê) ou a recusa.
func ValidateText(text string, allowedDomains []string, fields map[string]string) (flags []string, ref *Refusal) {
	t := strings.TrimSpace(text)
	n := utf8.RuneCountInString(t)
	if n == 0 {
		return nil, refuse("content", "O texto está vazio.")
	}
	if n > maxTextRunes {
		return nil, refuse("content", "O texto tem %d caracteres; o máximo é %d.", n, maxTextRunes)
	}
	if strings.Count(t, "\n") > 8 {
		return nil, refuse("content", "O texto tem linhas demais (máximo 9).")
	}
	for _, r := range t {
		if r == '\n' {
			continue
		}
		if unicode.IsControl(r) || r == 0x200b || r == 0x202e || r == 0xfeff { // invisíveis e inversão de leitura
			return nil, refuse("content", "O texto tem caracteres de controle ou invisíveis.")
		}
	}
	if strings.ContainsAny(t, "*`~") || reUnder.MatchString(t) {
		return nil, refuse("content", "Não use formatação do WhatsApp (* _ ~ `) no texto; a assinatura e o rodapé já são aplicados pelo sistema.")
	}
	for _, m := range rePlace.FindAllStringSubmatch(t, -1) {
		name := m[1]
		if name == "nome" {
			continue
		}
		if _, ok := fields[name]; !ok {
			return nil, refuse("content", "A variável {%s} não existe. Use {nome} ou um campo fixo do escritório (%s).", name, fieldList(fields))
		}
	}
	// As checagens abaixo ignoram o que está dentro de {chaves} (esses valores vêm do escritório, não da IA).
	plain := rePlace.ReplaceAllString(t, " ")
	if reEmail.MatchString(plain) {
		return nil, refuse("content", "Não coloque e-mails no texto.")
	}
	for _, link := range reURL.FindAllString(plain, -1) {
		if !domainAllowed(link, allowedDomains) {
			return nil, refuse("content", "Links não são permitidos no texto (%s). O administrador pode liberar domínios em Envios automáticos; guias e boletos vão em PDF.", shorten(link))
		}
	}
	if reDoc.MatchString(plain) {
		return nil, refuse("content", "Não coloque CPF ou CNPJ no texto.")
	}
	if reUUID.MatchString(plain) || reDigits.MatchString(plain) {
		return nil, refuse("content", "Não coloque números longos (telefone, documento, conta ou chave PIX) no texto. Dados de pagamento vão pelos campos fixos do escritório ou em PDF.")
	}
	low := strings.ToLower(plain)
	for _, term := range pressureTerms {
		if strings.Contains(low, term) {
			flags = append(flags, "pressao:"+term)
		}
	}
	return dedupe(flags), nil
}

func domainAllowed(link string, allowed []string) bool {
	raw := link
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range allowed {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" && (host == d || strings.HasSuffix(host, "."+d)) {
			return true
		}
	}
	return false
}

func shorten(s string) string {
	if utf8.RuneCountInString(s) > 40 {
		return string([]rune(s)[:40]) + "…"
	}
	return s
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func fieldList(fields map[string]string) string {
	if len(fields) == 0 {
		return "nenhum campo cadastrado"
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, "{"+k+"}")
	}
	return strings.Join(keys, ", ")
}

// firstName: o primeiro nome do contato para {nome}; sem nome cadastrado, "cliente".
func firstName(name string) string {
	f := strings.Fields(name)
	if len(f) == 0 || strings.HasPrefix(f[0], "+") || strings.ContainsAny(f[0], "0123456789") {
		return "cliente"
	}
	return f[0]
}

// Render monta o texto final: variáveis substituídas e rodapé de opt-out. O corpo armazenado e enviado é este.
func Render(text, contactName string, fields map[string]string) string {
	out := rePlace.ReplaceAllStringFunc(strings.TrimSpace(text), func(m string) string {
		name := strings.Trim(m, "{}")
		if name == "nome" {
			return firstName(contactName)
		}
		return fields[name]
	})
	return out + "\n\n" + optOutFooter
}

// Author: a assinatura em negrito para o cliente. Nunca o nome de uma pessoa.
func Author(signature string) string {
	s := strings.Join(strings.Fields(strings.ReplaceAll(signature, "*", "")), " ")
	if s == "" {
		return defaultSigText
	}
	return s
}

// CleanFields valida os campos fixos do escritório ({pix_escritorio} etc.).
func CleanFields(in map[string]string) (map[string]string, error) {
	if len(in) > 12 {
		return nil, fmt.Errorf("no máximo 12 campos fixos")
	}
	out := map[string]string{}
	for k, v := range in {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if !fieldKey.MatchString(k) || k == "nome" {
			return nil, fmt.Errorf("nome de campo inválido %q: use letras minúsculas e _ (ex.: pix_escritorio)", k)
		}
		if v == "" || utf8.RuneCountInString(v) > 200 || strings.ContainsAny(v, "\n\r*`~") {
			return nil, fmt.Errorf("o valor de %q deve ter de 1 a 200 caracteres, em uma linha e sem * ` ~", k)
		}
		out[k] = v
	}
	return out, nil
}

// CleanDomains valida os domínios permitidos em links.
func CleanDomains(in []string) ([]string, error) {
	if len(in) > 20 {
		return nil, fmt.Errorf("no máximo 20 domínios")
	}
	re := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	out := []string{}
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
		d = strings.TrimSuffix(d, "/")
		if d == "" {
			continue
		}
		if !re.MatchString(d) || len(d) > 80 {
			return nil, fmt.Errorf("domínio inválido %q (exemplo: gov.br)", d)
		}
		out = append(out, d)
	}
	return dedupe(out), nil
}

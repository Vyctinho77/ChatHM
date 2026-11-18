package automation

import (
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Source: uma página que a IA consultou para responder. O ChatHM não navega na internet: quem pesquisa é a IA externa (Grok, Claude,
// ChatGPT, Gemini...). O servidor garante que as fontes citadas são links https válidos e as mostra ao cliente no fim da mensagem.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

const maxSources = 5

// normalizeSources valida as fontes: até 5, https, domínio público (nada de IP, localhost ou endereço interno), sem repetição.
func normalizeSources(in []Source) ([]Source, *Refusal) {
	if len(in) > maxSources {
		return nil, refuse("fontes", "Cite no máximo %d fontes.", maxSources)
	}
	out := make([]Source, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		raw := strings.TrimSpace(s.URL)
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
			return nil, refuse("fontes", "Cada fonte precisa de um link https válido (recebi %q).", shorten(raw))
		}
		host := strings.ToLower(u.Hostname())
		if net.ParseIP(host) != nil || !strings.Contains(host, ".") || host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
			return nil, refuse("fontes", "A fonte %q não é um site público.", shorten(host))
		}
		u.Fragment = ""
		clean := u.String()
		if seen[clean] {
			continue
		}
		seen[clean] = true
		title := strings.Join(strings.Fields(s.Title), " ")
		if title == "" {
			title = strings.TrimPrefix(host, "www.")
		}
		if utf8.RuneCountInString(title) > 100 {
			title = string([]rune(title)[:100]) + "…"
		}
		out = append(out, Source{Title: title, URL: clean})
	}
	return out, nil
}

// withSources acrescenta ao texto a lista de fontes consultadas.
func withSources(text string, srcs []Source) string {
	if len(srcs) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\nFontes consultadas:")
	for _, s := range srcs {
		b.WriteString("\n• " + s.Title + ": " + s.URL)
	}
	return b.String()
}

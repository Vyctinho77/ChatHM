package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// webHandler serve o frontend compilado (SPA) na mesma origem da API: arquivos estáticos com cache
// longo para os hashados, index.html como fallback das rotas do React e uma CSP própria para as páginas.
// Os scripts inline do index.html entram na CSP por hash (sem 'unsafe-inline' em scripts).
func webHandler(dir string) (http.Handler, error) {
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return nil, err
	}
	script := "script-src 'self'"
	for _, m := range inlineScript.FindAllSubmatch(index, -1) {
		sum := sha256.Sum256(m[1])
		script += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}
	csp := strings.Join([]string{
		"default-src 'self'",
		script,
		"style-src 'self' 'unsafe-inline'", // estilos inline gerados pelas animações
		"img-src 'self' data: blob:",
		"media-src 'self' blob:",
		"font-src 'self' data:",
		"connect-src 'self' ws: wss:",
		"worker-src 'self' blob:",
		"frame-src 'self' blob:",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")

	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Security-Policy", csp)
		clean := path.Clean("/" + r.URL.Path)
		if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(clean))); err == nil && !st.IsDir() {
			if strings.HasPrefix(clean, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(w, r)
			return
		}
		// Rota do React: devolve o index.html (nunca para caminhos de API).
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	}), nil
}

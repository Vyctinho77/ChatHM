package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebHandlerServesSPAWithStrictCSP(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte(`<!doctype html><script>var t=1;</script><div id="root"></div>`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "assets", "app-abc.js"), []byte("console.log(1)"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "assets", "w.mjs"), []byte("export {}"), 0o644)
	h, err := webHandler(dir)
	if err != nil {
		t.Fatal(err)
	}
	get := func(p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		return w
	}
	// Rota do React → index.html, sem cache; CSP com hash do script inline e sem 'unsafe-inline' em script.
	w := get("/conversas/123")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `id="root"`) || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("fallback: %d %v", w.Code, w.Header())
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' 'sha256-") || strings.Contains(strings.Split(csp, "style-src")[0], "unsafe-inline") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("csp: %s", csp)
	}
	// Assets hashados: cache longo; .mjs com tipo JavaScript.
	w = get("/assets/app-abc.js")
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %v", w.Code, w.Header())
	}
	if ct := get("/assets/w.mjs").Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("mjs: %s", ct)
	}
	// Não escapa da pasta e não aceita escrita.
	if w = get("/../../etc/passwd"); strings.Contains(w.Body.String(), "root:") {
		t.Fatal("path traversal")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", w.Code)
	}
}

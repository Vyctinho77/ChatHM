package conversations

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
)

var pngHeader = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}

func TestMapTileProxy(t *testing.T) {
	var hits atomic.Int32
	var ua string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		ua = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngHeader)
	}))
	defer up.Close()
	old := tileBase
	tileBase = up.URL
	defer func() { tileBase = old }()

	r := chi.NewRouter()
	r.Get("/map-tiles/{z}/{x}/{y}", (&Handler{}).mapTile)
	get := func(p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		return w
	}
	if w := get("/map-tiles/16/21000/32000"); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("status %d", w.Code)
	}
	get("/map-tiles/16/21000/32000") // segunda vez vem do cache
	if hits.Load() != 1 {
		t.Fatalf("upstream chamado %d vezes, queria 1", hits.Load())
	}
	if ua == "" || ua == "Go-http-client/1.1" {
		t.Fatalf("User-Agent não identifica o ChatHM: %q", ua)
	}
	for _, bad := range []string{"/map-tiles/0/0/0", "/map-tiles/20/1/1", "/map-tiles/3/9/0", "/map-tiles/x/1/1", "/map-tiles/16/-1/1"} {
		if w := get(bad); w.Code != 404 {
			t.Fatalf("%s = %d, queria 404", bad, w.Code)
		}
	}
}

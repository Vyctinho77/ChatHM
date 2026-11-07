package conversations

import (
	"container/list"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Blocos do mapa (OpenStreetMap) servidos pelo próprio servidor: o OSM bloqueia (403) navegadores
// que não se identificam e pede cache. O servidor envia um User-Agent próprio e guarda os blocos.
const (
	tileUserAgent = "ChatHM/1.0 (+https://chathm.com.br; atendimento ao cliente)"
	tileMaxBytes  = 200 << 10
	tileCacheMax  = 400
)

type tileCache struct {
	mu    sync.Mutex
	ll    *list.List
	items map[string]*list.Element
}

type tileEntry struct {
	key  string
	data []byte
}

func newTileCache() *tileCache { return &tileCache{ll: list.New(), items: map[string]*list.Element{}} }

func (c *tileCache) get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		c.ll.MoveToFront(e)
		return e.Value.(*tileEntry).data, true
	}
	return nil, false
}

func (c *tileCache) put(k string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		e.Value.(*tileEntry).data = b
		c.ll.MoveToFront(e)
		return
	}
	c.items[k] = c.ll.PushFront(&tileEntry{k, b})
	for c.ll.Len() > tileCacheMax {
		old := c.ll.Back()
		c.ll.Remove(old)
		delete(c.items, old.Value.(*tileEntry).key)
	}
}

var (
	tiles      = newTileCache()
	tileClient = &http.Client{Timeout: 10 * time.Second}
	tileBase   = "https://tile.openstreetmap.org"
)

// mapTile: GET /map-tiles/{z}/{x}/{y} (exige sessão). Só zoom 1–19 e coordenadas válidas.
func (h *Handler) mapTile(w http.ResponseWriter, r *http.Request) {
	z, e1 := strconv.Atoi(chi.URLParam(r, "z"))
	x, e2 := strconv.Atoi(chi.URLParam(r, "x"))
	y, e3 := strconv.Atoi(chi.URLParam(r, "y"))
	if e1 != nil || e2 != nil || e3 != nil || z < 1 || z > 19 || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
		http.NotFound(w, r)
		return
	}
	key := fmt.Sprintf("%d/%d/%d", z, x, y)
	data, ok := tiles.get(key)
	if !ok {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, tileBase+"/"+key+".png", nil)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		req.Header.Set("User-Agent", tileUserAgent)
		resp, err := tileClient.Do(req)
		if err != nil {
			http.Error(w, "mapa indisponível", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			http.Error(w, "mapa indisponível", http.StatusBadGateway)
			return
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, tileMaxBytes+1))
		if err != nil || len(data) > tileMaxBytes || http.DetectContentType(data) != "image/png" {
			http.Error(w, "mapa indisponível", http.StatusBadGateway)
			return
		}
		tiles.put(key, data)
	}
	hd := w.Header()
	hd.Set("Content-Type", "image/png")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "private, max-age=604800")
	_, _ = w.Write(data)
}

// Package realtime distribui eventos aos navegadores conectados via WebSocket.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/vyctinho77/chathm/backend/internal/auth"
)

// Client é uma conexão WebSocket autenticada.
type Client struct {
	UserID string
	Admin  bool
	send   chan []byte
}

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Filter decide quem recebe um evento (nil = todos).
type Filter func(*Client) bool

func AdminsOnly(c *Client) bool { return c.Admin }

// Quem está online: a pessoa tem pelo menos uma conexão (aba do app aberta). Ao fechar a última, ela só some depois de uma carência
// (presenceGrace), para um F5 ou uma reconexão rápida não piscar "offline" para a equipe.
const presenceGrace = 20 * time.Second

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	conns   map[string]int         // pessoa -> conexões abertas
	online  map[string]bool        // quem está anunciado como online
	pending map[string]*time.Timer // saída em carência
	grace   time.Duration
	origins []string
	// recheck valida periodicamente se a sessão continua ativa (logout/revogação).
	recheck time.Duration
}

func NewHub(allowedOrigins []string) *Hub {
	return &Hub{
		clients: map[*Client]struct{}{}, conns: map[string]int{}, online: map[string]bool{}, pending: map[string]*time.Timer{},
		grace: presenceGrace, origins: allowedOrigins, recheck: time.Minute,
	}
}

// SetPresenceGrace muda a carência antes de anunciar que alguém saiu (testes).
func (h *Hub) SetPresenceGrace(d time.Duration) {
	h.mu.Lock()
	h.grace = d
	h.mu.Unlock()
}

// Online devolve quem está online agora (ordenado).
func (h *Hub) Online() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.online))
	for id := range h.online {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// PresenceHandler responde GET /presence com quem está online (o estado inicial; as mudanças chegam pelo evento "presence").
func (h *Hub) PresenceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"online": h.Online()})
	}
}

type presence struct {
	UserID string `json:"userId"`
	Online bool   `json:"online"`
}

// Publish envia o evento a todos os clientes que passam no filtro. Clientes
// lentos (buffer cheio) são desconectados em vez de travar o envio.
func (h *Hub) Publish(typ string, data any, filter Filter) {
	b, err := json.Marshal(Event{Type: typ, Data: data})
	if err != nil {
		slog.Error("realtime: serializando evento", "type", typ, "err", err)
		return
	}
	var slow []*Client
	h.mu.RLock()
	for c := range h.clients {
		if filter != nil && !filter(c) {
			continue
		}
		select {
		case c.send <- b:
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range slow {
		h.remove(c) // fecha o canal; o handler encerra a conexão
	}
}

// Count devolve o número de conexões ativas (métricas/testes).
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) add(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.conns[c.UserID]++
	if t := h.pending[c.UserID]; t != nil { // voltou durante a carência: nunca saiu
		t.Stop()
		delete(h.pending, c.UserID)
	}
	announce := !h.online[c.UserID]
	h.online[c.UserID] = true
	h.mu.Unlock()
	if announce {
		h.Publish("presence", presence{UserID: c.UserID, Online: true}, nil)
	}
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; !ok {
		h.mu.Unlock()
		return
	}
	delete(h.clients, c)
	close(c.send)
	h.conns[c.UserID]--
	if h.conns[c.UserID] <= 0 {
		delete(h.conns, c.UserID)
		id := c.UserID
		h.pending[id] = time.AfterFunc(h.grace, func() { h.gone(id) })
	}
	h.mu.Unlock()
}

// gone: acabou a carência; se a pessoa continua sem conexão, anuncia que saiu.
func (h *Hub) gone(id string) {
	h.mu.Lock()
	delete(h.pending, id)
	if h.conns[id] > 0 || !h.online[id] {
		h.mu.Unlock()
		return
	}
	delete(h.online, id)
	h.mu.Unlock()
	h.Publish("presence", presence{UserID: id, Online: false}, nil)
}

// Handler aceita a conexão. Deve ser montado atrás de RequireSession/RequireMFA.
// validate é chamado periodicamente; ao falhar (sessão encerrada), a conexão é fechada.
func (h *Hub) Handler(validate func(ctx context.Context, r *http.Request) bool) http.HandlerFunc {
	var patterns []string
	for _, o := range h.origins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			patterns = append(patterns, u.Host)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		sess := auth.MustFromContext(r.Context())
		// Prazos do servidor HTTP não se aplicam: net/http os remove no Hijack
		// (coberto por TestWebSocketSurvivesServerTimeouts). O ping cuida da saúde.
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: patterns})
		if err != nil {
			return // Accept já respondeu (ex.: origem não permitida)
		}
		defer conn.CloseNow()

		c := &Client{UserID: sess.UserID, Admin: sess.IsAdmin(), send: make(chan []byte, 512)}
		h.add(c)
		defer h.remove(c)

		ctx := conn.CloseRead(r.Context()) // não esperamos mensagens do cliente
		ping := time.NewTicker(25 * time.Second)
		defer ping.Stop()
		check := time.NewTicker(h.recheck)
		defer check.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-c.send:
				if !ok {
					_ = conn.Close(websocket.StatusPolicyViolation, "cliente lento")
					return
				}
				wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				cancel()
				if err != nil {
					return
				}
			case <-ping.C:
				pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Ping(pctx)
				cancel()
				if err != nil {
					return
				}
			case <-check.C:
				if validate != nil && !validate(ctx, r) {
					_ = conn.Close(4401, "sessão encerrada")
					return
				}
			}
		}
	}
}

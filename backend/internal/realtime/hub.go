// Package realtime distribui eventos aos navegadores conectados via WebSocket.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
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

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	origins []string
	// recheck valida periodicamente se a sessão continua ativa (logout/revogação).
	recheck time.Duration
}

func NewHub(allowedOrigins []string) *Hub {
	return &Hub{clients: map[*Client]struct{}{}, origins: allowedOrigins, recheck: time.Minute}
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
	h.mu.Unlock()
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
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

		c := &Client{UserID: sess.UserID, Admin: sess.IsAdmin(), send: make(chan []byte, 64)}
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

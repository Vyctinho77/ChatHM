package whatsapp

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/realtime"
)

var (
	ErrAlreadyConnected = errors.New("whatsapp: já conectado")
	ErrBusy             = errors.New("whatsapp: operação em andamento")
)

// Eventos em tempo real.
const (
	EventStatus = "whatsapp.status" // todos os usuários (sem QR)
	EventQR     = "whatsapp.qr"     // apenas administradores
)

// Status é o estado público da conexão (sem segredos).
type Status struct {
	State       State      `json:"state"`
	Phone       string     `json:"phone"`
	DisplayName string     `json:"displayName"`
	LastError   string     `json:"lastError,omitempty"`
	ConnectedAt *time.Time `json:"connectedAt,omitempty"`
}

// QR é o código temporário de pareamento (somente admin; nunca persistido).
type QR struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Publisher interface {
	Publish(typ string, data any, filter realtime.Filter)
}

// Ingestor grava mensagens recebidas (conversations.Service).
type Ingestor func(ctx context.Context, m InboundMessage) (bool, error)

// ReceiptHandler aplica recibos de entrega/leitura (conversations.Service).
type ReceiptHandler func(ctx context.Context, r Receipt)

// Manager coordena o provedor, o estado persistido e os eventos em tempo real.
type Manager struct {
	db     *pgxpool.Pool
	pub    Publisher
	ingest Ingestor
	onRcpt ReceiptHandler
	p      Provider
	log    *slog.Logger

	pairTimeout time.Duration

	setMu      sync.Mutex // serializa apply (memória → banco → eventos)
	mu         sync.Mutex
	status     Status
	qr         *QR
	pairCancel context.CancelFunc
}

func NewManager(db *pgxpool.Pool, pub Publisher, ingest Ingestor, factory Factory, log *slog.Logger) *Manager {
	m := &Manager{db: db, pub: pub, ingest: ingest, log: log.With("component", "whatsapp"), pairTimeout: 3 * time.Minute}
	m.p = factory(m)
	return m
}

// Start carrega o estado salvo e, havendo sessão válida, reconecta em segundo plano.
func (m *Manager) Start(ctx context.Context) error {
	var st Status
	var state string
	if err := m.db.QueryRow(ctx,
		`SELECT status, phone, display_name, last_error, connected_at FROM whatsapp_connections WHERE id = 1`,
	).Scan(&state, &st.Phone, &st.DisplayName, &st.LastError, &st.ConnectedAt); err != nil {
		return err
	}
	has, err := m.p.HasSession(ctx)
	if err != nil {
		m.log.Error("lendo sessão salva", "err", err)
		m.set(Status{State: StateError, LastError: "Não foi possível ler a sessão salva. Verifique a ENCRYPTION_KEY ou reconecte."}, nil)
		return nil
	}
	if !has {
		m.set(Status{State: StateDisconnected, LastError: st.LastError}, nil)
		return nil
	}
	// Até o evento "conectado", exibe o último número conhecido como reconectando.
	m.set(Status{State: StateError, Phone: st.Phone, DisplayName: st.DisplayName, LastError: "Reconectando…"}, nil)
	go func() {
		if err := m.p.Restore(context.Background()); err != nil {
			m.log.Error("restaurando sessão", "err", err)
			m.set(Status{State: StateError, Phone: st.Phone, DisplayName: st.DisplayName,
				LastError: "Falha ao reconectar. Tente reconectar ou gere um novo QR Code."}, nil)
		}
	}()
	return nil
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// CurrentQR devolve o QR vigente (admin) ou nil.
func (m *Manager) CurrentQR() *QR {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.qr == nil || time.Now().After(m.qr.ExpiresAt) {
		return nil
	}
	q := *m.qr
	return &q
}

// Connect inicia o pareamento por QR Code. Idempotente enquanto aguarda leitura.
func (m *Manager) Connect(ctx context.Context, actorID, ip string) error {
	m.mu.Lock()
	switch m.status.State {
	case StateConnected:
		m.mu.Unlock()
		return ErrAlreadyConnected
	case StatePendingQR:
		m.mu.Unlock()
		return nil
	}
	pctx, cancel := context.WithTimeout(context.Background(), m.pairTimeout)
	m.pairCancel = cancel
	m.mu.Unlock()

	events, err := m.p.Pair(pctx)
	if err != nil {
		cancel()
		m.set(Status{State: StateError, LastError: "Não foi possível iniciar a conexão com o WhatsApp."}, nil)
		m.log.Error("iniciando pareamento", "err", err)
		return err
	}
	m.audit(ctx, audit.Entry{ActorID: actorID, Action: audit.WhatsAppConnect, IP: ip})
	m.set(Status{State: StatePendingQR}, nil)

	go func() {
		defer cancel()
		for ev := range events {
			switch {
			case ev.QR != "":
				exp := ev.Expires
				if exp <= 0 {
					exp = 20 * time.Second
				}
				// Eventos de pareamento só valem enquanto o estado ainda é "aguardando QR":
				// um QR atrasado não pode sobrescrever uma conexão que já foi concluída.
				m.setIfPending(Status{State: StatePendingQR}, &QR{Code: ev.QR, ExpiresAt: time.Now().Add(exp)})
			case ev.Success:
				// OnConnected define o estado final com número e nome.
				return
			case ev.Timeout:
				if m.setIfPending(Status{State: StateDisconnected, LastError: "O QR Code expirou sem leitura. Gere um novo."}, nil) {
					m.p.Close()
				}
				return
			case ev.Err != nil:
				m.log.Error("pareamento", "err", ev.Err)
				if m.setIfPending(Status{State: StateError, LastError: "Falha no pareamento. Tente novamente."}, nil) {
					m.p.Close()
				}
				return
			}
		}
		if pctx.Err() != nil {
			if m.setIfPending(Status{State: StateDisconnected, LastError: "Tempo para leitura do QR Code esgotado."}, nil) {
				m.p.Close()
			}
		}
	}()
	return nil
}

// Disconnect desvincula o aparelho e apaga a sessão local.
func (m *Manager) Disconnect(ctx context.Context, actorID, ip string) error {
	m.mu.Lock()
	if m.pairCancel != nil {
		m.pairCancel()
		m.pairCancel = nil
	}
	m.mu.Unlock()

	err := m.p.Logout(ctx)
	if err != nil {
		m.log.Warn("logout no WhatsApp falhou; sessão local removida", "err", err)
	}
	m.audit(ctx, audit.Entry{ActorID: actorID, Action: audit.WhatsAppDisconnect, IP: ip})
	m.set(Status{State: StateDisconnected}, nil)
	return nil
}

// Shutdown encerra a conexão sem apagar a sessão (reinício do servidor).
func (m *Manager) Shutdown() {
	m.mu.Lock()
	if m.pairCancel != nil {
		m.pairCancel()
	}
	m.mu.Unlock()
	m.p.Close()
}

// --- Handler (eventos do provedor) -----------------------------------------

func (m *Manager) OnConnected(a Account) {
	now := time.Now()
	m.mu.Lock()
	connectedAt := m.status.ConnectedAt
	if m.status.State != StateConnected || connectedAt == nil {
		connectedAt = &now
	}
	if m.pairCancel != nil {
		m.pairCancel = nil
	}
	m.mu.Unlock()
	m.set(Status{State: StateConnected, Phone: a.Phone, DisplayName: a.Name, ConnectedAt: connectedAt}, nil)
}

func (m *Manager) OnDisconnected(reason string, loggedOut bool) {
	cur := m.Status()
	if !loggedOut && cur.State == StateDisconnected {
		return // queda após desconexão intencional
	}
	if loggedOut {
		m.set(Status{State: StateDisconnected, LastError: reason}, nil)
		return
	}
	m.set(Status{State: StateError, Phone: cur.Phone, DisplayName: cur.DisplayName, LastError: reason}, nil)
}

// SetReceiptHandler define quem processa os recibos (ligado depois da criação, pois
// o serviço de conversas também depende do Manager para enviar).
func (m *Manager) SetReceiptHandler(h ReceiptHandler) { m.onRcpt = h }

func (m *Manager) OnReceipt(r Receipt) {
	if m.onRcpt == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m.onRcpt(ctx, r)
}

// Connected informa se o WhatsApp está conectado e pronto para enviar.
func (m *Manager) Connected() bool { return m.Status().State == StateConnected }

func (m *Manager) NewMessageID() string { return m.p.NewMessageID() }

func (m *Manager) SendText(ctx context.Context, chatJID, messageID, text string) error {
	return m.p.SendText(ctx, chatJID, messageID, text)
}

func (m *Manager) SendMedia(ctx context.Context, chatJID, messageID string, md MediaSend) error {
	return m.p.SendMedia(ctx, chatJID, messageID, md)
}

func (m *Manager) SendLocation(ctx context.Context, chatJID, messageID string, l Location) error {
	return m.p.SendLocation(ctx, chatJID, messageID, l)
}

func (m *Manager) OnMessage(msg InboundMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := m.ingest(ctx, msg); err != nil {
		// Sem conteúdo da mensagem no log.
		m.log.Error("gravando mensagem recebida", "external_id", msg.ExternalID, "kind", msg.Kind, "err", err)
	}
}

// --- interno ---------------------------------------------------------------

func (m *Manager) set(st Status, qr *QR) { m.apply(st, qr, false) }

// setIfPending aplica o estado só se a conexão ainda estiver aguardando o QR Code.
func (m *Manager) setIfPending(st Status, qr *QR) bool { return m.apply(st, qr, true) }

func (m *Manager) apply(st Status, qr *QR, onlyIfPending bool) bool {
	// setMu mantém ordenadas a atualização em memória, a gravação e a publicação.
	m.setMu.Lock()
	defer m.setMu.Unlock()
	m.mu.Lock()
	if onlyIfPending && m.status.State != StatePendingQR {
		m.mu.Unlock()
		return false
	}
	m.status = st
	m.qr = qr
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.db.Exec(ctx, `
		UPDATE whatsapp_connections
		   SET status = $1, phone = $2, display_name = $3, last_error = $4, connected_at = $5
		 WHERE id = 1`, string(st.State), st.Phone, st.DisplayName, st.LastError, st.ConnectedAt); err != nil {
		m.log.Error("salvando status da conexão", "err", err)
	}
	if m.pub != nil {
		m.pub.Publish(EventStatus, st, nil)
		m.pub.Publish(EventQR, qr, realtime.AdminsOnly)
	}
	return true
}

func (m *Manager) audit(ctx context.Context, e audit.Entry) {
	if err := audit.Record(ctx, m.db, e); err != nil {
		m.log.Error("auditoria", "err", err)
	}
}

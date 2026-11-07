package whatsapp

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
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
	State           State            `json:"state"`
	Phone           string           `json:"phone"`
	DisplayName     string           `json:"displayName"`
	LastError       string           `json:"lastError,omitempty"`
	ConnectedAt     *time.Time       `json:"connectedAt,omitempty"`
	PairingComplete bool             `json:"pairingComplete,omitempty"`
	Sync            *HistoryProgress `json:"sync,omitempty"`
	HistoryRequest  string           `json:"historyRequest,omitempty"`
	Preparing       bool             `json:"preparing,omitempty"`
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
	db               *pgxpool.Pool
	pub              Publisher
	ingest           Ingestor
	onRcpt           ReceiptHandler
	onTyping         func(context.Context, Typing) error
	avatarMu         sync.Mutex
	avatarBusy       map[string]bool
	avatarSem        chan struct{}
	sweeping         atomic.Bool
	stopOnce         sync.Once
	stopCh           chan struct{} // fechado no Shutdown
	onHistoryContact func(context.Context, HistoryContact) error
	p                Provider
	log              *slog.Logger

	pairTimeout time.Duration

	setMu                sync.Mutex // serializa apply (memória → banco → eventos)
	mu                   sync.Mutex
	status               Status
	historyProgress      *HistoryProgress
	qr                   *QR
	pairCancel           context.CancelFunc
	lastHistoryRequest   time.Time
	initialHistoryDevice string
	historyRequest       string
}

func NewManager(db *pgxpool.Pool, pub Publisher, ingest Ingestor, factory Factory, log *slog.Logger) *Manager {
	m := &Manager{db: db, pub: pub, ingest: ingest, log: log.With("component", "whatsapp"), pairTimeout: 3 * time.Minute, stopCh: make(chan struct{})}
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
	st := m.status
	st.HistoryRequest = m.historyRequest
	if m.historyProgress != nil {
		progress := *m.historyProgress
		st.Sync = &progress
	}
	return st
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
	m.historyProgress = nil
	m.mu.Unlock()
	m.set(Status{State: StatePendingQR}, nil)

	events, err := m.p.Pair(pctx)
	if err != nil {
		cancel()
		m.set(Status{State: StateError, LastError: "Não foi possível iniciar a conexão com o WhatsApp."}, nil)
		m.log.Error("iniciando pareamento", "err", err)
		return err
	}
	m.audit(ctx, audit.Entry{ActorID: actorID, Action: audit.WhatsAppConnect, IP: ip})

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
				m.setIfPending(Status{State: StatePendingQR, PairingComplete: true, LastError: "QR Code lido. Confirmando conexão com o WhatsApp…"}, nil)
				// Pairing succeeded. Real connection/disconnection events now own
				// the state; a local timer cannot declare this linked device invalid.
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
	m.stopOnce.Do(func() { close(m.stopCh) })
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
	m.requestInitialHistory(a.DeviceJID)
	m.startAvatarSweep()
}

// The server has authenticated us, but whatsmeow is still preparing message
// keys. Show the confirmed connection immediately; enable messaging/history
// requests only on the later Connected event.
func (m *Manager) OnAuthenticated(a Account) {
	now := time.Now()
	m.set(Status{State: StateConnected, Preparing: true, Phone: a.Phone, DisplayName: a.Name, ConnectedAt: &now}, nil)
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
func (m *Manager) Connected() bool {
	st := m.Status()
	return st.State == StateConnected && !st.Preparing
}

func (m *Manager) NewMessageID() string { return m.p.NewMessageID() }

func (m *Manager) SendText(ctx context.Context, chatJID, messageID, text string) error {
	return m.p.SendText(ctx, chatJID, messageID, text)
}

func (m *Manager) SendTextQuoted(ctx context.Context, chatJID, messageID, text string, q Quote) error {
	return m.p.SendTextQuoted(ctx, chatJID, messageID, text, q)
}

func (m *Manager) EditText(ctx context.Context, chatJID, targetID, text string) error {
	return m.p.EditText(ctx, chatJID, targetID, text)
}

func (m *Manager) Revoke(ctx context.Context, chatJID, targetID string) error {
	return m.p.Revoke(ctx, chatJID, targetID)
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
		return
	}
	go m.refreshAvatar(msg.ContactWAID, msg.ChatJID)
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
	if st.Preparing && m.status.State == StateConnected {
		m.mu.Unlock()
		return false
	}
	m.status = st
	m.qr = qr
	st.HistoryRequest = m.historyRequest
	if m.historyProgress != nil {
		progress := *m.historyProgress
		st.Sync = &progress
	}
	m.mu.Unlock()

	// Notify the browser immediately; database latency must not keep the QR
	// visible after the provider has confirmed the live connection.
	if m.pub != nil {
		m.pub.Publish(EventStatus, st, nil)
		m.pub.Publish(EventQR, qr, realtime.AdminsOnly)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.db.Exec(ctx, `
		UPDATE whatsapp_connections
		   SET status = $1, phone = $2, display_name = $3, last_error = $4, connected_at = $5
		 WHERE id = 1`, string(st.State), st.Phone, st.DisplayName, st.LastError, st.ConnectedAt); err != nil {
		m.log.Error("salvando status da conexão", "err", err)
	}
	return true
}

func (m *Manager) SetHistoryContactHandler(h func(context.Context, HistoryContact) error) {
	m.onHistoryContact = h
}

func (m *Manager) OnHistoryMessage(msg InboundMessage) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	msg.Historical = true
	return m.ingest(ctx, msg)
}

func (m *Manager) OnHistoryContact(contact HistoryContact) error {
	if m.onHistoryContact == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return m.onHistoryContact(ctx, contact)
}

func (m *Manager) OnHistoryProgress(progress HistoryProgress) {
	m.mu.Lock()
	m.historyProgress = &progress
	m.mu.Unlock()
	if m.pub != nil {
		m.pub.Publish("whatsapp.sync", progress, realtime.AdminsOnly)
	}
}

func (m *Manager) RequestHistory(ctx context.Context) error {
	requester, ok := m.p.(interface{ RequestHistory(context.Context) error })
	if !ok || !m.Connected() {
		return ErrNotConnected
	}
	m.mu.Lock()
	if time.Since(m.lastHistoryRequest) < time.Minute {
		m.mu.Unlock()
		return ErrBusy
	}
	m.lastHistoryRequest = time.Now()
	m.historyRequest = "requesting"
	m.mu.Unlock()
	m.publishHistoryRequest()
	err := requester.RequestHistory(ctx)
	m.mu.Lock()
	if err != nil {
		m.historyRequest = "failed"
	} else {
		m.historyRequest = "requested"
	}
	m.mu.Unlock()
	m.publishHistoryRequest()
	return err
}

// Request once per linked device during this server run, including restoration
// of a saved session. Network reconnects and profile-name events must not repeat
// a potentially large transfer. A fresh QR pairing has a different DeviceJID.
func (m *Manager) requestInitialHistory(device string) {
	if device == "" {
		return
	}
	if _, ok := m.p.(interface{ RequestHistory(context.Context) error }); !ok {
		return
	}
	m.mu.Lock()
	if m.initialHistoryDevice == device {
		m.mu.Unlock()
		return
	}
	if m.initialHistoryDevice != "" {
		m.lastHistoryRequest = time.Time{}
	}
	m.initialHistoryDevice = device
	m.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := m.RequestHistory(ctx); err != nil {
			m.log.Warn("solicitação automática de histórico não concluída", "err", err)
		}
	}()
}

func (m *Manager) publishHistoryRequest() {
	if m.pub != nil {
		m.pub.Publish(EventStatus, m.Status(), nil)
	}
}

func (m *Manager) audit(ctx context.Context, e audit.Entry) {
	if err := audit.Record(ctx, m.db, e); err != nil {
		m.log.Error("auditoria", "err", err)
	}
}

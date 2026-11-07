// Package wameow é o adaptador WhatsAppProvider baseado em whatsmeow
// (sessão espelhada por QR Code; não oficial).
package wameow

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// NewFactory prepara o armazenamento (tabelas whatsmeow_* no mesmo PostgreSQL)
// e devolve a fábrica do provedor.
func NewFactory(ctx context.Context, pool *pgxpool.Pool, sqlDB *sql.DB, box *secretbox.Box, logger *slog.Logger) (whatsapp.Factory, error) {
	wlog := slogAdapter{l: logger.With("component", "whatsmeow")}
	container := sqlstore.NewWithDB(sqlDB, "postgres", wlog.Sub("store"))
	if err := container.Upgrade(ctx); err != nil {
		return nil, fmt.Errorf("preparando tabelas do whatsmeow: %w", err)
	}
	if _, err := sqlDB.ExecContext(ctx, protectStoreTables); err != nil {
		return nil, fmt.Errorf("protegendo tabelas do whatsmeow: %w", err)
	}
	sc := &secureContainer{inner: container, db: pool, box: box, log: wlog.Sub("device")}
	return func(h whatsapp.Handler) whatsapp.Provider {
		return &Provider{h: h, store: sc, log: wlog}
	}, nil
}

type Provider struct {
	h     whatsapp.Handler
	store *secureContainer
	log   waLog.Logger

	mu              sync.Mutex
	cli             *whatsmeow.Client
	historyMu       sync.Mutex
	historyClient   *whatsmeow.Client
	historyProgress whatsapp.HistoryProgress
	contactsSyncing bool
	groupNames      map[types.JID]groupNameEntry
}

func (p *Provider) HasSession(ctx context.Context) (bool, error) {
	d, err := p.store.Load(ctx)
	return d != nil, err
}

func (p *Provider) Restore(ctx context.Context) error {
	d, err := p.store.Load(ctx)
	if err != nil {
		return err
	}
	if d == nil {
		return whatsapp.ErrNoSession
	}
	cli := whatsmeow.NewClient(d, p.log.Sub("client"))
	cli.AddEventHandler(p.handle(cli))
	p.swap(cli)
	p.observeAuthentication(cli)
	return cli.Connect()
}

func (p *Provider) Pair(ctx context.Context) (<-chan whatsapp.PairEvent, error) {
	// Uma sessão anterior (inválida ou abandonada) é descartada antes do novo pareamento.
	if old, err := p.store.Load(ctx); err == nil && old != nil {
		_ = p.store.DeleteDevice(ctx, old)
	}
	d := p.store.NewDevice()
	cli := whatsmeow.NewClient(d, p.log.Sub("client"))
	cli.AddEventHandler(p.handle(cli))
	qr, err := cli.GetQRChannel(ctx)
	if err != nil {
		return nil, err
	}
	p.swap(cli)
	p.observeAuthentication(cli)
	if err := cli.Connect(); err != nil {
		return nil, err
	}

	out := make(chan whatsapp.PairEvent, 4)
	go func() {
		defer close(out)
		for item := range qr {
			switch {
			case item.Event == whatsmeow.QRChannelEventCode:
				out <- whatsapp.PairEvent{QR: item.Code, Expires: item.Timeout}
			case item == whatsmeow.QRChannelSuccess:
				out <- whatsapp.PairEvent{Success: true}
				return
			case item == whatsmeow.QRChannelTimeout:
				out <- whatsapp.PairEvent{Timeout: true}
				return
			case item.Event == whatsmeow.QRChannelEventError:
				out <- whatsapp.PairEvent{Err: item.Error}
				return
			default:
				out <- whatsapp.PairEvent{Err: fmt.Errorf("pareamento: %s", item.Event)}
				return
			}
		}
	}()
	return out, nil
}

func (p *Provider) Logout(ctx context.Context) error {
	p.mu.Lock()
	cli := p.cli
	p.cli = nil
	p.mu.Unlock()

	var logoutErr error
	if cli != nil && cli.Store.ID != nil && cli.IsConnected() {
		// Desvincula o aparelho no celular e apaga a sessão (via DeleteDevice).
		logoutErr = cli.Logout(ctx)
	}
	if cli != nil {
		cli.Disconnect()
	}
	// Garante a remoção local mesmo se o servidor estiver inacessível.
	if d, err := p.store.Load(ctx); err == nil && d != nil {
		if err := p.store.DeleteDevice(ctx, d); err != nil {
			return err
		}
	}
	return logoutErr
}

func (p *Provider) Close() {
	p.swap(nil)
}

// NewMessageID segue o formato dos IDs do WhatsApp Web ("3EB0" + 18 hex).
func (p *Provider) NewMessageID() string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return "3EB0" + strings.ToUpper(hex.EncodeToString(b))
}

func (p *Provider) SendText(ctx context.Context, chatJID, messageID, text string) error {
	p.mu.Lock()
	cli := p.cli
	p.mu.Unlock()
	if cli == nil || !cli.IsConnected() || cli.Store.ID == nil {
		return whatsapp.ErrNotConnected
	}
	to, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("conversa inválida: %w", err)
	}
	_, err = cli.SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(text)},
		whatsmeow.SendRequestExtra{ID: types.MessageID(messageID)})
	return err
}

// contextInfo monta a citação de uma mensagem. nil quando não há como identificar o autor (grupos antigos sem JID).
func contextInfo(cli *whatsmeow.Client, chat types.JID, q *whatsapp.Quote) *waE2E.ContextInfo {
	if q == nil || q.ExternalID == "" {
		return nil
	}
	participant := q.SenderJID
	switch {
	case q.FromMe && cli.Store.ID != nil:
		participant = cli.Store.ID.ToNonAD().String()
	case participant == "" && chat.Server != types.GroupServer:
		participant = chat.ToNonAD().String()
	}
	if participant == "" {
		return nil
	}
	ci := &waE2E.ContextInfo{StanzaID: proto.String(q.ExternalID), Participant: proto.String(participant)}
	if q.Text != "" {
		ci.QuotedMessage = &waE2E.Message{Conversation: proto.String(q.Text)}
	} else {
		ci.QuotedMessage = &waE2E.Message{Conversation: proto.String("")}
	}
	return ci
}

func (p *Provider) SendTextQuoted(ctx context.Context, chatJID, messageID, text string, q whatsapp.Quote) error {
	cli, err := p.client()
	if err != nil {
		return err
	}
	if cli == nil || !cli.IsConnected() || cli.Store.ID == nil {
		return whatsapp.ErrNotConnected
	}
	to, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("conversa inválida: %w", err)
	}
	msg := &waE2E.Message{Conversation: proto.String(text)}
	if ci := contextInfo(cli, to, &q); ci != nil {
		msg = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text), ContextInfo: ci}}
	}
	_, err = cli.SendMessage(ctx, to, msg, whatsmeow.SendRequestExtra{ID: types.MessageID(messageID)})
	return err
}

// EditText edita uma mensagem de texto enviada por este aparelho (dentro da janela do WhatsApp).
func (p *Provider) EditText(ctx context.Context, chatJID, targetID, text string) error {
	cli, err := p.client()
	if err != nil {
		return err
	}
	if !cli.IsConnected() || cli.Store.ID == nil {
		return whatsapp.ErrNotConnected
	}
	to, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("conversa inválida: %w", err)
	}
	_, err = cli.SendMessage(ctx, to, cli.BuildEdit(to, types.MessageID(targetID), &waE2E.Message{Conversation: proto.String(text)}))
	return err
}

// Revoke apaga para todos uma mensagem enviada pelo escritório.
func (p *Provider) Revoke(ctx context.Context, chatJID, targetID string) error {
	cli, err := p.client()
	if err != nil {
		return err
	}
	if !cli.IsConnected() || cli.Store.ID == nil {
		return whatsapp.ErrNotConnected
	}
	to, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("conversa inválida: %w", err)
	}
	_, err = cli.SendMessage(ctx, to, cli.BuildRevoke(to, types.EmptyJID, types.MessageID(targetID)))
	return err
}

func (p *Provider) client() (*whatsmeow.Client, error) {
	p.mu.Lock()
	cli := p.cli
	p.mu.Unlock()
	if cli == nil || !cli.IsConnected() || cli.Store.ID == nil {
		return nil, whatsapp.ErrNotConnected
	}
	return cli, nil
}

func (p *Provider) SendMedia(ctx context.Context, chatJID, messageID string, m whatsapp.MediaSend) error {
	cli, err := p.client()
	if err != nil {
		return err
	}
	to, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("conversa inválida: %w", err)
	}
	var msg *waE2E.Message
	switch m.Kind {
	case whatsapp.KindImage:
		up, err := cli.Upload(ctx, m.Data, whatsmeow.MediaImage)
		if err != nil {
			return err
		}
		msg = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			ContextInfo: contextInfo(cli, to, m.Quote),
			URL:         &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: &up.FileLength,
			Mimetype: proto.String(m.MimeType), Caption: proto.String(m.Caption),
			Width: proto.Uint32(uint32(m.Width)), Height: proto.Uint32(uint32(m.Height)),
		}}
	case whatsapp.KindAudio:
		up, err := cli.Upload(ctx, m.Data, whatsmeow.MediaAudio)
		if err != nil {
			return err
		}
		a := &waE2E.AudioMessage{
			ContextInfo: contextInfo(cli, to, m.Quote),
			URL:         &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: &up.FileLength,
			Mimetype: proto.String(m.MimeType), Seconds: proto.Uint32(uint32(m.DurationSec)), PTT: proto.Bool(true),
		}
		if len(m.Waveform) > 0 {
			a.Waveform = m.Waveform
		}
		msg = &waE2E.Message{AudioMessage: a}
	case whatsapp.KindDocument:
		up, err := cli.Upload(ctx, m.Data, whatsmeow.MediaDocument)
		if err != nil {
			return err
		}
		msg = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			ContextInfo: contextInfo(cli, to, m.Quote),
			URL:         &up.URL, DirectPath: &up.DirectPath, MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: &up.FileLength,
			Mimetype: proto.String(m.MimeType), FileName: proto.String(m.FileName), Title: proto.String(m.FileName),
			Caption: proto.String(m.Caption),
		}}
	default:
		return fmt.Errorf("tipo de mídia não suportado: %s", m.Kind)
	}
	_, err = cli.SendMessage(ctx, to, msg, whatsmeow.SendRequestExtra{ID: types.MessageID(messageID)})
	return err
}

func (p *Provider) SendLocation(ctx context.Context, chatJID, messageID string, l whatsapp.Location) error {
	cli, err := p.client()
	if err != nil {
		return err
	}
	to, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("conversa inválida: %w", err)
	}
	_, err = cli.SendMessage(ctx, to, &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude: proto.Float64(l.Lat), DegreesLongitude: proto.Float64(l.Lng),
		Name: proto.String(l.Name), Address: proto.String(l.Address),
	}}, whatsmeow.SendRequestExtra{ID: types.MessageID(messageID)})
	return err
}

func (p *Provider) swap(cli *whatsmeow.Client) {
	p.mu.Lock()
	old := p.cli
	p.cli = cli
	p.mu.Unlock()
	if old != nil && old != cli {
		old.Disconnect()
	}
}

func (p *Provider) handle(cli *whatsmeow.Client) func(any) {
	return func(evt any) {
		// Ignora eventos de clientes substituídos (ex.: pareamento anterior).
		p.mu.Lock()
		current := p.cli == cli
		p.mu.Unlock()
		if !current {
			return
		}
		switch e := evt.(type) {
		case *events.Connected:
			acc := whatsapp.Account{Name: cli.Store.PushName}
			if cli.Store.ID != nil {
				acc.Phone = "+" + cli.Store.ID.User
				acc.DeviceJID = cli.Store.ID.String()
			}
			p.h.OnConnected(acc)
			p.enableTyping(cli)
			p.scheduleContactSync(cli)
		case *events.PushNameSetting:
			if cli.Store.ID != nil && cli.IsLoggedIn() {
				p.h.OnConnected(whatsapp.Account{Phone: "+" + cli.Store.ID.User, Name: cli.Store.PushName, DeviceJID: cli.Store.ID.String()})
				p.enableTyping(cli)
			}
		case *events.LoggedOut:
			p.h.OnDisconnected("Aparelho desvinculado no celular.", true)
		case *events.StreamReplaced:
			p.h.OnDisconnected("Sessão aberta em outro local.", false)
		case *events.TemporaryBan:
			p.h.OnDisconnected(fmt.Sprintf("Número temporariamente bloqueado pelo WhatsApp (%s).", e.Expire.Round(time.Minute)), false)
		case *events.ClientOutdated:
			p.h.OnDisconnected("Versão do cliente desatualizada; atualize o ChatHM.", false)
		case *events.Disconnected:
			p.h.OnDisconnected("Conexão perdida; tentando reconectar.", false)
		case *events.Message:
			if m, ok := convertMessage(context.Background(), cli, e); ok {
				if m.IsGroup {
					if gj, err := types.ParseJID(m.ChatJID); err == nil {
						m.GroupName = p.groupName(cli, gj)
					}
				}
				p.h.OnMessage(m)
			}
		case *events.HistorySync:
			go p.importHistory(cli, e)
		case *events.Contact, *events.AppStateSyncComplete:
			p.scheduleContactSync(cli)
		case *events.ChatPresence:
			if h, ok := p.h.(whatsapp.TypingHandler); ok {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				if update, valid := convertTyping(ctx, cli, e); valid {
					h.OnTyping(update)
				}
				cancel()
			}
		case *events.Receipt:
			if r, ok := convertReceipt(e); ok {
				p.h.OnReceipt(r)
			}
		}
	}
}

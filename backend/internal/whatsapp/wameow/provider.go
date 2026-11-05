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
	sc := &secureContainer{inner: container, db: pool, box: box, log: wlog.Sub("device")}
	return func(h whatsapp.Handler) whatsapp.Provider {
		return &Provider{h: h, store: sc, log: wlog}
	}, nil
}

type Provider struct {
	h     whatsapp.Handler
	store *secureContainer
	log   waLog.Logger

	mu  sync.Mutex
	cli *whatsmeow.Client
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
		case *events.PushNameSetting:
			if cli.Store.ID != nil {
				p.h.OnConnected(whatsapp.Account{Phone: "+" + cli.Store.ID.User, Name: cli.Store.PushName, DeviceJID: cli.Store.ID.String()})
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
				p.h.OnMessage(m)
			}
		case *events.Receipt:
			if r, ok := convertReceipt(e); ok {
				p.h.OnReceipt(r)
			}
		}
	}
}

// Package app monta os serviços a partir da configuração (usado pela API e pela CLI).
package app

import (
	"context"
	"github.com/vyctinho77/chathm/backend/internal/aiaccess"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/automation"
	"github.com/vyctinho77/chathm/backend/internal/config"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/instance"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
	"github.com/vyctinho77/chathm/backend/internal/realtime"
	"github.com/vyctinho77/chathm/backend/internal/segments"
	"github.com/vyctinho77/chathm/backend/internal/storage"
	"github.com/vyctinho77/chathm/backend/internal/tags"
	"github.com/vyctinho77/chathm/backend/internal/teams"
	"github.com/vyctinho77/chathm/backend/internal/users"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp/fake"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp/wameow"
)

type Services struct {
	Auth          *auth.Service
	Users         *users.Service
	Teams         *teams.Service
	Hub           *realtime.Hub
	Conversations *conversations.Service
	Segments      *segments.Service
	Tags          *tags.Service
	WhatsApp      *whatsapp.Manager
	Instance      *instance.Service
	AI            *aiaccess.Service
	Automation    *automation.Service
	// Simulate entrega mensagens fictícias (somente provedor fake em development).
	Simulate func(whatsapp.InboundMessage)
}

type Options struct {
	Hasher auth.Hasher
	// WhatsAppFactory substitui o provedor (testes).
	WhatsAppFactory whatsapp.Factory
	Logger          *slog.Logger
}

func NewServices(ctx context.Context, cfg config.Config, db *pgxpool.Pool, opt Options) (*Services, error) {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.Hasher.Params.Memory == 0 {
		opt.Hasher = auth.NewHasher()
	}
	box, err := secretbox.New(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	authSvc, err := auth.NewService(db, auth.Config{
		CookieName:        cfg.SessionCookieName,
		CookieSecure:      cfg.CookieSecure,
		SessionTTL:        cfg.SessionTTL,
		MaxLifetime:       cfg.SessionMaxLifetime,
		IdleTimeout:       cfg.SessionIdle,
		RequireAdminMFA:   cfg.RequireAdminMFA,
		AllowedOrigins:    cfg.AllowedOrigins,
		AllowSelfRegister: cfg.AllowSelfRegister,
		SetupToken:        cfg.SetupToken,
	}, opt.Hasher, box)
	if err != nil {
		return nil, err
	}

	var store storage.Store
	switch cfg.StorageDriver {
	case "s3":
		store, err = storage.NewS3(ctx, storage.S3Config{Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
			AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, UseSSL: cfg.S3UseSSL})
	default:
		store, err = storage.NewFS(cfg.StorageDir, box)
	}
	if err != nil {
		return nil, err
	}
	var scanner *media.Scanner
	if cfg.ClamAVAddr != "" {
		scanner = media.NewScanner(cfg.ClamAVAddr)
	}

	hub := realtime.NewHub(cfg.AllowedOrigins)
	convs := conversations.NewService(db, hub)
	mediaSvc := media.NewService(cfg.MaxAttachment, scanner)
	if cfg.FFmpegBin != "" {
		mediaSvc.FFmpeg = cfg.FFmpegBin
	}
	if cfg.FFprobeBin != "" {
		mediaSvc.FFprobe = cfg.FFprobeBin
	}
	convs.SetMedia(store, mediaSvc)
	convs.SetHistoryMediaBudget(cfg.HistoryMediaBudget)
	s := &Services{
		Auth:          authSvc,
		Users:         users.NewService(db, opt.Hasher),
		Teams:         teams.NewService(db),
		Hub:           hub,
		Conversations: convs,
		Segments:      segments.NewService(db),
		Tags:          tags.NewService(db),
		AI:            aiaccess.NewService(db),
	}
	s.AI.Attach(convs, hub)
	s.AI.SetBox(box)               // segredo do webhook de avisos, cifrado com a ENCRYPTION_KEY
	convs.SetInboundNotifier(s.AI) // avisa o webhook quando o cliente escreve
	s.Automation = automation.NewService(db, convs, store, mediaSvc, opt.Logger)
	s.Automation.SetBilling(box) // chaves de API dos gateways de cobrança, cifradas com a ENCRYPTION_KEY
	if cfg.SeedExampleData {
		if err := s.Segments.EnsureDefaults(ctx); err != nil {
			return nil, err
		}
	}

	factory := opt.WhatsAppFactory
	if factory == nil {
		switch cfg.WhatsAppProvider {
		case "fake":
			var fp *fake.Provider
			factory = fake.Factory(5*time.Second, func(p *fake.Provider) { fp = p })
			defer func() {
				if fp != nil && cfg.IsDevelopment() {
					s.Simulate = fp.Receive
				}
			}()
		default:
			factory, err = wameow.NewFactory(ctx, db, stdlib.OpenDBFromPool(db), box, opt.Logger)
			if err != nil {
				return nil, err
			}
		}
	}
	s.WhatsApp = whatsapp.NewManager(db, hub, convs.Ingest, factory, opt.Logger)
	s.WhatsApp.SetReceiptHandler(convs.ApplyReceipt)
	s.WhatsApp.SetTypingHandler(convs.ApplyTyping)
	s.WhatsApp.SetHistoryContactHandler(convs.ImportContact)
	convs.SetSender(s.WhatsApp)
	s.Instance = instance.NewService(db, authSvc, s.WhatsApp, store, opt.Logger)
	authSvc.SetWhatsAppGate(s.WhatsApp.Connected) // a instalação inicial exige o WhatsApp do escritório conectado
	return s, nil
}

// Package instance: entrega da instância a um novo cliente (um escritório por servidor).
//
// "Entregar" desconecta o WhatsApp atual, apaga TODOS os dados do escritório (usuários, conversas, contatos,
// anexos, sessão do WhatsApp, registros) e devolve a instalação ao estado inicial, protegida por um código de
// uso único. Quem recebe o endereço e o código vira o administrador do próprio escritório.
package instance

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/storage"
)

// Confirmation é o texto que a pessoa precisa digitar para confirmar.
const Confirmation = "ENTREGAR"

// Disconnector desconecta o WhatsApp (satisfeito por *whatsapp.Manager).
type Disconnector interface {
	Disconnect(ctx context.Context, actorID, ip string) error
}

type Service struct {
	db    *pgxpool.Pool
	auth  *auth.Service
	wa    Disconnector
	store storage.Store
	log   *slog.Logger
}

func NewService(db *pgxpool.Pool, a *auth.Service, wa Disconnector, st storage.Store, log *slog.Logger) *Service {
	return &Service{db: db, auth: a, wa: wa, store: st, log: log}
}

// Tabelas que permanecem: controle de migrações, versão do esquema do whatsmeow e o código de entrega (recriado).
var keep = map[string]bool{"goose_db_version": true, "whatsmeow_version": true, "install_codes": true}

var (
	ErrConfirmation = httpx.FieldErrors{"confirm": "Digite ENTREGAR para confirmar."}
)

// Handover faz a entrega. Exige o código atual do autenticador do administrador.
func (s *Service) Handover(ctx context.Context, sess *auth.Session, confirm, code string, c auth.Client) (string, error) {
	if strings.ToUpper(strings.TrimSpace(confirm)) != Confirmation {
		return "", ErrConfirmation
	}
	if err := s.auth.ConfirmWithCode(ctx, sess, code, c); err != nil {
		return "", err
	}

	// Arquivos a apagar do armazenamento (as chaves somem junto com os dados).
	var keys []string
	rows, err := s.db.Query(ctx, `SELECT storage_key FROM attachments`)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return "", err
		}
		keys = append(keys, k)
	}
	rows.Close()

	// Desvincula o aparelho no celular e remove a sessão local.
	if err := s.wa.Disconnect(ctx, sess.UserID, c.IP); err != nil {
		s.log.Warn("entrega: desconectando o WhatsApp", "err", err)
	}

	var installCode string
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tabs, err := tx.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public'`)
		if err != nil {
			return err
		}
		var names []string
		for tabs.Next() {
			var n string
			if err := tabs.Scan(&n); err != nil {
				tabs.Close()
				return err
			}
			if !keep[n] {
				names = append(names, n)
			}
		}
		tabs.Close()
		for _, n := range names {
			if _, err := tx.Exec(ctx, `TRUNCATE TABLE public.`+pgx.Identifier{n}.Sanitize()+` RESTART IDENTITY CASCADE`); err != nil {
				return err
			}
		}
		// Linha única de estado da conexão (criada pela migração) volta ao inicial.
		if _, err := tx.Exec(ctx, `INSERT INTO whatsapp_connections (id) VALUES (1) ON CONFLICT (id) DO NOTHING`); err != nil {
			return err
		}
		if installCode, err = auth.IssueInstallCode(ctx, tx); err != nil {
			return err
		}
		// A auditoria foi apagada com tudo; registra só o fato da entrega (sem dados pessoais).
		return audit.Record(ctx, tx, audit.Entry{Action: "instance.handover", IP: c.IP})
	})
	if err != nil {
		return "", err
	}
	for _, k := range keys {
		if err := s.store.Delete(context.WithoutCancel(ctx), k); err != nil {
			s.log.Warn("entrega: apagando anexo", "err", err)
		}
	}
	return installCode, nil
}

// ---------------------------------------------------------------------------

type Handler struct {
	svc  *Service
	auth *auth.Service
}

func NewHandler(s *Service, a *auth.Service) *Handler { return &Handler{svc: s, auth: a} }

// Routes: somente administradores.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/office/handover", h.handover)
}

func (h *Handler) handover(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
		Code    string `json:"code"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	code, err := h.svc.Handover(r.Context(), sess, in.Confirm, in.Code, auth.ClientFrom(r))
	var fields httpx.FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.Validation(w, fields)
	case errors.Is(err, auth.ErrMFANotEnabled):
		httpx.Error(w, http.StatusConflict, "mfa_not_enabled", "Ative o autenticador antes de entregar a instância.")
	case errors.Is(err, auth.ErrInvalidCode):
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_code", "Código inválido. Use o código atual do app autenticador.")
	case errors.Is(err, auth.ErrTooManyAttempts):
		h.auth.ClearCookie(w)
		httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", "Muitas tentativas. Entre novamente.")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		h.auth.ClearCookie(w) // todas as sessões foram apagadas
		httpx.JSON(w, http.StatusOK, map[string]string{"installCode": code})
	}
}

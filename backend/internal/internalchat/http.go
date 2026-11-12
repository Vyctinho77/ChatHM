package internalchat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func viewer(r *http.Request) Viewer {
	sess := auth.MustFromContext(r.Context())
	return Viewer{ID: sess.UserID, Admin: sess.IsAdmin(), Supervisor: sess.IsManager()}
}

// Routes: toda pessoa autenticada (com MFA concluído) usa o chat interno.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/internal-chat/room", h.room)
	r.Get("/internal-chat/messages", h.messages)
	r.Post("/internal-chat/messages", h.send)
	r.Patch("/internal-chat/messages/{id}", h.edit)
	r.Delete("/internal-chat/messages/{id}", h.delete)
	r.Post("/internal-chat/read", h.read)
	r.Get("/internal-chat/attachments/{id}", h.attachment)
	r.Get("/internal-chat/shared-conversations/{id}", h.sharedConversation)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Registro não encontrado.")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden", "Só quem escreveu a mensagem (ou um gestor/administrador) pode fazer isso.")
	case errors.Is(err, ErrUnavailable):
		httpx.Error(w, http.StatusServiceUnavailable, "media_unavailable", "O envio de anexos não está disponível.")
	case errors.Is(err, ErrIDConflict):
		httpx.Error(w, http.StatusConflict, "id_conflict", "Identificador de mensagem já utilizado.")
	default:
		httpx.Internal(w, r, err)
	}
}

func (h *Handler) room(w http.ResponseWriter, r *http.Request) {
	room, err := h.svc.Room(r.Context(), viewer(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, room)
}

// messages: ?before=<seq>&limit=<n>. Sem before, as mais recentes; hasMore indica mensagens mais antigas.
func (h *Handler) messages(w http.ResponseWriter, r *http.Request) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ms, hasMore, err := h.svc.Messages(r.Context(), before, limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": ms, "hasMore": hasMore})
}

// send: 201 mensagem nova; 200 repetição de um envio já feito.
// JSON: texto ({id, text, replyTo?}) ou localização ({id, kind:"location", lat, lng, name?, address?}).
// multipart/form-data: anexo — campos id, kind (image|audio|document), text (legenda), hd, waveform, replyTo e "file".
func (h *Handler) send(w http.ResponseWriter, r *http.Request) {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var in SendInput
	if mt == "multipart/form-data" {
		var ok bool
		if in, ok = h.readUpload(w, r); !ok {
			return
		}
	} else {
		var body struct {
			ID      string  `json:"id"`
			Kind    string  `json:"kind"`
			Text    string  `json:"text"`
			Lat     float64 `json:"lat"`
			Lng     float64 `json:"lng"`
			Name    string  `json:"name"`
			Address string  `json:"address"`
			ReplyTo string  `json:"replyTo"`

			Mentions             []Mention `json:"mentions"`
			SharedConversationID string    `json:"sharedConversationId"`
		}
		if !httpx.Decode(w, r, &body) {
			return
		}
		if body.Kind != "" && body.Kind != "text" && body.Kind != "location" && body.Kind != "conversation" {
			httpx.Validation(w, httpx.FieldErrors{"kind": "Anexos são enviados como multipart/form-data."})
			return
		}
		in = SendInput{ID: body.ID, Kind: body.Kind, Text: body.Text, Lat: body.Lat, Lng: body.Lng, Name: body.Name, Address: body.Address, ReplyTo: body.ReplyTo,
			Mentions: body.Mentions, SharedConversationID: body.SharedConversationID}
	}
	m, created, err := h.svc.Send(r.Context(), viewer(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, m)
}

func (h *Handler) readUpload(w http.ResponseWriter, r *http.Request) (SendInput, bool) {
	if h.svc.media == nil || h.svc.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "media_unavailable", "O envio de anexos não está disponível.")
		return SendInput{}, false
	}
	limit := h.svc.media.Max
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_multipart", "Envio inválido.")
		return SendInput{}, false
	}
	var (
		in       SendInput
		data     []byte
		fileName string
		hasFile  bool
	)
	fail := func(err error) (SendInput, bool) {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "attachment_too_large", fmt.Sprintf("O arquivo passa do limite de %d MB.", limit>>20))
		} else {
			httpx.Error(w, http.StatusBadRequest, "invalid_multipart", "Envio inválido.")
		}
		return SendInput{}, false
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fail(err)
		}
		name := part.FormName()
		if name == "file" {
			fileName = part.FileName()
			if data, err = io.ReadAll(io.LimitReader(part, limit+1)); err != nil {
				return fail(err)
			}
			hasFile = true
			continue
		}
		val, err := io.ReadAll(io.LimitReader(part, 64<<10))
		if err != nil {
			return fail(err)
		}
		switch name {
		case "id":
			in.ID = string(val)
		case "kind":
			in.Kind = string(val)
		case "text":
			in.Text = string(val)
		case "replyTo":
			in.ReplyTo = string(val)
		case "hd":
			in.HD = string(val) == "true" || string(val) == "1"
		case "waveform":
			var wf []float64
			if json.Unmarshal(val, &wf) == nil && len(wf) <= 128 {
				for _, f := range wf {
					in.Waveform = append(in.Waveform, math.Max(0, math.Min(1, f)))
				}
			}
		}
	}
	kind := media.Kind(in.Kind)
	if !hasFile || (kind != media.Image && kind != media.Audio && kind != media.Document) {
		httpx.Validation(w, httpx.FieldErrors{"file": "Escolha um arquivo e o tipo de anexo."})
		return SendInput{}, false
	}
	p, err := h.svc.media.Process(r.Context(), kind, data, fileName, in.HD)
	if err != nil {
		var rej *media.RejectError
		switch {
		case errors.As(err, &rej):
			httpx.Validation(w, httpx.FieldErrors{"file": rej.Msg})
		case errors.Is(err, media.ErrUnavailable):
			httpx.Error(w, http.StatusServiceUnavailable, "media_unavailable", "Não foi possível processar o anexo agora. Tente novamente em instantes.")
		default:
			httpx.Internal(w, r, err)
		}
		return SendInput{}, false
	}
	in.Att = &p
	return in, true
}

func (h *Handler) edit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text     string    `json:"text"`
		Mentions []Mention `json:"mentions"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	m, err := h.svc.Edit(r.Context(), viewer(r), chi.URLParam(r, "id"), in.Text, in.Mentions)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), viewer(r), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	room, err := h.svc.MarkRead(r.Context(), viewer(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, room)
}

// attachment entrega o arquivo. Sem URL pública: a sessão é conferida a cada requisição.
func (h *Handler) attachment(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Attachment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	hd := w.Header()
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	ct := a.MimeType
	inline := strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "audio/")
	if !inline {
		ct = "application/octet-stream"
	}
	hd.Set("Content-Type", ct)
	disp := "attachment"
	if inline {
		disp = "inline"
		hd.Set("Cache-Control", "private, max-age=86400")
		hd.Set("Vary", "Cookie")
		if a.SHA256 != "" {
			hd.Set("ETag", `"`+a.SHA256+`"`)
		}
	} else {
		hd.Set("Cache-Control", "private, no-store")
	}
	name := media.CleanFileName(a.FileName)
	if name == "" {
		name = "arquivo"
	}
	hd.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": name}))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(a.Data))
}

// sharedConversation: o que o cartão de uma conversa compartilhada mostra; 404 para quem não pode ver a conversa.
func (h *Handler) sharedConversation(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.SharedConversation(r.Context(), viewer(r), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c) // as respostas JSON já saem com Cache-Control: no-store
}

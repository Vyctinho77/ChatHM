package conversations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

func viewer(r *http.Request) Viewer {
	sess := auth.MustFromContext(r.Context())
	return Viewer{ID: sess.UserID, Name: sess.Name, Admin: sess.IsAdmin()}
}

// Routes: usuários autenticados. A visibilidade por equipe é aplicada em cada consulta.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/conversations", h.list)
	r.Get("/conversations/{id}", h.get)
	r.Get("/conversations/{id}/messages", h.messages)
	r.Post("/conversations/{id}/messages", h.send)
	r.Post("/conversations/{id}/read", h.read)
	r.Get("/attachments/{id}", h.attachment)
	r.Post("/conversations/{id}/close", h.close)
	r.Post("/conversations/{id}/transfer", h.transfer)
	r.Put("/conversations/{id}/tags/{tagId}", h.addTag)
	r.Delete("/conversations/{id}/tags/{tagId}", h.removeTag)
	r.Patch("/contacts/{id}", h.updateContact)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.List(r.Context(), viewer(r))
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"conversations": cs})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), viewer(r), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) messages(w http.ResponseWriter, r *http.Request) {
	ms, err := h.svc.Messages(r.Context(), viewer(r), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": ms})
}

// send: 201 mensagem nova; 200 repetição de um envio já feito (idempotente).
// A mensagem devolvida pode estar com status "failed" se o WhatsApp recusou o envio:
// reenviar com o mesmo id tenta de novo sem duplicar.
//
// JSON: texto ({id, text}) ou localização ({id, kind:"location", lat, lng, name?, address?}).
// multipart/form-data: anexo — campos id, kind (image|audio|document), text (legenda), hd,
// waveform (JSON, 0–1) e o arquivo em "file".
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
		}
		if !httpx.Decode(w, r, &body) {
			return
		}
		if body.Kind != "" && body.Kind != "text" && body.Kind != "location" {
			httpx.Validation(w, httpx.FieldErrors{"kind": "Anexos são enviados como multipart/form-data."})
			return
		}
		in = SendInput{ID: body.ID, Kind: body.Kind, Text: body.Text, Lat: body.Lat, Lng: body.Lng, Name: body.Name, Address: body.Address}
	}
	m, created, err := h.svc.Send(r.Context(), viewer(r), chi.URLParam(r, "id"), in)
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
	if h.svc.media == nil {
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
		hdFlag   bool
	)
	fail := func(err error) (SendInput, bool) {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "attachment_too_large",
				fmt.Sprintf("O arquivo passa do limite de %d MB.", limit>>20))
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
			data, err = io.ReadAll(io.LimitReader(part, limit+1))
			if err != nil {
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
		case "hd":
			hdFlag = string(val) == "true" || string(val) == "1"
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
	p, err := h.svc.media.Process(r.Context(), kind, data, fileName, hdFlag)
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
	in.Att, in.HD = &p, hdFlag
	return in, true
}

// attachment entrega o arquivo de um anexo depois de checar a visibilidade da conversa.
// Sem URL pública nem assinada: a autorização é refeita a cada requisição (cookie de sessão).
func (h *Handler) attachment(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Attachment(r.Context(), viewer(r), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	hd := w.Header()
	hd.Set("Cache-Control", "private, no-store")
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
	}
	hd.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": a.FileName}))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(a.Data))
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.MarkRead(r.Context(), viewer(r), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) close(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Close(r.Context(), viewer(r), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// transfer passa o atendimento para outra pessoa do escritório (corpo: {userId}).
func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID string `json:"userId"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	err := h.svc.Transfer(r.Context(), viewer(r), chi.URLParam(r, "id"), in.UserID)
	var taken *TakenError
	if errors.As(err, &taken) {
		msg := "Só quem está atendendo (ou um administrador) pode transferir esta conversa."
		if taken.Assignee != "" {
			msg = taken.Assignee + " está atendendo; só ela (ou um administrador) pode transferir esta conversa."
		}
		httpx.Error(w, http.StatusConflict, "conversation_taken", msg)
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) addTag(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.AddTag(r.Context(), viewer(r), chi.URLParam(r, "id"), chi.URLParam(r, "tagId")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) removeTag(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RemoveTag(r.Context(), viewer(r), chi.URLParam(r, "id"), chi.URLParam(r, "tagId")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) updateContact(w http.ResponseWriter, r *http.Request) {
	var in ContactInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.UpdateContact(r.Context(), viewer(r), chi.URLParam(r, "id"), in); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	var taken *TakenError
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.As(err, &taken):
		msg := "Outra pessoa já assumiu este atendimento."
		if taken.Assignee != "" {
			msg = taken.Assignee + " já assumiu este atendimento."
		}
		httpx.Error(w, http.StatusConflict, "conversation_taken", msg)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Registro não encontrado.")
	case errors.Is(err, ErrClosed):
		httpx.Error(w, http.StatusConflict, "conversation_closed", "Este atendimento está encerrado. Aguarde uma nova mensagem do cliente.")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden", "Somente o responsável pelo atendimento pode fazer isso.")
	case errors.Is(err, ErrUnavailable):
		httpx.Error(w, http.StatusServiceUnavailable, "whatsapp_unavailable", "O WhatsApp está desconectado. Peça a um administrador para reconectar.")
	case errors.Is(err, ErrIDConflict):
		httpx.Error(w, http.StatusConflict, "id_conflict", "Identificador de mensagem já utilizado.")
	case errors.Is(err, ErrTagNotFound):
		httpx.Error(w, http.StatusNotFound, "tag_not_found", "Etiqueta não encontrada.")
	default:
		httpx.Internal(w, r, err)
	}
}

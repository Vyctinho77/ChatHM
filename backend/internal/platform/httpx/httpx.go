// Package httpx reúne utilitários HTTP compartilhados entre os módulos.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"

	"github.com/google/uuid"
)

const maxBodyBytes = 1 << 20

type ErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type errorBody struct {
	Error ErrorDetail `json:"error"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorBody{Error: ErrorDetail{Code: code, Message: message}})
}

// FieldErrors representa erros de validação por campo.
type FieldErrors map[string]string

func (f FieldErrors) Error() string {
	return fmt.Sprintf("validação falhou: %v", map[string]string(f))
}

// Err devolve nil quando não há erros, facilitando `if err := v.Err(); err != nil`.
func (f FieldErrors) Err() error {
	if len(f) == 0 {
		return nil
	}
	return f
}

func Validation(w http.ResponseWriter, fields FieldErrors) {
	JSON(w, http.StatusUnprocessableEntity, errorBody{Error: ErrorDetail{
		Code: "validation_failed", Message: "Dados inválidos.", Fields: fields,
	}})
}

// Internal registra o erro (sem dados do request) e responde 500 genérico.
func Internal(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "erro interno", "path", r.URL.Path, "err", err)
	Error(w, http.StatusInternalServerError, "internal_error", "Erro interno.")
}

// Decode lê um corpo JSON com limite de tamanho, exigindo Content-Type JSON
// (o que também impede envios cross-site via formulário HTML).
// Em caso de erro já escreve a resposta e retorna false.
func Decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		Error(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Envie application/json.")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			Error(w, http.StatusRequestEntityTooLarge, "body_too_large", "Corpo da requisição muito grande.")
			return false
		}
		Error(w, http.StatusBadRequest, "invalid_json", "JSON inválido.")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		Error(w, http.StatusBadRequest, "invalid_json", "JSON inválido.")
		return false
	}
	return true
}

// ClientIP devolve o IP do cliente (RemoteAddr, já ajustado pelo middleware
// RealIP quando cabeçalhos de proxy são confiáveis) ou "" se inválido.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

// ValidUUID informa se s é um UUID válido.
func ValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

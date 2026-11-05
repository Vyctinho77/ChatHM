package wameow

import (
	"fmt"
	"log/slog"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// slogAdapter envia os logs do whatsmeow ao slog. Debug/Info são descartados:
// podem conter identificadores e conteúdo de protocolo (logs sem dados sensíveis).
type slogAdapter struct {
	l *slog.Logger
}

func (a slogAdapter) Warnf(msg string, args ...any)  { a.l.Warn(fmt.Sprintf(msg, args...)) }
func (a slogAdapter) Errorf(msg string, args ...any) { a.l.Error(fmt.Sprintf(msg, args...)) }
func (a slogAdapter) Infof(string, ...any)           {}
func (a slogAdapter) Debugf(string, ...any)          {}
func (a slogAdapter) Sub(module string) waLog.Logger {
	return slogAdapter{l: a.l.With("module", module)}
}

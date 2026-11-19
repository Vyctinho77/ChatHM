package database

import (
	"context"
	"log/slog"
	"time"
)

// Pool é o que o vigia precisa do pool de conexões (o *pgxpool.Pool já cumpre).
type Pool interface {
	Ping(ctx context.Context) error
	Reset()
}

// WatchOptions ajusta o vigia. Zero em qualquer campo = padrão.
type WatchOptions struct {
	Every       time.Duration // intervalo entre verificações (15 s)
	PingTimeout time.Duration // tempo máximo de cada verificação (3 s)
	ResetAfter  int           // falhas seguidas até recriar as conexões (3, ou seja ~45 s)
	GiveUpAfter time.Duration // fora do ar há tanto tempo: pede a reinicialização do app (5 min)
	OnGiveUp    func()        // chamado uma vez ao desistir (o app sai e o Docker o sobe de novo)
	Logger      *slog.Logger
}

func (o *WatchOptions) defaults() {
	if o.Every <= 0 {
		o.Every = 15 * time.Second
	}
	if o.PingTimeout <= 0 {
		o.PingTimeout = 3 * time.Second
	}
	if o.ResetAfter <= 0 {
		o.ResetAfter = 3
	}
	if o.GiveUpAfter <= 0 {
		o.GiveUpAfter = 5 * time.Minute
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
}

// Watch confere a ligação com o banco de tempos em tempos e se recupera sozinho quando ela morre em silêncio
// (já aconteceu: o banco saudável, mas o app com todas as chamadas esperando até estourar o prazo).
//
//   - Algumas falhas seguidas: descarta todas as conexões do pool (Reset) para que novas sejam abertas.
//   - Fora do ar por GiveUpAfter mesmo assim: chama OnGiveUp, que reinicia o app.
//
// Uma queda real do banco também cai aqui; reiniciar nesse caso é inofensivo (o app volta sozinho quando o banco voltar).
// Bloqueia até ctx terminar.
func Watch(ctx context.Context, p Pool, o WatchOptions) {
	o.defaults()
	t := time.NewTicker(o.Every)
	defer t.Stop()

	fails := 0
	var downSince time.Time
	gaveUp := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		pctx, cancel := context.WithTimeout(ctx, o.PingTimeout)
		err := p.Ping(pctx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			if fails > 0 {
				o.Logger.Info("banco de volta", "falhas_seguidas", fails, "fora_do_ar", time.Since(downSince).Round(time.Second).String())
			}
			fails, gaveUp, downSince = 0, false, time.Time{}
		} else {
			if fails == 0 {
				downSince = time.Now()
			}
			fails++
			o.Logger.Warn("banco não respondeu", "falhas_seguidas", fails, "err", err)
			if fails%o.ResetAfter == 0 {
				o.Logger.Warn("recriando as conexões com o banco", "falhas_seguidas", fails)
				p.Reset()
			}
			if !gaveUp && time.Since(downSince) >= o.GiveUpAfter && o.OnGiveUp != nil {
				gaveUp = true
				o.Logger.Error("banco fora do ar por tempo demais; reiniciando o app", "fora_do_ar", time.Since(downSince).Round(time.Second).String())
				o.OnGiveUp()
			}
		}
	}
}

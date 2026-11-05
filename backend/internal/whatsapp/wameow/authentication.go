package wameow

import (
	"context"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
	"go.mau.fi/whatsmeow"
	"time"
)

func waitForAuthentication(ctx context.Context, authenticated, current func() bool, notify func()) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !current() {
			return
		}
		if authenticated() {
			notify()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Provider) observeAuthentication(cli *whatsmeow.Client) {
	h, ok := p.h.(interface{ OnAuthenticated(whatsapp.Account) })
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		waitForAuthentication(ctx, cli.IsLoggedIn, func() bool { return p.isCurrent(cli) }, func() {
			if cli.Store.ID == nil {
				return
			}
			h.OnAuthenticated(whatsapp.Account{Phone: "+" + cli.Store.ID.User, Name: cli.Store.PushName, DeviceJID: cli.Store.ID.String()})
		})
	}()
}

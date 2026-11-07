package wameow

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func presenceContact(ctx context.Context, cli *whatsmeow.Client, source types.MessageSource) types.JID {
	jid := source.Chat.ToNonAD()
	if jid.Server == types.HiddenUserServer {
		if source.SenderAlt.Server == types.DefaultUserServer {
			return source.SenderAlt.ToNonAD()
		}
		if cli != nil && cli.Store != nil && cli.Store.LIDs != nil {
			if pn, err := cli.Store.LIDs.GetPNForLID(ctx, jid); err == nil && !pn.IsEmpty() {
				return pn.ToNonAD()
			}
		}
	}
	return jid
}

func typingSender(source types.MessageSource) string {
	if !source.IsGroup && source.Chat.Server != types.GroupServer {
		return "contact"
	}
	if source.SenderAlt.Server == types.DefaultUserServer {
		return source.SenderAlt.ToNonAD().String()
	}
	return source.Sender.ToNonAD().String()
}

func convertTyping(ctx context.Context, cli *whatsmeow.Client, e *events.ChatPresence) (whatsapp.Typing, bool) {
	if e == nil || e.IsFromMe || e.Chat.IsEmpty() {
		return whatsapp.Typing{}, false
	}
	if e.Chat.Server != types.DefaultUserServer && e.Chat.Server != types.HiddenUserServer && e.Chat.Server != types.GroupServer {
		return whatsapp.Typing{}, false
	}
	if e.State != types.ChatPresenceComposing && e.State != types.ChatPresencePaused {
		return whatsapp.Typing{}, false
	}
	contact := presenceContact(ctx, cli, e.MessageSource)
	waid := contact.String()
	if contact.Server == types.DefaultUserServer {
		waid = contact.User
	}
	return whatsapp.Typing{ChatJID: e.Chat.ToNonAD().String(), ContactWAID: waid, SenderID: typingSender(e.MessageSource), Active: e.State == types.ChatPresenceComposing && e.Media == types.ChatPresenceMediaText}, true
}

// WhatsApp requires online presence before it sends chat typing notifications.
func (p *Provider) enableTyping(cli *whatsmeow.Client) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := cli.SendPresence(ctx, types.PresenceAvailable); err != nil {
			p.log.Debugf("Não foi possível habilitar indicadores de digitação: %v", err)
		}
	}()
}

package wameow

import (
	"context"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func contactName(c types.ContactInfo) string {
	for _, s := range []string{c.FullName, c.FirstName, c.BusinessName, c.PushName} {
		if s != "" {
			return s
		}
	}
	return ""
}

// App-state contacts are independent of HistorySync and must also be copied
// when restoring an existing session, without requiring another QR pairing.
func (p *Provider) scheduleContactSync(cli *whatsmeow.Client) {
	p.mu.Lock()
	if p.cli != cli || p.contactsSyncing {
		p.mu.Unlock()
		return
	}
	p.contactsSyncing = true
	p.mu.Unlock()
	go func() {
		defer func() { p.mu.Lock(); p.contactsSyncing = false; p.mu.Unlock() }()
		// Coalesce the burst of contact events emitted during initial app-state sync.
		time.Sleep(time.Second)
		p.syncSavedContacts(cli)
	}()
}

func (p *Provider) syncSavedContacts(cli *whatsmeow.Client) {
	h, ok := p.h.(whatsapp.HistoryHandler)
	if !ok || cli.Store.Contacts == nil {
		return
	}
	p.historyMu.Lock()
	defer p.historyMu.Unlock()
	if !p.isCurrent(cli) {
		return
	}
	if p.historyClient != cli {
		p.historyClient = cli
		p.historyProgress = whatsapp.HistoryProgress{}
	}
	progress := &p.historyProgress
	progress.State = "syncing"
	h.OnHistoryProgress(*progress)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	contacts, err := cli.Store.Contacts.GetAllContacts(ctx)
	cancel()
	if err != nil {
		progress.Failed++
		progress.State = "partial"
		h.OnHistoryProgress(*progress)
		return
	}
	for jid, info := range contacts {
		if !p.isCurrent(cli) {
			return
		}
		if jid.Server == types.HiddenUserServer && cli.Store.LIDs != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			pn, err := cli.Store.LIDs.GetPNForLID(ctx, jid)
			cancel()
			if err == nil && !pn.IsEmpty() {
				jid = pn
			}
		}
		if c, ok := historyContact(jid, contactName(info)); ok {
			if err := h.OnHistoryContact(c); err != nil {
				progress.Failed++
			} else {
				progress.Contacts++
			}
			if progress.Contacts%20 == 0 {
				h.OnHistoryProgress(*progress)
			}
		}
	}
	progress.State = "ready"
	if progress.Failed > 0 {
		progress.State = "partial"
	}
	h.OnHistoryProgress(*progress)
}

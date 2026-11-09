package wameow

import (
	"context"
	"sort"
	"strings"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func (p *Provider) isCurrent(cli *whatsmeow.Client) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cli == cli
}

func historyContact(jid types.JID, name string) (whatsapp.HistoryContact, bool) {
	jid = jid.ToNonAD()
	c := whatsapp.HistoryContact{Name: name}
	switch jid.Server {
	case types.DefaultUserServer:
		c.WAID = jid.User
		c.Phone = "+" + jid.User
	case types.HiddenUserServer:
		c.WAID = jid.String()
	default:
		return c, false
	}
	return c, c.WAID != ""
}

// Normalize historical WebMessages through the same parser as live messages,
// preserving IDs for deduplication and sorting each chunk chronologically.
func historyMessages(cli *whatsmeow.Client, conv *waHistorySync.Conversation, names map[string]string) []whatsapp.InboundMessage {
	chat, err := types.ParseJID(conv.GetID())
	if err != nil || (chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer && chat.Server != types.GroupServer) {
		return nil
	}
	name := conv.GetDisplayName()
	if name == "" {
		name = conv.GetName()
	}
	if name == "" {
		name = names[chat.String()]
	}
	var result []whatsapp.InboundMessage
	for _, item := range conv.GetMessages() {
		web := item.GetMessage()
		if web == nil || web.GetMessage() == nil || web.GetKey().GetID() == "" || web.GetMessageTimestamp() == 0 {
			continue
		}
		event, err := cli.ParseWebMessage(chat, web)
		if err != nil {
			continue
		}
		if pn, err := types.ParseJID(conv.GetPnJID()); err == nil && pn.Server == types.DefaultUserServer {
			event.Info.SenderAlt = pn
			event.Info.RecipientAlt = pn
		}
		msg, ok := convertMessage(context.Background(), cli, event)
		if !ok {
			continue
		}
		msg.Historical = true
		msg.Payload["historical"] = true
		// Mídia antiga é importada só como metadados, para não pesar. A exceção são os PDFs (guias, extratos, notas), que dão contexto
		// ao atendimento: ficam com o download, feito em segundo plano. O WhatsApp pode já ter apagado o arquivo; nesse caso falha em silêncio.
		if !historicalPDF(msg) {
			msg.Download = nil
		}
		if msg.IsGroup {
			msg.GroupName = name
		} else if msg.PushName == "" {
			msg.PushName = name
		}
		result = append(result, msg)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Timestamp.Before(result[j].Timestamp) })
	return result
}

func (p *Provider) importHistory(cli *whatsmeow.Client, e *events.HistorySync) {
	h, ok := p.h.(whatsapp.HistoryHandler)
	if !ok || e.Data == nil {
		return
	}
	// Keep batch imports serialized without blocking live connection events.
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
	names := map[string]string{}
	for _, pn := range e.Data.GetPushnames() {
		if !p.isCurrent(cli) {
			return
		}
		names[pn.GetID()] = pn.GetPushname()
		jid, err := types.ParseJID(pn.GetID())
		if err != nil {
			continue
		}
		if c, ok := historyContact(jid, pn.GetPushname()); ok {
			if err := h.OnHistoryContact(c); err != nil {
				progress.Failed++
			} else {
				progress.Contacts++
			}
		}
	}
	for _, conv := range e.Data.GetConversations() {
		if !p.isCurrent(cli) {
			return
		}
		jid, err := types.ParseJID(conv.GetID())
		if err != nil {
			continue
		}
		if pn, err := types.ParseJID(conv.GetPnJID()); err == nil && pn.Server == types.DefaultUserServer {
			jid = pn
		}
		name := conv.GetDisplayName()
		if name == "" {
			name = conv.GetName()
		}
		if name == "" {
			name = names[conv.GetID()]
		}
		if c, ok := historyContact(jid, name); ok {
			if err := h.OnHistoryContact(c); err != nil {
				progress.Failed++
			} else {
				progress.Contacts++
			}
		}
		for _, msg := range historyMessages(cli, conv, names) {
			if !p.isCurrent(cli) {
				return
			}
			created, err := h.OnHistoryMessage(msg)
			if err != nil {
				progress.Failed++
			} else if created {
				progress.Messages++
			}
			if (progress.Messages+progress.Failed)%20 == 0 {
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

// maxHistoricalPDF: PDFs maiores que isto ficam só como metadados.
const maxHistoricalPDF = 15 << 20

func historicalPDF(m whatsapp.InboundMessage) bool {
	if m.Kind != whatsapp.KindDocument || m.Download == nil {
		return false
	}
	mime, _ := m.Payload["mimeType"].(string)
	name, _ := m.Payload["fileName"].(string)
	size, _ := m.Payload["sizeBytes"].(uint64)
	pdf := strings.EqualFold(strings.TrimSpace(mime), "application/pdf") || strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), ".pdf")
	return pdf && size > 0 && size <= maxHistoricalPDF
}

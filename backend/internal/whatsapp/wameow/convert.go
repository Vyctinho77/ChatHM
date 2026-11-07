package wameow

import (
	"context"
	"encoding/base64"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// convertMessage normaliza uma mensagem do whatsmeow. Retorna ok=false para o que
// o MVP ignora: grupos, status, canais, reações, edições e mensagens de protocolo.
func convertMessage(ctx context.Context, cli *whatsmeow.Client, e *events.Message) (whatsapp.InboundMessage, bool) {
	info := e.Info
	chat := info.Chat
	if chat.Server == types.BroadcastServer || chat.Server == types.NewsletterServer {
		return whatsapp.InboundMessage{}, false
	}
	group := info.IsGroup || chat.Server == types.GroupServer

	out := whatsapp.InboundMessage{
		ExternalID: string(info.ID),
		ChatJID:    chat.ToNonAD().String(),
		FromMe:     info.IsFromMe,
		Timestamp:  info.Timestamp,
		Payload:    map[string]any{},
		SenderID:   typingSender(info.MessageSource),
	}
	if group {
		out.IsGroup = true
		out.ContactWAID = chat.ToNonAD().String()
		if !info.IsFromMe {
			out.SenderJID = info.Sender.ToNonAD().String()
			out.SenderName = info.PushName
			if out.SenderName == "" && info.Sender.Server == types.DefaultUserServer {
				out.SenderName = "+" + info.Sender.User
			}
		}
	} else if !info.IsFromMe {
		out.PushName = info.PushName
	}

	// Contato: preferir o número de telefone; conversas novas podem vir por LID.
	pn := chat.ToNonAD()
	if !group && pn.Server == types.HiddenUserServer {
		alt := info.SenderAlt
		if info.IsFromMe {
			alt = info.RecipientAlt
		}
		if alt.Server == types.DefaultUserServer {
			pn = alt.ToNonAD()
		} else if cli != nil && cli.Store.LIDs != nil {
			if found, err := cli.Store.LIDs.GetPNForLID(ctx, chat.ToNonAD()); err == nil && !found.IsEmpty() {
				pn = found
			}
		}
	}
	if group {
		// já definido acima
	} else if pn.Server == types.DefaultUserServer {
		out.ContactWAID = pn.User
		out.ContactPhone = "+" + pn.User
	} else {
		out.ContactWAID = pn.String()
	}

	m := e.Message
	switch {
	case e.IsEdit:
		// Edição feita no celular (ou por quem escreve): altera a mensagem original.
		pm := m.GetProtocolMessage()
		if pm == nil || pm.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT || pm.GetKey().GetID() == "" {
			return whatsapp.InboundMessage{}, false
		}
		ed := pm.GetEditedMessage()
		text := ed.GetConversation()
		if text == "" {
			text = ed.GetExtendedTextMessage().GetText()
		}
		if text == "" {
			text = ed.GetImageMessage().GetCaption()
		}
		if text == "" {
			text = ed.GetDocumentMessage().GetCaption()
		}
		if text == "" {
			return whatsapp.InboundMessage{}, false
		}
		out.Kind, out.Body, out.EditOf = whatsapp.KindText, text, pm.GetKey().GetID()
		return out, true
	case m.GetProtocolMessage().GetType() == waE2E.ProtocolMessage_REVOKE && m.GetProtocolMessage().GetKey().GetID() != "":
		// "Apagar para todos" feito no WhatsApp.
		out.Kind, out.RevokeOf = whatsapp.KindText, m.GetProtocolMessage().GetKey().GetID()
		return out, true
	case m.GetConversation() != "":
		out.Kind, out.Body = whatsapp.KindText, m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		out.Kind, out.Body = whatsapp.KindText, m.GetExtendedTextMessage().GetText()
	case m.GetImageMessage() != nil:
		img := m.GetImageMessage()
		out.Kind, out.Body = whatsapp.KindImage, img.GetCaption()
		out.Payload["mimeType"] = img.GetMimetype()
		out.Payload["sizeBytes"] = img.GetFileLength()
		out.Payload["width"] = img.GetWidth()
		out.Payload["height"] = img.GetHeight()
		out.Download = func(ctx context.Context) ([]byte, error) { return cli.Download(ctx, img) }
	case m.GetAudioMessage() != nil:
		a := m.GetAudioMessage()
		out.Kind = whatsapp.KindAudio
		out.Payload["durationSec"] = a.GetSeconds()
		out.Payload["mimeType"] = a.GetMimetype()
		out.Payload["sizeBytes"] = a.GetFileLength()
		out.Payload["voiceNote"] = a.GetPTT()
		out.Download = func(ctx context.Context) ([]byte, error) { return cli.Download(ctx, a) }
		if wf := a.GetWaveform(); len(wf) > 0 {
			bars := make([]float64, len(wf))
			for i, v := range wf {
				bars[i] = 0.2 + float64(v)/100*0.8
			}
			out.Payload["waveform"] = bars
		}
	case m.GetDocumentMessage() != nil:
		d := m.GetDocumentMessage()
		out.Kind, out.Body = whatsapp.KindDocument, d.GetCaption()
		name := d.GetFileName()
		if name == "" {
			name = d.GetTitle()
		}
		out.Payload["fileName"] = name
		out.Payload["mimeType"] = d.GetMimetype()
		out.Payload["sizeBytes"] = d.GetFileLength()
		out.Download = func(ctx context.Context) ([]byte, error) { return cli.Download(ctx, d) }
	case m.GetLocationMessage() != nil:
		l := m.GetLocationMessage()
		out.Kind = whatsapp.KindLocation
		out.Payload["lat"] = l.GetDegreesLatitude()
		out.Payload["lng"] = l.GetDegreesLongitude()
		out.Payload["name"] = l.GetName()
		out.Payload["address"] = l.GetAddress()
		// Miniatura do mapa que o próprio WhatsApp gera (JPEG pequeno).
		if t := l.GetJPEGThumbnail(); len(t) > 0 && len(t) <= 64<<10 {
			out.Payload["thumb"] = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(t)
		}
	case m.GetVideoMessage() != nil:
		out.Kind, out.Body = whatsapp.KindUnsupported, m.GetVideoMessage().GetCaption()
		out.Payload["label"] = "Vídeo"
	case m.GetStickerMessage() != nil:
		st := m.GetStickerMessage()
		out.Kind = whatsapp.KindSticker
		out.Payload["mimeType"] = "image/webp"
		out.Payload["fileName"] = "figurinha.webp"
		out.Payload["sizeBytes"] = st.GetFileLength()
		out.Payload["width"], out.Payload["height"] = st.GetWidth(), st.GetHeight()
		out.Payload["animated"] = st.GetIsAnimated()
		out.Download = func(ctx context.Context) ([]byte, error) { return cli.Download(ctx, st) }
	case m.GetContactMessage() != nil || m.GetContactsArrayMessage() != nil:
		out.Kind, out.Payload["label"] = whatsapp.KindUnsupported, "Contato"
	case m.GetLiveLocationMessage() != nil:
		out.Kind, out.Payload["label"] = whatsapp.KindUnsupported, "Localização em tempo real"
	case m.GetPollCreationMessage() != nil || m.GetPollCreationMessageV3() != nil:
		out.Kind, out.Payload["label"] = whatsapp.KindUnsupported, "Enquete"
	default:
		// Reações, outros protocolos, distribuição de chaves etc.
		return whatsapp.InboundMessage{}, false
	}
	out.Quote = quoteOf(cli, m)
	return out, true
}

// contextOf devolve o ContextInfo (citação, encaminhamento…) do conteúdo da mensagem.
func contextOf(m *waE2E.Message) *waE2E.ContextInfo {
	switch {
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetContextInfo()
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetContextInfo()
	case m.GetStickerMessage() != nil:
		return m.GetStickerMessage().GetContextInfo()
	case m.GetAudioMessage() != nil:
		return m.GetAudioMessage().GetContextInfo()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage().GetContextInfo()
	case m.GetLocationMessage() != nil:
		return m.GetLocationMessage().GetContextInfo()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetContextInfo()
	}
	return nil
}

// quoteOf lê a mensagem que esta responde (nil quando não é uma resposta).
func quoteOf(cli *whatsmeow.Client, m *waE2E.Message) *whatsapp.InboundQuote {
	ci := contextOf(m)
	if ci == nil || ci.GetStanzaID() == "" {
		return nil
	}
	q := &whatsapp.InboundQuote{ExternalID: ci.GetStanzaID(), SenderJID: ci.GetParticipant()}
	if cli != nil && cli.Store != nil && cli.Store.ID != nil {
		if p, err := types.ParseJID(ci.GetParticipant()); err == nil && p.User == cli.Store.ID.User {
			q.FromMe = true
		}
	}
	qm := ci.GetQuotedMessage()
	switch {
	case qm.GetConversation() != "":
		q.Text = qm.GetConversation()
	case qm.GetExtendedTextMessage().GetText() != "":
		q.Text = qm.GetExtendedTextMessage().GetText()
	case qm.GetImageMessage() != nil:
		q.Text = qm.GetImageMessage().GetCaption()
	case qm.GetStickerMessage() != nil:
		q.Text = "Figurinha"
	case qm.GetDocumentMessage() != nil:
		q.Text = qm.GetDocumentMessage().GetFileName()
	}
	return q
}

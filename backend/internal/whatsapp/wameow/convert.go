package wameow

import (
	"context"
	"encoding/base64"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// convertMessage normaliza uma mensagem do whatsmeow. Retorna ok=false para o que
// o MVP ignora: grupos, status, canais, reações, edições e mensagens de protocolo.
func convertMessage(ctx context.Context, cli *whatsmeow.Client, e *events.Message) (whatsapp.InboundMessage, bool) {
	info := e.Info
	chat := info.Chat
	if chat.Server == types.BroadcastServer || chat.Server == types.NewsletterServer || e.IsEdit {
		return whatsapp.InboundMessage{}, false
	}
	group := info.IsGroup || chat.Server == types.GroupServer

	out := whatsapp.InboundMessage{
		ExternalID: string(info.ID),
		ChatJID:    chat.ToNonAD().String(),
		FromMe:     info.IsFromMe,
		Timestamp:  info.Timestamp,
		Payload:    map[string]any{},
	}
	if group {
		out.IsGroup = true
		out.ContactWAID = chat.ToNonAD().String()
		if !info.IsFromMe {
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
		out.Kind, out.Payload["label"] = whatsapp.KindUnsupported, "Figurinha"
	case m.GetContactMessage() != nil || m.GetContactsArrayMessage() != nil:
		out.Kind, out.Payload["label"] = whatsapp.KindUnsupported, "Contato"
	case m.GetLiveLocationMessage() != nil:
		out.Kind, out.Payload["label"] = whatsapp.KindUnsupported, "Localização em tempo real"
	case m.GetPollCreationMessage() != nil || m.GetPollCreationMessageV3() != nil:
		out.Kind, out.Payload["label"] = whatsapp.KindUnsupported, "Enquete"
	default:
		// Reações, protocolo (apagar/editar), distribuição de chaves etc.
		return whatsapp.InboundMessage{}, false
	}
	return out, true
}

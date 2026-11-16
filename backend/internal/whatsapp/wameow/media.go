package wameow

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// mediaRef guarda a mensagem de mídia (endereço, chaves e hashes) sem a miniatura, que só pesaria.
func mediaRef(kind whatsapp.Kind, m *waE2E.Message) *whatsapp.MediaRef {
	c := proto.Clone(m).(*waE2E.Message)
	switch {
	case c.GetImageMessage() != nil:
		c.ImageMessage.JPEGThumbnail = nil
	case c.GetDocumentMessage() != nil:
		c.DocumentMessage.JPEGThumbnail = nil
	case c.GetStickerMessage() != nil:
		c.StickerMessage.PngThumbnail = nil
	}
	b, err := proto.Marshal(c)
	if err != nil {
		return nil
	}
	return &whatsapp.MediaRef{Kind: kind, Proto: b}
}

// downloadable devolve a mensagem de mídia guardada, a chave de mídia e como alterar o endereço (depois de um reenvio).
func downloadable(ref whatsapp.MediaRef) (dm whatsmeow.DownloadableMessage, key []byte, setPath func(string), err error) {
	var m waE2E.Message
	if err := proto.Unmarshal(ref.Proto, &m); err != nil {
		return nil, nil, nil, fmt.Errorf("referência de mídia inválida: %w", err)
	}
	switch {
	case m.GetImageMessage() != nil:
		x := m.ImageMessage
		return x, x.GetMediaKey(), func(p string) { x.DirectPath = proto.String(p) }, nil
	case m.GetAudioMessage() != nil:
		x := m.AudioMessage
		return x, x.GetMediaKey(), func(p string) { x.DirectPath = proto.String(p) }, nil
	case m.GetDocumentMessage() != nil:
		x := m.DocumentMessage
		return x, x.GetMediaKey(), func(p string) { x.DirectPath = proto.String(p) }, nil
	case m.GetStickerMessage() != nil:
		x := m.StickerMessage
		return x, x.GetMediaKey(), func(p string) { x.DirectPath = proto.String(p) }, nil
	}
	return nil, nil, nil, errors.New("referência de mídia sem arquivo")
}

// gone: o servidor do WhatsApp já não tem o arquivo (404/410/403, ou a referência não traz endereço).
func gone(err error) bool {
	return errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith404) || errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith410) ||
		errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith403) || errors.Is(err, whatsmeow.ErrNoURLPresent)
}

// FetchMedia baixa a mídia a partir da referência guardada.
func (p *Provider) FetchMedia(ctx context.Context, ref whatsapp.MediaRef) ([]byte, error) {
	cli, err := p.client()
	if err != nil {
		return nil, err
	}
	dm, _, _, err := downloadable(ref)
	if err != nil {
		return nil, err
	}
	data, err := cli.Download(ctx, dm)
	if gone(err) {
		return nil, fmt.Errorf("%w: %v", whatsapp.ErrMediaGone, err)
	}
	return data, err
}

// RequestMediaRetry pede ao celular que envie o arquivo de novo. A resposta chega como events.MediaRetry.
func (p *Provider) RequestMediaRetry(ctx context.Context, ref whatsapp.MediaRef, msg whatsapp.MediaMessage) error {
	cli, err := p.client()
	if err != nil {
		return err
	}
	_, key, _, err := downloadable(ref)
	if err != nil {
		return err
	}
	chat, err := types.ParseJID(msg.ChatJID)
	if err != nil {
		return err
	}
	info := &types.MessageInfo{ID: msg.ExternalID, MessageSource: types.MessageSource{Chat: chat, IsFromMe: msg.FromMe, IsGroup: msg.IsGroup}}
	if msg.SenderJID != "" {
		if s, err := types.ParseJID(msg.SenderJID); err == nil {
			info.Sender = s
		}
	}
	return cli.SendMediaRetryReceipt(ctx, info, key)
}

// SetMediaRetryHandler define quem recebe as respostas do celular aos pedidos de reenvio.
func (p *Provider) SetMediaRetryHandler(h whatsapp.MediaRetryHandler) {
	p.mu.Lock()
	p.onMediaRetry = h
	p.mu.Unlock()
}

// mediaRetry avisa a resposta do celular; Complete decifra o aviso (com a chave da referência guardada), troca o endereço e baixa.
func (p *Provider) mediaRetry(cli *whatsmeow.Client, e *events.MediaRetry) {
	p.mu.Lock()
	h := p.onMediaRetry
	p.mu.Unlock()
	if h == nil {
		return
	}
	h(whatsapp.MediaRetryEvent{
		MessageID: string(e.MessageID),
		ChatJID:   e.ChatID.String(),
		Complete: func(ctx context.Context, ref whatsapp.MediaRef) ([]byte, error) {
			dm, key, setPath, err := downloadable(ref)
			if err != nil {
				return nil, err
			}
			notif, err := whatsmeow.DecryptMediaRetryNotification(e, key)
			switch {
			case errors.Is(err, whatsmeow.ErrMediaNotAvailableOnPhone):
				return nil, whatsapp.ErrMediaNotOnPhone
			case err != nil:
				return nil, err
			case notif.GetResult() != waMmsRetry.MediaRetryNotification_SUCCESS:
				return nil, fmt.Errorf("%w: o celular respondeu %s", whatsapp.ErrMediaGone, notif.GetResult())
			}
			setPath(notif.GetDirectPath())
			data, err := cli.Download(ctx, dm)
			if gone(err) {
				return nil, fmt.Errorf("%w: %v", whatsapp.ErrMediaGone, err)
			}
			return data, err
		},
	})
}

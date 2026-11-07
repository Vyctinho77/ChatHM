package wameow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const maxAvatarBytes = 512 << 10

var avatarClient = &http.Client{Timeout: 15 * time.Second}

// avatarHostOK só aceita o CDN do WhatsApp/Meta (a URL vem do servidor do WhatsApp, mas não custa validar).
func avatarHostOK(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, suf := range []string{".whatsapp.net", ".whatsapp.com", ".fbcdn.net", ".fbsbx.com"} {
		if strings.HasSuffix(h, suf) {
			return true
		}
	}
	return false
}

// ProfilePicture baixa a miniatura da foto de perfil de um contato ou grupo.
func (p *Provider) ProfilePicture(ctx context.Context, jid string) ([]byte, error) {
	cli, err := p.client()
	if err != nil {
		return nil, err
	}
	to, err := types.ParseJID(jid)
	if err != nil {
		return nil, err
	}
	info, err := cli.GetProfilePictureInfo(ctx, to.ToNonAD(), &whatsmeow.GetProfilePictureParams{Preview: true})
	if errors.Is(err, whatsmeow.ErrProfilePictureNotSet) || errors.Is(err, whatsmeow.ErrProfilePictureUnauthorized) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info == nil || !avatarHostOK(info.URL) {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := avatarClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("foto de perfil: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxAvatarBytes || http.DetectContentType(b) != "image/jpeg" {
		return nil, nil
	}
	return b, nil
}

// groupName devolve o assunto do grupo (com cache curto, para não consultar a cada mensagem).
func (p *Provider) groupName(cli *whatsmeow.Client, jid types.JID) string {
	p.mu.Lock()
	if e, ok := p.groupNames[jid]; ok && time.Since(e.at) < 30*time.Minute {
		p.mu.Unlock()
		return e.name
	}
	p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := cli.GetGroupInfo(ctx, jid)
	if err != nil || info == nil {
		return "" // tenta de novo na próxima mensagem
	}
	p.mu.Lock()
	if p.groupNames == nil {
		p.groupNames = map[types.JID]groupNameEntry{}
	}
	p.groupNames[jid] = groupNameEntry{name: info.Name, at: time.Now()}
	p.mu.Unlock()
	return info.Name
}

type groupNameEntry struct {
	name string
	at   time.Time
}

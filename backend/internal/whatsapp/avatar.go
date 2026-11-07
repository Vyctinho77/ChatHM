package whatsapp

import (
	"bytes"
	"context"
	"time"
)

const (
	avatarRecheckWithPhoto = 7 * 24 * time.Hour
	avatarRecheckNoPhoto   = 24 * time.Hour
)

// refreshAvatar atualiza a foto de perfil do contato/grupo quando ainda não há ou está velha.
// Roda em segundo plano, no máximo 2 consultas simultâneas ao WhatsApp; falhas são ignoradas
// (nova tentativa na próxima mensagem).
func (m *Manager) refreshAvatar(waID, chatJID string) {
	if m.db == nil || waID == "" || chatJID == "" || !m.Connected() {
		return
	}
	m.avatarMu.Lock()
	if m.avatarBusy == nil {
		m.avatarBusy = map[string]bool{}
		m.avatarSem = make(chan struct{}, 2)
	}
	if m.avatarBusy[waID] {
		m.avatarMu.Unlock()
		return
	}
	m.avatarBusy[waID] = true
	sem := m.avatarSem
	m.avatarMu.Unlock()
	defer func() { m.avatarMu.Lock(); delete(m.avatarBusy, waID); m.avatarMu.Unlock() }()

	sem <- struct{}{}
	defer func() { <-sem }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var id string
	var has bool
	var checked *time.Time
	if err := m.db.QueryRow(ctx,
		`SELECT id::text, avatar IS NOT NULL, avatar_checked_at FROM contacts WHERE wa_id = $1`, waID).
		Scan(&id, &has, &checked); err != nil {
		return
	}
	wait := avatarRecheckNoPhoto
	if has {
		wait = avatarRecheckWithPhoto
	}
	if checked != nil && time.Since(*checked) < wait {
		return
	}
	pic, err := m.p.ProfilePicture(ctx, chatJID)
	if err != nil {
		m.log.Debug("foto de perfil indisponível", "err", err)
		return
	}
	if pic == nil {
		// Sem foto (ou privada): guarda só o horário da checagem; remove foto antiga se houver.
		_, _ = m.db.Exec(ctx, `UPDATE contacts SET avatar = NULL, avatar_version = 0, avatar_checked_at = now()
			WHERE id = $1`, id)
		return
	}
	var old []byte
	_ = m.db.QueryRow(ctx, `SELECT avatar FROM contacts WHERE id = $1`, id).Scan(&old)
	if bytes.Equal(old, pic) {
		_, _ = m.db.Exec(ctx, `UPDATE contacts SET avatar_checked_at = now() WHERE id = $1`, id)
		return
	}
	if _, err := m.db.Exec(ctx, `UPDATE contacts SET avatar = $2, avatar_version = avatar_version + 1,
		avatar_checked_at = now() WHERE id = $1`, id, pic); err != nil {
		m.log.Error("salvando foto de perfil", "err", err)
		return
	}
	if m.pub != nil {
		m.pub.Publish("contact.avatar", map[string]any{"contactId": id}, nil)
	}
}

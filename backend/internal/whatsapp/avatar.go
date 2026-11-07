package whatsapp

import (
	"bytes"
	"context"
	"strings"
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
	pic, err := m.p.ProfilePicture(ctx, chatJID, true)
	if err != nil {
		m.log.Debug("foto de perfil indisponível", "err", err)
		return
	}
	if pic == nil {
		// Sem foto (ou privada): guarda só o horário da checagem; remove foto antiga se houver.
		_, _ = m.db.Exec(ctx, `UPDATE contacts SET avatar = NULL, avatar_version = avatar_version + 1, avatar_checked_at = now()
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

// jidOfWAID converte o identificador do contato no JID de conversa.
func jidOfWAID(waID string) string {
	if strings.Contains(waID, "@") {
		return waID
	}
	return waID + "@s.whatsapp.net"
}

// ProfilePicture repassa ao provedor (usado pelo módulo de conversas para a foto grande).
func (m *Manager) ProfilePicture(ctx context.Context, jid string, preview bool) ([]byte, error) {
	if !m.Connected() {
		return nil, ErrNotConnected
	}
	return m.p.ProfilePicture(ctx, jid, preview)
}

const avatarSweepEvery = 6 * time.Hour

// startAvatarSweep busca em segundo plano a foto de TODOS os contatos e grupos (e as renova de tempos em tempos),
// devagar (uma consulta a cada ~2 s) para não pressionar o número. Uma só varredura roda por vez.
func (m *Manager) startAvatarSweep() {
	if m.db == nil || !m.sweeping.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer m.sweeping.Store(false)
		for m.Connected() {
			n := m.sweepAvatarsOnce()
			if n > 0 {
				continue // ainda há pendentes
			}
			select {
			case <-time.After(avatarSweepEvery):
			case <-m.sweepStop():
				return
			}
		}
	}()
}

func (m *Manager) sweepStop() <-chan struct{} { return m.shutdownCh() }

// sweepAvatarsOnce processa um lote e devolve quantos contatos tentou.
func (m *Manager) sweepAvatarsOnce() int {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	rows, err := m.db.Query(ctx, `
		SELECT wa_id FROM contacts
		 WHERE avatar_checked_at IS NULL
		    OR avatar_checked_at < now() - (CASE WHEN avatar IS NULL THEN interval '1 day' ELSE interval '7 days' END)
		 ORDER BY avatar_checked_at NULLS FIRST LIMIT 40`)
	if err != nil {
		cancel()
		return 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	cancel()
	for _, id := range ids {
		if !m.Connected() {
			return 0
		}
		m.refreshAvatar(id, jidOfWAID(id))
		select {
		case <-time.After(2 * time.Second):
		case <-m.sweepStop():
			return 0
		}
	}
	return len(ids)
}

func (m *Manager) shutdownCh() <-chan struct{} { return m.stopCh }

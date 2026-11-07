package conversations

import (
	"container/list"
	"context"
	"strings"
	"sync"
	"time"
)

const (
	fullAvatarCacheMax = 40
	fullAvatarTTL      = 6 * time.Hour
)

type fullAvatarEntry struct {
	key  string
	data []byte
	at   time.Time
}

var fullAvatars = struct {
	mu    sync.Mutex
	ll    *list.List
	items map[string]*list.Element
}{ll: list.New(), items: map[string]*list.Element{}}

// fullAvatar devolve a foto de perfil em tamanho grande (nil se indisponível), com cache curto em memória.
func (s *Service) fullAvatar(ctx context.Context, contactID, waID string) []byte {
	fullAvatars.mu.Lock()
	if e, ok := fullAvatars.items[contactID]; ok {
		ent := e.Value.(*fullAvatarEntry)
		if time.Since(ent.at) < fullAvatarTTL {
			fullAvatars.ll.MoveToFront(e)
			fullAvatars.mu.Unlock()
			return ent.data
		}
	}
	fullAvatars.mu.Unlock()
	if s.sender == nil || !s.sender.Connected() {
		return nil
	}
	jid := waID
	if !strings.Contains(jid, "@") {
		jid += "@s.whatsapp.net"
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	b, err := s.sender.ProfilePicture(cctx, jid, false)
	if err != nil || len(b) == 0 {
		return nil
	}
	fullAvatars.mu.Lock()
	defer fullAvatars.mu.Unlock()
	if e, ok := fullAvatars.items[contactID]; ok {
		fullAvatars.ll.Remove(e)
	}
	fullAvatars.items[contactID] = fullAvatars.ll.PushFront(&fullAvatarEntry{contactID, b, time.Now()})
	for fullAvatars.ll.Len() > fullAvatarCacheMax {
		old := fullAvatars.ll.Back()
		fullAvatars.ll.Remove(old)
		delete(fullAvatars.items, old.Value.(*fullAvatarEntry).key)
	}
	return b
}

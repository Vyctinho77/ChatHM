package wameow

import (
	"context"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"testing"
)

type cachedContactStore struct{ store.ContactStore }

func (c cachedContactStore) GetAllContacts(context.Context) (map[types.JID]types.ContactInfo, error) {
	return map[types.JID]types.ContactInfo{
		types.NewJID("5592999999999", types.DefaultUserServer): {FullName: "Nome salvo", PushName: "Nome público"},
		types.NewJID("123", types.GroupServer):                 {FullName: "Grupo"},
	}, nil
}
func TestSavedContactsImportWithoutHistorySync(t *testing.T) {
	h := &historyRecorder{messages: map[string]bool{}}
	cli := whatsmeow.NewClient(&store.Device{Contacts: cachedContactStore{}}, nil)
	p := &Provider{h: h, cli: cli}
	p.syncSavedContacts(cli)
	if len(h.contacts) != 1 || h.contacts[0].Name != "Nome salvo" || h.contacts[0].WAID != "5592999999999" {
		t.Fatalf("cached contacts were not imported: %+v", h.contacts)
	}
	if h.progress.Messages != 0 || h.progress.Contacts != 1 || h.progress.State != "ready" {
		t.Fatalf("contact sync incorrectly reported message history: %+v", h.progress)
	}
}

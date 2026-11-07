package wameow

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestConvertTyping(t *testing.T) {
	phone := types.NewJID("5592999991111", types.DefaultUserServer)
	for _, tc := range []struct {
		name          string
		state         types.ChatPresence
		media         types.ChatPresenceMedia
		active, valid bool
	}{
		{"composing", types.ChatPresenceComposing, types.ChatPresenceMediaText, true, true},
		{"paused", types.ChatPresencePaused, types.ChatPresenceMediaText, false, true},
		{"recording is not typing", types.ChatPresenceComposing, types.ChatPresenceMediaAudio, false, true},
		{"unknown state", types.ChatPresence("unknown"), types.ChatPresenceMediaText, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &events.ChatPresence{MessageSource: types.MessageSource{Chat: phone, Sender: phone}, State: tc.state, Media: tc.media}
			got, ok := convertTyping(context.Background(), nil, e)
			if ok != tc.valid || (ok && (got.Active != tc.active || got.ContactWAID != phone.User || got.SenderID != "contact")) {
				t.Fatalf("typing = %+v %v", got, ok)
			}
		})
	}
	lid := types.NewJID("123456789", types.HiddenUserServer)
	e := &events.ChatPresence{MessageSource: types.MessageSource{Chat: lid, Sender: lid, SenderAlt: phone}, State: types.ChatPresenceComposing}
	got, ok := convertTyping(context.Background(), nil, e)
	if !ok || got.ContactWAID != phone.User || got.ChatJID != lid.String() {
		t.Fatalf("LID not resolved: %+v", got)
	}
	e.Chat, e.IsGroup = types.NewJID("120363000000001", types.GroupServer), true
	got, ok = convertTyping(context.Background(), nil, e)
	if !ok || got.ContactWAID != e.Chat.String() || got.SenderID != phone.String() {
		t.Fatalf("group sender = %+v", got)
	}
	e.IsFromMe = true
	if _, ok := convertTyping(context.Background(), nil, e); ok {
		t.Fatal("own device triggered customer typing")
	}
	e.IsFromMe = false
	e.Chat = types.NewJID("status", types.BroadcastServer)
	if _, ok := convertTyping(context.Background(), nil, e); ok {
		t.Fatal("broadcast typing accepted")
	}
	if _, ok := convertTyping(context.Background(), nil, nil); ok {
		t.Fatal("nil event accepted")
	}
}

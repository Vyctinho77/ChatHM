package whatsapp

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/platform/testdb"
)

type picProvider struct {
	Provider
	pic   []byte
	calls int
}

func (p *picProvider) ProfilePicture(_ context.Context, _ string, preview bool) ([]byte, error) {
	p.calls++
	if !preview {
		return append([]byte("FULL"), p.pic...), nil
	}
	return p.pic, nil
}

func TestRefreshAvatarStoresAndRechecks(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	p := &picProvider{pic: []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 3}}
	m := NewManager(pool, nil, nil, func(Handler) Provider { return p }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.status = Status{State: StateConnected}
	if _, err := pool.Exec(ctx, `INSERT INTO contacts (wa_id, phone, name) VALUES ('5592911112222', '+5592911112222', 'Foto Teste')`); err != nil {
		t.Fatal(err)
	}

	m.refreshAvatar("5592911112222", "5592911112222@s.whatsapp.net")
	var ver int64
	var has bool
	if err := pool.QueryRow(ctx, `SELECT avatar_version, avatar IS NOT NULL FROM contacts WHERE wa_id = '5592911112222'`).Scan(&ver, &has); err != nil {
		t.Fatal(err)
	}
	if !has || ver != 1 || p.calls != 1 {
		t.Fatalf("foto não salva: has=%v ver=%d calls=%d", has, ver, p.calls)
	}

	// Recém-checada: não consulta o WhatsApp de novo.
	m.refreshAvatar("5592911112222", "5592911112222@s.whatsapp.net")
	if p.calls != 1 {
		t.Fatalf("rechecou cedo demais (%d chamadas)", p.calls)
	}

	// Foto igual depois do prazo: só renova a data, sem subir a versão.
	if _, err := pool.Exec(ctx, `UPDATE contacts SET avatar_checked_at = now() - interval '8 days' WHERE wa_id = '5592911112222'`); err != nil {
		t.Fatal(err)
	}
	m.refreshAvatar("5592911112222", "5592911112222@s.whatsapp.net")
	_ = pool.QueryRow(ctx, `SELECT avatar_version FROM contacts WHERE wa_id = '5592911112222'`).Scan(&ver)
	if p.calls != 2 || ver != 1 {
		t.Fatalf("foto igual não deveria mudar a versão: calls=%d ver=%d", p.calls, ver)
	}

	// Foto nova: versão sobe; sem foto: remove.
	p.pic = []byte{0xff, 0xd8, 0xff, 0xe0, 9, 9, 9}
	_, _ = pool.Exec(ctx, `UPDATE contacts SET avatar_checked_at = now() - interval '8 days' WHERE wa_id = '5592911112222'`)
	m.refreshAvatar("5592911112222", "5592911112222@s.whatsapp.net")
	_ = pool.QueryRow(ctx, `SELECT avatar_version FROM contacts WHERE wa_id = '5592911112222'`).Scan(&ver)
	if ver != 2 {
		t.Fatalf("versão = %d, queria 2", ver)
	}
	p.pic = nil
	_, _ = pool.Exec(ctx, `UPDATE contacts SET avatar_checked_at = now() - interval '8 days' WHERE wa_id = '5592911112222'`)
	m.refreshAvatar("5592911112222", "5592911112222@s.whatsapp.net")
	_ = pool.QueryRow(ctx, `SELECT avatar IS NOT NULL, avatar_version FROM contacts WHERE wa_id = '5592911112222'`).Scan(&has, &ver)
	if has || ver != 3 { // a versão nunca volta atrás: uma foto nova não reaproveita a URL antiga em cache
		t.Fatalf("foto removida no WhatsApp deveria sumir: has=%v ver=%d", has, ver)
	}
}

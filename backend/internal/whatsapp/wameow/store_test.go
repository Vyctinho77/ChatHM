package wameow

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"

	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
	"github.com/vyctinho77/chathm/backend/internal/platform/testdb"
)

func newContainer(t *testing.T) (*secureContainer, context.Context) {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secretbox.New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	inner := sqlstore.NewWithDB(stdlib.OpenDBFromPool(pool), "postgres", nil)
	if err := inner.Upgrade(ctx); err != nil {
		t.Fatal(err)
	}
	return &secureContainer{inner: inner, db: pool, box: box}, ctx
}

func pairedDevice(t *testing.T, c *secureContainer, ctx context.Context) *store.Device {
	t.Helper()
	d := c.NewDevice()
	jid := types.NewJID("5511999990000", types.DefaultUserServer)
	jid.Device = 3
	d.ID = &jid
	d.PushName = "Escritório"
	d.Account = &waAdv.ADVSignedDeviceIdentity{
		Details: []byte("details"), AccountSignature: bytes.Repeat([]byte{1}, 64),
		AccountSignatureKey: bytes.Repeat([]byte{2}, 32), DeviceSignature: bytes.Repeat([]byte{3}, 64),
	}
	if err := c.PutDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDeviceKeysAreEncrypted(t *testing.T) {
	c, ctx := newContainer(t)
	d := pairedDevice(t, c, ctx)

	// A linha do whatsmeow não contém as chaves reais.
	var noise, identity, adv []byte
	if err := c.db.QueryRow(ctx,
		`SELECT noise_key, identity_key, adv_key FROM whatsmeow_device WHERE jid = $1`, d.ID.String(),
	).Scan(&noise, &identity, &adv); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(noise, make([]byte, 32)) || !bytes.Equal(identity, make([]byte, 32)) || !bytes.Equal(adv, make([]byte, 32)) {
		t.Fatal("chaves do dispositivo gravadas em claro em whatsmeow_device")
	}

	var enc []byte
	_ = c.db.QueryRow(ctx, `SELECT device_enc FROM whatsapp_connections WHERE id = 1`).Scan(&enc)
	for name, secret := range map[string][]byte{"noise": d.NoiseKey.Priv[:], "identity": d.IdentityKey.Priv[:], "adv": d.AdvSecretKey} {
		if bytes.Contains(enc, secret) {
			t.Fatalf("chave %s aparece em claro em device_enc", name)
		}
	}

	// Recarregar devolve exatamente as mesmas chaves.
	got, err := c.Load(ctx)
	if err != nil || got == nil {
		t.Fatalf("load: %v %v", got, err)
	}
	if got.ID.String() != d.ID.String() || *got.NoiseKey.Priv != *d.NoiseKey.Priv || *got.IdentityKey.Priv != *d.IdentityKey.Priv ||
		*got.SignedPreKey.Priv != *d.SignedPreKey.Priv || *got.NoiseKey.Pub != *d.NoiseKey.Pub || *got.SignedPreKey.Signature != *d.SignedPreKey.Signature ||
		!bytes.Equal(got.AdvSecretKey, d.AdvSecretKey) || got.RegistrationID != d.RegistrationID ||
		!bytes.Equal(got.Account.DeviceSignature, d.Account.DeviceSignature) || got.PushName != "Escritório" {
		t.Fatal("dispositivo recarregado difere do original")
	}

	// Outra chave de criptografia não abre a sessão.
	other := *c
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	other.box, _ = secretbox.New(base64.StdEncoding.EncodeToString(k))
	if _, err := other.Load(ctx); err == nil {
		t.Fatal("sessão aberta com ENCRYPTION_KEY errada")
	}

	// Apagar remove tudo.
	if err := c.DeleteDevice(ctx, got); err != nil {
		t.Fatal(err)
	}
	if d2, err := c.Load(ctx); err != nil || d2 != nil {
		t.Fatalf("sessão deveria ter sido apagada: %v %v", d2, err)
	}
}

func TestSessionSecretsAreEncrypted(t *testing.T) {
	c, ctx := newContainer(t)
	d := pairedDevice(t, c, ctx)

	secret := []byte("estado-do-ratchet-signal-muito-secreto")
	if err := d.Sessions.PutSession(ctx, "5511888887777.0:1", secret); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := c.db.QueryRow(ctx, `SELECT session FROM whatsmeow_sessions WHERE their_id = $1`, "5511888887777.0:1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, secret) {
		t.Fatal("sessão Signal gravada em claro")
	}
	got, err := d.Sessions.GetSession(ctx, "5511888887777.0:1")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("get session = %q %v", got, err)
	}
	many, err := d.Sessions.GetManySessions(ctx, []string{"5511888887777.0:1", "inexistente"})
	if err != nil || !bytes.Equal(many["5511888887777.0:1"], secret) {
		t.Fatalf("get many = %v %v", many, err)
	}

	sk := []byte("sender-key-secreta")
	if err := d.SenderKeys.PutSenderKey(ctx, "grupo@g.us", "user", sk); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.SenderKeys.GetSenderKey(ctx, "grupo@g.us", "user"); !bytes.Equal(got, sk) {
		t.Fatal("sender key não confere")
	}

	ask := store.AppStateSyncKey{Data: []byte("app-state-key"), Fingerprint: []byte("fp"), Timestamp: 1}
	if err := d.AppStateKeys.PutAppStateSyncKey(ctx, []byte("id1"), ask); err != nil {
		t.Fatal(err)
	}
	var rawKey []byte
	_ = c.db.QueryRow(ctx, `SELECT key_data FROM whatsmeow_app_state_sync_keys`).Scan(&rawKey)
	if bytes.Contains(rawKey, ask.Data) {
		t.Fatal("chave de app state em claro")
	}
	if got, _ := d.AppStateKeys.GetAppStateSyncKey(ctx, []byte("id1")); got == nil || !bytes.Equal(got.Data, ask.Data) {
		t.Fatal("chave de app state não confere")
	}
}

func TestPreKeysAreEncrypted(t *testing.T) {
	c, ctx := newContainer(t)
	d := pairedDevice(t, c, ctx)

	ks, err := d.PreKeys.GetOrGenPreKeys(ctx, 5)
	if err != nil || len(ks) != 5 || ks[0].KeyID != 1 || ks[4].KeyID != 5 {
		t.Fatalf("prekeys = %v %v", ks, err)
	}
	again, _ := d.PreKeys.GetOrGenPreKeys(ctx, 5)
	if *again[2].Priv != *ks[2].Priv {
		t.Fatal("deveria reutilizar pre-keys ainda não enviadas")
	}
	var raw []byte
	_ = c.db.QueryRow(ctx, `SELECT key_enc FROM whatsapp_prekeys WHERE key_id = 3`).Scan(&raw)
	if bytes.Contains(raw, ks[2].Priv[:]) {
		t.Fatal("pre-key em claro")
	}
	if err := d.PreKeys.MarkPreKeysAsUploaded(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.PreKeys.UploadedPreKeyCount(ctx); n != 5 {
		t.Fatalf("enviadas = %d", n)
	}
	more, _ := d.PreKeys.GetOrGenPreKeys(ctx, 2)
	if more[0].KeyID != 6 {
		t.Fatalf("próxima pre-key = %d, esperado 6", more[0].KeyID)
	}
	if k, _ := d.PreKeys.GetPreKey(ctx, 3); k == nil || *k.Priv != *ks[2].Priv {
		t.Fatal("GetPreKey não confere")
	}
	_ = d.PreKeys.RemovePreKey(ctx, 3)
	if k, _ := d.PreKeys.GetPreKey(ctx, 3); k != nil {
		t.Fatal("pre-key removida ainda existe")
	}
	if one, err := d.PreKeys.GenOnePreKey(ctx); err != nil || one.KeyID != 8 {
		t.Fatalf("GenOnePreKey = %v %v", one, err)
	}
}

package wameow

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/util/keys"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
)

// secureContainer substitui o DeviceContainer do sqlstore para que nenhum
// segredo da sessão fique em texto puro no banco:
//
//   - chaves do dispositivo (noise, identidade, signed pre-key, ADV) ficam cifradas
//     em whatsapp_connections.device_enc; a linha em whatsmeow_device recebe zeros
//     (ela existe apenas para as chaves estrangeiras das demais tabelas);
//   - pre-keys ficam em whatsapp_prekeys, cifradas;
//   - sessões Signal, sender keys, chaves de app state, segredos de mensagem,
//     tokens de privacidade, salt NCT e o buffer de eventos (texto decifrado) são
//     cifrados ao passar pelo sqlstore (ver secureStore).
//
// Ficam em claro apenas dados não secretos: chaves públicas de identidade de
// contatos, nomes, mapeamentos LID↔telefone, versões de app state e configurações de chat.
type secureContainer struct {
	inner *sqlstore.Container
	db    *pgxpool.Pool
	box   *secretbox.Box
	log   waLog.Logger
}

var _ store.DeviceContainer = (*secureContainer)(nil)

type deviceSecrets struct {
	ID                    string `json:"id"`
	LID                   string `json:"lid"`
	RegistrationID        uint32 `json:"registrationId"`
	NoisePriv             []byte `json:"noisePriv"`
	IdentityPriv          []byte `json:"identityPriv"`
	SignedPreKeyPriv      []byte `json:"signedPreKeyPriv"`
	SignedPreKeyID        uint32 `json:"signedPreKeyId"`
	SignedPreKeySig       []byte `json:"signedPreKeySig"`
	AdvSecretKey          []byte `json:"advSecretKey"`
	AccountDetails        []byte `json:"accountDetails"`
	AccountSignature      []byte `json:"accountSignature"`
	AccountSignatureKey   []byte `json:"accountSignatureKey"`
	DeviceSignature       []byte `json:"deviceSignature"`
	Platform              string `json:"platform"`
	BusinessName          string `json:"businessName"`
	PushName              string `json:"pushName"`
	FacebookUUID          string `json:"facebookUuid"`
	LIDMigrationTimestamp int64  `json:"lidMigrationTimestamp"`
	CompanionMetaNonce    string `json:"companionMetaNonce"`
}

var aadDevice = []byte("wa:device")

func (c *secureContainer) NewDevice() *store.Device {
	var reg [4]byte
	_, _ = rand.Read(reg[:])
	adv := make([]byte, 32)
	_, _ = rand.Read(adv)
	d := &store.Device{
		Log:            c.log,
		Container:      c,
		NoiseKey:       keys.NewKeyPair(),
		IdentityKey:    keys.NewKeyPair(),
		RegistrationID: binary.BigEndian.Uint32(reg[:]),
		AdvSecretKey:   adv,
	}
	d.SignedPreKey = d.IdentityKey.CreateSignedPreKey(1)
	return d
}

func (c *secureContainer) PutDevice(ctx context.Context, d *store.Device) error {
	if d.ID == nil {
		return errors.New("wameow: device sem ID")
	}
	acc := d.Account
	if acc == nil {
		acc = &waAdv.ADVSignedDeviceIdentity{}
	}
	sec := deviceSecrets{
		ID: d.ID.String(), LID: d.LID.String(), RegistrationID: d.RegistrationID,
		NoisePriv: d.NoiseKey.Priv[:], IdentityPriv: d.IdentityKey.Priv[:],
		SignedPreKeyPriv: d.SignedPreKey.Priv[:], SignedPreKeyID: d.SignedPreKey.KeyID, SignedPreKeySig: d.SignedPreKey.Signature[:],
		AdvSecretKey:   d.AdvSecretKey,
		AccountDetails: acc.Details, AccountSignature: acc.AccountSignature,
		AccountSignatureKey: acc.AccountSignatureKey, DeviceSignature: acc.DeviceSignature,
		Platform: d.Platform, BusinessName: d.BusinessName, PushName: d.PushName,
		LIDMigrationTimestamp: d.LIDMigrationTimestamp, CompanionMetaNonce: d.CompanionMetaNonce,
	}
	if d.FacebookUUID != uuid.Nil {
		sec.FacebookUUID = d.FacebookUUID.String()
	}
	plain, err := json.Marshal(sec)
	if err != nil {
		return err
	}
	enc, err := c.box.Seal(plain, aadDevice)
	if err != nil {
		return err
	}
	if _, err := c.db.Exec(ctx,
		`UPDATE whatsapp_connections SET device_jid = $1, device_enc = $2 WHERE id = 1`, d.ID.String(), enc); err != nil {
		return err
	}

	// Linha "vazia" no sqlstore só para satisfazer as chaves estrangeiras.
	var zero [32]byte
	zk := keys.NewKeyPairFromPrivateKey(zero)
	redacted := &store.Device{
		ID: d.ID, LID: d.LID, RegistrationID: d.RegistrationID,
		NoiseKey: zk, IdentityKey: zk,
		SignedPreKey: &keys.PreKey{KeyPair: *zk, KeyID: d.SignedPreKey.KeyID, Signature: &[64]byte{}},
		AdvSecretKey: make([]byte, 32),
		// Tamanhos exigidos pelos CHECKs de whatsmeow_device, preenchidos com zeros.
		Account: &waAdv.ADVSignedDeviceIdentity{
			Details: []byte{}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64),
		},
		Platform: d.Platform, BusinessName: d.BusinessName, PushName: d.PushName,
		LIDMigrationTimestamp: d.LIDMigrationTimestamp,
		Initialized:           true, // impede o sqlstore de inicializar esta cópia
	}
	if err := c.inner.PutDevice(ctx, redacted); err != nil {
		return fmt.Errorf("registrando dispositivo: %w", err)
	}
	if !d.Initialized {
		c.initialize(d)
	}
	return nil
}

func (c *secureContainer) DeleteDevice(ctx context.Context, d *store.Device) error {
	if _, err := c.db.Exec(ctx, `UPDATE whatsapp_connections SET device_jid = '', device_enc = NULL WHERE id = 1`); err != nil {
		return err
	}
	if d.ID != nil {
		if _, err := c.db.Exec(ctx, `DELETE FROM whatsapp_prekeys WHERE device_jid = $1`, d.ID.String()); err != nil {
			return err
		}
		return c.inner.DeleteDevice(ctx, d)
	}
	return nil
}

// Load devolve o dispositivo salvo ou nil quando não há sessão.
func (c *secureContainer) Load(ctx context.Context) (*store.Device, error) {
	var enc []byte
	err := c.db.QueryRow(ctx, `SELECT device_enc FROM whatsapp_connections WHERE id = 1`).Scan(&enc)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && enc == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := c.box.Open(enc, aadDevice)
	if err != nil {
		return nil, fmt.Errorf("decifrando sessão do WhatsApp (ENCRYPTION_KEY mudou?): %w", err)
	}
	var s deviceSecrets
	if err := json.Unmarshal(plain, &s); err != nil {
		return nil, err
	}
	if len(s.NoisePriv) != 32 || len(s.IdentityPriv) != 32 || len(s.SignedPreKeyPriv) != 32 || len(s.SignedPreKeySig) != 64 {
		return nil, errors.New("wameow: chaves do dispositivo inválidas")
	}
	id, err := types.ParseJID(s.ID)
	if err != nil {
		return nil, err
	}
	d := &store.Device{
		Log: c.log, ID: &id, RegistrationID: s.RegistrationID,
		NoiseKey:     keys.NewKeyPairFromPrivateKey(*(*[32]byte)(s.NoisePriv)),
		IdentityKey:  keys.NewKeyPairFromPrivateKey(*(*[32]byte)(s.IdentityPriv)),
		AdvSecretKey: s.AdvSecretKey,
		Account: &waAdv.ADVSignedDeviceIdentity{
			Details: s.AccountDetails, AccountSignature: s.AccountSignature,
			AccountSignatureKey: s.AccountSignatureKey, DeviceSignature: s.DeviceSignature,
		},
		Platform: s.Platform, BusinessName: s.BusinessName, PushName: s.PushName,
		LIDMigrationTimestamp: s.LIDMigrationTimestamp, CompanionMetaNonce: s.CompanionMetaNonce,
	}
	if s.LID != "" {
		if lid, err := types.ParseJID(s.LID); err == nil {
			d.LID = lid
		}
	}
	if s.FacebookUUID != "" {
		d.FacebookUUID, _ = uuid.Parse(s.FacebookUUID)
	}
	d.SignedPreKey = &keys.PreKey{
		KeyPair:   *keys.NewKeyPairFromPrivateKey(*(*[32]byte)(s.SignedPreKeyPriv)),
		KeyID:     s.SignedPreKeyID,
		Signature: (*[64]byte)(s.SignedPreKeySig),
	}
	c.initialize(d)
	return d, nil
}

func (c *secureContainer) initialize(d *store.Device) {
	inner := sqlstore.NewSQLStore(c.inner, *d.ID)
	s := &secureStore{SQLStore: inner, box: c.box, db: c.db, jid: d.ID.String()}
	d.SetAllStores(s)
	d.EventBuffer = s
	d.LIDs = c.inner.LIDMap
	d.Container = c
	d.Initialized = true
}

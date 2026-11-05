package wameow

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/util/keys"

	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
)

// secureStore envolve o SQLStore cifrando os valores secretos antes de gravar
// e decifrando ao ler. Métodos não sobrescritos são delegados ao SQLStore.
type secureStore struct {
	*sqlstore.SQLStore
	box *secretbox.Box
	db  *pgxpool.Pool
	jid string
}

func (s *secureStore) seal(kind string, b []byte) ([]byte, error) {
	if b == nil {
		return nil, nil
	}
	return s.box.Seal(b, []byte("wa:"+kind))
}

func (s *secureStore) open(kind string, b []byte) ([]byte, error) {
	if len(b) == 0 {
		return b, nil
	}
	return s.box.Open(b, []byte("wa:"+kind))
}

// --- Sessões Signal ---------------------------------------------------------

func (s *secureStore) GetSession(ctx context.Context, address string) ([]byte, error) {
	b, err := s.SQLStore.GetSession(ctx, address)
	if err != nil {
		return nil, err
	}
	return s.open("session", b)
}

func (s *secureStore) GetManySessions(ctx context.Context, addresses []string) (map[string][]byte, error) {
	m, err := s.SQLStore.GetManySessions(ctx, addresses)
	if err != nil {
		return nil, err
	}
	for k, v := range m {
		if m[k], err = s.open("session", v); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (s *secureStore) PutSession(ctx context.Context, address string, session []byte) error {
	enc, err := s.seal("session", session)
	if err != nil {
		return err
	}
	return s.SQLStore.PutSession(ctx, address, enc)
}

func (s *secureStore) PutManySessions(ctx context.Context, sessions map[string][]byte) error {
	out := make(map[string][]byte, len(sessions))
	for k, v := range sessions {
		enc, err := s.seal("session", v)
		if err != nil {
			return err
		}
		out[k] = enc
	}
	return s.SQLStore.PutManySessions(ctx, out)
}

// --- Sender keys (grupos) ----------------------------------------------------

func (s *secureStore) PutSenderKey(ctx context.Context, group, user string, session []byte) error {
	enc, err := s.seal("senderkey", session)
	if err != nil {
		return err
	}
	return s.SQLStore.PutSenderKey(ctx, group, user, enc)
}

func (s *secureStore) GetSenderKey(ctx context.Context, group, user string) ([]byte, error) {
	b, err := s.SQLStore.GetSenderKey(ctx, group, user)
	if err != nil {
		return nil, err
	}
	return s.open("senderkey", b)
}

// --- Chaves de sincronização de app state -----------------------------------

func (s *secureStore) PutAppStateSyncKey(ctx context.Context, id []byte, key store.AppStateSyncKey) error {
	enc, err := s.seal("appstatekey", key.Data)
	if err != nil {
		return err
	}
	key.Data = enc
	return s.SQLStore.PutAppStateSyncKey(ctx, id, key)
}

func (s *secureStore) GetAppStateSyncKey(ctx context.Context, id []byte) (*store.AppStateSyncKey, error) {
	k, err := s.SQLStore.GetAppStateSyncKey(ctx, id)
	if err != nil || k == nil {
		return k, err
	}
	if k.Data, err = s.open("appstatekey", k.Data); err != nil {
		return nil, err
	}
	return k, nil
}

func (s *secureStore) GetAllAppStateSyncKeys(ctx context.Context) ([]*store.AppStateSyncKey, error) {
	ks, err := s.SQLStore.GetAllAppStateSyncKeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range ks {
		if k.Data, err = s.open("appstatekey", k.Data); err != nil {
			return nil, err
		}
	}
	return ks, nil
}

// --- Segredos de mensagem, tokens e salt ------------------------------------

func (s *secureStore) PutMessageSecrets(ctx context.Context, inserts []store.MessageSecretInsert) error {
	out := make([]store.MessageSecretInsert, len(inserts))
	for i, in := range inserts {
		enc, err := s.seal("msgsecret", in.Secret)
		if err != nil {
			return err
		}
		in.Secret = enc
		out[i] = in
	}
	return s.SQLStore.PutMessageSecrets(ctx, out)
}

func (s *secureStore) PutMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID, secret []byte) error {
	enc, err := s.seal("msgsecret", secret)
	if err != nil {
		return err
	}
	return s.SQLStore.PutMessageSecret(ctx, chat, sender, id, enc)
}

func (s *secureStore) GetMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID) ([]byte, types.JID, error) {
	b, jid, err := s.SQLStore.GetMessageSecret(ctx, chat, sender, id)
	if err != nil {
		return nil, jid, err
	}
	b, err = s.open("msgsecret", b)
	return b, jid, err
}

func (s *secureStore) PutPrivacyTokens(ctx context.Context, tokens ...store.PrivacyToken) error {
	out := make([]store.PrivacyToken, len(tokens))
	for i, t := range tokens {
		enc, err := s.seal("privacytoken", t.Token)
		if err != nil {
			return err
		}
		t.Token = enc
		out[i] = t
	}
	return s.SQLStore.PutPrivacyTokens(ctx, out...)
}

func (s *secureStore) GetPrivacyToken(ctx context.Context, user types.JID) (*store.PrivacyToken, error) {
	t, err := s.SQLStore.GetPrivacyToken(ctx, user)
	if err != nil || t == nil {
		return t, err
	}
	if t.Token, err = s.open("privacytoken", t.Token); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *secureStore) PutNCTSalt(ctx context.Context, salt []byte) error {
	enc, err := s.seal("nctsalt", salt)
	if err != nil {
		return err
	}
	return s.SQLStore.PutNCTSalt(ctx, enc)
}

func (s *secureStore) GetNCTSalt(ctx context.Context) ([]byte, error) {
	b, err := s.SQLStore.GetNCTSalt(ctx)
	if err != nil {
		return nil, err
	}
	return s.open("nctsalt", b)
}

// --- Buffer de eventos (contém mensagens já decifradas) ---------------------

func (s *secureStore) GetBufferedEvent(ctx context.Context, hash [32]byte) (*store.BufferedEvent, error) {
	e, err := s.SQLStore.GetBufferedEvent(ctx, hash)
	if err != nil || e == nil {
		return e, err
	}
	if e.Plaintext, err = s.open("eventbuffer", e.Plaintext); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *secureStore) PutBufferedEvent(ctx context.Context, hash [32]byte, plaintext []byte, serverTime time.Time) error {
	enc, err := s.seal("eventbuffer", plaintext)
	if err != nil {
		return err
	}
	return s.SQLStore.PutBufferedEvent(ctx, hash, enc, serverTime)
}

func (s *secureStore) GetOutgoingEvent(ctx context.Context, chat, altChat types.JID, id types.MessageID) (string, []byte, error) {
	f, b, err := s.SQLStore.GetOutgoingEvent(ctx, chat, altChat, id)
	if err != nil {
		return f, nil, err
	}
	b, err = s.open("outgoing", b)
	return f, b, err
}

func (s *secureStore) AddOutgoingEvent(ctx context.Context, chat types.JID, id types.MessageID, format string, plaintext []byte) error {
	enc, err := s.seal("outgoing", plaintext)
	if err != nil {
		return err
	}
	return s.SQLStore.AddOutgoingEvent(ctx, chat, id, format, enc)
}

// --- Pre-keys (tabela própria, cifrada) -------------------------------------

var aadPreKey = []byte("wa:prekey")

func (s *secureStore) insertPreKey(ctx context.Context, id uint32, uploaded bool) (*keys.PreKey, error) {
	k := keys.NewPreKey(id)
	enc, err := s.box.Seal(k.Priv[:], aadPreKey)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO whatsapp_prekeys (device_jid, key_id, key_enc, uploaded) VALUES ($1, $2, $3, $4)`,
		s.jid, int64(k.KeyID), enc, uploaded)
	return k, err
}

func (s *secureStore) nextPreKeyID(ctx context.Context) (uint32, error) {
	var last *int64
	if err := s.db.QueryRow(ctx, `SELECT max(key_id) FROM whatsapp_prekeys WHERE device_jid = $1`, s.jid).Scan(&last); err != nil {
		return 0, err
	}
	if last == nil {
		return 1, nil
	}
	return uint32(*last) + 1, nil
}

func (s *secureStore) decodePreKey(id int64, enc []byte) (*keys.PreKey, error) {
	priv, err := s.box.Open(enc, aadPreKey)
	if err != nil {
		return nil, err
	}
	if len(priv) != 32 {
		return nil, errors.New("wameow: pre-key inválida")
	}
	return &keys.PreKey{KeyPair: *keys.NewKeyPairFromPrivateKey(*(*[32]byte)(priv)), KeyID: uint32(id)}, nil
}

func (s *secureStore) GenOnePreKey(ctx context.Context) (*keys.PreKey, error) {
	id, err := s.nextPreKeyID(ctx)
	if err != nil {
		return nil, err
	}
	return s.insertPreKey(ctx, id, true)
}

func (s *secureStore) GetOrGenPreKeys(ctx context.Context, count uint32) ([]*keys.PreKey, error) {
	rows, err := s.db.Query(ctx,
		`SELECT key_id, key_enc FROM whatsapp_prekeys WHERE device_jid = $1 AND NOT uploaded ORDER BY key_id LIMIT $2`,
		s.jid, int64(count))
	if err != nil {
		return nil, err
	}
	var out []*keys.PreKey
	for rows.Next() {
		var id int64
		var enc []byte
		if err := rows.Scan(&id, &enc); err != nil {
			rows.Close()
			return nil, err
		}
		k, err := s.decodePreKey(id, enc)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if uint32(len(out)) < count {
		next, err := s.nextPreKeyID(ctx)
		if err != nil {
			return nil, err
		}
		for uint32(len(out)) < count {
			k, err := s.insertPreKey(ctx, next, false)
			if err != nil {
				return nil, err
			}
			out = append(out, k)
			next++
		}
	}
	return out, nil
}

func (s *secureStore) GetPreKey(ctx context.Context, id uint32) (*keys.PreKey, error) {
	var enc []byte
	err := s.db.QueryRow(ctx,
		`SELECT key_enc FROM whatsapp_prekeys WHERE device_jid = $1 AND key_id = $2`, s.jid, int64(id)).Scan(&enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.decodePreKey(int64(id), enc)
}

func (s *secureStore) RemovePreKey(ctx context.Context, id uint32) error {
	_, err := s.db.Exec(ctx, `DELETE FROM whatsapp_prekeys WHERE device_jid = $1 AND key_id = $2`, s.jid, int64(id))
	return err
}

func (s *secureStore) MarkPreKeysAsUploaded(ctx context.Context, upTo uint32) error {
	_, err := s.db.Exec(ctx,
		`UPDATE whatsapp_prekeys SET uploaded = true WHERE device_jid = $1 AND key_id <= $2`, s.jid, int64(upTo))
	return err
}

func (s *secureStore) UploadedPreKeyCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM whatsapp_prekeys WHERE device_jid = $1 AND uploaded`, s.jid).Scan(&n)
	return n, err
}

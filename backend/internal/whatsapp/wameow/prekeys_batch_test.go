package wameow

import (
	"go.mau.fi/whatsmeow/util/keys"
	"testing"
)

func TestConcurrentPreKeyBatchesReuseTheSameKeys(t *testing.T) {
	c, ctx := newContainer(t)
	d := pairedDevice(t, c, ctx)
	type result struct {
		keys []*keys.PreKey
		err  error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() { ks, err := d.PreKeys.GetOrGenPreKeys(ctx, 100); results <- result{ks, err} }()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("batch errors: %v %v", first.err, second.err)
	}
	if len(first.keys) != 100 || len(second.keys) != 100 {
		t.Fatal("incomplete batch")
	}
	for i, k := range first.keys {
		if k.KeyID != uint32(i+1) || *k.Priv != *second.keys[i].Priv {
			t.Fatal("concurrent batches generated conflicting keys")
		}
	}
	var count int
	if err := c.db.QueryRow(ctx, `SELECT count(*) FROM whatsapp_prekeys`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 100 {
		t.Fatalf("expected 100 keys, got %d", count)
	}
	one, err := d.PreKeys.GenOnePreKey(ctx)
	if err != nil || one.KeyID != 101 {
		t.Fatalf("single key collided with batch: %v %v", one, err)
	}
}

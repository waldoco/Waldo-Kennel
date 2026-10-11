package bridgeruntime

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeactivation"
)

type getOnlyKeys struct{}

func (getOnlyKeys) Get(context.Context, string) (ed25519.PrivateKey, error) { return nil, nil }

func TestRetireKeyDestroysTheRevokedIdentityKey(t *testing.T) {
	repo, store, d, keys, dir := realIdentity(t)
	f := NewFactory(Dependencies{Fence: repo, Keys: keys, SessionStore: store, KeyDirectory: dir, ReadyProbe: func(context.Context) error { return nil }})
	var retirer bridgeactivation.KeyRetirer = f
	if _, e := keys.Get(context.Background(), d.KeyCustodyRef); e != nil {
		t.Fatal("fixture key missing", e)
	}
	if e := retirer.RetireKey(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if _, e := keys.Get(context.Background(), d.KeyCustodyRef); e == nil {
		t.Fatal("private key still readable after retirement")
	}
	// Idempotent: a repeated retirement after a crash or retry is not an error.
	if e := retirer.RetireKey(context.Background(), d); e != nil {
		t.Fatal(e)
	}
}

func TestRetireKeyRefusesCustodyThatCannotClear(t *testing.T) {
	_, store, d, _, dir := realIdentity(t)
	f := NewFactory(Dependencies{Keys: getOnlyKeys{}, SessionStore: store, KeyDirectory: dir})
	if e := f.RetireKey(context.Background(), d); !errors.Is(e, bridgeactivation.ErrNotReady) {
		t.Fatalf("custody without Clear must be reported, got %v", e)
	}
}

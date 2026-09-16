package ownerproof

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
	"strings"
	"sync"
	"testing"
	"time"
)

func mintFixture(t *testing.T, k *Kernel, class domain.OwnerCommandClass) (Minted, Binding, time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	confirm := ""
	if class.Material() {
		confirm = "native-confirmation-1"
	}
	req := MintRequest{ID: "proof-1", AppRunID: "apprun-1", MissionID: "mission-1", ContentDigest: domain.DigestSHA256([]byte("content")), TargetID: "target-1", TargetGeneration: 2, Class: class, ConfirmationRef: confirm, ExpiresAt: now.Add(time.Minute), Now: now}
	minted, err := k.Mint(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return minted, Binding{ID: req.ID, AppRunID: req.AppRunID, MissionID: req.MissionID, ContentDigest: req.ContentDigest, TargetID: req.TargetID, TargetGeneration: req.TargetGeneration, Class: req.Class, ConfirmationRef: req.ConfirmationRef}, now
}
func TestMintVerifyConsumeRestartAndNoSecretShape(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	minted, b, now := mintFixture(t, NewWithRandom(store, bytes.NewReader(make([]byte, 64))), domain.OwnerCommandAnswer)
	if minted.Bearer == "" || minted.Proof.Verifier != "" {
		t.Fatal("bad public shape")
	}
	raw, _ := json.Marshal(minted.Proof)
	if strings.Contains(string(raw), minted.Bearer) || strings.Contains(string(raw), "Verifier") {
		t.Fatalf("secret json %s", raw)
	}
	if _, err := New(store).VerifyAndConsume(context.Background(), minted.Bearer, b, now); err != nil {
		t.Fatal(err)
	}
	if _, err := New(store).VerifyAndConsume(context.Background(), minted.Bearer, b, now); err == nil {
		t.Fatal("replay accepted")
	}
}
func TestWrongTupleExpiryAndMaterialConfirmation(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	minted, b, now := mintFixture(t, NewWithRandom(store, bytes.NewReader(bytes.Repeat([]byte{1}, 64))), domain.OwnerCommandAccept)
	mutations := []func(*Binding){func(x *Binding) { x.AppRunID = "other" }, func(x *Binding) { x.MissionID = "other" }, func(x *Binding) { x.ContentDigest = domain.DigestSHA256([]byte("other")) }, func(x *Binding) { x.TargetID = "other" }, func(x *Binding) { x.TargetGeneration++ }, func(x *Binding) { x.Class = domain.OwnerCommandTurn }, func(x *Binding) { x.ConfirmationRef = "other" }}
	for i, mutate := range mutations {
		x := b
		mutate(&x)
		if _, err := New(store).VerifyAndConsume(context.Background(), minted.Bearer, x, now); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
	if _, err := New(store).VerifyAndConsume(context.Background(), minted.Bearer, b, now.Add(time.Minute)); err == nil {
		t.Fatal("expired accepted")
	}
	if _, err := NewWithRandom(store, bytes.NewReader(bytes.Repeat([]byte{2}, 64))).Mint(context.Background(), MintRequest{ID: "bad", AppRunID: "a", MissionID: "m", ContentDigest: domain.DigestSHA256([]byte("c")), TargetID: "t", TargetGeneration: 1, Class: domain.OwnerCommandReplace, ExpiresAt: now.Add(time.Minute), Now: now}); err == nil {
		t.Fatal("material proof without confirmation accepted")
	}
}
func TestConcurrentConsumeOneWinner(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	minted, b, now := mintFixture(t, NewWithRandom(store, bytes.NewReader(bytes.Repeat([]byte{3}, 64))), domain.OwnerCommandTurn)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := New(store).VerifyAndConsume(context.Background(), minted.Bearer, b, now); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("wins=%d", wins)
	}
}
func TestExactMintReplayConvergesWithoutBearer(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	k := NewWithRandom(store, bytes.NewReader(bytes.Repeat([]byte{4}, 128)))
	first, b, now := mintFixture(t, k, domain.OwnerCommandSteer)
	second, err := k.Mint(context.Background(), MintRequest{ID: first.Proof.ID, AppRunID: b.AppRunID, MissionID: b.MissionID, ContentDigest: b.ContentDigest, TargetID: b.TargetID, TargetGeneration: b.TargetGeneration, Class: b.Class, ExpiresAt: first.Proof.ExpiresAt, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if second.Bearer != "" {
		t.Fatal("replay returned bearer")
	}
}

package store_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownerproof"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/store"
)

type commandFixture struct {
	store                                 *sqlite.Store
	request                               ports.HarnessCommandRequest
	connection                            *harnessconnection.Kernel
	questionID, conversationID, requestID string
}

func newCommandFixture(t *testing.T) commandFixture {
	t.Helper()
	s, _, conversationID := conversationFixture(t)
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	questionID, requestID := "question-generation-1", "provider-question-1"
	if err := s.UpsertActivity(context.Background(), conversationID, "", domain.ConversationActivity{ID: questionID, Kind: domain.ActivityKindApproval, Status: domain.ActivityStatusPending, RequestID: requestID, Summary: "approve"}, now); err != nil {
		t.Fatal(err)
	}
	command := domain.CanonicalHarnessCommand{Version: "v1", Class: domain.OwnerCommandAnswer, DecisionJSON: `{"id":"yes"}`}
	payload, _ := command.Bytes()
	target := domain.OwnerProofTarget{Version: domain.OwnerProofTargetVersion, Class: domain.OwnerCommandAnswer, QuestionID: questionID, QuestionGeneration: questionID}
	targetDigest, _ := target.Digest()
	connKernel := harnessconnection.NewWithRandom(s, bytes.NewReader(bytes.Repeat([]byte{7}, 128)))
	issued, err := connKernel.Issue(context.Background(), harnessconnection.IssueRequest{ConnectionID: "hc-command", InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "run", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityAnswer}, ExpiresAt: now.Add(time.Hour), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := ownerproof.NewWithRandom(s, bytes.NewReader(bytes.Repeat([]byte{8}, 128))).Mint(context.Background(), ownerproof.MintRequest{ID: "proof-command", AppRunID: "run", MissionID: "mission", ContentDigest: domain.DigestSHA256(payload), TargetDigest: targetDigest, Class: domain.OwnerCommandAnswer, ExpiresAt: now.Add(time.Minute), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return commandFixture{store: s, connection: connKernel, questionID: questionID, conversationID: conversationID, requestID: requestID, request: ports.HarnessCommandRequest{ConnectionBearer: issued.Bearer, ConnectionBinding: domain.HarnessConnectionBinding{ConnectionID: "hc-command", InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "run", Generation: 1, Class: domain.HarnessCapabilityAnswer}, OwnerProofID: "proof-command", OwnerProofBearer: proof.Bearer, Target: target, Command: command, AdapterRequestKey: "request-1", Now: now}}
}

func TestHarnessCommandClaimPersistsPayloadAndSurvivesRestart(t *testing.T) {
	f := newCommandFixture(t)
	claim, created, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	want, _ := f.request.Command.Bytes()
	if !bytes.Equal(claim.CanonicalPayload, want) || claim.DestinationID == "" || claim.ConnectionGeneration != 1 {
		t.Fatalf("claim=%+v", claim)
	}
	// Exact replay reads the durable claim and never consumes another proof or asks for payload again.
	replayed, created, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request)
	if err != nil || created || !bytes.Equal(replayed.CanonicalPayload, want) || replayed.DestinationID != claim.DestinationID {
		t.Fatalf("replay=%+v created=%v err=%v", replayed, created, err)
	}
}

func TestHarnessCommandPayloadMismatchAndStaleQuestionDoNotConsumeProof(t *testing.T) {
	f := newCommandFixture(t)
	changed := f.request
	changed.Command.DecisionJSON = `{"id":"no"}`
	if _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), changed); !errors.Is(err, domain.ErrHarnessCommandAuthentication) {
		t.Fatalf("payload err=%v", err)
	}
	if err := f.store.ResolveApproval(context.Background(), f.conversationID, f.requestID, `{}`, f.request.Now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request); !errors.Is(err, domain.ErrHarnessCommandAuthentication) {
		t.Fatalf("stale err=%v", err)
	}
	proof, found, err := f.store.GetOwnerProof(context.Background(), f.request.OwnerProofID)
	if err != nil || !found || proof.ConsumedAt != nil {
		t.Fatalf("proof=%+v found=%v err=%v", proof, found, err)
	}
}

func TestHarnessCommandClaimSerializesConnectionRotationAndRevocation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(commandFixture) error
	}{
		{"rotate", func(f commandFixture) error {
			_, err := f.connection.Rotate(context.Background(), "hc-command", 1, f.request.Now.Add(2*time.Hour), f.request.Now.Add(time.Second))
			return err
		}},
		{"revoke", func(f commandFixture) error {
			_, err := f.connection.Revoke(context.Background(), "hc-command", 1, f.request.Now.Add(time.Second))
			return err
		}},
	} {
		t.Run(tc.name+"_claim_first", func(t *testing.T) {
			f := newCommandFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			ctx := store.WithCommandClaimHook(context.Background(), func() { close(entered); <-release })
			var wg sync.WaitGroup
			wg.Add(1)
			var claimErr error
			go func() {
				defer wg.Done()
				_, _, claimErr = f.store.ValidateAuthoritiesAndCreateCommandClaim(ctx, f.request)
			}()
			<-entered
			mutDone := make(chan error, 1)
			go func() { mutDone <- tc.mutate(f) }()
			select {
			case <-mutDone:
				t.Fatal("mutation bypassed same writer")
			case <-time.After(20 * time.Millisecond):
			}
			close(release)
			wg.Wait()
			if claimErr != nil {
				t.Fatal(claimErr)
			}
			if err := <-mutDone; err != nil {
				t.Fatal(err)
			}
		})
		t.Run(tc.name+"_mutation_first", func(t *testing.T) {
			f := newCommandFixture(t)
			if err := tc.mutate(f); err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request); !errors.Is(err, domain.ErrHarnessCommandAuthentication) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestHarnessCommandClaimSerializesQuestionMutation(t *testing.T) {
	f := newCommandFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	ctx := store.WithCommandClaimHook(context.Background(), func() { close(entered); <-release })
	done := make(chan error, 1)
	go func() { _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(ctx, f.request); done <- err }()
	<-entered
	resolved := make(chan error, 1)
	go func() {
		resolved <- f.store.ResolveApproval(context.Background(), f.conversationID, f.requestID, `{}`, f.request.Now)
	}()
	select {
	case <-resolved:
		t.Fatal("target mutation bypassed same writer")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-resolved; err != nil {
		t.Fatal(err)
	}
}

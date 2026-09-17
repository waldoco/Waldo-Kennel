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
	s, _, _ := conversationFixture(t)
	return newCommandFixtureOnStore(t, s)
}

func newCommandFixtureOnStore(t *testing.T, s *sqlite.Store) commandFixture {
	t.Helper()
	ctx := context.Background()
	seedProject(t, s, "command-fixture")
	rec := sampleRecord("command-fixture")
	rec.Mode = domain.SessionModeChat
	session, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := s.CreateConversation(ctx, "conv-command-fixture", domain.ConversationScopeSession, "command-fixture", session.ID, histClock)
	if err != nil {
		t.Fatal(err)
	}
	conversationID := conversation.ID
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

type routineCommandFixture struct {
	commandFixture
	sessionID        domain.SessionID
	conversationID   string
	providerTurnID   string
	alternateAttempt domain.Attempt
	alternatePlan    domain.PlanRevision
}

func newRoutineCommandFixture(t *testing.T, class domain.OwnerCommandClass) routineCommandFixture {
	t.Helper()
	s := newTestStore(t)
	projectID := "ingress-" + string(class)
	plan, outcomeID := seedApprovedPlan(t, s, projectID)
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	attempt, err := s.CreateAttemptWithFence(context.Background(), admissionAt(outcomeID, plan, "attempt-"+string(class), domain.FenceSubjectForProject(domain.ProjectID(projectID)), now))
	if err != nil {
		t.Fatal(err)
	}
	rec := sampleRecord(projectID)
	rec.Mode = domain.SessionModeChat
	session, err := s.CreateSession(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := s.CreateConversation(context.Background(), "conv-"+string(class), domain.ConversationScopeSession, domain.ProjectID(projectID), session.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimChatControllerGeneration(context.Background(), session.ID, "gen-1", plan.ID.String(), "chat-v1:test", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindAttemptSession(context.Background(), domain.AttemptSessionRef{AttemptID: attempt.ID, SessionID: string(session.ID), Harness: domain.HarnessCodex, Mode: domain.SessionModeChat, RunBriefCoreDigest: plan.RunBriefCoreDigest, RunBriefCompiledDigest: plan.RunBriefCoreDigest, AdmissionSnapshot: `{}`, BoundAt: now}); err != nil {
		t.Fatal(err)
	}
	alternatePlan, _ := seedApprovedPlan(t, s, projectID+"-revision-2")
	providerTurnID := "provider-turn-1"
	if class == domain.OwnerCommandSteer || class == domain.OwnerCommandInterrupt {
		if err := s.AdoptProviderTurn(context.Background(), conversation.ID, session.ID, "gen-1", "turn-1", providerTurnID, now); err != nil {
			t.Fatal(err)
		}
	}
	target := domain.OwnerProofTarget{Version: domain.OwnerProofTargetVersion, Class: class, SessionID: string(session.ID), ControllerGeneration: "gen-1"}
	command := domain.CanonicalHarnessCommand{Version: "v1", Class: class}
	capability := domain.HarnessCapabilityTurn
	switch class {
	case domain.OwnerCommandTurn:
		target.ExpectedRevision = plan.ID.String()
		command.Text, command.ClientMessageID = "continue", "client-turn-1"
	case domain.OwnerCommandSteer:
		target.ExpectedRevision, target.ProviderTurnID = plan.ID.String(), providerTurnID
		command.Text, command.ClientMessageID = "focus", "client-steer-1"
		capability = domain.HarnessCapabilitySteer
	case domain.OwnerCommandInterrupt:
		target.ProviderTurnID = providerTurnID
		capability = domain.HarnessCapabilityInterrupt
	default:
		t.Fatalf("unsupported routine class %q", class)
	}
	payload, err := command.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	targetDigest, err := target.Digest()
	if err != nil {
		t.Fatal(err)
	}
	connection := harnessconnection.NewWithRandom(s, bytes.NewReader(bytes.Repeat([]byte{11}, 128)))
	issued, err := connection.Issue(context.Background(), harnessconnection.IssueRequest{ConnectionID: domain.HarnessConnectionID("hc-" + string(class)), InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "run", CapabilityClasses: []domain.HarnessCapabilityClass{capability}, ExpiresAt: now.Add(time.Hour), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := ownerproof.NewWithRandom(s, bytes.NewReader(bytes.Repeat([]byte{12}, 128))).Mint(context.Background(), ownerproof.MintRequest{ID: domain.OwnerProofID("proof-" + string(class)), AppRunID: "run", MissionID: "mission", ContentDigest: domain.DigestSHA256(payload), TargetDigest: targetDigest, Class: class, ExpiresAt: now.Add(time.Minute), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	request := ports.HarnessCommandRequest{ConnectionBearer: issued.Bearer, ConnectionBinding: domain.HarnessConnectionBinding{ConnectionID: issued.Connection.ID, InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "run", Generation: 1, Class: capability}, OwnerProofID: proof.Proof.ID, OwnerProofBearer: proof.Bearer, Target: target, Command: command, AdapterRequestKey: "request-" + string(class), Now: now}
	return routineCommandFixture{commandFixture: commandFixture{store: s, request: request, connection: connection}, sessionID: session.ID, conversationID: conversation.ID, providerTurnID: providerTurnID, alternatePlan: alternatePlan}
}

func testClaimFirstTargetMutation(t *testing.T, f commandFixture, mutate func() error) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	ctx := store.WithCommandClaimHook(context.Background(), func() { close(entered); <-release })
	claimed := make(chan error, 1)
	go func() { _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(ctx, f.request); claimed <- err }()
	<-entered
	mutated := make(chan error, 1)
	go func() { mutated <- mutate() }()
	select {
	case <-mutated:
		t.Fatal("target mutation bypassed same writer")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-claimed; err != nil {
		t.Fatal(err)
	}
	if err := <-mutated; err != nil {
		t.Fatal(err)
	}
}

func TestHarnessCommandClaimSerializesControllerGenerationMutation(t *testing.T) {
	t.Run("claim_first", func(t *testing.T) {
		f := newRoutineCommandFixture(t, domain.OwnerCommandTurn)
		testClaimFirstTargetMutation(t, f.commandFixture, func() error {
			return f.store.ClaimChatControllerGeneration(context.Background(), f.sessionID, "gen-2", f.request.Target.ExpectedRevision, "chat-v1:test", f.request.Now.Add(time.Second))
		})
	})
	t.Run("mutation_first", func(t *testing.T) {
		f := newRoutineCommandFixture(t, domain.OwnerCommandTurn)
		if err := f.store.ClaimChatControllerGeneration(context.Background(), f.sessionID, "gen-2", f.request.Target.ExpectedRevision, "chat-v1:test", f.request.Now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request); !errors.Is(err, domain.ErrHarnessCommandAuthentication) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestHarnessCommandClaimSerializesExpectedRevisionMutation(t *testing.T) {
	for _, class := range []domain.OwnerCommandClass{domain.OwnerCommandTurn, domain.OwnerCommandSteer} {
		t.Run(string(class)+"_claim_first", func(t *testing.T) {
			f := newRoutineCommandFixture(t, class)
			testClaimFirstTargetMutation(t, f.commandFixture, func() error {
				return f.store.ClaimChatControllerGeneration(context.Background(), f.sessionID, "gen-1", f.alternatePlan.ID.String(), "chat-v1:test", f.request.Now.Add(time.Second))
			})
		})
		t.Run(string(class)+"_mutation_first", func(t *testing.T) {
			f := newRoutineCommandFixture(t, class)
			if err := f.store.ClaimChatControllerGeneration(context.Background(), f.sessionID, "gen-1", f.alternatePlan.ID.String(), "chat-v1:test", f.request.Now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request); !errors.Is(err, domain.ErrHarnessCommandAuthentication) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestHarnessCommandClaimSerializesProviderTurnMutation(t *testing.T) {
	for _, class := range []domain.OwnerCommandClass{domain.OwnerCommandSteer, domain.OwnerCommandInterrupt} {
		t.Run(string(class)+"_claim_first", func(t *testing.T) {
			f := newRoutineCommandFixture(t, class)
			testClaimFirstTargetMutation(t, f.commandFixture, func() error {
				return f.store.SettleTurn(context.Background(), f.conversationID, f.providerTurnID, domain.TurnStateInterrupted, "", f.request.Now.Add(time.Second))
			})
		})
		t.Run(string(class)+"_mutation_first", func(t *testing.T) {
			f := newRoutineCommandFixture(t, class)
			if err := f.store.SettleTurn(context.Background(), f.conversationID, f.providerTurnID, domain.TurnStateInterrupted, "", f.request.Now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request); !errors.Is(err, domain.ErrHarnessCommandAuthentication) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestHarnessCommandClaimQuestionMutationFirstRejects(t *testing.T) {
	f := newCommandFixture(t)
	if err := f.store.ResolveApproval(context.Background(), f.conversationID, f.requestID, `{}`, f.request.Now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request); !errors.Is(err, domain.ErrHarnessCommandAuthentication) {
		t.Fatalf("err=%v", err)
	}
}

func TestHarnessCommandOutboxRecoveryReopensPersistedPayloadAndDestination(t *testing.T) {
	dataDir := t.TempDir()
	s, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	// Build against the explicitly located database, then close every connection.
	f := newCommandFixtureOnStore(t, s)
	claim, created, err := s.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	pending, err := reopened.ListPendingHarnessCommandOutbox(context.Background())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	want, _ := f.request.Command.Bytes()
	if pending[0].ClaimID != claim.ID || pending[0].DestinationID != claim.DestinationID || pending[0].DestinationType != claim.DestinationType || !bytes.Equal(pending[0].CanonicalPayload, want) {
		t.Fatalf("recovered=%+v claim=%+v", pending[0], claim)
	}
}

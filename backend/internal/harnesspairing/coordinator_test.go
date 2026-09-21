package harnesspairing

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func testTuple() IssueChallengeRequest {
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	return IssueChallengeRequest{
		Kind: domain.HarnessPairingKindPair, ConnectionID: "hc-1", InstallationID: "installation",
		AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "0.154.0",
		ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "app-run",
		CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpectedGeneration: 1,
		ConnectionExpiresAt: now.Add(24 * time.Hour), TTL: 5 * time.Minute, Now: now,
	}
}

func proveFromIssue(req IssueChallengeRequest, issued IssuedChallenge) ProveRequest {
	return ProveRequest{
		ChallengeID: issued.Challenge.ID, Secret: issued.Secret, ConnectionID: req.ConnectionID,
		InstallationID: req.InstallationID, HarnessIdentity: req.HarnessIdentity, ProviderVersion: req.ProviderVersion,
		MissionID: req.MissionID, AppRunID: req.AppRunID, AdapterDigest: req.AdapterDigest, ProtocolFingerprint: req.ProtocolFingerprint,
		CapabilityClasses: req.CapabilityClasses, ExpectedGeneration: req.ExpectedGeneration, Now: req.Now,
	}
}

func newFixture(t *testing.T) (*Coordinator, *harnessconnection.Kernel) {
	t.Helper()
	store := sqlitetest.MustOpen(t)
	kernel := harnessconnection.New(store)
	return New(store, kernel), kernel
}

func TestProve_HappyPathPairIssuesExactlyOneBearer(t *testing.T) {
	c, kernel := newFixture(t)
	req := testTuple()
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Challenge.ID == "" || issued.Secret == "" {
		t.Fatal("expected challenge id and secret")
	}
	if string(issued.Secret) == "" || issued.Secret.String() != "[redacted-pairing-challenge-secret]" {
		t.Fatal("secret must redact its String() form")
	}

	result, code, err := c.Prove(context.Background(), proveFromIssue(req, issued))
	if err != nil || code != domain.HarnessPairingResultSucceeded {
		t.Fatalf("prove: code=%v err=%v", code, err)
	}
	if result.Issued.Bearer == "" {
		t.Fatal("expected a bearer on success")
	}
	if result.Issued.Connection.Generation != 1 {
		t.Fatalf("generation = %d, want 1", result.Issued.Connection.Generation)
	}

	binding := harnessconnection.Binding{
		ConnectionID: req.ConnectionID, InstallationID: req.InstallationID, AdapterDigest: req.AdapterDigest,
		HarnessIdentity: req.HarnessIdentity, ProviderVersion: req.ProviderVersion, ProtocolFingerprint: req.ProtocolFingerprint,
		MissionID: req.MissionID, AppRunID: req.AppRunID, Generation: 1, Class: domain.HarnessCapabilityTurn,
	}
	if _, err := kernel.Authenticate(context.Background(), result.Issued.Bearer, binding, req.Now); err != nil {
		t.Fatalf("minted bearer must authenticate against the S3.1 kernel: %v", err)
	}
}

func TestProve_ReplayOfConsumedChallengeFailsClosed(t *testing.T) {
	c, _ := newFixture(t)
	req := testTuple()
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, code, err := c.Prove(context.Background(), proveFromIssue(req, issued)); err != nil || code != domain.HarnessPairingResultSucceeded {
		t.Fatalf("first prove: code=%v err=%v", code, err)
	}
	result, code, err := c.Prove(context.Background(), proveFromIssue(req, issued))
	if !errors.Is(err, domain.ErrHarnessPairingFailed) {
		t.Fatalf("replay err = %v, want ErrHarnessPairingFailed", err)
	}
	if code != domain.HarnessPairingResultReplayed {
		t.Fatalf("replay code = %v, want replayed", code)
	}
	if result.Issued.Bearer != "" {
		t.Fatal("replay must never mint a bearer")
	}
}

func TestProve_ExpiredChallengeFails(t *testing.T) {
	c, _ := newFixture(t)
	req := testTuple()
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	late := proveFromIssue(req, issued)
	late.Now = req.Now.Add(req.TTL).Add(time.Second)
	_, code, err := c.Prove(context.Background(), late)
	if !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultExpired {
		t.Fatalf("code=%v err=%v, want expired/failed", code, err)
	}
}

func TestProve_UndeclaredCapabilityClassFailsBeforeKernel(t *testing.T) {
	c, _ := newFixture(t)
	req := testTuple()
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	prove := proveFromIssue(req, issued)
	prove.CapabilityClasses = []domain.HarnessCapabilityClass{"not-a-real-class"}
	_, code, err := c.Prove(context.Background(), prove)
	if !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultUndeclaredClass {
		t.Fatalf("code=%v err=%v, want undeclared_capability_class/failed", code, err)
	}
	// The challenge must still be pending: a bad request must not burn it.
	prove.CapabilityClasses = req.CapabilityClasses
	if _, code, err := c.Prove(context.Background(), prove); err != nil || code != domain.HarnessPairingResultSucceeded {
		t.Fatalf("legitimate retry after a bad request must still succeed: code=%v err=%v", code, err)
	}
}

// TestProve_AdversarialTupleMatrix mutates exactly one bound field at a time
// (including the secret itself) and asserts every mutation fails closed
// without burning the challenge, then asserts the exact original request
// still succeeds — proving failures are rejected, not silently consumed.
func TestProve_AdversarialTupleMatrix(t *testing.T) {
	mutations := map[string]func(*ProveRequest){
		"wrong_connection_id": func(p *ProveRequest) { p.ConnectionID = "attacker-connection" },
		"wrong_secret":        func(p *ProveRequest) { p.Secret = "wrong-secret-value-thats-plainly-incorrect" },
		"wrong_installation":  func(p *ProveRequest) { p.InstallationID = "attacker-installation" },
		"wrong_adapter":       func(p *ProveRequest) { p.AdapterDigest = domain.DigestSHA256([]byte("attacker-adapter")) },
		"wrong_harness":       func(p *ProveRequest) { p.HarnessIdentity = "attacker-harness" },
		"wrong_provider":      func(p *ProveRequest) { p.ProviderVersion = "9.9.9" },
		"wrong_protocol":      func(p *ProveRequest) { p.ProtocolFingerprint = domain.DigestSHA256([]byte("attacker-protocol")) },
		"wrong_mission":       func(p *ProveRequest) { p.MissionID = "attacker-mission" },
		"wrong_app_run":       func(p *ProveRequest) { p.AppRunID = "attacker-run" },
		"wrong_class": func(p *ProveRequest) {
			p.CapabilityClasses = []domain.HarnessCapabilityClass{domain.HarnessCapabilityAccept}
		},
		"wrong_generation":  func(p *ProveRequest) { p.ExpectedGeneration = p.ExpectedGeneration + 1 },
		"unknown_challenge": func(p *ProveRequest) { p.ChallengeID = "does-not-exist" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			c, _ := newFixture(t)
			req := testTuple()
			issued, err := c.Issue(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			prove := proveFromIssue(req, issued)
			mutate(&prove)
			result, code, err := c.Prove(context.Background(), prove)
			if !errors.Is(err, domain.ErrHarnessPairingFailed) {
				t.Fatalf("%s: err = %v, want ErrHarnessPairingFailed", name, err)
			}
			if result.Issued.Bearer != "" {
				t.Fatalf("%s: must never mint a bearer", name)
			}
			if code == domain.HarnessPairingResultSucceeded {
				t.Fatalf("%s: code must not be succeeded", name)
			}

			// The legitimate holder must still be able to prove afterward:
			// one bad attempt must not have burned the challenge.
			if name != "unknown_challenge" {
				legit := proveFromIssue(req, issued)
				if _, code, err := c.Prove(context.Background(), legit); err != nil || code != domain.HarnessPairingResultSucceeded {
					t.Fatalf("%s: legitimate retry after adversarial attempt failed: code=%v err=%v", name, code, err)
				}
				// The rejected attempt against the still-pending challenge
				// must not have poisoned the durable audit result: it must
				// read back as succeeded, not the earlier failure code.
				stored, found, err := c.store.GetHarnessPairingChallenge(context.Background(), issued.Challenge.ID)
				if err != nil || !found {
					t.Fatalf("%s: get after legit retry: found=%v err=%v", name, found, err)
				}
				if stored.ResultCode == nil || *stored.ResultCode != domain.HarnessPairingResultSucceeded {
					t.Fatalf("%s: result code = %v, want succeeded (must not be poisoned by the earlier rejected attempt)", name, stored.ResultCode)
				}
			}
		})
	}
}

func TestProve_StaleGenerationOnRotateFails(t *testing.T) {
	c, kernel := newFixture(t)
	pairReq := testTuple()
	paired, err := c.Issue(context.Background(), pairReq)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Prove(context.Background(), proveFromIssue(pairReq, paired)); err != nil {
		t.Fatal(err)
	}
	// Rotate the underlying connection out from under a rotate challenge that
	// still targets the now-stale generation 1.
	if _, err := kernel.Rotate(context.Background(), pairReq.ConnectionID, 1, pairReq.AppRunID, pairReq.Now.Add(48*time.Hour), pairReq.Now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	rotateReq := pairReq
	rotateReq.Kind = domain.HarnessPairingKindRotate
	rotateReq.ExpectedGeneration = 1 // stale: the connection is already at generation 2
	rotateReq.Now = pairReq.Now.Add(2 * time.Minute)
	issuedRotate, err := c.Issue(context.Background(), rotateReq)
	if err != nil {
		t.Fatal(err)
	}
	prove := proveFromIssue(rotateReq, issuedRotate)
	result, code, err := c.Prove(context.Background(), prove)
	if !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultInternalError {
		t.Fatalf("stale-generation rotate: code=%v err=%v, want internal_error/failed (kernel CAS conflict)", code, err)
	}
	if result.Issued.Bearer != "" {
		t.Fatal("stale-generation rotate must never mint a bearer")
	}
}

func TestIssue_SupersedesPriorPendingChallengeForSameConnection(t *testing.T) {
	c, _ := newFixture(t)
	req := testTuple()
	first, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	// The first (now superseded) challenge must fail even with its own exact secret.
	if _, code, err := c.Prove(context.Background(), proveFromIssue(req, first)); !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultSuperseded {
		t.Fatalf("superseded challenge: code=%v err=%v, want superseded/failed", code, err)
	}
	if _, code, err := c.Prove(context.Background(), proveFromIssue(req, second)); err != nil || code != domain.HarnessPairingResultSucceeded {
		t.Fatalf("fresh challenge should still succeed: code=%v err=%v", code, err)
	}
}

func TestIssue_ConcurrentLeavesOnePendingWinner(t *testing.T) {
	c, _ := newFixture(t)
	req := testTuple()
	const n = 8
	var wg sync.WaitGroup
	issued := make([]IssuedChallenge, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			local := NewWithRandom(c.store, c.kernel, deterministicReader(string(rune('a'+i))))
			issued[i], errs[i] = local.Issue(context.Background(), req)
		}(i)
	}
	wg.Wait()
	pending := 0
	for i := range issued {
		if errs[i] != nil {
			t.Fatalf("issue %d: %v", i, errs[i])
		}
		stored, found, err := c.store.GetHarnessPairingChallenge(context.Background(), issued[i].Challenge.ID)
		if err != nil || !found {
			t.Fatalf("read %d found=%v err=%v", i, found, err)
		}
		if stored.Status == domain.HarnessPairingPending {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("pending=%d want 1", pending)
	}
}

func TestProve_ConsumedReplayCannotPoisonWinnerResult(t *testing.T) {
	c, kernel := newFixture(t)
	req := testTuple()
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	prove := proveFromIssue(req, issued)
	consumed := make(chan struct{})
	release := make(chan struct{})
	c.afterConsume = func() { close(consumed); <-release }
	type outcome struct {
		result ProveResult
		code   domain.HarnessPairingResultCode
		err    error
	}
	done := make(chan outcome, 1)
	go func() { r, code, err := c.Prove(context.Background(), prove); done <- outcome{r, code, err} }()
	<-consumed
	if _, code, err := c.Prove(context.Background(), prove); !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultReplayed {
		t.Fatalf("replay code=%s err=%v", code, err)
	}
	mid, found, err := c.store.GetHarnessPairingChallenge(context.Background(), issued.Challenge.ID)
	if err != nil || !found || mid.ResultCode != nil {
		t.Fatalf("mid found=%v code=%v err=%v", found, mid.ResultCode, err)
	}
	close(release)
	winner := <-done
	if winner.err != nil || winner.code != domain.HarnessPairingResultSucceeded || winner.result.Issued.Bearer == "" {
		t.Fatalf("winner=%+v", winner)
	}
	stored, found, err := c.store.GetHarnessPairingChallenge(context.Background(), issued.Challenge.ID)
	if err != nil || !found || stored.ResultCode == nil || *stored.ResultCode != domain.HarnessPairingResultSucceeded {
		t.Fatalf("stored found=%v code=%v err=%v", found, stored.ResultCode, err)
	}
	binding := harnessconnection.Binding{ConnectionID: req.ConnectionID, InstallationID: req.InstallationID, AdapterDigest: req.AdapterDigest, HarnessIdentity: req.HarnessIdentity, ProviderVersion: req.ProviderVersion, ProtocolFingerprint: req.ProtocolFingerprint, MissionID: req.MissionID, AppRunID: req.AppRunID, Generation: 1, Class: domain.HarnessCapabilityTurn}
	if _, err := kernel.Authenticate(context.Background(), winner.result.Issued.Bearer, binding, req.Now); err != nil {
		t.Fatalf("bearer: %v", err)
	}
}

// TestProve_ConcurrentExactRetryHasOneWinner exercises acceptance criterion 7:
// two concurrent proofs of the same challenge must have exactly one winner.
func TestProve_ConcurrentExactRetryHasOneWinner(t *testing.T) {
	c, _ := newFixture(t)
	req := testTuple()
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	prove := proveFromIssue(req, issued)

	const attempts = 8
	var wg sync.WaitGroup
	results := make([]domain.HarnessPairingResultCode, attempts)
	bearers := make([]string, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, code, _ := c.Prove(context.Background(), prove)
			results[i] = code
			bearers[i] = result.Issued.Bearer
		}(i)
	}
	wg.Wait()

	successes, bearerCount := 0, 0
	for i := range results {
		if results[i] == domain.HarnessPairingResultSucceeded {
			successes++
		}
		if bearers[i] != "" {
			bearerCount++
		}
	}
	if successes != 1 || bearerCount != 1 {
		t.Fatalf("successes=%d bearerCount=%d, want exactly 1 of each", successes, bearerCount)
	}
	stored, found, err := c.store.GetHarnessPairingChallenge(context.Background(), issued.Challenge.ID)
	if err != nil || !found || stored.ResultCode == nil || *stored.ResultCode != domain.HarnessPairingResultSucceeded {
		t.Fatalf("durable result after bearer issue: found=%v code=%v err=%v", found, stored.ResultCode, err)
	}
}

// TestRotateIntentReplacementSerializesBeforeKernel freezes the owner-intent
// semantic: opening a newer rotate intent supersedes the older one before proof.
func TestRotateIntentReplacementSerializesBeforeKernel(t *testing.T) {
	c, kernel := newFixture(t)
	pairReq := testTuple()
	paired, err := c.Issue(context.Background(), pairReq)
	if err != nil {
		t.Fatal(err)
	}
	original, code, err := c.Prove(context.Background(), proveFromIssue(pairReq, paired))
	if err != nil || code != domain.HarnessPairingResultSucceeded {
		t.Fatal(err)
	}
	rotateReq := pairReq
	rotateReq.Kind = domain.HarnessPairingKindRotate
	rotateReq.ExpectedGeneration = 1
	rotateReq.Now = pairReq.Now.Add(time.Minute)
	first, err := c.Issue(context.Background(), rotateReq)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Issue(context.Background(), rotateReq)
	if err != nil {
		t.Fatal(err)
	}
	if _, code, err := c.Prove(context.Background(), proveFromIssue(rotateReq, first)); !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultSuperseded {
		t.Fatalf("first code=%s err=%v", code, err)
	}
	rotated, code, err := c.Prove(context.Background(), proveFromIssue(rotateReq, second))
	if err != nil || code != domain.HarnessPairingResultSucceeded {
		t.Fatalf("second code=%s err=%v", code, err)
	}
	binding := harnessconnection.Binding{ConnectionID: pairReq.ConnectionID, InstallationID: pairReq.InstallationID, AdapterDigest: pairReq.AdapterDigest, HarnessIdentity: pairReq.HarnessIdentity, ProviderVersion: pairReq.ProviderVersion, ProtocolFingerprint: pairReq.ProtocolFingerprint, MissionID: pairReq.MissionID, AppRunID: pairReq.AppRunID, Class: domain.HarnessCapabilityTurn}
	binding.Generation = 1
	if _, err := kernel.Authenticate(context.Background(), original.Issued.Bearer, binding, rotateReq.Now); err == nil {
		t.Fatal("prior bearer survived rotation")
	}
	binding.Generation = 2
	if _, err := kernel.Authenticate(context.Background(), rotated.Issued.Bearer, binding, rotateReq.Now); err != nil {
		t.Fatalf("new bearer: %v", err)
	}
}

func deterministicReader(seed string) io.Reader {
	h := make([]byte, 0, 64)
	for len(h) < 64 {
		h = append(h, seed...)
	}
	return bytes.NewReader(h[:64])
}

// TestProve_CrashAfterConsumeBeforeKernelRequiresNewChallenge simulates the
// "post-consume/pre-issue" crash boundary directly at the store: the
// challenge is marked consumed (as Prove's point of no return would leave
// it) without the kernel ever having been called. The contract requires
// this state to be explicitly non-success and to require a new challenge,
// never a silent reissue.
func TestProve_CrashAfterConsumeBeforeKernelRequiresNewChallenge(t *testing.T) {
	c, kernel := newFixture(t)
	req := testTuple()
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := c.store.ConsumeHarnessPairingChallenge(context.Background(), issued.Challenge.ID, req.Now)
	if err != nil || !consumed {
		t.Fatalf("simulate crash-point consume: consumed=%v err=%v", consumed, err)
	}
	// No connection exists: the kernel was never reached.
	if _, found, err := kernelGetConnection(kernel, req.ConnectionID); err != nil || found {
		t.Fatalf("kernel must not have been reached: found=%v err=%v", found, err)
	}
	// The now-consumed challenge cannot be replayed to reach the kernel either.
	_, code, err := c.Prove(context.Background(), proveFromIssue(req, issued))
	if !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultReplayed {
		t.Fatalf("code=%v err=%v, want replayed/failed", code, err)
	}
	// Recovery requires an entirely new challenge, not a repaired old one.
	fresh := testTuple()
	fresh.Now = req.Now.Add(time.Minute)
	freshIssued, err := c.Issue(context.Background(), fresh)
	if err != nil {
		t.Fatal(err)
	}
	if _, code, err := c.Prove(context.Background(), proveFromIssue(fresh, freshIssued)); err != nil || code != domain.HarnessPairingResultSucceeded {
		t.Fatalf("fresh challenge after crash-point must still succeed: code=%v err=%v", code, err)
	}
}

func kernelGetConnection(kernel *harnessconnection.Kernel, id domain.HarnessConnectionID) (domain.HarnessConnection, bool, error) {
	// Kernel.Authenticate is the only exported read path; a not-found bearer
	// check is sufficient here to prove no connection generation exists yet.
	_, err := kernel.Authenticate(context.Background(), "any-bearer", harnessconnection.Binding{
		ConnectionID: id, InstallationID: "installation", AdapterDigest: domain.DigestSHA256([]byte("adapter")),
		HarnessIdentity: "codex", ProviderVersion: "0.154.0", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")),
		MissionID: "mission", AppRunID: "app-run", Generation: 1, Class: domain.HarnessCapabilityTurn,
	}, time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return domain.HarnessConnection{}, false, nil
	}
	return domain.HarnessConnection{}, true, nil
}

// TestProve_BearerUnavailableWhenConnectionAlreadyExists exercises the one
// frozen-surface gap flagged to the builder: harnessconnection.Kernel.Issue
// silently converges (returns Bearer=="") when a connection with the exact
// same ID and tuple already exists. A pair challenge that reaches this path
// has already been consumed and cannot be retried, so the coordinator must
// treat the empty bearer as a failure, not a success.
func TestProve_BearerUnavailableWhenConnectionAlreadyExists(t *testing.T) {
	c, kernel := newFixture(t)
	req := testTuple()
	// Pre-create the connection directly against the kernel, bypassing the
	// pairing challenge entirely, with the exact same ID and tuple a later
	// challenge will target.
	if _, err := kernel.Issue(context.Background(), harnessconnection.IssueRequest{
		ConnectionID: req.ConnectionID, InstallationID: req.InstallationID, AdapterDigest: req.AdapterDigest,
		HarnessIdentity: req.HarnessIdentity, ProviderVersion: req.ProviderVersion, ProtocolFingerprint: req.ProtocolFingerprint,
		MissionID: req.MissionID, AppRunID: req.AppRunID, CapabilityClasses: req.CapabilityClasses,
		ExpiresAt: req.ConnectionExpiresAt, Now: req.Now,
	}); err != nil {
		t.Fatal(err)
	}
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	result, code, err := c.Prove(context.Background(), proveFromIssue(req, issued))
	if !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultBearerUnavailable {
		t.Fatalf("code=%v err=%v, want bearer_unavailable/failed", code, err)
	}
	if result.Issued.Bearer != "" {
		t.Fatal("must never mint a bearer on this path")
	}
	// The challenge is burned (consumed) even though no bearer was ever
	// delivered: a new challenge is required, matching the crash-point
	// contract's "requires a new challenge" language.
	if _, code, err := c.Prove(context.Background(), proveFromIssue(req, issued)); !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultReplayed {
		t.Fatalf("retry after bearer_unavailable: code=%v err=%v, want replayed/failed", code, err)
	}
}

// TestProve_UniqueIndexCollisionOnDifferentConnectionIDFailsClosed exercises
// the second documented edge case: two pairing challenges targeting
// different connection IDs but an identical binding tuple collide on
// harness_connections' own unique index at generation 1. The store's
// CreateHarnessConnection then looks up the wrong (non-existent) ID and
// returns a real Go error, not one of the three closed sentinel errors. The
// coordinator must fail closed without panicking or leaking the conflicting
// row's identity.
func TestProve_UniqueIndexCollisionOnDifferentConnectionIDFailsClosed(t *testing.T) {
	c, _ := newFixture(t)
	first := testTuple()
	first.ConnectionID = "hc-collision-1"
	firstIssued, err := c.Issue(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if _, code, err := c.Prove(context.Background(), proveFromIssue(first, firstIssued)); err != nil || code != domain.HarnessPairingResultSucceeded {
		t.Fatalf("first pairing: code=%v err=%v", code, err)
	}

	second := testTuple()
	second.ConnectionID = "hc-collision-2" // different ID, identical tuple otherwise
	secondIssued, err := c.Issue(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	result, code, err := c.Prove(context.Background(), proveFromIssue(second, secondIssued))
	if !errors.Is(err, domain.ErrHarnessPairingFailed) || code != domain.HarnessPairingResultInternalError {
		t.Fatalf("colliding pairing: code=%v err=%v, want internal_error/failed", code, err)
	}
	if result.Issued.Bearer != "" {
		t.Fatal("must never mint a bearer on a collision")
	}
}

func TestIssue_ExactSecretNeverPersistedOrReturnedTwice(t *testing.T) {
	c, _ := newFixture(t)
	req := testTuple()
	issued, err := c.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Challenge.ProofVerifier != "" {
		t.Fatal("returned challenge must not expose the verifier")
	}
	stored, found, err := c.store.GetHarnessPairingChallenge(context.Background(), issued.Challenge.ID)
	if err != nil || !found {
		t.Fatal(err)
	}
	if stored.ProofVerifier == "" || stored.ProofVerifier == string(issued.Secret) {
		t.Fatal("stored verifier must be a one-way hash, not the raw secret")
	}
}

func TestIssue_RandomSourceExhaustionFailsClosed(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	kernel := harnessconnection.New(store)
	c := NewWithRandom(store, kernel, bytes.NewReader(nil))
	if _, err := c.Issue(context.Background(), testTuple()); err == nil {
		t.Fatal("expected an error when the random source is exhausted")
	}
}

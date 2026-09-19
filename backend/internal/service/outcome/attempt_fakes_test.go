package outcome_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// attemptFakeStore extends the plan fake with a faithful in-memory
// implementation of the Act & Observe store seam: numbered attempts,
// exclusive open custody fences, guarded transitions, ordered observations,
// and append-only receipts. SQLite-level fidelity is proven in the storage
// suites; this fake exists so admission orchestration is tested against real
// state transitions instead of mocks.
type attemptFakeStore struct {
	*planFakeStore

	mu             sync.Mutex
	projectOfSpace map[domain.ResponsibilitySpaceID]domain.ProjectID
	attempts       map[domain.OutcomeID][]domain.Attempt
	attemptByKey   map[string]domain.AttemptID
	fences         map[string][]domain.AttemptFence
	refs           map[domain.AttemptID][]domain.AttemptSessionRef
	obs            map[domain.AttemptID][]domain.AttemptObservation
	usage          map[domain.AttemptID][]domain.ExecutionUsageSample
	budgetStops    map[domain.AttemptID]domain.AttemptBudgetStop
	receipts       map[domain.AttemptID][]domain.AttemptRecoveryReceipt

	// provenance holds recorded protocol-negotiation episodes by session ID,
	// unordered; the read applies the same at-or-before rule as the sqlite
	// store so tests exercise the real selection semantics.
	provenance map[string][]domain.ChatProtocolProvenance

	// dropActivationOnce simulates losing the queued->running promotion race.
	dropActivationOnce bool

	// failBindOnce simulates losing the durable session-binding write AFTER a
	// live spawn — the exact post-spawn failure the round-3 review required
	// be injectable end to end.
	failBindOnce bool

	// renewals counts fence-lease refreshes the liveness loop performs.
	renewals int

	// injectRunningTerminationFailureAt, when "observation" or "release",
	// fires once inside TerminateRunningAttemptWithObservation at that stage
	// and returns an error before any state is mutated — modeling the real
	// store's single all-or-nothing transaction, where a mid-operation
	// failure rolls back the status transition too instead of stranding a
	// partial commit.
	injectRunningTerminationFailureAt string
}

func newAttemptFakeStore() *attemptFakeStore {
	return &attemptFakeStore{
		planFakeStore:  newPlanFakeStore(),
		projectOfSpace: map[domain.ResponsibilitySpaceID]domain.ProjectID{},
		attempts:       map[domain.OutcomeID][]domain.Attempt{},
		attemptByKey:   map[string]domain.AttemptID{},
		fences:         map[string][]domain.AttemptFence{},
		refs:           map[domain.AttemptID][]domain.AttemptSessionRef{},
		obs:            map[domain.AttemptID][]domain.AttemptObservation{},
		usage:          map[domain.AttemptID][]domain.ExecutionUsageSample{},
		budgetStops:    map[domain.AttemptID]domain.AttemptBudgetStop{},
		receipts:       map[domain.AttemptID][]domain.AttemptRecoveryReceipt{},
	}
}

func (f *attemptFakeStore) EnsureWorkResponsibilitySpace(ctx context.Context, projectID domain.ProjectID) (domain.ResponsibilitySpace, error) {
	space, err := f.planFakeStore.EnsureWorkResponsibilitySpace(ctx, projectID)
	if err != nil {
		return space, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.projectOfSpace[space.ID] = projectID
	return space, nil
}

// GetOutcomeProjectID resolves the project through the Outcome's space,
// mirroring the SQL join.
func (f *attemptFakeStore) GetOutcomeProjectID(_ context.Context, outcomeID domain.OutcomeID) (domain.ProjectID, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	outcome, ok := f.outcomes[outcomeID]
	if !ok {
		return "", false, nil
	}
	id, ok := f.projectOfSpace[outcome.SpaceID]
	return id, ok, nil
}

func (f *attemptFakeStore) FindAttemptByIdempotencyKey(_ context.Context, key string) (domain.Attempt, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.attemptByKey[key]
	if !ok {
		return domain.Attempt{}, false, nil
	}
	for _, list := range f.attempts {
		for _, attempt := range list {
			if attempt.ID == id {
				return attempt, true, nil
			}
		}
	}
	return domain.Attempt{}, false, nil
}

func (f *attemptFakeStore) CreateAttemptWithFence(_ context.Context, admission ports.AttemptAdmission) (domain.Attempt, error) {
	outcomeID, requestKey, subject, at := admission.OutcomeID, admission.RequestKey, admission.FenceSubject, admission.At
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, held := f.openFenceLocked(subject); held {
		holder, _ := f.openFenceLocked(subject)
		return domain.Attempt{}, &ports.AttemptFenceHeldError{Subject: subject, Holder: holder.AttemptID, OutcomeID: outcomeID}
	}
	if requestKey != "" {
		if existingKey, taken := f.attemptByKey[requestKey]; taken {
			for _, list := range f.attempts {
				for _, prior := range list {
					if prior.ID == existingKey {
						return prior, &ports.AttemptReplayError{Attempt: prior}
					}
				}
			}
		}
	}
	fakeAttemptCounter++
	attempt := domain.Attempt{
		ID:                     domain.AttemptID("att-" + string(rune('a'+fakeAttemptCounter%26)) + timeToSuffix(at)),
		OutcomeID:              outcomeID,
		PlanRevisionID:         admission.PlanRevisionID,
		WorkUnitID:             admission.WorkUnitID,
		Number:                 int64(len(f.attempts[outcomeID]) + 1),
		Status:                 domain.AttemptQueued,
		ContractRevisionNumber: admission.ContractRevisionNumber,
		// The durable store binds the admitting authorization generation inside
		// the admission transaction. Dropping it here would make this fake
		// unable to tell which authorization an Attempt belongs to, which is
		// exactly what decides whether an older stop may act on it.
		RunIntentGeneration: admission.RunIntentGeneration,
		RequestKey:          requestKey,
		CreatedAt:           at,
		UpdatedAt:           at,
	}
	f.attempts[outcomeID] = append(f.attempts[outcomeID], attempt)
	if requestKey != "" {
		f.attemptByKey[requestKey] = attempt.ID
	}
	f.fences[subject] = append(f.fences[subject], domain.AttemptFence{
		ID: "fence-" + string(attempt.ID), Subject: subject, AttemptID: attempt.ID, IssuedAt: at,
	})
	return attempt, nil
}

func (f *attemptFakeStore) FailAttemptBeforeLaunch(_ context.Context, in ports.AttemptPrelaunchFailure) (domain.AttemptObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, attempt := range f.attempts[in.OutcomeID] {
		if attempt.ID != in.AttemptID || attempt.Status != domain.AttemptQueued {
			continue
		}
		var fenceSubject string
		var fenceIndex int
		var foundFence bool
		for subject, history := range f.fences {
			for j, fence := range history {
				if fence.AttemptID == in.AttemptID && fence.Open() {
					fenceSubject, fenceIndex, foundFence = subject, j, true
				}
			}
		}
		if !foundFence {
			return domain.AttemptObservation{}, errors.New("open fence required")
		}
		observation := domain.AttemptObservation{ID: "obs-" + strings.ToLower(in.ObservationKind) + "-" + strconv.Itoa(len(f.obs[in.AttemptID])+1), AttemptID: in.AttemptID, Seq: int64(len(f.obs[in.AttemptID]) + 1), Kind: in.ObservationKind, Payload: in.ObservationPayload, CreatedAt: in.At}
		if err := observation.Validate(); err != nil {
			return domain.AttemptObservation{}, err
		}
		f.obs[in.AttemptID] = append(f.obs[in.AttemptID], observation)
		f.attempts[in.OutcomeID][i].Status = domain.AttemptFailed
		f.attempts[in.OutcomeID][i].UpdatedAt = in.At
		f.fences[fenceSubject][fenceIndex].ReleasedAt = in.At
		f.fences[fenceSubject][fenceIndex].ReleaseReason = in.ReleaseReason
		return observation, nil
	}
	return domain.AttemptObservation{}, errors.New("queued attempt required")
}

// TerminateRunningAttemptWithObservation mirrors the real store's single
// atomic transaction: the status transition, the observation append, and
// (when ReleaseReason is set) the custody release either all land together
// or none do. An injected failure at either the "observation" or "release"
// stage returns an error with NO state mutated at all, including the status
// transition — proving a mid-operation failure can never leave the Attempt
// terminal with its fence still held.
func (f *attemptFakeStore) TerminateRunningAttemptWithObservation(_ context.Context, in ports.AttemptRunningTermination) (domain.AttemptObservation, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, attempt := range f.attempts[in.OutcomeID] {
		if attempt.ID != in.AttemptID || attempt.Status != domain.AttemptRunning {
			continue
		}
		if !domain.AttemptTransitionLegal(domain.AttemptRunning, in.TargetStatus) {
			return domain.AttemptObservation{}, false, errors.New("illegal attempt status transition")
		}
		if f.injectRunningTerminationFailureAt == "observation" {
			f.injectRunningTerminationFailureAt = ""
			return domain.AttemptObservation{}, false, errors.New("injected observation write failure")
		}
		observation := domain.AttemptObservation{
			ID:        "obs-" + strings.ToLower(in.ObservationKind) + "-" + strconv.Itoa(len(f.obs[in.AttemptID])+1),
			AttemptID: in.AttemptID, Seq: int64(len(f.obs[in.AttemptID]) + 1),
			Kind: in.ObservationKind, Payload: in.ObservationPayload, CreatedAt: in.At,
		}
		if err := observation.Validate(); err != nil {
			return domain.AttemptObservation{}, false, err
		}
		var fenceSubject string
		var fenceIndex int
		var foundFence bool
		if in.ReleaseReason != "" {
			for subject, history := range f.fences {
				for j, fence := range history {
					if fence.AttemptID == in.AttemptID && fence.Open() {
						fenceSubject, fenceIndex, foundFence = subject, j, true
					}
				}
			}
			if f.injectRunningTerminationFailureAt == "release" {
				f.injectRunningTerminationFailureAt = ""
				return domain.AttemptObservation{}, false, errors.New("injected custody release failure")
			}
		}
		// Nothing durable is written until every step that can still fail has
		// succeeded, mirroring the real store's single-transaction commit.
		f.attempts[in.OutcomeID][i].Status = in.TargetStatus
		f.attempts[in.OutcomeID][i].UpdatedAt = in.At
		f.obs[in.AttemptID] = append(f.obs[in.AttemptID], observation)
		if foundFence {
			f.fences[fenceSubject][fenceIndex].ReleasedAt = in.At
			f.fences[fenceSubject][fenceIndex].ReleaseReason = in.ReleaseReason
		}
		return observation, true, nil
	}
	return domain.AttemptObservation{}, false, nil
}

// openFenceLocked resolves the open fence over a subject. ok=false when free.
func (f *attemptFakeStore) openFenceLocked(subject string) (domain.AttemptFence, bool) {
	for _, fence := range f.fences[subject] {
		if fence.Open() {
			return fence, true
		}
	}
	return domain.AttemptFence{}, false
}

func (f *attemptFakeStore) GetAttempt(_ context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (domain.Attempt, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, attempt := range f.attempts[outcomeID] {
		if attempt.ID == attemptID {
			return attempt, true, nil
		}
	}
	return domain.Attempt{}, false, nil
}

func (f *attemptFakeStore) ListAttempts(_ context.Context, outcomeID domain.OutcomeID) ([]domain.Attempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.Attempt, len(f.attempts[outcomeID]))
	copy(out, f.attempts[outcomeID])
	return out, nil
}

func (f *attemptFakeStore) TransitionAttemptStatus(_ context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID, expected, next domain.AttemptStatus, at time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dropActivationOnce && next == domain.AttemptRunning {
		f.dropActivationOnce = false
		return 0, nil // simulate losing the promotion race
	}
	for i, attempt := range f.attempts[outcomeID] {
		if attempt.ID != attemptID || attempt.Status != expected {
			continue
		}
		if !domain.AttemptTransitionLegal(expected, next) {
			return 0, errors.New("illegal attempt status transition")
		}
		f.attempts[outcomeID][i].Status = next
		f.attempts[outcomeID][i].UpdatedAt = at
		return 1, nil
	}
	return 0, nil
}

func (f *attemptFakeStore) ListAttemptsByStatus(_ context.Context, status domain.AttemptStatus) ([]domain.Attempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Attempt
	for _, list := range f.attempts {
		for _, attempt := range list {
			if attempt.Status == status {
				out = append(out, attempt)
			}
		}
	}
	return out, nil
}

func (f *attemptFakeStore) BindAttemptSession(_ context.Context, ref domain.AttemptSessionRef) (domain.AttemptSessionRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failBindOnce {
		f.failBindOnce = false
		return domain.AttemptSessionRef{}, errors.New("injected binding write failure")
	}
	ref.Seq = int64(len(f.refs[ref.AttemptID]) + 1)
	ref.ID = domain.AttemptSessionRefID("asr-" + string(ref.AttemptID) + "-" + strconv.FormatInt(ref.Seq, 10))
	f.refs[ref.AttemptID] = append(f.refs[ref.AttemptID], ref)
	return ref, nil
}

func (f *attemptFakeStore) LatestAttemptSessionRef(_ context.Context, attemptID domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.refs[attemptID]
	if len(list) == 0 {
		return domain.AttemptSessionRef{}, false, nil
	}
	return list[len(list)-1], true, nil
}

func (f *attemptFakeStore) ListAttemptSessionRefs(_ context.Context, attemptID domain.AttemptID) ([]domain.AttemptSessionRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.AttemptSessionRef, len(f.refs[attemptID]))
	copy(out, f.refs[attemptID])
	return out, nil
}

func (f *attemptFakeStore) ChatProtocolProvenanceForBinding(_ context.Context, sessionID string, boundAt time.Time) (domain.ChatProtocolProvenance, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	episodes := f.provenance[sessionID]
	if len(episodes) == 0 {
		return domain.ChatProtocolProvenance{}, false, nil
	}
	best := -1
	for i, episode := range episodes {
		if !episode.NegotiatedAt.After(boundAt) && (best == -1 || episode.NegotiatedAt.After(episodes[best].NegotiatedAt)) {
			best = i
		}
	}
	if best == -1 {
		return domain.ChatProtocolProvenance{}, false, nil
	}
	return episodes[best], true, nil
}

func (f *attemptFakeStore) AppendAttemptObservation(_ context.Context, attemptID domain.AttemptID, kind string, payload string, at time.Time) (domain.AttemptObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	observation := domain.AttemptObservation{
		ID:        "obs-" + strings.ToLower(kind) + "-" + strconv.FormatInt(int64(len(f.obs[attemptID])+1), 10),
		AttemptID: attemptID,
		Seq:       int64(len(f.obs[attemptID]) + 1),
		Kind:      kind,
		Payload:   payload,
		CreatedAt: at,
	}
	if err := observation.Validate(); err != nil {
		return domain.AttemptObservation{}, err
	}
	f.obs[attemptID] = append(f.obs[attemptID], observation)
	return observation, nil
}

func (f *attemptFakeStore) ListAttemptObservations(_ context.Context, attemptID domain.AttemptID) ([]domain.AttemptObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.AttemptObservation, len(f.obs[attemptID]))
	copy(out, f.obs[attemptID])
	return out, nil
}

func (f *attemptFakeStore) OpenFenceForSubject(_ context.Context, subject string) (domain.AttemptFence, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fence, ok := f.openFenceLocked(subject)
	return fence, ok, nil
}

func (f *attemptFakeStore) ReleaseFenceForAttempt(_ context.Context, attemptID domain.AttemptID, reason string, at time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if reason == "" {
		return 0, errors.New("release reason required")
	}
	for subject, history := range f.fences {
		for i, fence := range history {
			if fence.AttemptID == attemptID && fence.Open() {
				f.fences[subject][i].ReleasedAt = at
				f.fences[subject][i].ReleaseReason = reason
				return 1, nil
			}
		}
	}
	return 0, nil
}

func (f *attemptFakeStore) RenewFenceForAttempt(_ context.Context, attemptID domain.AttemptID, at time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for subject, history := range f.fences {
		for i, fence := range history {
			if fence.AttemptID == attemptID && fence.Open() {
				f.fences[subject][i].LastRenewedAt = at
				f.renewals++
				return 1, nil
			}
		}
	}
	return 0, nil
}

func (f *attemptFakeStore) CreateRecoveryReceipt(_ context.Context, receipt domain.AttemptRecoveryReceipt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := receipt.Validate(); err != nil {
		return err
	}
	f.receipts[receipt.AttemptID] = append(f.receipts[receipt.AttemptID], receipt)
	return nil
}

func (f *attemptFakeStore) ListRecoveryReceipts(_ context.Context, attemptID domain.AttemptID) ([]domain.AttemptRecoveryReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.AttemptRecoveryReceipt, len(f.receipts[attemptID]))
	copy(out, f.receipts[attemptID])
	return out, nil
}

var fakeAttemptCounter int

func timeToSuffix(t time.Time) string {
	return strconv.FormatInt(t.UnixNano()%100000, 10)
}

// fakeSpawner is the execution seam double: it records what admission asked
// for and answers with whatever failure the current scenario injects. NO
// fallback: the requested harness is echoed verbatim.
type fakeSpawner struct {
	afterPrelaunch   func()
	sessionMetadata  domain.SessionMetadata
	sessionWorktrees []domain.SessionWorktreeRecord
	mu               sync.Mutex
	readiness        ports.AgentProfileReadiness
	readinessErr     error
	readinessN       int
	spawnErr         error
	terminateErr     error
	// terminateResult shapes the next successful Terminate answer; nil means
	// a clean proven stop whose workspace was freed. Tests inject the
	// dirty-preserved shape {ProviderStopped:true, WorkspaceFreed:false} to
	// prove workspace preservation is not provider liveness.
	terminateResult    *ports.TerminationResult
	terminated         []string
	spawned            []ports.AttemptSpawnRequest
	completionBoundary domain.AttemptCompletionBoundary
	sessionN           int
}

func (f *fakeSpawner) ProfileReadiness(_ context.Context, _ domain.ProjectID, _ domain.ExecutionBinding, _ *domain.AttemptExecutionPolicy) (ports.AgentProfileReadiness, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readinessN++
	if f.readinessErr != nil {
		return ports.AgentProfileReadiness{}, f.readinessErr
	}
	return f.readiness, nil
}

func (f *fakeSpawner) readinessCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readinessN
}

func (f *fakeSpawner) setReadiness(readiness ports.AgentProfileReadiness) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readiness = readiness
}

func (f *fakeSpawner) Spawn(_ context.Context, req ports.AttemptSpawnRequest) (ports.AttemptSpawnResult, error) {
	f.mu.Lock()
	f.spawned = append(f.spawned, req)
	if f.spawnErr != nil {
		err := f.spawnErr
		f.spawnErr = nil
		f.mu.Unlock()
		return ports.AttemptSpawnResult{}, err
	}
	f.sessionN++
	rec := domain.SessionRecord{
		ID:       domain.SessionID("sess-provider-" + string(rune('a'+f.sessionN))),
		Mode:     domain.SessionModeTUI,
		Harness:  req.Harness,
		Metadata: domain.SessionMetadata{WorkspacePath: "/tmp/kennel-fake-attempt-workspace"},
		Activity: domain.Activity{
			State:          domain.ActivityActive,
			LastActivityAt: time.Now(),
		},
	}
	if f.sessionMetadata.WorkspaceRepoPath != "" {
		rec.Metadata.WorkspaceRepoPath = f.sessionMetadata.WorkspaceRepoPath
		rec.Metadata.DiffBaseSHA = f.sessionMetadata.DiffBaseSHA
		rec.Metadata.DiffBaseRef = f.sessionMetadata.DiffBaseRef
	}
	bound, err := req.ExecutionPolicy.BindWorkspaceRoot(rec.Metadata.WorkspacePath)
	f.mu.Unlock()
	if err != nil {
		return ports.AttemptSpawnResult{}, err
	}
	if req.BeforeProviderLaunch == nil {
		return ports.AttemptSpawnResult{}, errors.New("missing prelaunch persistence callback")
	}
	if err := req.BeforeProviderLaunch(context.Background(), rec, bound, f.sessionWorktrees); err != nil {
		return ports.AttemptSpawnResult{}, &ports.AttemptPrelaunchError{Stage: "before_provider_launch", Err: err}
	}
	if f.afterPrelaunch != nil {
		f.afterPrelaunch()
	}
	return ports.AttemptSpawnResult{Session: domain.Session{SessionRecord: rec}, ExecutionPolicy: &bound, CompletionBoundary: f.completionBoundary}, nil
}

// Terminate records the request; failures AND result shapes are injectable
// per scenario. Tests pair a successful Terminate with heartbeats.terminate(...)
// to mirror the real flow, where Kill writes the durable is_terminated fact.
func (f *fakeSpawner) Terminate(_ context.Context, _ domain.ProjectID, sessionID string) (ports.TerminationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.terminateErr != nil {
		return ports.TerminationResult{}, f.terminateErr
	}
	f.terminated = append(f.terminated, sessionID)
	if f.terminateResult != nil {
		return *f.terminateResult, nil
	}
	return ports.TerminationResult{ProviderStopped: true, WorkspaceFreed: true}, nil
}

func (f *fakeSpawner) spawnCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.spawned)
}

func (f *fakeSpawner) failNextSpawn(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spawnErr = err
}

func (f *fakeSpawner) failNextTerminate(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminateErr = err
}

// setTerminateResult injects the two-fact answer a successful stop reports.
func (f *fakeSpawner) setTerminateResult(res ports.TerminationResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminateResult = &res
}

// fakeHeartbeats stands in for the sessions table: tests mutate it to model
// signalled, silent, GC'd, and terminated provider sessions.
type fakeHeartbeats struct {
	mu       sync.Mutex
	sessions map[domain.SessionID]domain.SessionRecord
}

func newFakeHeartbeats() *fakeHeartbeats {
	return &fakeHeartbeats{sessions: map[domain.SessionID]domain.SessionRecord{}}
}

func (f *fakeHeartbeats) GetSession(_ context.Context, id domain.SessionID) (domain.SessionRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.sessions[id]
	return rec, ok, nil
}

func (f *fakeHeartbeats) signal(id domain.SessionID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec := f.sessions[id]
	rec.FirstSignalAt = time.Now().UTC()
	rec.Activity = domain.Activity{State: domain.ActivityActive, LastActivityAt: time.Now().UTC()}
	f.sessions[id] = rec
}

func (f *fakeHeartbeats) terminate(id domain.SessionID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec := f.sessions[id]
	rec.IsTerminated = true
	f.sessions[id] = rec
}

// backdate ages the session's durable activity WITHOUT terminating it — the
// stale-heartbeat shape the lease gate must refuse to renew.
func (f *fakeHeartbeats) backdate(id domain.SessionID, age time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec := f.sessions[id]
	rec.Activity = domain.Activity{State: domain.ActivityActive, LastActivityAt: time.Now().Add(-age)}
	f.sessions[id] = rec
}

func (f *fakeHeartbeats) forget(id domain.SessionID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, id)
}

// The bare contract/plan fakes satisfy the widened OutcomeStore interface
// with inert stubs; attemptFakeStore above shadows every one of them with the
// real in-memory behavior the execution tests exercise.
// GetOutcomeProjectID resolves the Outcome's project through its
// ResponsibilitySpace. Planning needs it to read Project preferences, so a
// stub that always answered "no such Outcome" would make every plan fail.
func (f *fakeStore) GetOutcomeProjectID(_ context.Context, id domain.OutcomeID) (domain.ProjectID, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	outcome, ok := f.outcomes[id]
	if !ok {
		return "", false, nil
	}
	for projectID, space := range f.spaces {
		if space.ID == outcome.SpaceID {
			return projectID, true, nil
		}
	}
	return "", false, nil
}
func (f *fakeStore) FindAttemptByIdempotencyKey(context.Context, string) (domain.Attempt, bool, error) {
	return domain.Attempt{}, false, nil
}
func (f *fakeStore) CreateAttemptWithFence(context.Context, ports.AttemptAdmission) (domain.Attempt, error) {
	return domain.Attempt{}, nil
}
func (f *fakeStore) FailAttemptBeforeLaunch(context.Context, ports.AttemptPrelaunchFailure) (domain.AttemptObservation, error) {
	return domain.AttemptObservation{}, nil
}
func (f *fakeStore) TerminateRunningAttemptWithObservation(context.Context, ports.AttemptRunningTermination) (domain.AttemptObservation, bool, error) {
	return domain.AttemptObservation{}, false, nil
}
func (f *fakeStore) GetAttempt(context.Context, domain.OutcomeID, domain.AttemptID) (domain.Attempt, bool, error) {
	return domain.Attempt{}, false, nil
}
func (f *fakeStore) ListAttempts(context.Context, domain.OutcomeID) ([]domain.Attempt, error) {
	return nil, nil
}
func (f *fakeStore) TransitionAttemptStatus(context.Context, domain.OutcomeID, domain.AttemptID, domain.AttemptStatus, domain.AttemptStatus, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeStore) ListAttemptsByStatus(context.Context, domain.AttemptStatus) ([]domain.Attempt, error) {
	return nil, nil
}
func (f *fakeStore) BindAttemptSession(context.Context, domain.AttemptSessionRef) (domain.AttemptSessionRef, error) {
	return domain.AttemptSessionRef{}, nil
}
func (f *fakeStore) LatestAttemptSessionRef(context.Context, domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
	return domain.AttemptSessionRef{}, false, nil
}
func (f *fakeStore) ListAttemptSessionRefs(context.Context, domain.AttemptID) ([]domain.AttemptSessionRef, error) {
	return nil, nil
}
func (f *fakeStore) ChatProtocolProvenanceForBinding(context.Context, string, time.Time) (domain.ChatProtocolProvenance, bool, error) {
	return domain.ChatProtocolProvenance{}, false, nil
}
func (f *fakeStore) AppendAttemptObservation(context.Context, domain.AttemptID, string, string, time.Time) (domain.AttemptObservation, error) {
	return domain.AttemptObservation{}, nil
}
func (f *fakeStore) ListAttemptObservations(context.Context, domain.AttemptID) ([]domain.AttemptObservation, error) {
	return nil, nil
}
func (f *fakeStore) OpenFenceForSubject(context.Context, string) (domain.AttemptFence, bool, error) {
	return domain.AttemptFence{}, false, nil
}
func (f *fakeStore) ReleaseFenceForAttempt(context.Context, domain.AttemptID, string, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeStore) CreateRecoveryReceipt(context.Context, domain.AttemptRecoveryReceipt) error {
	return nil
}
func (f *fakeStore) ListRecoveryReceipts(context.Context, domain.AttemptID) ([]domain.AttemptRecoveryReceipt, error) {
	return nil, nil
}

func (f *fakeStore) RenewFenceForAttempt(context.Context, domain.AttemptID, time.Time) (int64, error) {
	return 0, nil
}

func (f *attemptFakeStore) AppendAttemptExecutionUsage(_ context.Context, sample domain.ExecutionUsageSample) (domain.ExecutionUsageSample, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.usage[sample.AttemptID]
	if len(list) > 0 {
		p := list[len(list)-1]
		if sample.Sequence == p.Sequence && sample.InputTokens == p.InputTokens && sample.OutputTokens == p.OutputTokens {
			return sample, false, nil
		}
		if sample.Sequence <= p.Sequence || sample.InputTokens < p.InputTokens || sample.OutputTokens < p.OutputTokens {
			return domain.ExecutionUsageSample{}, false, errors.New("execution usage is not monotonic")
		}
		sample.InputDelta = sample.InputTokens - p.InputTokens
		sample.OutputDelta = sample.OutputTokens - p.OutputTokens
	} else {
		sample.InputDelta = sample.InputTokens
		sample.OutputDelta = sample.OutputTokens
	}
	f.usage[sample.AttemptID] = append(list, sample)
	return sample, true, nil
}
func (f *attemptFakeStore) WorkUnitExecutionUsage(_ context.Context, id domain.WorkUnitID) (domain.ExecutionUsageTotals, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var t domain.ExecutionUsageTotals
	for _, attempts := range f.attempts {
		for _, a := range attempts {
			if a.WorkUnitID == id {
				for _, u := range f.usage[a.ID] {
					t.InputTokens += u.InputDelta
					t.OutputTokens += u.OutputDelta
				}
			}
		}
	}
	return t, nil
}

func (f *attemptFakeStore) ClaimAttemptBudgetStop(_ context.Context, claim domain.AttemptBudgetStop) (domain.AttemptBudgetStop, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.budgetStops == nil {
		f.budgetStops = map[domain.AttemptID]domain.AttemptBudgetStop{}
	}
	if old, ok := f.budgetStops[claim.AttemptID]; ok {
		if old.SessionID != claim.SessionID || old.Reason != claim.Reason {
			return old, false, errors.New("attempt budget stop claim conflicts with durable claim")
		}
		return old, false, nil
	}
	f.budgetStops[claim.AttemptID] = claim
	return claim, true, nil
}
func (f *attemptFakeStore) RecordAttemptBudgetProviderStopped(_ context.Context, id domain.AttemptID, session string, reason domain.RuntimeBudgetReasonCode, result string, at time.Time) (domain.AttemptBudgetStop, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	x, ok := f.budgetStops[id]
	if !ok || x.SessionID != session || x.Reason != reason {
		return domain.AttemptBudgetStop{}, errors.New("budget machine stop result conflicts with durable claim")
	}
	if x.ProviderStoppedAt != nil {
		if !domain.CanonicalJSONEqual(x.MachineResult, result) {
			return x, errors.New("budget machine stop result conflicts with durable result")
		}
		return x, nil
	}
	x.ProviderStoppedAt = &at
	x.MachineResult = result
	f.budgetStops[id] = x
	return x, nil
}
func (f *attemptFakeStore) GetAttemptBudgetStop(_ context.Context, id domain.AttemptID) (domain.AttemptBudgetStop, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	x, ok := f.budgetStops[id]
	return x, ok, nil
}
func (f *attemptFakeStore) ListUnfinishedAttemptBudgetStops(_ context.Context) ([]domain.AttemptBudgetStop, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.AttemptBudgetStop
	for _, x := range f.budgetStops {
		out = append(out, x)
	}
	return out, nil
}

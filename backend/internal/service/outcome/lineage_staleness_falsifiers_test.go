package outcome_test

import (
	"errors"
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/artifactstore"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	sqlitestore "github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/store"
)

// stalenessWorld is one durable outcome with a three-unit chain A -> B -> C,
// driven entirely through the real sqlite store and read back through the
// real service seams. Setup writes are the same store methods the landed
// admission/retention paths use, so what the falsifiers read is durable
// state, never a fixture shortcut.
type stalenessWorld struct {
	t      *testing.T
	ctx    context.Context
	dir    string
	store  *sqlitestore.Store
	svc    *outcome.Service
	at     time.Time
	outID  domain.OutcomeID
	revID  domain.ContractRevisionID
	critA  domain.CriterionID
	critB  domain.CriterionID
	critC  domain.CriterionID
	planID domain.PlanRevisionID
}

func newStalenessWorld(t *testing.T, dir string) *stalenessWorld {
	t.Helper()
	w := &stalenessWorld{t: t, ctx: context.Background(), dir: dir, at: time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)}
	policy := testAdmissionPolicy()
	store, err := sqlite.Open(dir)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	w.store = store
	w.svc = outcome.New(store, func() time.Time { return w.at }).WithAttemptManifests(store).
		WithPlanning(intelligencetest.Provider{}, &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(&fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}, newFakeHeartbeats()).
		WithRunIntents(store).
		WithNeedsYou(store, nil).
		WithCapabilityEscalations(store)
	w.svc.AdmissionPolicy = policy
	if err := store.UpsertProject(w.ctx, domain.ProjectRecord{ID: "mer", Path: dir, DisplayName: "mer", RegisteredAt: w.at}); err != nil {
		t.Fatalf("register project: %v", err)
	}
	view, err := w.svc.Create(w.ctx, outcome.CreateInput{
		ProjectID: "mer", Title: "Staleness Ledger",
		Goal:            "Prove that superseded lineages stop counting as proof.",
		SuccessCriteria: []string{"A is proven.", "B is proven.", "C is proven."},
		Review:          "Deterministic falsifiers.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true},
		RequestKey:       "req-staleness-create",
	})
	if err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	w.outID = view.Outcome.ID
	w.revID = view.Current.ID
	if len(view.Current.Criteria) != 3 {
		t.Fatalf("expected three criteria, got %+v", view.Current.Criteria)
	}
	w.critA, w.critB, w.critC = view.Current.Criteria[0].ID, view.Current.Criteria[1].ID, view.Current.Criteria[2].ID
	unit := func(id domain.WorkUnitID, title string, criterion domain.CriterionID, dependsOn ...domain.WorkUnitID) domain.WorkUnit {
		return domain.WorkUnit{
			ID: id, Kind: domain.WorkUnitDirect, Intent: domain.WorkUnitIntentModifyAndExecute, Title: title,
			ContractRevisionNumber: 1, OutputSummary: title + " result", EvidenceChecks: []string{title + " is checked"},
			VerificationRequirement: title + " proof is verified", StopConditions: []string{"stop on contradiction"},
			ExecutionBudget: domain.ExecutionBudget{WallTimeLimit: time.Hour, RetryLimit: 1, TokenAccounting: domain.TokenAccountingUnsupported, Source: domain.ExecutionBudgetPolicyDefault, PolicyID: policy.ID, PolicyVersion: policy.Version, PolicyDigest: policy.Digest},
			Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault,
			RequiredCapabilities: []string{domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite, domain.CapabilityWorktreeExec},
			DependsOn:             dependsOn,
			CriterionIDs:          []domain.CriterionID{criterion},
		}
	}
	unitA := unit("wu-a", "A", w.critA)
	unitA.Role = domain.WorkUnitRoleImplement
	unitB := unit("wu-b", "B", w.critB, "wu-a")
	unitB.Role = domain.WorkUnitRoleImplement
	unitB.Inputs = []domain.WorkUnitInput{{FromWorkUnitID: "wu-a", Required: "A result", Position: 1}}
	unitC := unit("wu-c", "C", w.critC, "wu-b")
	unitC.Role = domain.WorkUnitRoleImplement
	unitC.Inputs = []domain.WorkUnitInput{{FromWorkUnitID: "wu-b", Required: "B result", Position: 1}}
	units := []domain.WorkUnit{unitA, unitB, unitC}
	grants := []domain.CapabilityGrant{{ID: "grant-read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}, {ID: "grant-write", Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"}, {ID: "grant-exec", Name: domain.CapabilityWorktreeExec, Scope: "worktree/*"}}
	routing := []domain.WorkUnitRoutingDecision{}
	for _, u := range units {
		routing = append(routing, domain.WorkUnitRoutingDecision{WorkUnitID: u.ID, Decision: domain.RoutingDecision{
			Status: domain.RoutingDecisionRecommended, PolicyVersion: domain.RoutingPolicyVersion, Role: domain.RoutingRoleWorker,
			RecommendedCandidateID: string(domain.HarnessCodex), RecommendedProvider: string(domain.HarnessCodex),
			RecommendedModelSelection: domain.ExecutionBindingModelProviderDefault,
		}})
	}
	digest, err := domain.ComputePlanRunBriefCoreDigest(view.Current, units, grants)
	if err != nil {
		t.Fatalf("run brief digest: %v", err)
	}
	plan, err := store.AppendPlanRevision(w.ctx, w.outID, domain.PlanRevision{
		ID: "plan-stale", Number: 1, ContractRevisionNumber: 1, Status: domain.PlanStatusProposed,
		Summary: "Three-unit chain for staleness falsifiers",
		RunBriefCoreDigest:     digest,
		RunBriefCompiledDigest: string(domain.DigestSHA256([]byte("compiled"))),
		CreatedAt:              w.at,
		WorkUnits:              units,
		Grants:                 grants,
		RoutingDecisions:       routing,
	})
	if err != nil {
		t.Fatalf("append plan: %v", err)
	}
	w.planID = plan.ID
	if _, err := w.svc.ApprovePlan(w.ctx, w.outID, outcome.ApprovePlanInput{PlanRevisionID: plan.ID, ExpectedContractRevision: 1}); err != nil {
		t.Fatalf("approve plan: %#v", err)
	}
	return w
}

func (w *stalenessWorld) attempt(unit domain.WorkUnitID, key string) domain.Attempt {
	w.t.Helper()
	attempt, err := w.store.CreateAttemptWithFence(w.ctx, ports.AttemptAdmission{
		OutcomeID: w.outID, PlanRevisionID: w.planID, WorkUnitID: unit,
		ContractRevisionNumber: 1, RequestKey: key, FenceSubject: "fence-" + key, At: w.at,
	})
	if err != nil {
		w.t.Fatalf("admit %s: %v", key, err)
	}
	return attempt
}

func (w *stalenessWorld) sealInputs(attempt domain.Attempt, refs ...domain.AttemptManifestInputRef) {
	w.t.Helper()
	manifest, err := domain.NewAttemptInputManifest(domain.AttemptInputManifest{
		AttemptID: attempt.ID, OutcomeID: w.outID, PlanRevisionID: w.planID, WorkUnitID: attempt.WorkUnitID,
		ContractRevisionNumber: 1, WorkspaceKind: domain.WorkspaceStagedFolder,
		RunBriefCoreDigest: string(domain.DigestSHA256([]byte("core"))), RunBriefCompiledDigest: string(domain.DigestSHA256([]byte("compiled"))), ExecutionPolicyDigest: string(domain.DigestSHA256([]byte("policy"))),
		Inputs: refs,
	}, w.at)
	if err != nil {
		w.t.Fatalf("build input manifest for %s: %v", attempt.ID, err)
	}
	if err := w.store.SaveAttemptManifest(w.ctx, manifest); err != nil {
		w.t.Fatalf("seal input manifest for %s: %v", attempt.ID, err)
	}
}

func (w *stalenessWorld) retain(attempt domain.Attempt, content string) domain.AttemptReceipt {
	w.t.Helper()
	files := []domain.ArtifactFile{{
		ID: "artifact-" + content, AttemptID: attempt.ID, RelativePath: "out/" + content + ".txt",
		ChangeKind: domain.ArtifactAdded, ContentDigest: string(domain.DigestSHA256([]byte(content))),
	}}
	receipt := domain.AttemptReceipt{
		AttemptID: attempt.ID, OutcomeID: w.outID, PlanRevisionID: w.planID, WorkUnitID: attempt.WorkUnitID,
		ContractRevisionNumber: 1, ArtifactVersion: string(domain.ArtifactManifestDigest(files)),
		WorkspaceKind: domain.WorkspaceStagedFolder, WorkspacePath: w.dir,
		RetentionState: domain.RetentionRetained,
		ObservedAt: w.at, CreatedAt: w.at, UpdatedAt: w.at, Files: files,
	}
	if err := w.store.SaveAttemptReceipt(w.ctx, receipt); err != nil {
		w.t.Fatalf("retain %s for %s: %v", content, attempt.ID, err)
	}
	return receipt
}

func (w *stalenessWorld) prove(attempt domain.Attempt, criterion domain.CriterionID, key string) {
	w.t.Helper()
	evidence := domain.EvidenceItem{
		ID: domain.EvidenceItemID("ev-" + key), OutcomeID: w.outID, ContractRevisionID: w.revID, CriterionID: criterion,
		SubjectType: domain.ProofSubjectAttempt, SubjectID: string(attempt.ID), SubjectRevision: "1",
		Kind: domain.EvidenceSupporting, SourceType: domain.EvidenceSourceArtifact, SourceRef: "retained://" + key,
		ProducerType: domain.EvidenceProducerTool, ProducerRef: "falsifier",
		Summary: "supporting proof for " + key,
		ContentDigest: string(domain.DigestSHA256([]byte("evidence-" + key))), RequestKey: "ev-rk-" + key,
		RequestFingerprint: string(domain.DigestSHA256([]byte("ev-fp-" + key))), CreatedAt: w.at,
	}
	if err := w.store.CreateEvidenceItem(w.ctx, evidence); err != nil {
		w.t.Fatalf("evidence %s: %v", key, err)
	}
	run := domain.VerificationRun{
		ID: domain.VerificationRunID("vr-" + key), OutcomeID: w.outID, ContractRevisionID: w.revID, CriterionID: criterion,
		SubjectType: domain.ProofSubjectAttempt, SubjectID: string(attempt.ID), SubjectRevision: "1",
		EvidenceItemIDs: []domain.EvidenceItemID{evidence.ID}, Method: "deterministic-replay",
		IndependenceClass: domain.VerificationDeterministic, Result: domain.VerificationPassed,
		VerifierRef: "falsifier", RequestKey: "vr-rk-" + key,
		RequestFingerprint: string(domain.DigestSHA256([]byte("vr-fp-" + key))), CreatedAt: w.at.Add(time.Second),
	}
	if err := w.store.CreateVerificationRun(w.ctx, run); err != nil {
		w.t.Fatalf("verification %s: %v", key, err)
	}
}

func (w *stalenessWorld) criterionReady(t *testing.T, criterion domain.CriterionID) bool {
	t.Helper()
	proof, err := w.svc.GetProof(w.ctx, w.outID)
	if err != nil {
		t.Fatalf("get proof: %v", err)
	}
	for _, view := range proof.Criteria {
		if view.Criterion.ID == criterion {
			return view.Ready
		}
	}
	t.Fatalf("criterion %s missing from proof", criterion)
	return false
}

func (w *stalenessWorld) scheduleEntry(t *testing.T, unit domain.WorkUnitID) outcome.WorkUnitScheduleView {
	t.Helper()
	schedule, err := w.svc.GetSchedule(w.ctx, w.outID, w.planID)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	for _, entry := range schedule.WorkUnits {
		if entry.WorkUnit.ID == unit {
			return entry
		}
	}
	t.Fatalf("unit %s missing from schedule", unit)
	return outcome.WorkUnitScheduleView{}
}

func (w *stalenessWorld) staleness(t *testing.T) domain.LineageStaleness {
	t.Helper()
	proof, err := w.svc.GetProof(w.ctx, w.outID)
	if err != nil {
		t.Fatalf("get proof: %v", err)
	}
	return proof.LineageStaleness
}

func (w *stalenessWorld) crash(t *testing.T) {
	t.Helper()
	if err := w.store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
}

// ref is the admitted-input shape the 8A input half seals.
func ref(attempt domain.Attempt, receipt domain.AttemptReceipt) domain.AttemptManifestInputRef {
	return domain.AttemptManifestInputRef{AttemptID: attempt.ID, WorkUnitID: attempt.WorkUnitID, ArtifactVersion: receipt.ArtifactVersion}
}

// Falsifier (a): upstream invalidation. A is proven; B, depending on A, is
// proven; C, depending on B, is proven. Rework retains a new result for A at
// a different artifact version. B and transitive C must go stale, B must
// stop being proven, and B's and C's stale proof must stop counting toward
// their criteria.
func TestLineageStalenessFalsifierUpstreamInvalidation(t *testing.T) {
	w := newStalenessWorld(t, t.TempDir())
	defer w.store.Close()
	a1 := w.attempt("wu-a", "att-a1")
	ra1 := w.retain(a1, "a-v1")
	w.prove(a1, w.critA, "a1")
	b1 := w.attempt("wu-b", "att-b1")
	w.sealInputs(b1, ref(a1, ra1))
	rb1 := w.retain(b1, "b-v1")
	w.prove(b1, w.critB, "b1")
	c1 := w.attempt("wu-c", "att-c1")
	w.sealInputs(c1, ref(b1, rb1))
	w.retain(c1, "c-v1")
	w.prove(c1, w.critC, "c1")

	if !w.criterionReady(t, w.critA) || !w.criterionReady(t, w.critB) || !w.criterionReady(t, w.critC) {
		t.Fatalf("setup: all three criteria must start ready")
	}
	if got := w.scheduleEntry(t, "wu-b"); got.State != outcome.WorkUnitScheduleProven {
		t.Fatalf("setup: wu-b must start proven, got %s", got.State)
	}

	// Rework: A is re-executed and retains a DIFFERENT artifact version.
	a2 := w.attempt("wu-a", "att-a2")
	w.retain(a2, "a-v2")

	stale := w.staleness(t)
	if !stale.AttemptStale(b1.ID) {
		t.Fatalf("b1 admitted a1 at the superseded version and must be stale")
	}
	if !stale.AttemptStale(c1.ID) {
		t.Fatalf("c1 consumed b1's stale lineage and must be stale transitively")
	}
	if stale.AttemptStale(a1.ID) || stale.AttemptStale(a2.ID) {
		t.Fatalf("upstream attempts must never be condemned by their own rework")
	}
	if w.criterionReady(t, w.critB) {
		t.Fatalf("b1's stale evidence must stop satisfying crit-b")
	}
	if w.criterionReady(t, w.critC) {
		t.Fatalf("c1's transitively stale evidence must stop satisfying crit-c")
	}
	if !w.criterionReady(t, w.critA) {
		t.Fatalf("a1's own evidence was never superseded and must keep satisfying crit-a")
	}
	entryB := w.scheduleEntry(t, "wu-b")
	if entryB.State == outcome.WorkUnitScheduleProven {
		t.Fatalf("wu-b must stop being scheduled as proven after upstream rework")
	}
	if !entryB.StaleLineage || entryB.StaleLineageDetail == "" {
		t.Fatalf("wu-b schedule entry must explain its stale lineage: %+v", entryB)
	}
	entryC := w.scheduleEntry(t, "wu-c")
	if !entryC.StaleLineage {
		t.Fatalf("wu-c must surface the inherited staleness")
	}
}

// Falsifier (b): no resurrection. After B re-executes against the current
// upstream result, B is proven through the new attempt while the OLD attempt
// stays stale forever.
func TestLineageStalenessFalsifierNoResurrect(t *testing.T) {
	w := newStalenessWorld(t, t.TempDir())
	defer w.store.Close()
	a1 := w.attempt("wu-a", "att-a1")
	ra1 := w.retain(a1, "a-v1")
	b1 := w.attempt("wu-b", "att-b1")
	w.sealInputs(b1, ref(a1, ra1))
	w.retain(b1, "b-v1")
	w.prove(b1, w.critB, "b1")
	a2 := w.attempt("wu-a", "att-a2")
	ra2 := w.retain(a2, "a-v2")

	b2 := w.attempt("wu-b", "att-b2")
	w.sealInputs(b2, ref(a2, ra2))
	w.retain(b2, "b-v2")
	w.prove(b2, w.critB, "b2")

	stale := w.staleness(t)
	if !stale.AttemptStale(b1.ID) {
		t.Fatalf("the old attempt stays stale; re-execution never rewrites its lineage")
	}
	if stale.AttemptStale(b2.ID) {
		t.Fatalf("b2 admitted the current version and must be current")
	}
	if !w.criterionReady(t, w.critB) {
		t.Fatalf("crit-b must be satisfied again through b2's fresh proof")
	}
	if got := w.scheduleEntry(t, "wu-b"); got.State != outcome.WorkUnitScheduleProven || got.StaleLineage {
		t.Fatalf("wu-b must be proven through b2 with no stale flag, got state=%s stale=%v", got.State, got.StaleLineage)
	}
}

// Falsifiers (c) and (d): restart replay and projection determinism. The
// staleness report is a pure derivation over durable rows, so a restarted
// process and a rebuilt projection must reproduce it exactly.
func TestLineageStalenessFalsifierRestartAndProjectionReplay(t *testing.T) {
	dir := t.TempDir()
	w := newStalenessWorld(t, dir)
	a1 := w.attempt("wu-a", "att-a1")
	ra1 := w.retain(a1, "a-v1")
	b1 := w.attempt("wu-b", "att-b1")
	w.sealInputs(b1, ref(a1, ra1))
	w.retain(b1, "b-v1")
	a2 := w.attempt("wu-a", "att-a2")
	w.retain(a2, "a-v2")
	before := w.staleness(t)
	projection1, err := w.svc.GetMissionProjection(w.ctx, w.outID, w.planID)
	if err != nil {
		t.Fatalf("projection: %v", err)
	}
	projection2, err := w.svc.GetMissionProjection(w.ctx, w.outID, w.planID)
	if err != nil {
		t.Fatalf("projection: %v", err)
	}
	staleFlags := func(p outcome.MissionProjection) map[string]bool {
		flags := map[string]bool{}
		for _, node := range p.Nodes {
			flags[node.WorkUnitID] = node.StaleLineage
		}
		return flags
	}
	if !reflect.DeepEqual(staleFlags(projection1), staleFlags(projection2)) {
		t.Fatalf("two projections over the same state disagree on staleness")
	}
	if !staleFlags(projection1)["wu-b"] {
		t.Fatalf("projection must surface wu-b as stale")
	}
	w.crash(t)

	// Restart: a fresh service over the same durable rows.
	reopened, err := sqlite.Open(dir)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	defer reopened.Close()
	w.store = reopened
	w.svc = outcome.New(reopened, func() time.Time { return w.at }).WithAttemptManifests(reopened)
	after := w.staleness(t)
	if !reflect.DeepEqual(before.StaleAttempts, after.StaleAttempts) || !reflect.DeepEqual(before.StaleWorkUnits, after.StaleWorkUnits) {
		t.Fatalf("restart replay must derive the identical stale set\nbefore: %+v\nafter:  %+v", before, after)
	}
}

// Falsifier (e): currency is evaluated at decision time against durable
// state. Two derivations bracketing an upstream acceptance return monotone
// supersets, with no cached staleness verdict surviving across calls on the
// same service.
func TestLineageStalenessFalsifierMonotoneCurrencyAtDecisionTime(t *testing.T) {
	w := newStalenessWorld(t, t.TempDir())
	defer w.store.Close()
	a1 := w.attempt("wu-a", "att-a1")
	ra1 := w.retain(a1, "a-v1")
	b1 := w.attempt("wu-b", "att-b1")
	w.sealInputs(b1, ref(a1, ra1))
	w.retain(b1, "b-v1")
	c1 := w.attempt("wu-c", "att-c1")
	_ = c1

	before := w.staleness(t)
	if len(before.StaleAttempts) != 0 {
		t.Fatalf("nothing is stale before rework: %+v", before.StaleAttempts)
	}
	// Upstream acceptance of the reworked result: A retains a new version.
	a2 := w.attempt("wu-a", "att-a2")
	w.retain(a2, "a-v2")
	after := w.staleness(t)
	for attemptID, facts := range before.StaleAttempts {
		if !reflect.DeepEqual(after.StaleAttempts[attemptID], facts) {
			t.Fatalf("stale sets must grow monotonically; %s lost facts across an acceptance", attemptID)
		}
	}
	if len(after.StaleAttempts) <= len(before.StaleAttempts) {
		t.Fatalf("the derivation after acceptance must be a strict superset once a version moved")
	}
	// The same service instance saw the new durable state on the very next
	// read: the scheduler's currency check runs at decision time.
	entryB := w.scheduleEntry(t, "wu-b")
	if !entryB.StaleLineage {
		t.Fatalf("the scheduler must reflect the moved version immediately, without any restart")
	}
	_ = c1
}

// Falsifier (f): delivery refuses a stale lineage even when its retained
// artifact is complete and every earlier gate was satisfied.
func TestLineageStalenessFalsifierDeliveryRefusesStaleLineage(t *testing.T) {
	w := newStalenessWorld(t, t.TempDir())
	defer w.store.Close()
	artifacts, err := artifactstore.New(artifactstore.Config{Root: filepath.Join(w.dir, "artifacts")})
	if err != nil {
		t.Fatalf("artifact store: %v", err)
	}
	w.svc.WithDelivery(w.store, artifacts)

	a1 := w.attempt("wu-a", "att-a1")
	ra1 := w.retain(a1, "a-v1")
	b1 := w.attempt("wu-b", "att-b1")
	w.sealInputs(b1, ref(a1, ra1))
	rb1 := w.retain(b1, "b-v1")
	a2 := w.attempt("wu-a", "att-a2")
	w.retain(a2, "a-v2")

	_, err = w.svc.RequestDelivery(w.ctx, w.outID, outcome.RequestDeliveryInput{
		AttemptID: b1.ID, ArtifactVersion: rb1.ArtifactVersion, Destination: filepath.Join(w.dir, "delivery"),
		Disposition: domain.DeliveryDraft, RequestKey: "deliver-stale-b1",
	})
	if err == nil {
		t.Fatalf("delivering a stale attempt's artifact must be refused")
	}
	var apiError *apierr.Error
	if !errors.As(err, &apiError) || apiError.Code != "DELIVERY_LINEAGE_STALE" {
		t.Fatalf("refusal must name the stale lineage, got: %v", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// linkEvidence attaches one evidence item to an Attempt at its retained
// artifact version, the join the mission projection publishes.
func (w *stalenessWorld) linkEvidence(attempt domain.Attempt, receipt domain.AttemptReceipt, criterion domain.CriterionID, key string) {
	w.t.Helper()
	evidence := domain.EvidenceItem{
		ID: domain.EvidenceItemID("ev-" + key), OutcomeID: w.outID, ContractRevisionID: w.revID, CriterionID: criterion,
		SubjectType: domain.ProofSubjectAttempt, SubjectID: string(attempt.ID), SubjectRevision: receipt.ArtifactVersion,
		Kind: domain.EvidenceSupporting, SourceType: domain.EvidenceSourceArtifact, SourceRef: "retained://" + key,
		ProducerType: domain.EvidenceProducerTool, ProducerRef: "falsifier",
		Summary: "mission evidence for " + key,
		ContentDigest: string(domain.DigestSHA256([]byte("mission-evidence-" + key))), RequestKey: "mev-rk-" + key,
		RequestFingerprint: string(domain.DigestSHA256([]byte("mev-fp-" + key))), CreatedAt: w.at,
	}
	if err := w.store.CreateEvidenceItem(w.ctx, evidence); err != nil {
		w.t.Fatalf("mission evidence %s: %v", key, err)
	}
}

// Falsifier (g): the mission projection is a proof surface. Evidence whose
// subject Attempt is lineage-stale must not publish as an available evidence
// link; the nodes StaleLineage flag carries that story. Current evidence
// stays visible.
func TestLineageStalenessFalsifierMissionProjectionExcludesStaleEvidence(t *testing.T) {
	dir := t.TempDir()
	w := newStalenessWorld(t, dir)
	a1 := w.attempt("wu-a", "att-a1")
	ra1 := w.retain(a1, "a-v1")
	b1 := w.attempt("wu-b", "att-b1")
	w.sealInputs(b1, ref(a1, ra1))
	rb1 := w.retain(b1, "b-v1")
	w.linkEvidence(b1, rb1, w.critB, "stale-b1")
	a2 := w.attempt("wu-a", "att-a2")
	ra2 := w.retain(a2, "a-v2")
	w.linkEvidence(a2, ra2, w.critA, "current-a2")

	projection, err := w.svc.GetMissionProjection(w.ctx, w.outID, w.planID)
	if err != nil {
		t.Fatalf("get mission projection: %v", err)
	}
	nodes := map[string]outcome.MissionNode{}
	for _, node := range projection.Nodes {
		nodes[node.WorkUnitID] = node
	}
	bNode, ok := nodes["wu-b"]
	if !ok {
		t.Fatalf("wu-b missing from projection")
	}
	if !bNode.StaleLineage {
		t.Fatalf("wu-b node must be marked stale after upstream rework")
	}
	for _, link := range bNode.Links {
		if link.Kind == "evidence" {
			t.Fatalf("stale attempt b1 must publish no evidence link, got %+v", link)
		}
	}
	aNode, ok := nodes["wu-a"]
	if !ok {
		t.Fatalf("wu-a missing from projection")
	}
	foundCurrent := false
	for _, link := range aNode.Links {
		if link.Kind == "evidence" && link.ID == "ev-current-a2" {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Fatalf("current evidence must remain visible on wu-a, links: %+v", aNode.Links)
	}
}

// errManifests injects a custody read failure into the staleness derivation.
type errManifests struct {
	ports.AttemptManifestStore
	err error
}

func (e errManifests) ListAttemptManifestsForOutcome(context.Context, domain.OutcomeID) ([]domain.AttemptManifest, error) {
	return nil, e.err
}

// Falsifier (h): a custody store that cannot be read must fail the
// derivation closed. No surface may publish a silently empty staleness
// report when currency could not be checked.
func TestLineageStalenessFalsifierFailClosedOnCustodyReadError(t *testing.T) {
	dir := t.TempDir()
	w := newStalenessWorld(t, dir)
	a1 := w.attempt("wu-a", "att-a1")
	ra1 := w.retain(a1, "a-v1")
	b1 := w.attempt("wu-b", "att-b1")
	w.sealInputs(b1, ref(a1, ra1))
	w.retain(b1, "b-v1")

	broken := outcome.New(w.store, func() time.Time { return w.at }).
		WithAttemptManifests(errManifests{w.store, errors.New("injected custody read failure")})
	if _, err := broken.GetProof(w.ctx, w.outID); err == nil || !strings.Contains(err.Error(), "staleness walk") {
		t.Fatalf("GetProof with an unreadable custody store must fail closed, got: %v", err)
	}
	if _, err := broken.GetSchedule(w.ctx, w.outID, w.planID); err == nil || !strings.Contains(err.Error(), "staleness walk") {
		t.Fatalf("GetSchedule with an unreadable custody store must fail closed, got: %v", err)
	}
	if _, err := broken.GetMissionProjection(w.ctx, w.outID, w.planID); err == nil || !strings.Contains(err.Error(), "staleness walk") {
		t.Fatalf("GetMissionProjection with an unreadable custody store must fail closed, got: %v", err)
	}
}

// secondReadFailsManifests fails any custody read after the first: durable
// state moved under any code path that derives the staleness walk twice.
type secondReadFailsManifests struct {
	ports.AttemptManifestStore
	calls int
}

func (m *secondReadFailsManifests) ListAttemptManifestsForOutcome(ctx context.Context, outcomeID domain.OutcomeID) ([]domain.AttemptManifest, error) {
	m.calls++
	if m.calls > 1 {
		return nil, errors.New("second custody read: durable state moved under a re-derived walk")
	}
	return m.AttemptManifestStore.ListAttemptManifestsForOutcome(ctx, outcomeID)
}

// Falsifier (i): one projection is one snapshot. The mission projection must
// perform exactly ONE staleness walk - the same report that marks a nodes
// StaleLineage flag also decides its evidence links - so a custody store
// that fails (or moves) on a second read cannot split flag and link into two
// disagreeing points in time.
func TestLineageStalenessFalsifierMissionProjectionIsOneWalk(t *testing.T) {
	dir := t.TempDir()
	w := newStalenessWorld(t, dir)
	a1 := w.attempt("wu-a", "att-a1")
	ra1 := w.retain(a1, "a-v1")
	b1 := w.attempt("wu-b", "att-b1")
	w.sealInputs(b1, ref(a1, ra1))
	rb1 := w.retain(b1, "b-v1")
	w.linkEvidence(b1, rb1, w.critB, "stale-b1")
	a2 := w.attempt("wu-a", "att-a2")
	ra2 := w.retain(a2, "a-v2")
	w.linkEvidence(a2, ra2, w.critA, "current-a2")

	counter := &secondReadFailsManifests{AttemptManifestStore: w.store}
	single := outcome.New(w.store, func() time.Time { return w.at }).WithAttemptManifests(counter)
	projection, err := single.GetMissionProjection(w.ctx, w.outID, w.planID)
	if err != nil {
		t.Fatalf("mission projection must complete with exactly one staleness walk, got: %v", err)
	}
	if counter.calls != 1 {
		t.Fatalf("mission projection derived the staleness walk %d times, want exactly 1", counter.calls)
	}
	nodes := map[string]outcome.MissionNode{}
	for _, node := range projection.Nodes {
		nodes[node.WorkUnitID] = node
	}
	if !nodes["wu-b"].StaleLineage {
		t.Fatalf("wu-b node must be marked stale from the single walk")
	}
	for _, link := range nodes["wu-b"].Links {
		if link.Kind == "evidence" {
			t.Fatalf("the same walk that marks wu-b stale must hide its evidence link, got %+v", link)
		}
	}
}

package outcome

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func TestMissionTopologyFingerprintIsStableForStateAndChangesForTopology(t *testing.T) {
	plan := schedulerPlanFixture()
	first := missionTopologyFingerprint(plan)
	if len(first) != 64 {
		t.Fatalf("fingerprint=%q", first)
	}
	copyPlan := plan
	copyPlan.Summary = "state-only metadata"
	if got := missionTopologyFingerprint(copyPlan); got != first {
		t.Fatalf("non-topology field changed fingerprint: %s -> %s", first, got)
	}
	copyPlan.WorkUnits = append([]domain.WorkUnit(nil), plan.WorkUnits...)
	copyPlan.WorkUnits[1].DependsOn = nil
	if got := missionTopologyFingerprint(copyPlan); got == first {
		t.Fatal("dependency change retained topology fingerprint")
	}
	copyPlan = plan
	copyPlan.ID = "another-plan"
	if got := missionTopologyFingerprint(copyPlan); got == first {
		t.Fatal("authorized plan swap retained topology fingerprint")
	}
}

func TestMissionProjectionShapeForkJoinSerialCustody(t *testing.T) {
	plan := schedulerPlanFixture()
	plan.WorkUnits = []domain.WorkUnit{
		plan.WorkUnits[0],
		{ID: "wu-b", Kind: domain.WorkUnitDirect, Role: domain.WorkUnitRoleImplement, Title: "B", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-a"}, Inputs: []domain.WorkUnitInput{{FromWorkUnitID: "wu-a", Required: "findings", Position: 1}}, CriterionIDs: []domain.CriterionID{"crit-b"}},
		{ID: "wu-c", Kind: domain.WorkUnitDirect, Title: "C", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-a"}, CriterionIDs: []domain.CriterionID{"crit-c"}},
		{ID: "wu-d", Kind: domain.WorkUnitDirect, Title: "Consolidate", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-b", "wu-c"}, CriterionIDs: []domain.CriterionID{"crit-d"}},
	}
	now := time.Unix(100, 0).UTC()
	schedule := ScheduleView{Plan: plan, NextRunnableID: "", CustodyHeldBy: "wu-b", WorkUnits: []WorkUnitScheduleView{
		{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleProven, CriterionReady: map[domain.CriterionID]bool{"crit-a": true}},
		{WorkUnit: plan.WorkUnits[1], State: WorkUnitScheduleExecuting, Attempts: []domain.Attempt{{ID: "att-b", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-b", Number: 2, ContractRevisionNumber: 1, Status: domain.AttemptRunning, CreatedAt: now, UpdatedAt: now}}, CriterionReady: map[domain.CriterionID]bool{"crit-b": false}},
		{WorkUnit: plan.WorkUnits[2], State: WorkUnitScheduleBlocked, BlockedReason: BlockedCustodyHeld, CriterionReady: map[domain.CriterionID]bool{"crit-c": false}},
		{WorkUnit: plan.WorkUnits[3], State: WorkUnitScheduleBlocked, BlockedReason: BlockedAwaitingDependencyProof, BlockingDependencies: []domain.WorkUnitID{"wu-b", "wu-c"}, CriterionReady: map[domain.CriterionID]bool{"crit-d": false}},
	}}
	view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp-project", CurrentRevisionNumber: 1}, schedule, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Nodes) != 4 || len(view.Edges) != 4 {
		t.Fatalf("nodes/edges=%d/%d", len(view.Nodes), len(view.Edges))
	}
	if view.MissionID != "rsp-project" || view.CustodyHeldBy != "wu-b" {
		t.Fatalf("scope/custody=%s/%s", view.MissionID, view.CustodyHeldBy)
	}
	if view.Nodes[1].Role != string(domain.WorkUnitRoleImplement) || len(view.Nodes[1].Inputs) != 1 || view.Nodes[1].Inputs[0].Required != "findings" {
		t.Fatalf("role/input projection=%+v", view.Nodes[1])
	}
	if view.Nodes[2].NextAction != "" || view.Nodes[2].BlockedReason != string(BlockedCustodyHeld) {
		t.Fatalf("parallel branch falsely actionable: %+v", view.Nodes[2])
	}
	if view.Nodes[3].NextAction != "" || strings.Join(stringWorkUnitIDsForTest(view.Nodes[3].BlockingDependencies), ",") != "wu-b,wu-c" {
		t.Fatalf("join=%+v", view.Nodes[3])
	}
}

func stringWorkUnitIDsForTest(in []domain.WorkUnitID) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func TestMissionGenerationsCoverEveryProjectedStateAndAvoidMillisecondCollision(t *testing.T) {
	plan := schedulerPlanFixture()
	now := time.Unix(100, 123456).UTC()
	base := ScheduleView{Plan: plan, NextRunnableID: "wu-a", WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRunnable, CriterionReady: map[domain.CriterionID]bool{"crit-a": false}}, {WorkUnit: plan.WorkUnits[1], State: WorkUnitScheduleBlocked, BlockedReason: BlockedAwaitingDependencyProof, CriterionReady: map[domain.CriterionID]bool{"crit-b": false}}}}
	record := domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}
	first, err := composeMissionProjection(record, base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.WorkUnits = append([]WorkUnitScheduleView(nil), base.WorkUnits...)
	changed.WorkUnits[0] = base.WorkUnits[0]
	changed.WorkUnits[0].CriterionReady = map[domain.CriterionID]bool{"crit-a": true}
	changed.WorkUnits[0].State = WorkUnitScheduleProven
	changed.NextRunnableID = "wu-b"
	second, err := composeMissionProjection(record, changed, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation == second.Generation || first.Nodes[0].Generation == second.Nodes[0].Generation {
		t.Fatal("proof/schedule change retained generation")
	}
	withRef, err := composeMissionProjection(record, base, nil, func(id domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
		return domain.AttemptSessionRef{ID: "ref", AttemptID: id, Seq: 1, SessionID: "s", Harness: domain.HarnessCodex, Mode: domain.SessionModeChat, BoundAt: now}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Add an Attempt without changing the millisecond bucket, then attach a ref.
	base.WorkUnits[0].Attempts = []domain.Attempt{{ID: "a", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Number: 1, ContractRevisionNumber: 1, Status: domain.AttemptRunning, CreatedAt: now, UpdatedAt: now}}
	withoutRef, err := composeMissionProjection(record, base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	withRef, err = composeMissionProjection(record, base, nil, func(id domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
		return domain.AttemptSessionRef{ID: "ref", AttemptID: id, Seq: 1, SessionID: "s", Harness: domain.HarnessCodex, Mode: domain.SessionModeChat, BoundAt: now.Add(time.Nanosecond)}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if withoutRef.Generation == withRef.Generation || withoutRef.Nodes[0].Generation == withRef.Nodes[0].Generation {
		t.Fatal("session ref change inside one millisecond retained generation")
	}
}

func TestMissionUnresolvedApprovalAttentionOutranksRetryableStart(t *testing.T) {
	plan := schedulerPlanFixture()
	now := time.Unix(100, 0).UTC()
	schedule := ScheduleView{Plan: plan, NextRunnableID: "wu-a", WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRetryable, CriterionReady: map[domain.CriterionID]bool{}}}}
	q := domain.NeedsYouQuestion{ID: "q", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Kind: domain.NeedsYouApproval, Reason: "Approve elevated capability", Generation: "q", Status: domain.NeedsYouOpen, UpdatedAt: now}
	view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}, schedule, map[domain.WorkUnitID][]domain.NeedsYouQuestion{"wu-a": {q}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if view.Nodes[0].NextAction != "" {
		t.Fatalf("attention and start rendered as two decisions: nextAction=%q", view.Nodes[0].NextAction)
	}
	if view.Nodes[0].Attention == nil || view.Nodes[0].Attention.Kind != "needs_approval" || view.Nodes[0].Attention.Summary != q.Reason {
		t.Fatalf("attention=%+v", view.Nodes[0].Attention)
	}
}

func TestMissionProjectionExecutionBindingIsOnlyCanonicalApprovedSemantics(t *testing.T) {
	plan := schedulerPlanFixture()
	plan.WorkUnits[0].Provider = domain.HarnessCodex
	plan.WorkUnits[0].ModelSelection = domain.ExecutionBindingModelExplicit
	plan.WorkUnits[0].Model = "gpt-test"
	plan.WorkUnits[1].Provider = domain.HarnessClaudeCode
	plan.WorkUnits[1].ModelSelection = domain.ExecutionBindingModelHistoricalUnbound
	schedule := ScheduleView{Plan: plan, WorkUnits: []WorkUnitScheduleView{
		{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRunnable},
		{WorkUnit: plan.WorkUnits[1], State: WorkUnitScheduleBlocked},
	}}
	view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}, schedule, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := view.Nodes[0].ExecutionBinding; got == nil || got.Provider != "codex" || got.ModelSelection != "explicit" || got.Model != "gpt-test" {
		t.Fatalf("binding=%+v", got)
	}
	if view.Nodes[1].ExecutionBinding != nil {
		t.Fatalf("historical unbound execution projected as planned authority: %+v", view.Nodes[1].ExecutionBinding)
	}
	if view.Nodes[0].Links == nil || len(view.Nodes[0].Links) != 0 {
		t.Fatalf("safe links must be an empty array, got %#v", view.Nodes[0].Links)
	}
}

func TestMissionGenerationIncludesDisplayEnrichmentsButTopologyDoesNot(t *testing.T) {
	plan := schedulerPlanFixture()
	schedule := ScheduleView{Plan: plan, WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRunnable}}}
	view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}, schedule, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseTopology, baseGeneration := view.TopologyFingerprint, missionProjectionGeneration(view)
	view.MissionLabel = "Friendly project"
	view.Nodes[0].Links = append(view.Nodes[0].Links, MissionLink{Kind: "retained_result", ID: "att-1", Label: "Retained result", State: "available"})
	if got := missionProjectionGeneration(view); got == baseGeneration {
		t.Fatal("display enrichment retained projection generation")
	}
	if view.TopologyFingerprint != baseTopology {
		t.Fatal("display enrichment changed topology")
	}
}

func TestMeasuredMissionChangesRequiresFrozenCompleteFullyMeasuredReceipt(t *testing.T) {
	zero, two, frozen := int64(0), int64(2), time.Unix(200, 0).UTC()
	base := domain.AttemptReceipt{AttemptID: "att", ArtifactVersion: "artifact", RetentionState: domain.RetentionRetained, FrozenAt: &frozen, Files: []domain.ArtifactFile{{Additions: &two, Deletions: &zero}}}
	if got := measuredMissionChanges(base); got == nil || got.Additions != 2 || got.FilesChanged != 1 || got.SourceAttemptID != "att" || got.ArtifactVersion != "artifact" {
		t.Fatalf("summary=%+v", got)
	}
	unfrozen := base
	unfrozen.FrozenAt = nil
	if measuredMissionChanges(unfrozen) != nil {
		t.Fatal("unfrozen receipt projected measurements")
	}
	partial := base
	partial.RetentionState = domain.RetentionIncomplete
	if measuredMissionChanges(partial) != nil {
		t.Fatal("partial receipt projected measurements")
	}
	ambiguous := base
	ambiguous.Files[0].Deletions = nil
	if measuredMissionChanges(ambiguous) != nil {
		t.Fatal("partly measured receipt projected summary")
	}
}

func TestMissionNodeGenerationIsIdempotentAfterEnrichment(t *testing.T) {
	node := MissionNode{WorkUnitID: "wu", Links: []MissionLink{{Kind: "evidence", ID: "ev", Label: "Evidence", State: "available"}}}
	first := missionNodeGeneration(node)
	node.Generation = first
	if second := missionNodeGeneration(node); second != first {
		t.Fatalf("generation drifted on recompute: %d -> %d", first, second)
	}
}

func TestMissionEvidenceLabelsNeverProjectFreeText(t *testing.T) {
	unsafe := "private/client/brief.txt https://example.test/?token=secret raw prompt"
	cases := []struct {
		source domain.EvidenceSourceType
		want   string
	}{
		{domain.EvidenceSourceArtifact, "Artifact evidence"},
		{domain.EvidenceSourceDeterministicCheck, "Check evidence"},
		{domain.EvidenceSourceProviderOutput, "Provider evidence"},
		{domain.EvidenceSourceOwnerWalkthrough, "Owner walkthrough evidence"},
		{domain.EvidenceSourceType("future"), "Evidence"},
	}
	for _, tc := range cases {
		item := domain.EvidenceItem{SourceType: tc.source, Summary: unsafe, SourceRef: unsafe}
		got := missionEvidenceLabel(item)
		if got != tc.want {
			t.Fatalf("source=%q label=%q want %q", tc.source, got, tc.want)
		}
		if strings.Contains(got, "private") || strings.Contains(got, "https") || strings.Contains(got, "secret") {
			t.Fatalf("unsafe provenance leaked: %q", got)
		}
	}
}

func TestMissionDocumentLabelIsControlledCopy(t *testing.T) {
	if got := missionDocumentLabel(); got != "Approved documents" {
		t.Fatalf("label=%q", got)
	}
}

func TestMissionResolvedAndStaleQuestionsDoNotGateStart(t *testing.T) {
	plan := schedulerPlanFixture()
	schedule := ScheduleView{Plan: plan, NextRunnableID: "wu-a", WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRunnable}}}
	for _, tc := range []struct {
		name     string
		question domain.NeedsYouQuestion
	}{
		{name: "actual acknowledged record", question: domain.NeedsYouQuestion{ID: "resolved", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Kind: domain.NeedsYouApproval, Generation: "current", Status: domain.NeedsYouAcknowledged}},
		{name: "stale generation record", question: domain.NeedsYouQuestion{ID: "stale", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Kind: domain.NeedsYouApproval, Generation: "new-but-stale", Status: domain.NeedsYouOpen}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attention := missionAttentionByWorkUnit([]domain.NeedsYouQuestion{tc.question}, plan.OutcomeID, plan.ID)
			view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}, schedule, attention, nil)
			if err != nil {
				t.Fatal(err)
			}
			if view.Nodes[0].Attention != nil || view.Nodes[0].NextAction != "start" {
				t.Fatalf("resolved/stale card=%+v", view.Nodes[0])
			}
		})
	}
}

func TestMissionQuestionStatusAndAttemptSessionFences(t *testing.T) {
	for _, status := range []domain.NeedsYouStatus{domain.NeedsYouOpen, domain.NeedsYouAnswerQueued, domain.NeedsYouAnswerSent, domain.NeedsYouDeliveryUnknown} {
		if !missionQuestionUnresolved(domain.NeedsYouQuestion{ID: "g", Generation: "g", Status: status}) {
			t.Fatalf("actionable status filtered: %s", status)
		}
	}
	for _, status := range []domain.NeedsYouStatus{domain.NeedsYouAcknowledged, domain.NeedsYouSuperseded, domain.NeedsYouRefused} {
		if missionQuestionUnresolved(domain.NeedsYouQuestion{ID: "g", Generation: "g", Status: status}) {
			t.Fatalf("resolved status gated: %s", status)
		}
	}
	node := MissionNode{CurrentAttempt: &MissionAttempt{ID: "attempt-current", Session: &MissionSession{SessionID: "session-current"}}}
	current := domain.NeedsYouQuestion{ID: "g", Generation: "g", Status: domain.NeedsYouOpen, AttemptID: "attempt-current", SessionID: "session-current"}
	if !missionQuestionMatchesNode(current, node) {
		t.Fatal("current attempt/session question rejected")
	}
	staleAttempt := current
	staleAttempt.AttemptID = "attempt-old"
	if missionQuestionMatchesNode(staleAttempt, node) {
		t.Fatal("stale attempt question accepted")
	}
	staleSession := current
	staleSession.SessionID = "session-old"
	if missionQuestionMatchesNode(staleSession, node) {
		t.Fatal("stale session question accepted")
	}
}

func TestMissionProjection9PSliceNotePinsHostMappingRule(t *testing.T) {
	if missionProjection9PNote.Consumer != "PR #188 host mapping if it consumes live MissionNode DTOs" {
		t.Fatalf("consumer=%q", missionProjection9PNote.Consumer)
	}
	if missionProjection9PNote.Rule != "suppress nextAction while self-consistent unresolved attention exists" {
		t.Fatalf("rule=%q", missionProjection9PNote.Rule)
	}
}

func TestMissionStoreProjectionFiltersResolvedAndStaleBeforeComposition(t *testing.T) {
	plan := schedulerPlanFixture()
	schedule := ScheduleView{Plan: plan, NextRunnableID: "wu-a", WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRunnable}}}
	record := domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}
	for _, tc := range []struct {
		name          string
		q             domain.NeedsYouQuestion
		wantAttention bool
	}{
		{"current unresolved", domain.NeedsYouQuestion{ID: "open", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Kind: domain.NeedsYouApproval, Generation: "open", Status: domain.NeedsYouOpen}, true},
		{"resolved store record", domain.NeedsYouQuestion{ID: "done", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Kind: domain.NeedsYouApproval, Generation: "done", Status: domain.NeedsYouAcknowledged}, false},
		{"stale generation store record", domain.NeedsYouQuestion{ID: "stale", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Kind: domain.NeedsYouApproval, Generation: "new-but-stale", Status: domain.NeedsYouOpen}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attention := missionAttentionByWorkUnit([]domain.NeedsYouQuestion{tc.q}, plan.OutcomeID, plan.ID)
			view, err := composeMissionProjection(record, schedule, attention, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantAttention {
				if view.Nodes[0].Attention == nil || view.Nodes[0].NextAction != "" {
					t.Fatalf("unresolved did not suppress Start: %+v", view.Nodes[0])
				}
			} else if view.Nodes[0].Attention != nil || view.Nodes[0].NextAction != "start" {
				t.Fatalf("resolved/stale record gated Start: %+v", view.Nodes[0])
			}
		})
	}
}

func TestMissionNewestMatchingQuestionWinsAfterAttemptSessionFence(t *testing.T) {
	plan := schedulerPlanFixture()
	now := time.Unix(500, 0).UTC()
	attempt := domain.Attempt{ID: "attempt-live", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Number: 2, Status: domain.AttemptRunning, UpdatedAt: now}
	schedule := ScheduleView{Plan: plan, NextRunnableID: "wu-a", WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRetryable, Attempts: []domain.Attempt{attempt}}}}
	live := domain.NeedsYouQuestion{ID: "question-live", Generation: "question-live", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", AttemptID: attempt.ID, SessionID: "session-live", Kind: domain.NeedsYouApproval, Status: domain.NeedsYouOpen, UpdatedAt: now}
	newerStale := domain.NeedsYouQuestion{ID: "question-stale", Generation: "question-stale", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", AttemptID: "attempt-old", SessionID: "session-old", Kind: domain.NeedsYouApproval, Status: domain.NeedsYouOpen, UpdatedAt: now.Add(time.Second)}
	latestRef := func(id domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
		return domain.AttemptSessionRef{ID: "ref", AttemptID: id, SessionID: "session-live"}, true, nil
	}
	for _, questions := range [][]domain.NeedsYouQuestion{{live, newerStale}, {newerStale, live}} {
		view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}, schedule, missionAttentionByWorkUnit(questions, plan.OutcomeID, plan.ID), latestRef)
		if err != nil {
			t.Fatal(err)
		}
		if view.Nodes[0].Attention == nil || view.Nodes[0].Attention.QuestionID != live.ID || view.Nodes[0].NextAction != "" {
			t.Fatalf("newer stale record hid live attention: %+v", view.Nodes[0])
		}
	}
}

func TestMissionEqualTimestampQuestionSelectionIsDeterministic(t *testing.T) {
	now := time.Unix(600, 0).UTC()
	node := MissionNode{}
	a := domain.NeedsYouQuestion{ID: "a", Generation: "a", Status: domain.NeedsYouOpen, UpdatedAt: now}
	b := domain.NeedsYouQuestion{ID: "b", Generation: "b", Status: domain.NeedsYouOpen, UpdatedAt: now}
	for _, order := range [][]domain.NeedsYouQuestion{{a, b}, {b, a}} {
		got, ok := newestMissionQuestionForNode(order, node)
		if !ok || got.ID != "b" {
			t.Fatalf("order-dependent selection: %+v %v", got, ok)
		}
	}
}

func TestMissionLegacyUnboundQuestionRequiresSelfGeneration(t *testing.T) {
	node := MissionNode{}
	legacy := domain.NeedsYouQuestion{ID: "legacy", Generation: "legacy", Status: domain.NeedsYouDeliveryUnknown}
	if !missionQuestionMatchesNode(legacy, node) {
		t.Fatal("current legacy unbound question rejected")
	}
	legacy.Generation = "nonblank-stale-generation"
	if missionQuestionMatchesNode(legacy, node) {
		t.Fatal("nonblank stale legacy generation accepted")
	}
}

func TestGetMissionProjectionSQLiteRetainsLiveAttentionWhenNewerQuestionIsStale(t *testing.T) {
	for _, order := range []string{"live-first", "stale-first"} {
		t.Run(order, func(t *testing.T) {
			ctx := context.Background()
			store := sqlitetest.MustOpen(t)
			now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
			projectID := domain.ProjectID("mission-attention-" + order)
			if err := store.UpsertProject(ctx, domain.ProjectRecord{ID: string(projectID), Path: t.TempDir(), DisplayName: "Attention", RegisteredAt: now}); err != nil {
				t.Fatal(err)
			}
			space, err := store.EnsureWorkResponsibilitySpace(ctx, projectID)
			if err != nil {
				t.Fatal(err)
			}
			outcomeID := domain.OutcomeID("out-" + order)
			contract := domain.ContractRevision{ID: domain.ContractRevisionID("cr-" + order), OutcomeID: outcomeID, Number: 1, Goal: "Do the work", SuccessCriteria: []string{"Work is done"}, Review: "Review result"}
			record := domain.Outcome{ID: outcomeID, SpaceID: space.ID, Title: "Attention precedence"}
			if err := store.CreateOutcomeWithContract(ctx, record, contract, "create-"+order); err != nil {
				t.Fatal(err)
			}
			unit := domain.WorkUnit{ID: domain.WorkUnitID("wu-" + order), Kind: domain.WorkUnitDirect, Intent: domain.WorkUnitIntentModifyAndExecute, Title: "Implement", ContractRevisionNumber: 1, OutputSummary: "result", EvidenceChecks: []string{"check"}, VerificationRequirement: "verify", StopConditions: []string{"stop"}, Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault, RequiredCapabilities: []string{domain.CapabilityWorktreeRead}}
			grants := []domain.CapabilityGrant{{ID: domain.CapabilityGrantID("grant-" + order), Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}}
			digest, err := domain.ComputeRunBriefCoreDigest(contract, unit, grants)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := store.AppendPlanRevision(ctx, outcomeID, domain.PlanRevision{ID: domain.PlanRevisionID("plan-" + order), OutcomeID: outcomeID, ContractRevisionNumber: 1, Status: domain.PlanStatusProposed, Summary: "one unit", WorkUnits: []domain.WorkUnit{unit}, Grants: grants, RoutingDecisions: []domain.WorkUnitRoutingDecision{{WorkUnitID: unit.ID, Decision: domain.RoutingDecision{Status: domain.RoutingDecisionRecommended, PolicyVersion: domain.RoutingPolicyVersion, Role: domain.RoutingRoleWorker, RecommendedCandidateID: string(domain.HarnessCodex), RecommendedProvider: string(domain.HarnessCodex), RecommendedModelSelection: domain.ExecutionBindingModelProviderDefault}}}, RunBriefCoreDigest: digest})
			if err != nil {
				t.Fatal(err)
			}
			plan, found, err := store.ApprovePlanRevision(ctx, outcomeID, plan.ID)
			if err != nil || !found {
				t.Fatalf("approve: found=%v err=%v", found, err)
			}
			attempt, err := store.CreateAttemptWithFence(ctx, ports.AttemptAdmission{OutcomeID: outcomeID, PlanRevisionID: plan.ID, WorkUnitID: unit.ID, ContractRevisionNumber: 1, RequestKey: "attempt-" + order, FenceSubject: domain.FenceSubjectForProject(projectID), At: now})
			if err != nil {
				t.Fatal(err)
			}
			session, err := store.CreateSession(ctx, domain.SessionRecord{ProjectID: projectID, Kind: domain.KindWorker, Harness: domain.HarnessCodex, Mode: domain.SessionModeChat, Activity: domain.Activity{State: domain.ActivityActive, LastActivityAt: now}, Metadata: domain.SessionMetadata{Branch: "test", WorkspacePath: t.TempDir()}, CreatedAt: now, UpdatedAt: now})
			if err != nil {
				t.Fatal(err)
			}
			conversation, err := store.CreateConversation(ctx, "conv-"+order, domain.ConversationScopeSession, projectID, session.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.BindAttemptSession(ctx, domain.AttemptSessionRef{AttemptID: attempt.ID, SessionID: string(session.ID), Harness: domain.HarnessCodex, Mode: domain.SessionModeChat, RunBriefCoreDigest: plan.RunBriefCoreDigest, RunBriefCompiledDigest: plan.RunBriefCoreDigest, AdmissionSnapshot: `{}`, BoundAt: now}); err != nil {
				t.Fatal(err)
			}
			live := domain.ConversationActivity{ID: "q-live", Kind: domain.ActivityKindApproval, Status: domain.ActivityStatusPending, Summary: "Current approval", Detail: []byte(`{"decisions":[{"id":"yes"}]}`), RequestID: "req-live", ProviderItemID: "item-live"}
			stale := domain.ConversationActivity{ID: "q-stale", Kind: domain.ActivityKindApproval, Status: domain.ActivityStatusPending, Summary: "Stale approval", Detail: []byte(`{"decisions":[{"id":"yes"}]}`), RequestID: "req-stale", ProviderItemID: "item-stale"}
			insert := func(a domain.ConversationActivity, at time.Time) {
				if err := store.UpsertActivity(ctx, conversation.ID, "", a, at); err != nil {
					t.Fatal(err)
				}
			}
			if order == "live-first" {
				insert(live, now.Add(10*time.Second))
				insert(stale, now.Add(20*time.Second))
			} else {
				insert(stale, now.Add(20*time.Second))
				insert(live, now.Add(10*time.Second))
			}
			stale.Status = domain.ActivityStatusFailed
			insert(stale, now.Add(21*time.Second))
			view, err := New(store, func() time.Time { return now.Add(30 * time.Second) }).WithNeedsYou(store, nil).GetMissionProjection(ctx, outcomeID, plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(view.Nodes) != 1 || view.Nodes[0].Attention == nil || view.Nodes[0].Attention.QuestionID != "q-live" {
				t.Fatalf("projection=%+v", view.Nodes)
			}
			if view.Nodes[0].NextAction == "start" {
				t.Fatalf("live attention did not suppress Start: %+v", view.Nodes[0])
			}
		})
	}
}

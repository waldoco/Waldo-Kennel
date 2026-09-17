package outcome

import (
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
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
		{ID: "wu-b", Kind: domain.WorkUnitDirect, Title: "B", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-a"}, CriterionIDs: []domain.CriterionID{"crit-b"}},
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

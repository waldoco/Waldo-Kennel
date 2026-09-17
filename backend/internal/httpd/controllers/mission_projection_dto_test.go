package controllers

import (
	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"testing"
)

func TestMissionResponseProjectsOnlyTypedCardEnrichments(t *testing.T) {
	view := outcomevc.MissionProjection{MissionLabel: "Project", Nodes: []outcomevc.MissionNode{{WorkUnitID: "wu", PlanRevisionID: "plan", Links: []outcomevc.MissionLink{{Kind: "evidence", ID: "ev", Label: "Proof", State: "available"}}, ExecutionBinding: &outcomevc.MissionExecutionBinding{Provider: "codex", ModelSelection: "provider_default"}, ChangeSummary: &outcomevc.MissionChangeSummary{Additions: 4, Deletions: 2, FilesChanged: 1, SourceAttemptID: "att", ArtifactVersion: "v", MeasurementState: "measured"}}}}
	got := missionResponse(view)
	if got.MissionLabel != "Project" || len(got.Nodes) != 1 {
		t.Fatalf("response=%+v", got)
	}
	node := got.Nodes[0]
	if node.ExecutionBinding == nil || node.ExecutionBinding.Provider != "codex" || node.ChangeSummary == nil || node.ChangeSummary.Additions != 4 {
		t.Fatalf("node=%+v", node)
	}
	if len(node.Links) != 1 || node.Links[0].Kind != "evidence" || node.Links[0].ID != "ev" {
		t.Fatalf("links=%+v", node.Links)
	}
}

func TestMissionResponseUsesEmptySafeLinkArray(t *testing.T) {
	got := missionResponse(outcomevc.MissionProjection{Nodes: []outcomevc.MissionNode{{WorkUnitID: "wu", Links: []outcomevc.MissionLink{}}}})
	if got.Nodes[0].Links == nil || len(got.Nodes[0].Links) != 0 {
		t.Fatalf("links=%#v", got.Nodes[0].Links)
	}
}

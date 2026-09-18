package planning

import (
	"strings"
	"testing"
)

const d = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const e = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func validSchema() Schema {
	contract := Binding{ID: "contract-1", Digest: d}
	plan := Binding{ID: "plan-1", Digest: e}
	unit := WorkUnitBinding{Binding: Binding{ID: "integrate", Digest: d}, Integration: true}
	edges := []DependencyEdge{{FromWorkUnitID: "build-a", ToWorkUnitID: "integrate", ArtifactID: "artifact-a", ArtifactDigest: d}, {FromWorkUnitID: "build-b", ToWorkUnitID: "integrate", ArtifactID: "artifact-b", ArtifactDigest: e}}
	input := InputManifest{Version: SchemaVersion, Contract: contract, Plan: plan, WorkUnit: unit, Sources: []Artifact{{ID: "artifact-a", Digest: d}, {ID: "artifact-b", Digest: e}}, Dependencies: edges}
	inputDigest, _ := input.Digest()
	nodes := []ResultNode{{ID: "integrated", Digest: e}, {ID: "child", Digest: d, ParentID: "integrated"}}
	treeDigest, _ := resultTreeDigest(nodes)
	output := OutputManifest{Version: SchemaVersion, Contract: contract, Plan: plan, WorkUnit: unit, InputManifestDigest: inputDigest, Artifacts: []Artifact{{ID: "integrated", Digest: e}}, ResultTreeDigest: treeDigest, ResultTree: nodes}
	outputDigest, _ := output.Digest()
	return Schema{Version: SchemaVersion, Input: input, InputDigest: inputDigest, Output: output, OutputDigest: outputDigest, Verification: VerificationSnapshot{Version: SchemaVersion, Contract: contract, Plan: plan, IntegrationWorkUnit: unit, OutputManifestDigest: outputDigest, ResultTreeDigest: treeDigest, ReviewDigest: e, Checks: []VerificationCheck{{ID: "check", ResultTreeDigest: treeDigest, DefinitionDigest: e}}}, Dependencies: append([]DependencyEdge(nil), edges...)}
}
func TestSchemaAcceptsFrozenIntegratedResultTree(t *testing.T) {
	if err := validSchema().Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestSchemaFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Schema)
		contains string
	}{
		{"version", func(s *Schema) { s.Version = "planning.v2" }, "version"},
		{"malformed digest", func(s *Schema) { s.Input.Contract.Digest = "ABC" }, "digest"},
		{"missing lineage", func(s *Schema) { s.Output.Plan.ID = "" }, "plan id"},
		{"mismatched contract", func(s *Schema) { s.Output.Contract.ID = "other" }, "output digest"},
		{"stale edge", func(s *Schema) { s.Dependencies[0].ArtifactDigest = e }, "stale or mismatched"},
		{"edge target", func(s *Schema) { s.Input.Dependencies[0].ToWorkUnitID = "other" }, "lineage"},
		{"duplicate source", func(s *Schema) { s.Input.Sources = append(s.Input.Sources, s.Input.Sources[0]) }, "duplicate source"},
		{"duplicate edge", func(s *Schema) { s.Input.Dependencies = append(s.Input.Dependencies, s.Input.Dependencies[0]) }, "duplicate dependency"},
		{"duplicate result node", func(s *Schema) { s.Output.ResultTree = append(s.Output.ResultTree, s.Output.ResultTree[0]) }, "duplicate result"},
		{"fan in not integration", func(s *Schema) {
			s.Input.WorkUnit.Integration = false
			s.Output.WorkUnit.Integration = false
			s.Verification.IntegrationWorkUnit.Integration = false
		}, "fan-in"},
		{"verification not integration", func(s *Schema) { s.Verification.IntegrationWorkUnit.Integration = false }, "explicit integration"},
		{"no checks", func(s *Schema) { s.Verification.Checks = nil }, "requires checks"},
		{"check different tree", func(s *Schema) { s.Verification.Checks[0].ResultTreeDigest = e }, "frozen integrated result tree"},
		{"review missing", func(s *Schema) { s.Verification.ReviewDigest = "" }, "review digest"},
		{"snapshot stale output", func(s *Schema) { s.Verification.OutputManifestDigest = d }, "lineage"},
		{"tree missing parent", func(s *Schema) { s.Output.ResultTree[1].ParentID = "missing" }, "missing parent"},
		{"two roots", func(s *Schema) { s.Output.ResultTree[1].ParentID = "" }, "one root"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := validSchema()
			tc.mutate(&s)
			err := s.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("err=%v, want containing %q", err, tc.contains)
			}
		})
	}
}
func TestDependencyComparisonIsOrderIndependent(t *testing.T) {
	s := validSchema()
	s.Dependencies[0], s.Dependencies[1] = s.Dependencies[1], s.Dependencies[0]
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestResultTreeRejectsDisconnectedCycles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Schema)
	}{
		{"self-parent", func(s *Schema) { s.Output.ResultTree[1].ParentID = "child" }},
		{"two-node-cycle", func(s *Schema) {
			s.Output.ResultTree = append(s.Output.ResultTree, ResultNode{ID: "other", Digest: d, ParentID: "child"})
			s.Output.ResultTree[1].ParentID = "other"
		}},
		{"disconnected-cycle", func(s *Schema) {
			s.Output.ResultTree = append(s.Output.ResultTree, ResultNode{ID: "x", Digest: d, ParentID: "y"}, ResultNode{ID: "y", Digest: e, ParentID: "x"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := validSchema()
			tc.mutate(&s)
			digest, _ := resultTreeDigest(s.Output.ResultTree)
			s.Output.ResultTreeDigest = digest
			s.Verification.ResultTreeDigest = digest
			s.Verification.Checks[0].ResultTreeDigest = digest
			if err := s.Validate(); err == nil {
				t.Fatal("malformed tree accepted")
			}
		})
	}
}
func TestCanonicalContentAddressingRejectsUnhashedMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Schema)
	}{
		{"source content", func(s *Schema) {
			s.Input.Sources[0].Digest = e
			s.Input.Dependencies[0].ArtifactDigest = e
			s.Dependencies[0].ArtifactDigest = e
		}},
		{"result node content", func(s *Schema) { s.Output.ResultTree[1].Digest = e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := validSchema()
			tc.mutate(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("unhashed mutation accepted")
			}
		})
	}
}
func TestExactArtifactBindings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Schema)
	}{
		{"nonexistent dependency artifact", func(s *Schema) { s.Input.Dependencies[0].ArtifactID = "missing" }},
		{"different dependency digest", func(s *Schema) { s.Input.Dependencies[0].ArtifactDigest = e }},
		{"nonexistent result artifact", func(s *Schema) { s.Output.Artifacts[0].ID = "missing" }},
		{"different result artifact digest", func(s *Schema) { s.Output.Artifacts[0].Digest = d }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := validSchema()
			tc.mutate(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("mismatched artifact accepted")
			}
		})
	}
}
func TestNestedVersionsAndEmptyIdentitiesFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Schema)
	}{
		{"input version", func(s *Schema) { s.Input.Version = "x" }}, {"output version", func(s *Schema) { s.Output.Version = "x" }}, {"verification version", func(s *Schema) { s.Verification.Version = "x" }},
		{"uppercase digest", func(s *Schema) { s.Input.Contract.Digest = strings.ToUpper(d) }}, {"duplicate checks", func(s *Schema) { s.Verification.Checks = append(s.Verification.Checks, s.Verification.Checks[0]) }},
		{"empty artifact id", func(s *Schema) { s.Input.Sources[0].ID = "" }}, {"missing artifact digest", func(s *Schema) { s.Input.Sources[0].Digest = "" }}, {"empty node id", func(s *Schema) { s.Output.ResultTree[0].ID = "" }}, {"missing node digest", func(s *Schema) { s.Output.ResultTree[0].Digest = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := validSchema()
			tc.mutate(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("invalid schema accepted")
			}
		})
	}
}

func TestDependencyArtifactBindingsRequireExactMultiplicityOneBijection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*Schema)
		wantErr bool
	}{
		{"valid bijective fixture accepts", func(*Schema) {}, false},
		{"duplicate binding rejects", func(s *Schema) { s.Input.Dependencies = append(s.Input.Dependencies, s.Input.Dependencies[0]) }, true},
		{"omission masked by duplicate count equality rejects", func(s *Schema) {
			s.Input.Dependencies[1].ArtifactID = s.Input.Dependencies[0].ArtifactID
			s.Input.Dependencies[1].ArtifactDigest = s.Input.Dependencies[0].ArtifactDigest
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := validSchema()
			tc.mutate(&s)
			err := s.Input.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("invalid artifact binding accepted")
			}
			if !tc.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}

package planning

import (
	"encoding/json"
	"reflect"
	"testing"
)

func compileFixture() CompileInput {
	return CompileInput{Contract: Binding{ID: "contract", Digest: d}, Plan: Binding{ID: "plan", Digest: e}, WorkUnit: WorkUnitBinding{Binding: Binding{ID: "integrate", Digest: d}, Integration: true}, Sources: []Artifact{{ID: "b", Digest: e}, {ID: "a", Digest: d}}, Dependencies: []DependencyEdge{{FromWorkUnitID: "build-b", ToWorkUnitID: "integrate", ArtifactID: "b", ArtifactDigest: e}, {FromWorkUnitID: "build-a", ToWorkUnitID: "integrate", ArtifactID: "a", ArtifactDigest: d}}, ResultTree: []ResultNode{{ID: "leaf", Digest: d, ParentID: "root"}, {ID: "root", Digest: e}}, Artifacts: []Artifact{{ID: "root", Digest: e}}, ReviewDigest: d, Checks: []CompileCheck{{ID: "z", DefinitionDigest: e}, {ID: "a", DefinitionDigest: d}}}
}
func TestCompileDerivesClosedLineage(t *testing.T) {
	in := compileFixture()
	got, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != SchemaVersion || got.Input.Version != SchemaVersion || got.Output.Version != SchemaVersion || got.Verification.Version != SchemaVersion {
		t.Fatalf("versions=%+v", got)
	}
	if got.Output.Contract != in.Contract || got.Output.Plan != in.Plan || got.Output.WorkUnit != in.WorkUnit || got.Output.InputManifestDigest != got.InputDigest {
		t.Fatal("output lineage not derived")
	}
	if got.Verification.Contract != in.Contract || got.Verification.Plan != in.Plan || got.Verification.IntegrationWorkUnit != in.WorkUnit || got.Verification.OutputManifestDigest != got.OutputDigest || got.Verification.ResultTreeDigest != got.Output.ResultTreeDigest {
		t.Fatal("verification lineage not derived")
	}
	if !reflect.DeepEqual(got.Dependencies, got.Input.Dependencies) {
		t.Fatal("dependency mirror differs")
	}
	for _, c := range got.Verification.Checks {
		if c.ResultTreeDigest != got.Output.ResultTreeDigest {
			t.Fatal("check tree binding differs")
		}
	}
}
func TestCompileDeterministicAcrossPermutations(t *testing.T) {
	a := compileFixture()
	b := compileFixture()
	b.Sources[0], b.Sources[1] = b.Sources[1], b.Sources[0]
	b.Dependencies[0], b.Dependencies[1] = b.Dependencies[1], b.Dependencies[0]
	b.ResultTree[0], b.ResultTree[1] = b.ResultTree[1], b.ResultTree[0]
	b.Checks[0], b.Checks[1] = b.Checks[1], b.Checks[0]
	x, err := Compile(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := Compile(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(x, y) {
		t.Fatalf("outputs differ")
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	if string(xb) != string(yb) {
		t.Fatal("JSON differs")
	}
}
func TestCompileReturnedSlicesDoNotAliasInput(t *testing.T) {
	in := compileFixture()
	got, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Sources[0].ID = "mutated"
	in.Dependencies[0].ArtifactID = "mutated"
	in.ResultTree[0].ID = "mutated"
	in.Artifacts[0].ID = "mutated"
	in.Checks[0].ID = "mutated"
	if err := got.Validate(); err != nil {
		t.Fatalf("compiled schema changed through input alias: %v", err)
	}
}
func TestCompileFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*CompileInput)
	}{
		{"empty contract", func(in *CompileInput) { in.Contract.ID = "" }}, {"uppercase digest", func(in *CompileInput) {
			in.Plan.Digest = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		}}, {"duplicate source", func(in *CompileInput) { in.Sources = append(in.Sources, in.Sources[0]) }}, {"duplicate count masking", func(in *CompileInput) {
			in.Dependencies[1].ArtifactID = in.Dependencies[0].ArtifactID
			in.Dependencies[1].ArtifactDigest = in.Dependencies[0].ArtifactDigest
		}}, {"omitted source", func(in *CompileInput) { in.Dependencies = in.Dependencies[:1] }}, {"stale edge digest", func(in *CompileInput) { in.Dependencies[0].ArtifactDigest = d }}, {"fan in without integration", func(in *CompileInput) { in.WorkUnit.Integration = false }}, {"missing root", func(in *CompileInput) { in.ResultTree[1].ParentID = "leaf" }}, {"disconnected cycle", func(in *CompileInput) {
			in.ResultTree = append(in.ResultTree, ResultNode{ID: "x", Digest: d, ParentID: "y"}, ResultNode{ID: "y", Digest: e, ParentID: "x"})
		}}, {"artifact missing node", func(in *CompileInput) { in.Artifacts[0].ID = "missing" }}, {"artifact digest mismatch", func(in *CompileInput) { in.Artifacts[0].Digest = d }}, {"missing review", func(in *CompileInput) { in.ReviewDigest = "" }}, {"missing checks", func(in *CompileInput) { in.Checks = nil }}, {"duplicate checks", func(in *CompileInput) { in.Checks = append(in.Checks, in.Checks[0]) }}, {"malformed check", func(in *CompileInput) { in.Checks[0].DefinitionDigest = "bad" }}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := compileFixture()
			tc.mutate(&in)
			got, err := Compile(in)
			if err == nil {
				t.Fatalf("accepted: %+v", got)
			}
			if !reflect.DeepEqual(got, Schema{}) {
				t.Fatalf("nonzero schema on error: %+v", got)
			}
		})
	}
}
func TestCompileRejectsIndependentlyCheckableUnhashedMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*CompileInput)
	}{
		{"source without matching edge", func(in *CompileInput) { in.Sources[0].Digest = d }},
		{"artifact-bearing node without matching artifact", func(in *CompileInput) { in.ResultTree[1].Digest = d }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := compileFixture()
			tc.mutate(&in)
			if got, err := Compile(in); err == nil || !reflect.DeepEqual(got, Schema{}) {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}

// planning.v1 treats a non-artifact ResultNode digest as opaque evidence: the
// compiler does not invent what it hashes. Its integrity is the recomputed
// canonical ResultTreeDigest and OutputDigest; independent semantic validation
// of that opaque evidence would require a later schema version.
func TestCompileRecomputesAggregateDigestsForOpaqueNonArtifactNode(t *testing.T) {
	in := compileFixture()
	before, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	in.ResultTree[0].Digest = e
	after, err := Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	if before.Output.ResultTreeDigest == after.Output.ResultTreeDigest || before.OutputDigest == after.OutputDigest {
		t.Fatal("opaque node mutation did not change aggregate digests")
	}
}

package planning

import (
	"fmt"
	"sort"
)

// CompileCheck is the caller-authored part of a verification check. Compile
// derives its immutable result-tree binding.
type CompileCheck struct {
	ID               string
	DefinitionDigest string
}

// CompileInput contains only authoritative claims. Compile derives all
// versions, aggregate digests and repeated lineage fields.
type CompileInput struct {
	Contract     Binding
	Plan         Binding
	WorkUnit     WorkUnitBinding
	Sources      []Artifact
	Dependencies []DependencyEdge
	ResultTree   []ResultNode
	Artifacts    []Artifact
	ReviewDigest string
	Checks       []CompileCheck
}

// Compile builds one closed handoff Schema. It is pure: inputs are copied,
// canonicalized and validated without repair or normalization.
func Compile(in CompileInput) (Schema, error) {
	sources := append([]Artifact(nil), in.Sources...)
	edges := append([]DependencyEdge(nil), in.Dependencies...)
	nodes := append([]ResultNode(nil), in.ResultTree...)
	artifacts := append([]Artifact(nil), in.Artifacts...)
	checks := append([]CompileCheck(nil), in.Checks...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID < sources[j].ID })
	sort.Slice(edges, func(i, j int) bool { return edgeKey(edges[i]) < edgeKey(edges[j]) })
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].ID < artifacts[j].ID })
	sort.Slice(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })

	input := InputManifest{Version: SchemaVersion, Contract: in.Contract, Plan: in.Plan, WorkUnit: in.WorkUnit, Sources: sources, Dependencies: edges}
	inputDigest, err := input.Digest()
	if err != nil {
		return Schema{}, fmt.Errorf("compile input manifest: %w", err)
	}
	treeDigest, err := resultTreeDigest(nodes)
	if err != nil {
		return Schema{}, fmt.Errorf("compile result tree: %w", err)
	}
	output := OutputManifest{Version: SchemaVersion, Contract: in.Contract, Plan: in.Plan, WorkUnit: in.WorkUnit, InputManifestDigest: inputDigest, Artifacts: artifacts, ResultTreeDigest: treeDigest, ResultTree: nodes}
	outputDigest, err := output.Digest()
	if err != nil {
		return Schema{}, fmt.Errorf("compile output manifest: %w", err)
	}
	verificationChecks := make([]VerificationCheck, len(checks))
	for i, check := range checks {
		verificationChecks[i] = VerificationCheck{ID: check.ID, ResultTreeDigest: treeDigest, DefinitionDigest: check.DefinitionDigest}
	}
	schema := Schema{Version: SchemaVersion, Input: input, InputDigest: inputDigest, Output: output, OutputDigest: outputDigest, Verification: VerificationSnapshot{Version: SchemaVersion, Contract: in.Contract, Plan: in.Plan, IntegrationWorkUnit: in.WorkUnit, OutputManifestDigest: outputDigest, ResultTreeDigest: treeDigest, ReviewDigest: in.ReviewDigest, Checks: verificationChecks}, Dependencies: append([]DependencyEdge(nil), edges...)}
	if err := schema.Validate(); err != nil {
		return Schema{}, fmt.Errorf("compile planning schema: %w", err)
	}
	return schema, nil
}

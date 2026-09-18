// Package planning defines the closed, content-addressed handoff schema used
// between planning stages. It contains validation only; it derives no state.
package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const SchemaVersion = "planning.v1"

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Binding struct{ ID, Digest string }
type Artifact struct{ ID, Digest string }
type DependencyEdge struct{ FromWorkUnitID, ToWorkUnitID, ArtifactID, ArtifactDigest string }
type WorkUnitBinding struct {
	Binding
	Integration bool
}

type InputManifest struct {
	Version        string
	Contract, Plan Binding
	WorkUnit       WorkUnitBinding
	Sources        []Artifact
	Dependencies   []DependencyEdge
}

type ResultNode struct{ ID, Digest, ParentID string }
type OutputManifest struct {
	Version             string
	Contract, Plan      Binding
	WorkUnit            WorkUnitBinding
	InputManifestDigest string
	Artifacts           []Artifact
	ResultTreeDigest    string
	ResultTree          []ResultNode
}

type VerificationCheck struct{ ID, ResultTreeDigest, DefinitionDigest string }
type VerificationSnapshot struct {
	Version                                              string
	Contract, Plan                                       Binding
	IntegrationWorkUnit                                  WorkUnitBinding
	OutputManifestDigest, ResultTreeDigest, ReviewDigest string
	Checks                                               []VerificationCheck
}

type Schema struct {
	Version      string
	Input        InputManifest
	InputDigest  string
	Output       OutputManifest
	OutputDigest string
	Verification VerificationSnapshot
	Dependencies []DependencyEdge
}

func validBinding(name string, b Binding) error {
	if strings.TrimSpace(b.ID) == "" {
		return fmt.Errorf("%s id is required", name)
	}
	if !digestPattern.MatchString(b.Digest) {
		return fmt.Errorf("%s digest must be lowercase sha256", name)
	}
	return nil
}
func validDigest(name, d string) error {
	if !digestPattern.MatchString(d) {
		return fmt.Errorf("%s must be lowercase sha256", name)
	}
	return nil
}
func uniqueIDs(kind string, ids []string) error {
	seen := map[string]struct{}{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("%s id is required", kind)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate %s id %q", kind, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
func edgeKey(e DependencyEdge) string {
	return e.FromWorkUnitID + "\x00" + e.ToWorkUnitID + "\x00" + e.ArtifactID
}

func hashCanonical(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func (m InputManifest) Digest() (string, error) {
	copy := m
	copy.Sources = append([]Artifact(nil), m.Sources...)
	copy.Dependencies = append([]DependencyEdge(nil), m.Dependencies...)
	sort.Slice(copy.Sources, func(i, j int) bool { return copy.Sources[i].ID < copy.Sources[j].ID })
	sort.Slice(copy.Dependencies, func(i, j int) bool { return edgeKey(copy.Dependencies[i]) < edgeKey(copy.Dependencies[j]) })
	return hashCanonical(copy)
}
func resultTreeDigest(nodes []ResultNode) (string, error) {
	copy := append([]ResultNode(nil), nodes...)
	sort.Slice(copy, func(i, j int) bool { return copy[i].ID < copy[j].ID })
	return hashCanonical(copy)
}
func (m OutputManifest) Digest() (string, error) {
	copy := m
	copy.Artifacts = append([]Artifact(nil), m.Artifacts...)
	copy.ResultTree = append([]ResultNode(nil), m.ResultTree...)
	sort.Slice(copy.Artifacts, func(i, j int) bool { return copy.Artifacts[i].ID < copy.Artifacts[j].ID })
	sort.Slice(copy.ResultTree, func(i, j int) bool { return copy.ResultTree[i].ID < copy.ResultTree[j].ID })
	return hashCanonical(copy)
}

func (m InputManifest) Validate() error {
	if m.Version != SchemaVersion {
		return fmt.Errorf("unsupported input manifest version %q", m.Version)
	}
	if err := validBinding("contract", m.Contract); err != nil {
		return err
	}
	if err := validBinding("plan", m.Plan); err != nil {
		return err
	}
	if err := validBinding("work unit", m.WorkUnit.Binding); err != nil {
		return err
	}
	ids := make([]string, len(m.Sources))
	for i, a := range m.Sources {
		ids[i] = a.ID
		if err := validDigest("source artifact digest", a.Digest); err != nil {
			return err
		}
	}
	if err := uniqueIDs("source artifact", ids); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	from := map[string]struct{}{}
	for _, e := range m.Dependencies {
		if strings.TrimSpace(e.FromWorkUnitID) == "" || e.ToWorkUnitID != m.WorkUnit.ID || strings.TrimSpace(e.ArtifactID) == "" {
			return fmt.Errorf("dependency edge has missing or mismatched lineage")
		}
		if err := validDigest("dependency artifact digest", e.ArtifactDigest); err != nil {
			return err
		}
		k := edgeKey(e)
		if _, ok := seen[k]; ok {
			return fmt.Errorf("duplicate dependency edge")
		}
		seen[k] = struct{}{}
		from[e.FromWorkUnitID] = struct{}{}
	}
	if len(from) > 1 && !m.WorkUnit.Integration {
		return fmt.Errorf("fan-in requires an explicit integration work unit")
	}
	sources := map[string]string{}
	for _, a := range m.Sources {
		sources[a.ID] = a.Digest
	}
	consumed := map[string]bool{}
	for _, edge := range m.Dependencies {
		digest, exists := sources[edge.ArtifactID]
		if !exists || digest != edge.ArtifactDigest {
			return fmt.Errorf("dependency edge artifact does not match source artifact")
		}
		if consumed[edge.ArtifactID] {
			return fmt.Errorf("source artifact %q is consumed more than once", edge.ArtifactID)
		}
		consumed[edge.ArtifactID] = true
	}
	if len(consumed) != len(sources) {
		return fmt.Errorf("dependency edges omit a source artifact")
	}
	return nil
}
func (m OutputManifest) Validate() error {
	if m.Version != SchemaVersion {
		return fmt.Errorf("unsupported output manifest version %q", m.Version)
	}
	if err := validBinding("contract", m.Contract); err != nil {
		return err
	}
	if err := validBinding("plan", m.Plan); err != nil {
		return err
	}
	if err := validBinding("work unit", m.WorkUnit.Binding); err != nil {
		return err
	}
	if err := validDigest("input manifest digest", m.InputManifestDigest); err != nil {
		return err
	}
	if err := validDigest("result tree digest", m.ResultTreeDigest); err != nil {
		return err
	}
	ids := make([]string, len(m.Artifacts))
	for i, a := range m.Artifacts {
		ids[i] = a.ID
		if err := validDigest("output artifact digest", a.Digest); err != nil {
			return err
		}
	}
	if err := uniqueIDs("output artifact", ids); err != nil {
		return err
	}
	nids := make([]string, len(m.ResultTree))
	roots := 0
	known := map[string]struct{}{}
	for i, n := range m.ResultTree {
		nids[i] = n.ID
		if err := validDigest("result node digest", n.Digest); err != nil {
			return err
		}
		known[n.ID] = struct{}{}
	}
	if err := uniqueIDs("result node", nids); err != nil {
		return err
	}
	for _, n := range m.ResultTree {
		if n.ParentID == "" {
			roots++
		} else if _, ok := known[n.ParentID]; !ok {
			return fmt.Errorf("result node %q has missing parent", n.ID)
		}
	}
	if len(m.ResultTree) == 0 || roots != 1 {
		return fmt.Errorf("result tree must have exactly one root")
	}
	children := map[string][]string{}
	root := ""
	for _, n := range m.ResultTree {
		if n.ParentID == "" {
			root = n.ID
		} else {
			if n.ParentID == n.ID {
				return fmt.Errorf("result node %q is self-parented", n.ID)
			}
			children[n.ParentID] = append(children[n.ParentID], n.ID)
		}
	}
	visited := map[string]bool{}
	active := map[string]bool{}
	var walk func(string) error
	walk = func(id string) error {
		if active[id] {
			return fmt.Errorf("result tree contains a cycle")
		}
		if visited[id] {
			return fmt.Errorf("result node %q visited more than once", id)
		}
		active[id] = true
		visited[id] = true
		for _, child := range children[id] {
			if err := walk(child); err != nil {
				return err
			}
		}
		active[id] = false
		return nil
	}
	if err := walk(root); err != nil {
		return err
	}
	if len(visited) != len(m.ResultTree) {
		return fmt.Errorf("result tree contains disconnected nodes or a cycle")
	}
	computed, err := resultTreeDigest(m.ResultTree)
	if err != nil {
		return err
	}
	if computed != m.ResultTreeDigest {
		return fmt.Errorf("result tree digest does not match canonical content")
	}
	nodes := map[string]string{}
	for _, n := range m.ResultTree {
		nodes[n.ID] = n.Digest
	}
	for _, a := range m.Artifacts {
		if nodes[a.ID] != a.Digest {
			return fmt.Errorf("output artifact does not match result tree node")
		}
	}
	return nil
}
func (v VerificationSnapshot) Validate() error {
	if v.Version != SchemaVersion {
		return fmt.Errorf("unsupported verification snapshot version %q", v.Version)
	}
	if err := validBinding("contract", v.Contract); err != nil {
		return err
	}
	if err := validBinding("plan", v.Plan); err != nil {
		return err
	}
	if err := validBinding("integration work unit", v.IntegrationWorkUnit.Binding); err != nil {
		return err
	}
	if !v.IntegrationWorkUnit.Integration {
		return fmt.Errorf("verification must bind an explicit integration work unit")
	}
	if err := validDigest("output manifest digest", v.OutputManifestDigest); err != nil {
		return err
	}
	if err := validDigest("verification result tree digest", v.ResultTreeDigest); err != nil {
		return err
	}
	if err := validDigest("review digest", v.ReviewDigest); err != nil {
		return err
	}
	if len(v.Checks) == 0 {
		return fmt.Errorf("verification requires checks bound to the integrated result tree")
	}
	ids := make([]string, len(v.Checks))
	for i, c := range v.Checks {
		ids[i] = c.ID
		if c.ResultTreeDigest != v.ResultTreeDigest {
			return fmt.Errorf("check %q is not bound to the frozen integrated result tree", c.ID)
		}
		if err := validDigest("check definition digest", c.DefinitionDigest); err != nil {
			return err
		}
	}
	return uniqueIDs("verification check", ids)
}
func (s Schema) Validate() error {
	if s.Version != SchemaVersion {
		return fmt.Errorf("unsupported schema version %q", s.Version)
	}
	if err := s.Input.Validate(); err != nil {
		return fmt.Errorf("input: %w", err)
	}
	if err := validDigest("input digest", s.InputDigest); err != nil {
		return err
	}
	computedInput, err := s.Input.Digest()
	if err != nil {
		return err
	}
	if computedInput != s.InputDigest {
		return fmt.Errorf("input digest does not match canonical content")
	}
	if err := s.Output.Validate(); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	if err := validDigest("output digest", s.OutputDigest); err != nil {
		return err
	}
	computedOutput, err := s.Output.Digest()
	if err != nil {
		return err
	}
	if computedOutput != s.OutputDigest {
		return fmt.Errorf("output digest does not match canonical content")
	}
	if err := s.Verification.Validate(); err != nil {
		return fmt.Errorf("verification: %w", err)
	}
	if s.Output.Contract != s.Input.Contract || s.Output.Plan != s.Input.Plan || s.Output.WorkUnit != s.Input.WorkUnit || s.Output.InputManifestDigest != s.InputDigest {
		return fmt.Errorf("output lineage does not match input")
	}
	if s.Verification.Contract != s.Output.Contract || s.Verification.Plan != s.Output.Plan || s.Verification.IntegrationWorkUnit != s.Output.WorkUnit || s.Verification.OutputManifestDigest != s.OutputDigest || s.Verification.ResultTreeDigest != s.Output.ResultTreeDigest {
		return fmt.Errorf("verification lineage does not match frozen output")
	}
	want := append([]DependencyEdge(nil), s.Input.Dependencies...)
	got := append([]DependencyEdge(nil), s.Dependencies...)
	sort.Slice(want, func(i, j int) bool { return edgeKey(want[i]) < edgeKey(want[j]) })
	sort.Slice(got, func(i, j int) bool { return edgeKey(got[i]) < edgeKey(got[j]) })
	if len(want) != len(got) {
		return fmt.Errorf("dependency edges are stale or mismatched")
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Errorf("dependency edges are stale or mismatched")
		}
	}
	return nil
}

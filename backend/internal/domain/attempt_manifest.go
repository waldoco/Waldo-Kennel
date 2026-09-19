package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AttemptManifestHalf names which custody boundary one manifest record binds.
//
// The halves are written at different times by different components - admission
// writes the input half, custody close writes the output half - so each is its
// own sealed record rather than a row that gets updated.
type AttemptManifestHalf string

const (
	// AttemptManifestInput records the exact execution inputs an Attempt was
	// admitted with: lineage, frozen digests, predecessor artifact versions,
	// approved documents, and the authorized checks.
	AttemptManifestInput AttemptManifestHalf = "input"
	// AttemptManifestOutput records what custody close retained: the immutable
	// artifact version and retention truth of the Attempt's result.
	AttemptManifestOutput AttemptManifestHalf = "output"
)

// Valid reports whether h is a known manifest half.
func (h AttemptManifestHalf) Valid() bool {
	return h == AttemptManifestInput || h == AttemptManifestOutput
}

// AttemptManifestInputRef is one predecessor result an Attempt was admitted
// with. The artifact version is part of the reference, not something resolved
// later: admission authorized these exact bytes, and re-resolving "the latest
// result of that WorkUnit" could hand the successor work the owner never
// authorized.
type AttemptManifestInputRef struct {
	AttemptID       AttemptID  `json:"attemptId"`
	WorkUnitID      WorkUnitID `json:"workUnitId"`
	ArtifactVersion string     `json:"artifactVersion"`
}

// Validate checks one admitted input reference.
func (r AttemptManifestInputRef) Validate() error {
	if r.AttemptID.IsZero() {
		return fmt.Errorf("input reference attempt id is required")
	}
	if r.WorkUnitID.IsZero() {
		return fmt.Errorf("input reference work unit id is required")
	}
	if !SHA256Digest(r.ArtifactVersion).Valid() {
		return fmt.Errorf("input reference %s artifact version is not a content digest", r.AttemptID)
	}
	return nil
}

// AttemptManifestRepo binds one materialized repo worktree of a
// workspace-project Attempt: canonical identity plus the exact base revision
// the Attempt started from. A workspace source tree without a resolved base
// is not custody evidence, so the base is required.
type AttemptManifestRepo struct {
	RepoName     string `json:"repoName"`
	WorktreePath string `json:"worktreePath"`
	BaseSHA      string `json:"baseSha"`
	BaseRef      string `json:"baseRef,omitempty"`
}

// Validate checks one bound repo source tree.
func (r AttemptManifestRepo) Validate() error {
	if strings.TrimSpace(r.RepoName) == "" {
		return fmt.Errorf("repo name is required")
	}
	if strings.TrimSpace(r.WorktreePath) == "" {
		return fmt.Errorf("repo worktree path is required")
	}
	if strings.TrimSpace(r.BaseSHA) == "" {
		return fmt.Errorf("repo %s base revision is required", r.RepoName)
	}
	return nil
}

// AttemptManifestCheck is one owner-authorized check bound at admission, with
// the exact argv the daemon may run. Rebinding happens only through a new
// Plan revision, never by editing this record.
type AttemptManifestCheck struct {
	ID             ApprovedCheckID `json:"id"`
	CriterionID    CriterionID     `json:"criterionId"`
	Argv           []string        `json:"argv"`
	TimeoutSeconds int64           `json:"timeoutSeconds"`
}

// Validate checks one bound check against the ApprovedCheck invariants.
func (c AttemptManifestCheck) Validate() error {
	return ApprovedCheck{ID: c.ID, CriterionID: c.CriterionID, Argv: c.Argv, TimeoutSeconds: c.TimeoutSeconds}.Validate()
}

// AttemptManifestDocument binds the approved supplied-document snapshot an
// Attempt stages from, by revision and digest rather than by path.
type AttemptManifestDocument struct {
	ContextID DocumentContextID `json:"contextId"`
	Revision  int64             `json:"revision"`
	Digest    string            `json:"digest"`
}

// Validate checks the bound document snapshot reference.
func (d AttemptManifestDocument) Validate() error {
	if d.ContextID.IsZero() {
		return fmt.Errorf("document context id is required")
	}
	if d.Revision < 1 {
		return fmt.Errorf("document context revision must be positive")
	}
	if strings.TrimSpace(d.Digest) == "" {
		return fmt.Errorf("document context digest is required")
	}
	return nil
}

// AttemptInputManifest is the typed payload of the input half: everything
// admission authorized this Attempt to consume, frozen at launch.
type AttemptInputManifest struct {
	AttemptID              AttemptID      `json:"attemptId"`
	OutcomeID              OutcomeID      `json:"outcomeId"`
	PlanRevisionID         PlanRevisionID `json:"planRevisionId"`
	WorkUnitID             WorkUnitID     `json:"workUnitId"`
	ContractRevisionNumber int64          `json:"contractRevisionNumber"`
	// WorkspaceKind and BaseRevision identify the source tree the Attempt
	// started from. Empty for a staged folder, never a fabricated revision.
	WorkspaceKind WorkspaceKind `json:"workspaceKind"`
	BaseRevision  string        `json:"baseRevision,omitempty"`
	BaseRef       string        `json:"baseRef,omitempty"`
	// Repos binds the per-repo source trees of a workspace-project Attempt.
	// Empty for single-repo and staged-folder shapes, whose base evidence
	// lives in BaseRevision/BaseRef.
	Repos                  []AttemptManifestRepo     `json:"repos,omitempty"`
	RunBriefCoreDigest     string                    `json:"runBriefCoreDigest"`
	RunBriefCompiledDigest string                    `json:"runBriefCompiledDigest"`
	ExecutionPolicyDigest  string                    `json:"executionPolicyDigest"`
	Inputs                 []AttemptManifestInputRef `json:"inputs"`
	Documents              *AttemptManifestDocument  `json:"documents,omitempty"`
	Checks                 []AttemptManifestCheck    `json:"checks"`
}

// Validate checks the input payload's structural invariants.
func (m AttemptInputManifest) Validate() error {
	if err := validateManifestLineage(m.AttemptID, m.OutcomeID, m.PlanRevisionID, m.WorkUnitID, m.ContractRevisionNumber); err != nil {
		return err
	}
	if !m.WorkspaceKind.Valid() {
		return fmt.Errorf("workspace kind %q is invalid", m.WorkspaceKind)
	}
	// Frozen digests are provider/runtime-format values; Kennel requires them
	// present and never recomputes them from anything but the source bytes.
	for name, digest := range map[string]string{
		"run brief core digest":     m.RunBriefCoreDigest,
		"run brief compiled digest": m.RunBriefCompiledDigest,
		"execution policy digest":   m.ExecutionPolicyDigest,
	} {
		if strings.TrimSpace(digest) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	for _, input := range m.Inputs {
		if err := input.Validate(); err != nil {
			return err
		}
	}
	for _, repo := range m.Repos {
		if err := repo.Validate(); err != nil {
			return err
		}
	}
	// Exactly one source-tree representation: a staged folder binds none, a
	// single-repo git worktree binds its resolved base revision, a workspace
	// project binds its per-repo inventory. Anything else is not custody.
	switch m.WorkspaceKind {
	case WorkspaceStagedFolder:
		if strings.TrimSpace(m.BaseRevision) != "" || strings.TrimSpace(m.BaseRef) != "" || len(m.Repos) != 0 {
			return fmt.Errorf("staged folder source tree must not bind a base revision, base ref, or repo inventory")
		}
	case WorkspaceGitWorktree:
		hasBase := strings.TrimSpace(m.BaseRevision) != ""
		hasRepos := len(m.Repos) != 0
		if hasBase == hasRepos {
			return fmt.Errorf("git worktree source tree must bind exactly one of a base revision or a repo inventory")
		}
		if hasRepos && strings.TrimSpace(m.BaseRef) != "" {
			return fmt.Errorf("workspace repo inventory binds per-repo bases; top-level base ref must be empty")
		}
	}
	if m.Documents != nil {
		if err := m.Documents.Validate(); err != nil {
			return err
		}
	}
	for _, check := range m.Checks {
		if err := check.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// AttemptOutputManifest is the typed payload of the output half: what custody
// close retained, bound to the immutable artifact version.
type AttemptOutputManifest struct {
	AttemptID              AttemptID      `json:"attemptId"`
	OutcomeID              OutcomeID      `json:"outcomeId"`
	PlanRevisionID         PlanRevisionID `json:"planRevisionId"`
	WorkUnitID             WorkUnitID     `json:"workUnitId"`
	ContractRevisionNumber int64          `json:"contractRevisionNumber"`
	ArtifactVersion        string         `json:"artifactVersion"`
	ResultRevision         string         `json:"resultRevision,omitempty"`
	RetentionState         RetentionState `json:"retentionState"`
	RetentionDetail        string         `json:"retentionDetail,omitempty"`
	TerminationReason      string         `json:"terminationReason"`
	ObservedAt             time.Time      `json:"observedAt"`
}

// Validate checks the output payload's structural invariants.
func (m AttemptOutputManifest) Validate() error {
	if err := validateManifestLineage(m.AttemptID, m.OutcomeID, m.PlanRevisionID, m.WorkUnitID, m.ContractRevisionNumber); err != nil {
		return err
	}
	if !SHA256Digest(m.ArtifactVersion).Valid() {
		return fmt.Errorf("artifact version is not a content digest")
	}
	if !m.RetentionState.Valid() {
		return fmt.Errorf("retention state %q is invalid", m.RetentionState)
	}
	if m.ObservedAt.IsZero() {
		return fmt.Errorf("observed-at is required")
	}
	return nil
}

func validateManifestLineage(attemptID AttemptID, outcomeID OutcomeID, planRevisionID PlanRevisionID, workUnitID WorkUnitID, contractRevisionNumber int64) error {
	if attemptID.IsZero() {
		return fmt.Errorf("attempt id is required")
	}
	if outcomeID.IsZero() {
		return fmt.Errorf("outcome id is required")
	}
	if planRevisionID.IsZero() {
		return fmt.Errorf("plan revision id is required")
	}
	if workUnitID.IsZero() {
		return fmt.Errorf("work unit id is required")
	}
	if contractRevisionNumber < 1 {
		return fmt.Errorf("contract revision number must be positive")
	}
	return nil
}

// AttemptManifest is one sealed custody record: canonical payload bytes plus
// the digest over exactly those bytes. The envelope repeats the payload's
// identity keys so storage can index without parsing, and Validate proves the
// two cannot disagree.
type AttemptManifest struct {
	AttemptID     AttemptID
	OutcomeID     OutcomeID
	Half          AttemptManifestHalf
	Payload       []byte
	PayloadDigest SHA256Digest
	RecordedAt    time.Time
}

// NewAttemptInputManifest seals the input half: canonical bytes first, digest
// over exactly those bytes, so no later edit can keep the old digest.
func NewAttemptInputManifest(in AttemptInputManifest, at time.Time) (AttemptManifest, error) {
	if err := in.Validate(); err != nil {
		return AttemptManifest{}, err
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return AttemptManifest{}, fmt.Errorf("encode input manifest: %w", err)
	}
	return AttemptManifest{
		AttemptID: in.AttemptID, OutcomeID: in.OutcomeID, Half: AttemptManifestInput,
		Payload: payload, PayloadDigest: DigestSHA256(payload), RecordedAt: at.UTC(),
	}, nil
}

// NewAttemptOutputManifest seals the output half.
func NewAttemptOutputManifest(out AttemptOutputManifest, at time.Time) (AttemptManifest, error) {
	if err := out.Validate(); err != nil {
		return AttemptManifest{}, err
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return AttemptManifest{}, fmt.Errorf("encode output manifest: %w", err)
	}
	return AttemptManifest{
		AttemptID: out.AttemptID, OutcomeID: out.OutcomeID, Half: AttemptManifestOutput,
		Payload: payload, PayloadDigest: DigestSHA256(payload), RecordedAt: at.UTC(),
	}, nil
}

// Validate checks the sealed record: well-formed envelope, digest recomputed
// from the stored bytes, and payload lineage identical to the envelope. A
// record whose bytes were altered after sealing fails here.
func (m AttemptManifest) Validate() error {
	if m.AttemptID.IsZero() {
		return fmt.Errorf("attempt id is required")
	}
	if m.OutcomeID.IsZero() {
		return fmt.Errorf("outcome id is required")
	}
	if !m.Half.Valid() {
		return fmt.Errorf("manifest half %q is invalid", m.Half)
	}
	if len(m.Payload) == 0 {
		return fmt.Errorf("manifest payload is required")
	}
	if !m.PayloadDigest.Valid() {
		return fmt.Errorf("manifest payload digest is not a SHA-256 digest")
	}
	if DigestSHA256(m.Payload) != m.PayloadDigest {
		return fmt.Errorf("manifest payload does not match its sealed digest")
	}
	if m.RecordedAt.IsZero() {
		return fmt.Errorf("manifest recorded-at is required")
	}
	switch m.Half {
	case AttemptManifestInput:
		body, err := m.DecodeInput()
		if err != nil {
			return err
		}
		if body.AttemptID != m.AttemptID || body.OutcomeID != m.OutcomeID {
			return fmt.Errorf("manifest payload lineage does not match its envelope")
		}
	case AttemptManifestOutput:
		body, err := m.DecodeOutput()
		if err != nil {
			return err
		}
		if body.AttemptID != m.AttemptID || body.OutcomeID != m.OutcomeID {
			return fmt.Errorf("manifest payload lineage does not match its envelope")
		}
	}
	return nil
}

// DecodeInput parses the sealed payload as an input half.
func (m AttemptManifest) DecodeInput() (AttemptInputManifest, error) {
	if m.Half != AttemptManifestInput {
		return AttemptInputManifest{}, fmt.Errorf("manifest %s is not the input half", m.AttemptID)
	}
	var body AttemptInputManifest
	if err := json.Unmarshal(m.Payload, &body); err != nil {
		return AttemptInputManifest{}, fmt.Errorf("decode input manifest: %w", err)
	}
	if err := body.Validate(); err != nil {
		return AttemptInputManifest{}, err
	}
	return body, nil
}

// DecodeOutput parses the sealed payload as an output half.
func (m AttemptManifest) DecodeOutput() (AttemptOutputManifest, error) {
	if m.Half != AttemptManifestOutput {
		return AttemptOutputManifest{}, fmt.Errorf("manifest %s is not the output half", m.AttemptID)
	}
	var body AttemptOutputManifest
	if err := json.Unmarshal(m.Payload, &body); err != nil {
		return AttemptOutputManifest{}, fmt.Errorf("decode output manifest: %w", err)
	}
	if err := body.Validate(); err != nil {
		return AttemptOutputManifest{}, err
	}
	return body, nil
}

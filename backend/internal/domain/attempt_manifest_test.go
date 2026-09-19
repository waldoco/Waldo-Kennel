package domain

import (
	"strings"
	"testing"
	"time"
)

func validInputManifest() AttemptInputManifest {
	return AttemptInputManifest{
		AttemptID: "att-1", OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1",
		ContractRevisionNumber: 3,
		WorkspaceKind:          WorkspaceGitWorktree, BaseRevision: "6293eecfdf7f93342fc25a2ef85b26d3a5982bcd",
		RunBriefCoreDigest:     strings.Repeat("a", 64),
		RunBriefCompiledDigest: strings.Repeat("b", 64),
		ExecutionPolicyDigest:  strings.Repeat("c", 64),
		Inputs: []AttemptManifestInputRef{
			{AttemptID: "att-0", WorkUnitID: "wu-0", ArtifactVersion: strings.Repeat("d", 64)},
		},
		Checks: []AttemptManifestCheck{
			{ID: "chk-1", CriterionID: "crit-1", Argv: []string{"go", "test", "./..."}, TimeoutSeconds: 600},
		},
	}
}

func TestAttemptManifestSealsInputHalfWithDigestOverExactBytes(t *testing.T) {
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	first, err := NewAttemptInputManifest(validInputManifest(), at)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewAttemptInputManifest(validInputManifest(), at)
	if err != nil {
		t.Fatal(err)
	}
	if first.PayloadDigest != second.PayloadDigest {
		t.Fatal("same admitted inputs must seal to the same digest")
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("sealed manifest must validate: %v", err)
	}
	body, err := first.DecodeInput()
	if err != nil {
		t.Fatal(err)
	}
	if body.Inputs[0].ArtifactVersion != strings.Repeat("d", 64) {
		t.Fatal("round trip lost the admitted artifact version")
	}
}

func TestAttemptManifestDigestChangesWhenAnyAdmittedFactChanges(t *testing.T) {
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	base, err := NewAttemptInputManifest(validInputManifest(), at)
	if err != nil {
		t.Fatal(err)
	}
	changed := validInputManifest()
	changed.Inputs[0].ArtifactVersion = strings.Repeat("e", 64)
	other, err := NewAttemptInputManifest(changed, at)
	if err != nil {
		t.Fatal(err)
	}
	if base.PayloadDigest == other.PayloadDigest {
		t.Fatal("a different admitted artifact version sealed to the same digest")
	}
}

func TestAttemptManifestValidateRefusesAlteredPayload(t *testing.T) {
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	m, err := NewAttemptInputManifest(validInputManifest(), at)
	if err != nil {
		t.Fatal(err)
	}
	// Tamper: rewrite one byte of the sealed payload without resealing.
	m.Payload[len(m.Payload)-3] = '9'
	if err := m.Validate(); err == nil {
		t.Fatal("altered payload still validated against its sealed digest")
	}
}

func TestAttemptManifestValidateRefusesEnvelopePayloadLineageMismatch(t *testing.T) {
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	m, err := NewAttemptInputManifest(validInputManifest(), at)
	if err != nil {
		t.Fatal(err)
	}
	m.OutcomeID = "out-other"
	if err := m.Validate(); err == nil {
		t.Fatal("envelope and payload may disagree about lineage")
	}
}

func TestAttemptManifestRejectsIncompleteInputs(t *testing.T) {
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	broken := validInputManifest()
	broken.Inputs[0].ArtifactVersion = "not-a-digest"
	if _, err := NewAttemptInputManifest(broken, at); err == nil {
		t.Fatal("input reference without a content digest sealed")
	}
	broken = validInputManifest()
	broken.ExecutionPolicyDigest = ""
	if _, err := NewAttemptInputManifest(broken, at); err == nil {
		t.Fatal("manifest without an execution policy digest sealed")
	}
	broken = validInputManifest()
	broken.Checks[0].Argv = nil
	if _, err := NewAttemptInputManifest(broken, at); err == nil {
		t.Fatal("check without argv sealed")
	}
}

func TestAttemptManifestSealsOutputHalf(t *testing.T) {
	at := time.Date(2026, 9, 20, 2, 0, 0, 0, time.UTC)
	out := AttemptOutputManifest{
		AttemptID: "att-1", OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1",
		ContractRevisionNumber: 3,
		ArtifactVersion:        strings.Repeat("f", 64),
		RetentionState:         RetentionRetained, TerminationReason: "succeeded", ObservedAt: at,
	}
	m, err := NewAttemptOutputManifest(out, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("sealed output manifest must validate: %v", err)
	}
	if _, err := m.DecodeInput(); err == nil {
		t.Fatal("output half decoded as input")
	}
	body, err := m.DecodeOutput()
	if err != nil {
		t.Fatal(err)
	}
	if body.ArtifactVersion != strings.Repeat("f", 64) || body.RetentionState != RetentionRetained {
		t.Fatal("round trip lost output facts")
	}
}

func TestAttemptManifestRejectsUnknownHalf(t *testing.T) {
	m := AttemptManifest{AttemptID: "att-1", OutcomeID: "out-1", Half: "sideways", RecordedAt: time.Now()}
	if err := m.Validate(); err == nil {
		t.Fatal("unknown manifest half validated")
	}
}

func TestAttemptManifestValidateChecksBoundRepos(t *testing.T) {
	valid := validInputManifest()
	valid.BaseRevision, valid.BaseRef = "", ""
	valid.Repos = []AttemptManifestRepo{
		{RepoName: "root", WorktreePath: "/ws/root", BaseSHA: "sha-root", BaseRef: "main"},
		{RepoName: "api", WorktreePath: "/ws/api", BaseSHA: "sha-api"},
	}
	sealed, err := NewAttemptInputManifest(valid, time.Now())
	if err != nil {
		t.Fatalf("workspace inventory with resolved bases must seal: %v", err)
	}
	if err := sealed.Validate(); err != nil {
		t.Fatalf("sealed workspace manifest must validate: %v", err)
	}
	body, err := sealed.DecodeInput()
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Repos) != 2 || body.Repos[0].BaseSHA != "sha-root" {
		t.Fatalf("round trip lost the bound repo inventory: %#v", body.Repos)
	}

	baseless := valid
	baseless.Repos = []AttemptManifestRepo{{RepoName: "root", WorktreePath: "/ws/root", BaseSHA: ""}}
	if _, err := NewAttemptInputManifest(baseless, time.Now()); err == nil {
		t.Fatal("a workspace source tree without a resolved base must be refused")
	}
	nameless := valid
	nameless.Repos = []AttemptManifestRepo{{RepoName: " ", WorktreePath: "/ws/root", BaseSHA: "sha-root"}}
	if _, err := NewAttemptInputManifest(nameless, time.Now()); err == nil {
		t.Fatal("a repo fact without a canonical name must be refused")
	}
	pathless := valid
	pathless.Repos = []AttemptManifestRepo{{RepoName: "root", WorktreePath: "", BaseSHA: "sha-root"}}
	if _, err := NewAttemptInputManifest(pathless, time.Now()); err == nil {
		t.Fatal("a repo fact without a worktree path must be refused")
	}
}

func TestAttemptManifestValidateRequiresExactlyOneSourceTreeRepresentation(t *testing.T) {
	base := validInputManifest()
	seal := func(m AttemptInputManifest) error {
		sealed, err := NewAttemptInputManifest(m, time.Now())
		if err != nil {
			return err
		}
		return sealed.Validate()
	}
	// single-repo git worktree: base revision, no repos - valid (the helper).
	if err := seal(base); err != nil {
		t.Fatalf("single-repo base representation must seal: %v", err)
	}
	// workspace: repos, no base - valid.
	ws := base
	ws.BaseRevision, ws.BaseRef = "", ""
	ws.Repos = []AttemptManifestRepo{{RepoName: "root", WorktreePath: "/ws/root", BaseSHA: "sha-root"}}
	if err := seal(ws); err != nil {
		t.Fatalf("workspace repo representation must seal: %v", err)
	}
	// neither: refused.
	neither := base
	neither.BaseRevision, neither.BaseRef = "", ""
	if err := seal(neither); err == nil {
		t.Fatal("git worktree with neither base nor repos must be refused")
	}
	// both: refused.
	both := base
	both.Repos = []AttemptManifestRepo{{RepoName: "root", WorktreePath: "/ws/root", BaseSHA: "sha-root"}}
	if err := seal(both); err == nil {
		t.Fatal("git worktree with both base and repos must be refused")
	}
	// workspace with a top-level base ref: refused.
	refd := ws
	refd.BaseRef = "main"
	if err := seal(refd); err == nil {
		t.Fatal("workspace shape with a top-level base ref must be refused")
	}
	// staged folder: no representation - valid; with one - refused.
	staged := base
	staged.WorkspaceKind = WorkspaceStagedFolder
	staged.BaseRevision, staged.BaseRef = "", ""
	if err := seal(staged); err != nil {
		t.Fatalf("staged folder without a representation must seal: %v", err)
	}
	stagedWithBase := staged
	stagedWithBase.BaseRevision = "abc123"
	if err := seal(stagedWithBase); err == nil {
		t.Fatal("staged folder with a base revision must be refused")
	}
}

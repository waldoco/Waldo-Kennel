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
	// Exhaustive (kind x base x ref x repos) matrix: exactly four shapes may
	// seal - staged folder with no git identity, single-repo git worktree
	// with a base revision (ref optional), and a workspace with only a repo
	// inventory. Every other combination must be refused.
	repo := AttemptManifestRepo{RepoName: "root", WorktreePath: "/ws/root", BaseSHA: "sha-root"}
	type shape struct {
		kind       WorkspaceKind
		base, ref  string
		repos      []AttemptManifestRepo
		wantSealed bool
	}
	var shapes []shape
	for _, kind := range []WorkspaceKind{WorkspaceStagedFolder, WorkspaceGitWorktree} {
		for _, base := range []string{"", "base-sha"} {
			for _, ref := range []string{"", "main"} {
				for _, repos := range [][]AttemptManifestRepo{nil, {repo}} {
					want := false
					switch kind {
					case WorkspaceStagedFolder:
						want = base == "" && ref == "" && len(repos) == 0
					case WorkspaceGitWorktree:
						if len(repos) == 0 {
							want = base != ""
						} else {
							want = base == "" && ref == ""
						}
					}
					shapes = append(shapes, shape{kind, base, ref, repos, want})
				}
			}
		}
	}
	if len(shapes) != 16 {
		t.Fatalf("matrix has %d shapes, want the full 16", len(shapes))
	}
	sealed := 0
	for _, sh := range shapes {
		m := validInputManifest()
		m.WorkspaceKind = sh.kind
		m.BaseRevision, m.BaseRef, m.Repos = sh.base, sh.ref, sh.repos
		_, err := NewAttemptInputManifest(m, time.Now())
		if sh.wantSealed && err != nil {
			t.Errorf("kind=%s base=%q ref=%q repos=%d: must seal, got %v", sh.kind, sh.base, sh.ref, len(sh.repos), err)
		}
		if !sh.wantSealed && err == nil {
			t.Errorf("kind=%s base=%q ref=%q repos=%d: must be refused", sh.kind, sh.base, sh.ref, len(sh.repos))
		}
		if err == nil {
			sealed++
		}
	}
	if sealed != 4 {
		t.Fatalf("%d shapes sealed, want exactly 4", sealed)
	}
}

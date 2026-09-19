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

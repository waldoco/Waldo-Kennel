package store

import (
	"database/sql"
	"encoding/json"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"testing"
	"time"
)

func TestProjectNeedsYouTypedShapesAndClosedStates(t *testing.T) {
	now := time.Now().UTC()
	base := func(kind domain.ActivityKind, detail string, state sql.NullString, attempt domain.AttemptStatus) (domain.NeedsYouQuestion, error) {
		return projectNeedsYou("q", "c", "r", "g", "pending", now, now, kind, "why", detail, domain.ActivityStatusPending, "o", "p", "w", "a", attempt, "s", sql.NullString{String: "cmd", Valid: state.Valid}, state)
	}
	q, err := base(domain.ActivityKindApproval, `{"recommendation":"accept","decisions":[{"id":"yes","label":"Yes"},{"id":"no"}]}`, sql.NullString{}, domain.AttemptRunning)
	if err != nil {
		t.Fatal(err)
	}
	if q.Kind != domain.NeedsYouChoice || q.Status != domain.NeedsYouOpen || q.Recommendation != "accept" || len(q.Options) != 2 {
		t.Fatalf("projection=%+v", q)
	}
	q, err = base(domain.ActivityKindUserInput, `{"inputMode":"form","schema":{"type":"object"}}`, sql.NullString{String: "delivery_unknown", Valid: true}, domain.AttemptRunning)
	if err != nil {
		t.Fatal(err)
	}
	if q.Kind != domain.NeedsYouInput || q.InputMode != "form" || q.Status != domain.NeedsYouDeliveryUnknown {
		t.Fatalf("input=%+v", q)
	}
	q, err = base(domain.ActivityKindApproval, `{"decisions":[{"id":"yes"}]}`, sql.NullString{}, domain.AttemptFailed)
	if err != nil || q.Status != domain.NeedsYouSuperseded {
		t.Fatalf("terminal=%+v err=%v", q, err)
	}
	if _, err = base(domain.ActivityKindApproval, `{`, sql.NullString{}, domain.AttemptRunning); err == nil {
		t.Fatal("malformed provider detail did not fail closed")
	}
}

func TestProjectNeedsYouSupersessionDominatesEveryCommandState(t *testing.T) {
	now := time.Now().UTC()
	states := []string{"claimed", "dispatching", "acknowledged", "rejected", "delivery_unknown", "reconciled"}
	for _, state := range states {
		for _, tc := range []struct {
			name, qstatus string
			astatus       domain.ActivityStatus
			attempt       domain.AttemptStatus
		}{{"terminal", "pending", domain.ActivityStatusPending, domain.AttemptFailed}, {"failed-question", "failed", domain.ActivityStatusFailed, domain.AttemptRunning}, {"resolved-question", "resolved", domain.ActivityStatusResolved, domain.AttemptRunning}} {
			t.Run(tc.name+"/"+state, func(t *testing.T) {
				q, err := projectNeedsYou("q", "c", "r", "g", tc.qstatus, now, now, domain.ActivityKindApproval, "why", `{"decisions":[{"id":"yes"}]}`, tc.astatus, "o", "p", "w", "a", tc.attempt, "s", sql.NullString{String: "cmd", Valid: true}, sql.NullString{String: state, Valid: true})
				if err != nil {
					t.Fatal(err)
				}
				if q.Status != domain.NeedsYouSuperseded {
					t.Fatalf("status=%s", q.Status)
				}
			})
		}
	}
}

func TestNeedsYouReconcileRequiresAffirmativeProviderResolution(t *testing.T) {
	base := domain.NeedsYouQuestion{CommandState: domain.GovernedCommandDeliveryUnknown}
	for _, tc := range []struct {
		name, qstatus string
		astatus       domain.ActivityStatus
		want          bool
	}{
		{"controller-cleanup-failed", "failed", domain.ActivityStatusFailed, false},
		{"superseded-without-resolution", "failed", domain.ActivityStatusCancelled, false},
		{"question-only-resolved", "resolved", domain.ActivityStatusFailed, false},
		{"activity-only-resolved", "failed", domain.ActivityStatusResolved, false},
		{"genuine-provider-resolution", "resolved", domain.ActivityStatusResolved, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := base
			q.QuestionStatus = tc.qstatus
			q.ActivityStatus = tc.astatus
			if got := hasAffirmativeNeedsYouResolution(q); got != tc.want {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestProjectNeedsYouPostRunCheckQuestionRemainsAnswerable(t *testing.T) {
	now := time.Now().UTC()
	e := domain.CapabilityEscalation{Version: domain.CapabilityEscalationVersion, ExecutorKind: "governed_check", OutcomeID: "o", ContractRevisionNumber: 1, PlanRevisionID: "p", WorkUnitID: "w", AttemptID: "a", AttemptGeneration: 1, AttemptSessionRefID: "ref", SessionID: "s", SessionGeneration: 1, PolicyDigest: "policy", ArtifactVersion: "artifact", CheckID: "check", RequestedCapability: domain.CapabilityWorktreeExec, DenialSource: "governed_check", GrantFingerprint: "policy", OperationID: "check:check", RequestFingerprint: "operation", QuestionGeneration: "q"}
	e.Digest, _ = e.ComputedDigest()
	detail, _ := json.Marshal(map[string]any{"capabilityEscalation": e, "decisions": domain.CapabilityEscalationOptionsFor("governed_check")})
	q, err := projectNeedsYou("q", "c", "check:check", "q", "pending", now, now, domain.ActivityKindApproval, "why", string(detail), domain.ActivityStatusPending, "o", "p", "w", "a", domain.AttemptReconciled, "s", sql.NullString{}, sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	if q.Status != domain.NeedsYouOpen || len(q.Options) != 2 || q.Options[0].ID != domain.CapabilityEscalationWidenContract {
		t.Fatalf("question=%+v", q)
	}
}

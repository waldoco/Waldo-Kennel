package store

import (
	"database/sql"
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

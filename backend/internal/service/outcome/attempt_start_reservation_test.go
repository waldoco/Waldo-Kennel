package outcome

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"testing"
)

type startEvaluatorFake struct{ calls int }

func (f *startEvaluatorFake) EvaluateAdmissionStage(context.Context, ports.AdmissionStageInput) (ports.AdmissionStageResult, error) {
	f.calls++
	return ports.AdmissionStageResult{Eligible: true}, nil
}
func TestFreshStartEvaluationRequiresCurrentRoutingSnapshot(t *testing.T) {
	f := &startEvaluatorFake{}
	s := AttemptStartReservationService{Evaluator: f}
	if _, err := s.FreshStartEvaluation(context.Background(), ports.AdmissionStageInput{Stage: ports.AdmissionStageStart}); err == nil {
		t.Fatal("stored/no routing proof passed")
	}
	snap := &ports.RoutingInventorySnapshot{}
	if _, err := s.FreshStartEvaluation(context.Background(), ports.AdmissionStageInput{Stage: ports.AdmissionStageStart, RoutingSnapshot: snap}); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatalf("calls=%d", f.calls)
	}
}

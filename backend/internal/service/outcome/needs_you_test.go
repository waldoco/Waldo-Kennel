package outcome

import (
	"context"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"sync"
	"testing"
)

type needsStoreFake struct {
	mu sync.Mutex
	q  domain.NeedsYouQuestion
}

func (f *needsStoreFake) ListCurrentNeedsYouQuestions(context.Context, domain.OutcomeID) ([]domain.NeedsYouQuestion, error) {
	return []domain.NeedsYouQuestion{f.q}, nil
}
func (f *needsStoreFake) GetNeedsYouQuestion(_ context.Context, _ domain.OutcomeID, _ string) (domain.NeedsYouQuestion, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.q, true, nil
}
func (f *needsStoreFake) ReconcileNeedsYouAnswer(context.Context, domain.OutcomeID, string, string) (domain.NeedsYouQuestion, error) {
	return f.q, nil
}

type needsDispatchFake struct {
	mu    sync.Mutex
	calls int
}

func (f *needsDispatchFake) ResolveWithKey(context.Context, domain.SessionID, string, string, ports.ChatDecision) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return nil
}
func (f *needsDispatchFake) ResolveInputWithKey(context.Context, domain.SessionID, string, string, ports.ChatInputResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return nil
}
func TestAnswerNeedsYouGenerationAndChoiceFence(t *testing.T) {
	st := &needsStoreFake{q: domain.NeedsYouQuestion{ID: "q", OutcomeID: "o", SessionID: "s", RequestID: "r", Generation: "g", Kind: domain.NeedsYouChoice, Status: domain.NeedsYouOpen, Options: []domain.NeedsYouOption{{ID: "yes"}}}}
	d := &needsDispatchFake{}
	svc := New(nil, nil).WithNeedsYou(st, d)
	_, err := svc.AnswerNeedsYou(context.Background(), "o", "q", domain.NeedsYouAnswer{RequestKey: "k", Generation: "old", Decision: &domain.ChatDecisionAnswer{ID: "yes"}})
	if !errors.Is(err, ErrNeedsYouStale) {
		t.Fatalf("stale err=%v", err)
	}
	if d.calls != 0 {
		t.Fatal("stale answer crossed provider boundary")
	}
	_, err = svc.AnswerNeedsYou(context.Background(), "o", "q", domain.NeedsYouAnswer{RequestKey: "k", Generation: "g", Decision: &domain.ChatDecisionAnswer{ID: "invented"}})
	if !errors.Is(err, ErrNeedsYouAnswerInvalid) {
		t.Fatalf("choice err=%v", err)
	}
	if d.calls != 0 {
		t.Fatal("invalid choice crossed provider boundary")
	}
	_, err = svc.AnswerNeedsYou(context.Background(), "o", "q", domain.NeedsYouAnswer{RequestKey: "k", Generation: "g", Decision: &domain.ChatDecisionAnswer{ID: "yes"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.calls != 1 {
		t.Fatalf("calls=%d", d.calls)
	}
}
func TestAnswerNeedsYouTypedInputDoesNotUseApprovalPath(t *testing.T) {
	st := &needsStoreFake{q: domain.NeedsYouQuestion{ID: "q", OutcomeID: "o", SessionID: "s", RequestID: "r", Generation: "g", Kind: domain.NeedsYouInput, Status: domain.NeedsYouOpen}}
	d := &needsDispatchFake{}
	svc := New(nil, nil).WithNeedsYou(st, d)
	_, err := svc.AnswerNeedsYou(context.Background(), "o", "q", domain.NeedsYouAnswer{RequestKey: "k", Generation: "g", Input: &domain.ChatInputAnswer{Action: "accept", Content: map[string]any{"name": "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if d.calls != 1 {
		t.Fatalf("calls=%d", d.calls)
	}
}

func TestAnswerNeedsYouSupersededNeverDispatches(t *testing.T) {
	st := &needsStoreFake{q: domain.NeedsYouQuestion{ID: "q", OutcomeID: "o", SessionID: "s", RequestID: "r", Generation: "g", Kind: domain.NeedsYouChoice, Status: domain.NeedsYouSuperseded, Options: []domain.NeedsYouOption{{ID: "yes"}}}}
	d := &needsDispatchFake{}
	svc := New(nil, nil).WithNeedsYou(st, d)
	_, err := svc.AnswerNeedsYou(context.Background(), "o", "q", domain.NeedsYouAnswer{RequestKey: "k", Generation: "g", Decision: &domain.ChatDecisionAnswer{ID: "yes"}})
	if !errors.Is(err, ErrNeedsYouStale) {
		t.Fatalf("err=%v", err)
	}
	if d.calls != 0 {
		t.Fatal("superseded answer crossed provider boundary")
	}
}

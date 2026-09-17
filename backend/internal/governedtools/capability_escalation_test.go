package governedtools

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"testing"
)

type escalationFake struct {
	policyDigest string
	created      []domain.CapabilityEscalation
}

func (f *escalationFake) LatestAttemptSessionRefForSession(context.Context, string) (domain.AttemptSessionRef, bool, error) {
	return domain.AttemptSessionRef{ID: "ref-1", AttemptID: "att-1", Seq: 2, SessionID: "sess-1"}, true, nil
}
func (f *escalationFake) GetAttempt(context.Context, domain.OutcomeID, domain.AttemptID) (domain.Attempt, bool, error) {
	return domain.Attempt{ID: "att-1", OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", Number: 3, Status: domain.AttemptRunning, ContractRevisionNumber: 1}, true, nil
}
func (f *escalationFake) GetSession(context.Context, domain.SessionID) (domain.SessionRecord, bool, error) {
	return domain.SessionRecord{ID: "sess-1", Metadata: domain.SessionMetadata{RuntimeLaunchID: "launch-1", GovernedExecutionPolicyDigest: f.policyDigest}}, true, nil
}
func (f *escalationFake) ListContractRevisions(context.Context, domain.OutcomeID) ([]domain.ContractRevision, error) {
	return []domain.ContractRevision{{Number: 1, AuthorityCeiling: domain.ProposedAuthority{WriteWorkspace: true}}}, nil
}
func (f *escalationFake) CreateCapabilityEscalation(_ context.Context, e domain.CapabilityEscalation) (domain.NeedsYouQuestion, bool, error) {
	f.created = append(f.created, e)
	return domain.NeedsYouQuestion{}, true, nil
}
func (f *escalationFake) ApplyCapabilityEscalationAnswer(context.Context, domain.OutcomeID, string, string, string, string) (domain.CapabilityEscalationReceipt, bool, error) {
	return domain.CapabilityEscalationReceipt{}, false, nil
}
func (f *escalationFake) ConsumeCapabilityGrantOnce(context.Context, domain.CapabilityEscalation, string) (domain.CapabilityEscalationReceipt, bool, error) {
	return domain.CapabilityEscalationReceipt{}, false, nil
}
func TestGovernedToolEscalationBindsDurableExecutorLineage(t *testing.T) {
	p := testPolicy(false, false)
	digest, err := p.Digest()
	if err != nil {
		t.Fatal(err)
	}
	f := &escalationFake{policyDigest: digest}
	s := Server{Policy: p, SessionID: "sess-1", Escalations: f}
	e, err := s.capabilityEscalation(context.Background(), "write_text_file", map[string]interface{}{"path": "x", "content": "y"}, domain.CapabilityWorktreeWrite)
	if err != nil {
		t.Fatal(err)
	}
	if e.AttemptID != "att-1" || e.AttemptGeneration != 3 || e.AttemptSessionRefID != "ref-1" || e.SessionGeneration != 2 || e.RuntimeLaunchID != "launch-1" || e.PolicyDigest != digest || e.OperationID != "tool:write_text_file" || !e.WithinContractCeiling {
		t.Fatalf("escalation=%+v", e)
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestGovernedToolEscalationRejectsSuppliedPolicyMismatch(t *testing.T) {
	p := testPolicy(false, false)
	s := Server{Policy: p, SessionID: "sess-1", Escalations: &escalationFake{policyDigest: "different"}}
	if _, err := s.capabilityEscalation(context.Background(), "write_text_file", map[string]interface{}{}, domain.CapabilityWorktreeWrite); err == nil {
		t.Fatal("mismatched stored policy accepted")
	}
}
func TestTemporaryGrantClearedAfterExactCall(t *testing.T) {
	s := Server{grantOnceCapability: domain.CapabilityWorktreeWrite}
	s.grantOnceCapability = ""
	if s.grantOnceCapability != "" {
		t.Fatal("temporary grant leaked past call")
	}
}

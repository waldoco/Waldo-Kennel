package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
	"github.com/google/uuid"
)

func escalationDetail(e domain.CapabilityEscalation) string {
	b, _ := json.Marshal(map[string]any{"recommendation": domain.CapabilityEscalationDeny, "decisions": domain.CapabilityEscalationOptionsFor(e.ExecutorKind), "capabilityEscalation": e})
	return string(b)
}
func escalationQuestion(e domain.CapabilityEscalation, conversation string, now time.Time) domain.NeedsYouQuestion {
	return domain.NeedsYouQuestion{ID: e.QuestionGeneration, OutcomeID: e.OutcomeID, PlanRevisionID: e.PlanRevisionID, WorkUnitID: e.WorkUnitID, AttemptID: e.AttemptID, SessionID: e.SessionID, ConversationID: conversation, RequestID: e.OperationID, Generation: e.QuestionGeneration, Kind: domain.NeedsYouChoice, Reason: "A governed operation requires an ungranted capability", Recommendation: domain.CapabilityEscalationDeny, Options: domain.CapabilityEscalationOptions(), Status: domain.NeedsYouOpen, CreatedAt: now, UpdatedAt: now, QuestionStatus: "pending", ActivityStatus: domain.ActivityStatusPending, CapabilityEscalation: &e}
}
func sameEscalation(a, b gen.CapabilityEscalation) bool { return a.Digest == b.Digest }

func (s *Store) CreateCapabilityEscalation(ctx context.Context, e domain.CapabilityEscalation) (domain.NeedsYouQuestion, bool, error) {
	if err := e.Validate(); err != nil {
		return domain.NeedsYouQuestion{}, false, err
	}
	unlock := func() {}
	if _, inProjectionTx := ctx.Value(conversationProjectionTxKey{}).(*gen.Queries); !inProjectionTx {
		s.writeMu.Lock()
		unlock = s.writeMu.Unlock
	}
	defer unlock()
	var out domain.NeedsYouQuestion
	created := false
	err := s.inTx(ctx, "create capability escalation", func(q *gen.Queries) error {
		at, err := q.GetAttempt(ctx, gen.GetAttemptParams{ID: e.AttemptID, OutcomeID: e.OutcomeID})
		if err != nil {
			return err
		}
		if (e.ExecutorKind == "governed_tool" && at.Status != domain.AttemptRunning) || (at.Status.Terminal() && e.ExecutorKind != "governed_check") || at.Number != e.AttemptGeneration || at.PlanRevisionID != e.PlanRevisionID || at.ContractRevisionNumber != e.ContractRevisionNumber || at.WorkUnitID != e.WorkUnitID {
			return fmt.Errorf("capability escalation lineage is stale")
		}
		ref, err := q.LatestAttemptSessionRef(ctx, e.AttemptID)
		if err != nil {
			return err
		}
		if ref.ID != string(e.AttemptSessionRefID) || ref.SessionID != string(e.SessionID) || ref.Seq != e.SessionGeneration {
			return fmt.Errorf("capability escalation session is stale")
		}
		plan, err := q.GetLatestPlanRevision(ctx, e.OutcomeID)
		if err != nil {
			return err
		}
		if plan.ID != e.PlanRevisionID || plan.ContractRevisionNumber != e.ContractRevisionNumber {
			return fmt.Errorf("capability escalation plan is stale")
		}
		conv, err := q.SelectConversationBySession(ctx, func() *domain.SessionID { x := e.SessionID; return &x }())
		if err != nil {
			return fmt.Errorf("current attempt session has no conversation: %w", err)
		}
		now := time.Now().UTC()
		if old, er := q.GetLiveCapabilityEscalation(ctx, gen.GetLiveCapabilityEscalationParams{AttemptID: string(e.AttemptID), AttemptGeneration: e.AttemptGeneration, RequestedCapability: e.RequestedCapability, OperationID: e.OperationID}); er == nil {
			if old.Digest == e.Digest {
				out = escalationQuestion(e, conv.ID, old.CreatedAt)
				return nil
			}
			// A different request fingerprint under the same executor lineage is an
			// idempotency conflict. A durable successor session may supersede it.
			if old.SessionID == string(e.SessionID) && old.SessionGeneration == e.SessionGeneration && old.AttemptSessionRefID == string(e.AttemptSessionRefID) {
				return fmt.Errorf("capability escalation operation conflicts with live request identity")
			}
			if n, err := q.SupersedeCapabilityEscalation(ctx, gen.SupersedeCapabilityEscalationParams{SupersededAt: sql.NullTime{Time: now, Valid: true}, Digest: old.Digest}); err != nil || n != 1 {
				return fmt.Errorf("supersede capability escalation affected %d rows: %w", n, err)
			}
			if n, err := q.FailCapabilityEscalationQuestion(ctx, gen.FailCapabilityEscalationQuestionParams{UpdatedAt: now, ID: old.QuestionID}); err != nil || n != 1 {
				return fmt.Errorf("fail predecessor question affected %d rows: %w", n, err)
			}
			if n, err := q.CancelCapabilityEscalationActivity(ctx, gen.CancelCapabilityEscalationActivityParams{UpdatedAt: now, ID: old.QuestionID}); err != nil || n != 1 {
				return fmt.Errorf("cancel predecessor activity affected %d rows: %w", n, err)
			}
		} else if !errors.Is(er, sql.ErrNoRows) {
			return er
		}
		seq, err := q.NextConversationSequence(ctx, gen.NextConversationSequenceParams{UpdatedAt: now, ID: conv.ID})
		if err != nil {
			return err
		}
		if _, err = q.InsertOwnerAnswerQuestion(ctx, gen.InsertOwnerAnswerQuestionParams{ID: e.QuestionGeneration, ConversationID: conv.ID, RequestID: e.OperationID, Generation: e.QuestionGeneration, Status: "pending", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if err = q.InsertConversationActivity(ctx, gen.InsertConversationActivityParams{ID: e.QuestionGeneration, ConversationID: conv.ID, Sequence: seq, Kind: domain.ActivityKindApproval, Status: domain.ActivityStatusPending, Summary: "A governed operation requires an ungranted capability", DetailJson: escalationDetail(e), RequestID: e.OperationID, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		n, err := q.InsertCapabilityEscalation(ctx, gen.InsertCapabilityEscalationParams{Digest: e.Digest, QuestionID: e.QuestionGeneration, Version: e.Version, OutcomeID: string(e.OutcomeID), ContractRevisionNumber: e.ContractRevisionNumber, PlanRevisionID: string(e.PlanRevisionID), WorkUnitID: string(e.WorkUnitID), AttemptID: string(e.AttemptID), AttemptGeneration: e.AttemptGeneration, SessionID: string(e.SessionID), SessionGeneration: e.SessionGeneration, ExecutorKind: e.ExecutorKind, AttemptSessionRefID: string(e.AttemptSessionRefID), RuntimeLaunchID: e.RuntimeLaunchID, ControllerGeneration: e.ControllerGeneration, PolicyDigest: e.PolicyDigest, ArtifactVersion: e.ArtifactVersion, CheckID: string(e.CheckID), RequestedCapability: e.RequestedCapability, DenialSource: e.DenialSource, GrantFingerprint: e.GrantFingerprint, OperationID: e.OperationID, RequestFingerprint: e.RequestFingerprint, QuestionGeneration: e.QuestionGeneration, WithinContractCeiling: escalationBoolInt(e.WithinContractCeiling), CreatedAt: now})
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("capability escalation insert lost")
		}
		out = escalationQuestion(e, conv.ID, now)
		created = true
		return nil
	})
	return out, created, err
}
func escalationBoolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
func receiptFromRow(r gen.CapabilityEscalationReceipt) domain.CapabilityEscalationReceipt {
	var consumed *time.Time
	if r.ConsumedAt.Valid {
		x := r.ConsumedAt.Time
		consumed = &x
	}
	return domain.CapabilityEscalationReceipt{ID: r.ID, EscalationDigest: r.EscalationDigest, QuestionID: r.QuestionID, QuestionGeneration: r.QuestionGeneration, Consequence: domain.CapabilityEscalationConsequence(r.Consequence), AttemptID: domain.AttemptID(r.AttemptID), AttemptGeneration: r.AttemptGeneration, SessionID: domain.SessionID(r.SessionID), SessionGeneration: r.SessionGeneration, ControllerGeneration: r.ControllerGeneration, Capability: r.Capability, OperationID: r.OperationID, RequestFingerprint: r.RequestFingerprint, GrantFingerprint: r.GrantFingerprint, AnswerRequestKey: r.AnswerRequestKey, CreatedAt: r.CreatedAt, ConsumedAt: consumed}
}
func (s *Store) ApplyCapabilityEscalationAnswer(ctx context.Context, outcome domain.OutcomeID, questionID, generation, option, requestKey string) (domain.CapabilityEscalationReceipt, bool, error) {
	unlock := func() {}
	if _, inProjectionTx := ctx.Value(conversationProjectionTxKey{}).(*gen.Queries); !inProjectionTx {
		s.writeMu.Lock()
		unlock = s.writeMu.Unlock
	}
	defer unlock()
	var out domain.CapabilityEscalationReceipt
	created := false
	err := s.inTx(ctx, "apply capability escalation answer", func(q *gen.Queries) error {
		row, err := q.GetCapabilityEscalationByQuestion(ctx, questionID)
		if err != nil {
			return err
		}
		if row.OutcomeID != string(outcome) || row.QuestionID != questionID || row.QuestionGeneration != generation || row.SupersededAt.Valid {
			return fmt.Errorf("capability escalation question is stale")
		}
		var consequence domain.CapabilityEscalationConsequence
		switch option {
		case domain.CapabilityEscalationGrantOnce:
			if row.ExecutorKind != "governed_tool" {
				return fmt.Errorf("grant once is unavailable for post-run checks")
			}
			if row.WithinContractCeiling == 0 {
				return fmt.Errorf("grant once exceeds contract ceiling")
			}
			consequence = domain.CapabilityConsequenceGrantOnce
		case domain.CapabilityEscalationWidenContract:
			consequence = domain.CapabilityConsequenceRevisionRequested
		case domain.CapabilityEscalationDeny:
			consequence = domain.CapabilityConsequenceDenied
		default:
			return fmt.Errorf("invalid capability escalation answer")
		}
		if old, er := q.GetCapabilityEscalationReceipt(ctx, gen.GetCapabilityEscalationReceiptParams{QuestionID: questionID, QuestionGeneration: generation}); er == nil {
			out = receiptFromRow(old)
			if out.AnswerRequestKey != requestKey {
				return fmt.Errorf("capability escalation answer request key conflicts")
			}
			if out.Consequence != consequence {
				return fmt.Errorf("capability escalation already answered differently")
			}
			return nil
		} else if !errors.Is(er, sql.ErrNoRows) {
			return er
		}
		at, err := q.GetAttempt(ctx, gen.GetAttemptParams{ID: domain.AttemptID(row.AttemptID), OutcomeID: outcome})
		if err != nil {
			return err
		}
		if (row.ExecutorKind == "governed_tool" && at.Status != domain.AttemptRunning) || (at.Status.Terminal() && row.ExecutorKind != "governed_check") || at.Number != row.AttemptGeneration || at.PlanRevisionID != domain.PlanRevisionID(row.PlanRevisionID) {
			return fmt.Errorf("capability escalation attempt is stale")
		}
		ref, err := q.LatestAttemptSessionRef(ctx, at.ID)
		if err != nil {
			return err
		}
		if ref.SessionID != row.SessionID || ref.Seq != row.SessionGeneration {
			return fmt.Errorf("capability escalation session is stale")
		}
		if row.ExecutorKind == "governed_tool" {
			session, sessionErr := q.GetSession(ctx, domain.SessionID(row.SessionID))
			if sessionErr != nil {
				return sessionErr
			}
			if session.RuntimeLaunchID != row.RuntimeLaunchID || session.GovernedExecutionPolicyDigest != row.PolicyDigest {
				return fmt.Errorf("capability escalation runtime is stale")
			}
		}
		plan, err := q.GetLatestPlanRevision(ctx, outcome)
		if err != nil {
			return err
		}
		if plan.ID != domain.PlanRevisionID(row.PlanRevisionID) || plan.ContractRevisionNumber != row.ContractRevisionNumber {
			return fmt.Errorf("capability escalation plan is stale")
		}
		question, err := q.GetOwnerAnswerQuestion(ctx, questionID)
		if err != nil {
			return err
		}
		if question.Status != "pending" {
			return fmt.Errorf("capability escalation question is not pending")
		}
		now := time.Now().UTC()
		id := "cer-" + uuid.NewString()
		n, err := q.InsertCapabilityEscalationReceipt(ctx, gen.InsertCapabilityEscalationReceiptParams{ID: id, EscalationDigest: row.Digest, QuestionID: questionID, QuestionGeneration: generation, Consequence: string(consequence), AttemptID: row.AttemptID, AttemptGeneration: row.AttemptGeneration, SessionID: row.SessionID, SessionGeneration: row.SessionGeneration, ControllerGeneration: row.ControllerGeneration, Capability: row.RequestedCapability, OperationID: row.OperationID, RequestFingerprint: row.RequestFingerprint, GrantFingerprint: row.GrantFingerprint, AnswerRequestKey: requestKey, CreatedAt: now})
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("capability escalation answer race lost")
		}
		if _, err = q.ResolveOwnerAnswerQuestion(ctx, gen.ResolveOwnerAnswerQuestionParams{UpdatedAt: now, ConversationID: mustQuestionConversation(ctx, q, questionID), RequestID: row.OperationID}); err != nil {
			return err
		}
		if _, err = q.ResolveCapabilityEscalationActivity(ctx, gen.ResolveCapabilityEscalationActivityParams{UpdatedAt: now, ID: questionID}); err != nil {
			return err
		}
		out = domain.CapabilityEscalationReceipt{ID: id, EscalationDigest: row.Digest, QuestionID: questionID, QuestionGeneration: generation, Consequence: consequence, AttemptID: domain.AttemptID(row.AttemptID), AttemptGeneration: row.AttemptGeneration, SessionID: domain.SessionID(row.SessionID), SessionGeneration: row.SessionGeneration, ControllerGeneration: row.ControllerGeneration, Capability: row.RequestedCapability, OperationID: row.OperationID, RequestFingerprint: row.RequestFingerprint, GrantFingerprint: row.GrantFingerprint, AnswerRequestKey: requestKey, CreatedAt: now}
		created = true
		return nil
	})
	return out, created, err
}
func mustQuestionConversation(ctx context.Context, q *gen.Queries, id string) string {
	c, _ := q.GetOwnerQuestionConversation(ctx, id)
	return c
}
func (s *Store) ConsumeCapabilityGrantOnce(ctx context.Context, e domain.CapabilityEscalation, consumerFingerprint string) (domain.CapabilityEscalationReceipt, bool, error) {
	if err := e.Validate(); err != nil {
		return domain.CapabilityEscalationReceipt{}, false, err
	}
	if consumerFingerprint != e.RequestFingerprint {
		return domain.CapabilityEscalationReceipt{}, false, fmt.Errorf("grant consumer fingerprint mismatch")
	}
	unlock := func() {}
	if _, inProjectionTx := ctx.Value(conversationProjectionTxKey{}).(*gen.Queries); !inProjectionTx {
		s.writeMu.Lock()
		unlock = s.writeMu.Unlock
	}
	defer unlock()
	var out domain.CapabilityEscalationReceipt
	won := false
	err := s.inTx(ctx, "consume capability grant once", func(q *gen.Queries) error {
		r, err := q.GetCapabilityEscalationReceipt(ctx, gen.GetCapabilityEscalationReceiptParams{QuestionID: e.QuestionGeneration, QuestionGeneration: e.QuestionGeneration})
		if err != nil {
			return err
		}
		out = receiptFromRow(r)
		if r.EscalationDigest != e.Digest || r.Consequence != string(domain.CapabilityConsequenceGrantOnce) || r.AttemptID != string(e.AttemptID) || r.AttemptGeneration != e.AttemptGeneration || r.SessionID != string(e.SessionID) || r.SessionGeneration != e.SessionGeneration || r.ControllerGeneration != e.ControllerGeneration || r.Capability != e.RequestedCapability || r.OperationID != e.OperationID || r.RequestFingerprint != e.RequestFingerprint || r.GrantFingerprint != e.GrantFingerprint {
			return fmt.Errorf("grant receipt lineage mismatch")
		}
		at, err := q.GetAttempt(ctx, gen.GetAttemptParams{ID: e.AttemptID, OutcomeID: e.OutcomeID})
		if err != nil {
			return err
		}
		if at.Status != domain.AttemptRunning || at.Number != e.AttemptGeneration {
			return fmt.Errorf("grant receipt attempt is stale")
		}
		plan, err := q.GetLatestPlanRevision(ctx, e.OutcomeID)
		if err != nil {
			return err
		}
		if plan.ID != e.PlanRevisionID || plan.ContractRevisionNumber != e.ContractRevisionNumber {
			return fmt.Errorf("grant receipt plan is stale")
		}
		ref, err := q.LatestAttemptSessionRef(ctx, e.AttemptID)
		if err != nil {
			return err
		}
		if ref.ID != string(e.AttemptSessionRefID) || ref.SessionID != string(e.SessionID) || ref.Seq != e.SessionGeneration {
			return fmt.Errorf("grant receipt session is stale")
		}
		session, sessionErr := q.GetSession(ctx, e.SessionID)
		if sessionErr != nil {
			return sessionErr
		}
		if session.RuntimeLaunchID != e.RuntimeLaunchID || session.GovernedExecutionPolicyDigest != e.PolicyDigest {
			return fmt.Errorf("grant receipt runtime is stale")
		}
		now := time.Now().UTC()
		n, err := q.ConsumeCapabilityEscalationReceipt(ctx, gen.ConsumeCapabilityEscalationReceiptParams{ConsumedAt: sql.NullTime{Time: now, Valid: true}, ID: r.ID})
		if err != nil {
			return err
		}
		won = n == 1
		if won {
			out.ConsumedAt = &now
		}
		return nil
	})
	return out, won, err
}

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
)

type needsYouRow interface{}

func (s *Store) ListCurrentNeedsYouQuestions(ctx context.Context, outcomeID domain.OutcomeID) ([]domain.NeedsYouQuestion, error) {
	rows, err := s.qr.ListCurrentNeedsYouQuestionRows(ctx, outcomeID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.NeedsYouQuestion, 0, len(rows))
	for _, r := range rows {
		q, err := projectNeedsYou(r.ID, r.ConversationID, r.RequestID, r.Generation, r.QuestionStatus, r.CreatedAt, r.UpdatedAt, r.Kind, r.Summary, r.DetailJson, r.ActivityStatus, r.OutcomeID, r.PlanRevisionID, r.WorkUnitID, r.AttemptID, r.AttemptStatus, r.SessionID, r.CommandID, r.CommandState)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}
func (s *Store) GetNeedsYouQuestion(ctx context.Context, outcomeID domain.OutcomeID, id string) (domain.NeedsYouQuestion, bool, error) {
	r, err := s.qr.GetNeedsYouQuestionRow(ctx, gen.GetNeedsYouQuestionRowParams{OutcomeID: outcomeID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NeedsYouQuestion{}, false, nil
	}
	if err != nil {
		return domain.NeedsYouQuestion{}, false, err
	}
	q, err := projectNeedsYou(r.ID, r.ConversationID, r.RequestID, r.Generation, r.QuestionStatus, r.CreatedAt, r.UpdatedAt, r.Kind, r.Summary, r.DetailJson, r.ActivityStatus, r.OutcomeID, r.PlanRevisionID, r.WorkUnitID, r.AttemptID, r.AttemptStatus, r.SessionID, r.CommandID, r.CommandState)
	return q, true, err
}
func projectNeedsYou(id, conversationID, requestID, generation, questionStatus string, createdAt, updatedAt time.Time, kind domain.ActivityKind, summary, detail string, activityStatus domain.ActivityStatus, outcomeID domain.OutcomeID, planID domain.PlanRevisionID, workID domain.WorkUnitID, attemptID domain.AttemptID, attemptStatus domain.AttemptStatus, session string, commandID, commandState sql.NullString) (domain.NeedsYouQuestion, error) {
	q := domain.NeedsYouQuestion{ID: id, ConversationID: conversationID, RequestID: requestID, Generation: generation, Reason: summary, OutcomeID: outcomeID, PlanRevisionID: planID, WorkUnitID: workID, AttemptID: attemptID, SessionID: domain.SessionID(session), CreatedAt: createdAt, UpdatedAt: updatedAt, CommandID: commandID.String, QuestionStatus: questionStatus, ActivityStatus: activityStatus, CommandState: domain.GovernedCommandState(commandState.String)}
	var d map[string]any
	if err := json.Unmarshal([]byte(detail), &d); err != nil {
		return q, fmt.Errorf("decode needs-you question %s: %w", id, err)
	}
	if rec, ok := d["recommendation"].(string); ok {
		q.Recommendation = rec
	}
	if raw, ok := d["capabilityEscalation"]; ok {
		b, _ := json.Marshal(raw)
		var escalation domain.CapabilityEscalation
		if err := json.Unmarshal(b, &escalation); err != nil || escalation.Validate() != nil {
			return q, fmt.Errorf("decode typed capability escalation %s", id)
		}
		q.CapabilityEscalation = &escalation
	}
	switch kind {
	case domain.ActivityKindApproval:
		q.Kind = domain.NeedsYouApproval
		if raw, ok := d["decisions"].([]any); ok {
			for _, x := range raw {
				m, _ := x.(map[string]any)
				oid, _ := m["id"].(string)
				label, _ := m["label"].(string)
				if oid != "" {
					q.Options = append(q.Options, domain.NeedsYouOption{ID: oid, Label: label})
				}
			}
		}
		if len(q.Options) > 1 {
			q.Kind = domain.NeedsYouChoice
		}
	case domain.ActivityKindUserInput:
		q.Kind = domain.NeedsYouInput
		if v, ok := d["inputMode"].(string); ok {
			q.InputMode = v
		}
		if v, ok := d["schema"].(map[string]any); ok {
			q.InputSchema = v
		}
		q.URL, _ = d["url"].(string)
	default:
		return q, fmt.Errorf("question %s has unsupported activity kind %s", id, kind)
	}
	q.Status = domain.NeedsYouOpen
	terminalStales := attemptStatus.Terminal() && (q.CapabilityEscalation == nil || q.CapabilityEscalation.ExecutorKind != "governed_check")
	if terminalStales || questionStatus != "pending" || activityStatus != domain.ActivityStatusPending {
		q.Status = domain.NeedsYouSuperseded
		return q, nil
	}
	if commandState.Valid {
		switch domain.GovernedCommandState(commandState.String) {
		case domain.GovernedCommandClaimed:
			q.Status = domain.NeedsYouAnswerQueued
		case domain.GovernedCommandDispatching:
			q.Status = domain.NeedsYouAnswerSent
		case domain.GovernedCommandAcknowledged, domain.GovernedCommandReconciled:
			q.Status = domain.NeedsYouAcknowledged
		case domain.GovernedCommandRejected:
			q.Status = domain.NeedsYouRefused
		case domain.GovernedCommandDeliveryUnknown:
			q.Status = domain.NeedsYouDeliveryUnknown
		}
	}
	return q, nil
}

func (s *Store) ReconcileNeedsYouAnswer(ctx context.Context, outcomeID domain.OutcomeID, id, generation string) (domain.NeedsYouQuestion, error) {
	q, found, err := s.GetNeedsYouQuestion(ctx, outcomeID, id)
	if err != nil {
		return q, err
	}
	if !found {
		return q, fmt.Errorf("needs-you question not found")
	}
	if q.Generation != generation {
		return q, fmt.Errorf("needs-you generation is stale")
	}
	if q.CommandState != domain.GovernedCommandDeliveryUnknown {
		return q, fmt.Errorf("needs-you command is not delivery_unknown")
	}
	if !hasAffirmativeNeedsYouResolution(q) {
		return q, fmt.Errorf("needs-you delivery remains unknown: no affirmative matching provider resolution")
	}
	rec, found, err := s.GetGovernedControlCommand(ctx, q.CommandID)
	if err != nil || !found {
		return q, fmt.Errorf("read needs-you command: %w", err)
	}
	rec.State = domain.GovernedCommandReconciled
	rec.UpdatedAt = time.Now().UTC()
	ok, err := s.AdvanceGovernedControlCommand(ctx, rec, domain.GovernedCommandDeliveryUnknown, rec.ControllerGeneration, rec.ExpectedRevision, rec.CapabilityFingerprint)
	if err != nil {
		return q, err
	}
	if !ok {
		return q, fmt.Errorf("needs-you reconcile lost transition fence")
	}
	return s.mustNeedsYou(ctx, outcomeID, id)
}
func (s *Store) mustNeedsYou(ctx context.Context, outcomeID domain.OutcomeID, id string) (domain.NeedsYouQuestion, error) {
	q, found, err := s.GetNeedsYouQuestion(ctx, outcomeID, id)
	if err == nil && !found {
		err = fmt.Errorf("needs-you question vanished")
	}
	return q, err
}

func hasAffirmativeNeedsYouResolution(q domain.NeedsYouQuestion) bool {
	return q.CommandState == domain.GovernedCommandDeliveryUnknown && q.QuestionStatus == "resolved" && q.ActivityStatus == domain.ActivityStatusResolved
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

// CreateIntake atomically persists exact intent, provenance refs, and idempotency.
func (s *Store) CreateIntake(ctx context.Context, session domain.IntakeSession, refs []domain.IntakeConversationRef, request ports.IntakeIdempotency) (ports.IntakeSnapshot, error) {
	if err := session.Validate(); err != nil {
		return ports.IntakeSnapshot{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if replay, err := s.qw.FindIntakeByRequestKey(ctx, request.Key); err == nil {
		if replay.RequestFingerprint != request.Fingerprint {
			return ports.IntakeSnapshot{}, &ports.IntakeIdempotencyConflictError{Key: request.Key}
		}
		return s.intakeSnapshot(ctx, s.qw, replay)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ports.IntakeSnapshot{}, fmt.Errorf("find intake replay: %w", err)
	}
	err := s.inTx(ctx, "create intake", func(q *gen.Queries) error {
		if err := q.CreateIntakeSession(ctx, gen.CreateIntakeSessionParams{
			ID: session.ID.String(), SourceSurface: string(session.SourceSurface), Purpose: string(session.Purpose),
			ProjectID: nullString(string(session.ProjectID)), SourceOpenLoopID: nullString(session.SourceOpenLoopID.String()),
			Statement: session.Statement, Status: string(session.Status), CurrentProposalRevision: session.CurrentProposalRevision,
			ClarificationCount: session.ClarificationCount, ConfirmedOutcomeID: nullString(session.ConfirmedOutcomeID.String()),
			FailureCode: session.FailureCode, CancellationReason: session.CancellationReason,
			RequestKey: request.Key, RequestFingerprint: request.Fingerprint, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
		}); err != nil {
			return err
		}
		for _, ref := range refs {
			if err := q.CreateIntakeConversationRef(ctx, gen.CreateIntakeConversationRefParams{IntakeID: session.ID.String(), EpisodeID: ref.EpisodeID, TurnID: ref.TurnID, Position: ref.Position}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	return s.intakeSnapshot(ctx, s.qw, gen.IntakeSession{
		ID: session.ID.String(), SourceSurface: string(session.SourceSurface), Purpose: string(session.Purpose), ProjectID: nullString(string(session.ProjectID)),
		SourceOpenLoopID: nullString(session.SourceOpenLoopID.String()), Statement: session.Statement, Status: string(session.Status),
		CurrentProposalRevision: session.CurrentProposalRevision, ClarificationCount: session.ClarificationCount,
		ConfirmedOutcomeID: nullString(session.ConfirmedOutcomeID.String()), FailureCode: session.FailureCode, CancellationReason: session.CancellationReason,
		RequestKey: request.Key, RequestFingerprint: request.Fingerprint, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	})
}

// GetIntake reconstructs one intake snapshot from durable rows.
func (s *Store) GetIntake(ctx context.Context, id domain.IntakeSessionID) (ports.IntakeSnapshot, bool, error) {
	row, err := s.qr.GetIntakeSession(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return ports.IntakeSnapshot{}, false, nil
	}
	if err != nil {
		return ports.IntakeSnapshot{}, false, fmt.Errorf("get intake %s: %w", id, err)
	}
	snapshot, err := s.intakeSnapshot(ctx, s.qr, row)
	return snapshot, err == nil, err
}

// BeginIntakeAnalysis enters analyzing under optimistic concurrency.
func (s *Store) BeginIntakeAnalysis(ctx context.Context, id domain.IntakeSessionID, expected int64, at time.Time) (ports.IntakeSnapshot, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	row, err := s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	rows, err := s.qw.UpdateIntakeAnalysisState(ctx, gen.UpdateIntakeAnalysisStateParams{Status: string(domain.IntakeStatusAnalyzing), UpdatedAt: at, ID: id.String(), CurrentProposalRevision: expected, Status_2: row.Status})
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	if rows != 1 {
		return ports.IntakeSnapshot{}, revisionConflict(row, id, expected)
	}
	row.Status, row.UpdatedAt = string(domain.IntakeStatusAnalyzing), at
	return s.intakeSnapshot(ctx, s.qw, row)
}

// CompleteIntakeWithProposal appends the analyzer's immutable proposal.
func (s *Store) CompleteIntakeWithProposal(ctx context.Context, id domain.IntakeSessionID, expected int64, proposal domain.OutcomeContractProposal, at time.Time) (ports.IntakeSnapshot, error) {
	return s.appendProposal(ctx, id, expected, proposal, at)
}

// AppendIntakeProposalRevision persists one user-authored immutable revision.
func (s *Store) AppendIntakeProposalRevision(ctx context.Context, id domain.IntakeSessionID, expected int64, proposal domain.OutcomeContractProposal, at time.Time) (ports.IntakeSnapshot, error) {
	return s.appendProposal(ctx, id, expected, proposal, at)
}

func (s *Store) appendProposal(ctx context.Context, id domain.IntakeSessionID, expected int64, proposal domain.OutcomeContractProposal, at time.Time) (ports.IntakeSnapshot, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	err := s.inTx(ctx, "append intake proposal", func(q *gen.Queries) error {
		row, err := q.GetIntakeSession(ctx, id.String())
		if err != nil {
			return err
		}
		if row.CurrentProposalRevision != expected {
			return revisionConflict(row, id, expected)
		}
		if err := insertIntakeProposal(ctx, q, proposal); err != nil {
			return err
		}
		rows, err := q.UpdateIntakeWithProposal(ctx, gen.UpdateIntakeWithProposalParams{CurrentProposalRevision: proposal.Revision, UpdatedAt: at, ID: id.String(), CurrentProposalRevision_2: expected})
		if err != nil {
			return err
		}
		if rows != 1 {
			return revisionConflict(row, id, expected)
		}
		return nil
	})
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	row, err := s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	return s.intakeSnapshot(ctx, s.qw, row)
}

// CompleteIntakeWithClarification persists the only allowed material question.
func (s *Store) CompleteIntakeWithClarification(ctx context.Context, id domain.IntakeSessionID, expected int64, clarification domain.ClarificationRequest, at time.Time) (ports.IntakeSnapshot, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	alternatives, _ := json.Marshal(clarification.Alternatives)
	err := s.inTx(ctx, "record intake clarification", func(q *gen.Queries) error {
		row, err := q.GetIntakeSession(ctx, id.String())
		if err != nil {
			return err
		}
		if row.CurrentProposalRevision != expected {
			return revisionConflict(row, id, expected)
		}
		if err := q.CreateIntakeClarification(ctx, gen.CreateIntakeClarificationParams{ID: string(clarification.ID), IntakeID: id.String(), Question: clarification.Question, Reason: clarification.Reason, Recommendation: clarification.Recommendation, Alternatives: string(alternatives), DeferralConsequence: clarification.DeferralConsequence, CreatedAt: clarification.CreatedAt}); err != nil {
			return err
		}
		rows, err := q.UpdateIntakeWithClarification(ctx, gen.UpdateIntakeWithClarificationParams{UpdatedAt: at, ID: id.String(), CurrentProposalRevision: expected})
		if err != nil {
			return err
		}
		if rows != 1 {
			return fmt.Errorf("intake %s cannot accept another clarification", id)
		}
		return nil
	})
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	row, err := s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	return s.intakeSnapshot(ctx, s.qw, row)
}

// AnswerIntakeClarification stores the answer and resumes analysis.
func (s *Store) AnswerIntakeClarification(ctx context.Context, id domain.IntakeSessionID, expected int64, answer string, at time.Time) (ports.IntakeSnapshot, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	err := s.inTx(ctx, "answer intake clarification", func(q *gen.Queries) error {
		row, err := q.GetIntakeSession(ctx, id.String())
		if err != nil {
			return err
		}
		if row.CurrentProposalRevision != expected {
			return revisionConflict(row, id, expected)
		}
		clarification, err := q.GetIntakeClarification(ctx, id.String())
		if err != nil {
			return err
		}
		if err := q.CreateIntakeClarificationAnswer(ctx, gen.CreateIntakeClarificationAnswerParams{ClarificationID: clarification.ID, Answer: answer, AnsweredAt: at}); err != nil {
			return err
		}
		rows, err := q.UpdateIntakeAnalysisState(ctx, gen.UpdateIntakeAnalysisStateParams{Status: string(domain.IntakeStatusAnalyzing), UpdatedAt: at, ID: id.String(), CurrentProposalRevision: expected, Status_2: string(domain.IntakeStatusNeedsUser)})
		if err != nil {
			return err
		}
		if rows != 1 {
			return fmt.Errorf("intake %s is not waiting for an answer", id)
		}
		return nil
	})
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	row, err := s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	return s.intakeSnapshot(ctx, s.qw, row)
}

// FailIntakeAnalysis preserves retryable failure truth on the intake.
func (s *Store) FailIntakeAnalysis(ctx context.Context, id domain.IntakeSessionID, expected int64, code string, at time.Time) (ports.IntakeSnapshot, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	row, err := s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	rows, err := s.qw.FailIntakeAnalysis(ctx, gen.FailIntakeAnalysisParams{FailureCode: code, UpdatedAt: at, ID: id.String(), CurrentProposalRevision: expected})
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	if rows != 1 {
		return ports.IntakeSnapshot{}, revisionConflict(row, id, expected)
	}
	row, err = s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	return s.intakeSnapshot(ctx, s.qw, row)
}

// RecoverInterruptedIntakeAnalyses makes transient analyzing state retryable after restart.
func (s *Store) RecoverInterruptedIntakeAnalyses(ctx context.Context, at time.Time) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.qw.RecoverInterruptedIntakeAnalyses(ctx, at)
}

// CancelIntake records conscious release without creating an Outcome.
func (s *Store) CancelIntake(ctx context.Context, id domain.IntakeSessionID, expected int64, reason string, at time.Time) (ports.IntakeSnapshot, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	row, err := s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	rows, err := s.qw.CancelIntake(ctx, gen.CancelIntakeParams{CancellationReason: reason, UpdatedAt: at, ID: id.String(), CurrentProposalRevision: expected})
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	if rows != 1 {
		if row.CurrentProposalRevision != expected {
			return ports.IntakeSnapshot{}, revisionConflict(row, id, expected)
		}
		return ports.IntakeSnapshot{}, fmt.Errorf("intake %s cannot be cancelled from %s", id, row.Status)
	}
	row, err = s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	return s.intakeSnapshot(ctx, s.qw, row)
}

// ConfirmIntakeWithOutcome atomically creates exactly one Outcome and ContractRevision.
func (s *Store) ConfirmIntakeWithOutcome(ctx context.Context, id domain.IntakeSessionID, expected int64, outcome domain.Outcome, contract domain.ContractRevision, request ports.IntakeIdempotency, at time.Time) (ports.IntakeSnapshot, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if confirmation, err := s.qw.FindIntakeConfirmationByRequestKey(ctx, request.Key); err == nil {
		if confirmation.RequestFingerprint != request.Fingerprint {
			return ports.IntakeSnapshot{}, &ports.IntakeIdempotencyConflictError{Key: request.Key}
		}
		row, getErr := s.qw.GetIntakeSession(ctx, confirmation.IntakeID)
		if getErr != nil {
			return ports.IntakeSnapshot{}, getErr
		}
		return s.intakeSnapshot(ctx, s.qw, row)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ports.IntakeSnapshot{}, err
	}
	row, err := s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	if row.CurrentProposalRevision != expected {
		return ports.IntakeSnapshot{}, revisionConflict(row, id, expected)
	}
	if row.Status == string(domain.IntakeStatusConfirmed) {
		return s.intakeSnapshot(ctx, s.qw, row)
	}
	err = s.inTx(ctx, "confirm intake outcome", func(q *gen.Queries) error {
		row, err := q.GetIntakeSession(ctx, id.String())
		if err != nil {
			return err
		}
		if row.CurrentProposalRevision != expected {
			return revisionConflict(row, id, expected)
		}
		if row.Status != string(domain.IntakeStatusReady) {
			return fmt.Errorf("intake %s is not ready", id)
		}
		if err := outcome.Validate(); err != nil {
			return err
		}
		if err := q.CreateOutcome(ctx, gen.CreateOutcomeParams{ID: outcome.ID, SpaceID: outcome.SpaceID, Title: outcome.Title, CurrentRevisionNumber: 0}); err != nil {
			return err
		}
		contract.Number = 1
		if err := insertContractRevision(ctx, q, contract); err != nil {
			return err
		}
		rows, err := q.AdvanceOutcomeCurrentRevision(ctx, gen.AdvanceOutcomeCurrentRevisionParams{CurrentRevisionNumber: 1, UpdatedAt: at, ID: outcome.ID, CurrentRevisionNumber_2: 0})
		if err != nil {
			return err
		}
		if rows != 1 {
			return fmt.Errorf("outcome %s revision pointer did not advance", outcome.ID)
		}
		if err := q.CreateIntakeConfirmation(ctx, gen.CreateIntakeConfirmationParams{IntakeID: id.String(), ProposalRevision: expected, OutcomeID: outcome.ID.String(), ContractRevisionID: contract.ID.String(), RequestKey: request.Key, RequestFingerprint: request.Fingerprint, ConfirmedAt: at}); err != nil {
			return err
		}
		rows, err = q.ConfirmIntake(ctx, gen.ConfirmIntakeParams{ConfirmedOutcomeID: nullString(outcome.ID.String()), UpdatedAt: at, ID: id.String(), CurrentProposalRevision: expected})
		if err != nil {
			return err
		}
		if rows != 1 {
			return revisionConflict(row, id, expected)
		}
		return nil
	})
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	row, err = s.qw.GetIntakeSession(ctx, id.String())
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	return s.intakeSnapshot(ctx, s.qw, row)
}

func (s *Store) intakeSnapshot(ctx context.Context, q *gen.Queries, row gen.IntakeSession) (ports.IntakeSnapshot, error) {
	snapshot := ports.IntakeSnapshot{Session: intakeSessionFromRow(row)}
	refs, err := q.ListIntakeConversationRefs(ctx, row.ID)
	if err != nil {
		return ports.IntakeSnapshot{}, err
	}
	for _, ref := range refs {
		snapshot.ConversationRefs = append(snapshot.ConversationRefs, domain.IntakeConversationRef{EpisodeID: ref.EpisodeID, TurnID: ref.TurnID, Position: ref.Position})
	}
	if proposalRow, err := q.GetLatestIntakeProposal(ctx, row.ID); err == nil {
		proposal, err := intakeProposalFromRow(proposalRow)
		if err != nil {
			return ports.IntakeSnapshot{}, err
		}
		snapshot.Proposal = &proposal
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ports.IntakeSnapshot{}, err
	}
	if clarificationRow, err := q.GetIntakeClarification(ctx, row.ID); err == nil {
		clarification, err := intakeClarificationFromRow(clarificationRow)
		if err != nil {
			return ports.IntakeSnapshot{}, err
		}
		snapshot.Clarification = &clarification
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ports.IntakeSnapshot{}, err
	}
	if row.ConfirmedOutcomeID.Valid {
		outcomeRow, err := q.GetOutcome(ctx, domain.OutcomeID(row.ConfirmedOutcomeID.String))
		if err != nil {
			return ports.IntakeSnapshot{}, err
		}
		outcome := outcomeFromRow(outcomeRow)
		snapshot.ConfirmedOutcome = &outcome
		confirmation, err := q.GetIntakeConfirmation(ctx, row.ID)
		if err != nil {
			return ports.IntakeSnapshot{}, err
		}
		contractRow, err := q.GetContractRevision(ctx, domain.ContractRevisionID(confirmation.ContractRevisionID))
		if err != nil {
			return ports.IntakeSnapshot{}, err
		}
		contract, err := contractRevisionFromRow(contractRow)
		if err != nil {
			return ports.IntakeSnapshot{}, err
		}
		criteria, err := q.ListContractCriteriaForRevision(ctx, string(contract.ID))
		if err != nil {
			return ports.IntakeSnapshot{}, err
		}
		for _, criterion := range criteria {
			contract.Criteria = append(contract.Criteria, domain.ContractCriterion{ID: domain.CriterionID(criterion.ID), ContractRevisionID: domain.ContractRevisionID(criterion.ContractRevisionID), Position: criterion.Position, Text: criterion.Text})
		}
		core, err := q.GetContractRevisionIntakeCore(ctx, contract.ID.String())
		if err != nil {
			return ports.IntakeSnapshot{}, err
		}
		if err := applyContractIntakeCore(&contract, core); err != nil {
			return ports.IntakeSnapshot{}, err
		}
		snapshot.ConfirmedContract = &contract
	}
	return snapshot, nil
}

func intakeSessionFromRow(row gen.IntakeSession) domain.IntakeSession {
	return domain.IntakeSession{ID: domain.IntakeSessionID(row.ID), SourceSurface: domain.IntakeSourceSurface(row.SourceSurface), Purpose: domain.IntakePurpose(row.Purpose), ProjectID: domain.ProjectID(row.ProjectID.String), SourceOpenLoopID: domain.OpenLoopID(row.SourceOpenLoopID.String), Statement: row.Statement, Status: domain.IntakeStatus(row.Status), CurrentProposalRevision: row.CurrentProposalRevision, ClarificationCount: row.ClarificationCount, ConfirmedOutcomeID: domain.OutcomeID(row.ConfirmedOutcomeID.String), FailureCode: row.FailureCode, CancellationReason: row.CancellationReason, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func insertIntakeProposal(ctx context.Context, q *gen.Queries, proposal domain.OutcomeContractProposal) error {
	criteria, _ := json.Marshal(proposal.Criteria)
	constraints, _ := json.Marshal(proposal.Constraints)
	nonGoals, _ := json.Marshal(proposal.NonGoals)
	authority, _ := json.Marshal(proposal.AuthorityCeiling)
	stops, _ := json.Marshal(proposal.StopConditions)
	notes, _ := json.Marshal(proposal.ClarificationNotes)
	facets, _ := json.Marshal(proposal.Facets)
	var temporal sql.NullString
	if proposal.TemporalCondition != nil {
		temporal = nullString(*proposal.TemporalCondition)
	}
	return q.CreateIntakeProposal(ctx, gen.CreateIntakeProposalParams{ID: string(proposal.ID), IntakeID: proposal.IntakeID.String(), Revision: proposal.Revision, Title: proposal.Title, DesiredState: proposal.DesiredState, Criteria: string(criteria), ReviewMethod: proposal.ReviewMethod, Constraints: string(constraints), NonGoals: string(nonGoals), AuthorityCeiling: string(authority), StopConditions: string(stops), ClarificationNotes: string(notes), TemporalCondition: temporal, Facets: string(facets), CreatedAt: proposal.CreatedAt})
}

func intakeProposalFromRow(row gen.IntakeProposalRevision) (domain.OutcomeContractProposal, error) {
	proposal := domain.OutcomeContractProposal{ID: domain.ProposalRevisionID(row.ID), IntakeID: domain.IntakeSessionID(row.IntakeID), Revision: row.Revision, Title: row.Title, DesiredState: row.DesiredState, ReviewMethod: row.ReviewMethod, CreatedAt: row.CreatedAt}
	fields := []struct {
		data   string
		target any
	}{
		{row.Criteria, &proposal.Criteria}, {row.Constraints, &proposal.Constraints},
		{row.NonGoals, &proposal.NonGoals}, {row.AuthorityCeiling, &proposal.AuthorityCeiling},
		{row.StopConditions, &proposal.StopConditions}, {row.ClarificationNotes, &proposal.ClarificationNotes},
		{row.Facets, &proposal.Facets},
	}
	for _, field := range fields {
		if err := json.Unmarshal([]byte(field.data), field.target); err != nil {
			return domain.OutcomeContractProposal{}, err
		}
	}
	if row.TemporalCondition.Valid {
		value := row.TemporalCondition.String
		proposal.TemporalCondition = &value
	}
	return proposal, nil
}

func intakeClarificationFromRow(row gen.GetIntakeClarificationRow) (domain.ClarificationRequest, error) {
	var alternatives []string
	if err := json.Unmarshal([]byte(row.Alternatives), &alternatives); err != nil {
		return domain.ClarificationRequest{}, err
	}
	clarification := domain.ClarificationRequest{ID: domain.ClarificationRequestID(row.ID), IntakeID: domain.IntakeSessionID(row.IntakeID), Question: row.Question, Reason: row.Reason, Recommendation: row.Recommendation, Alternatives: alternatives, DeferralConsequence: row.DeferralConsequence, Answer: row.Answer.String, CreatedAt: row.CreatedAt}
	if row.AnsweredAt.Valid {
		value := row.AnsweredAt.Time
		clarification.AnsweredAt = &value
	}
	return clarification, nil
}

func revisionConflict(row gen.IntakeSession, id domain.IntakeSessionID, expected int64) error {
	return &ports.IntakeRevisionConflictError{IntakeID: id, ExpectedRevision: expected, CurrentRevision: row.CurrentProposalRevision}
}
func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

// insertContractIntakeCore persists the part of a contract revision that lives
// outside the base relation. Called from insertContractRevision so EVERY
// creator writes it, not only intake confirmation.
func insertContractIntakeCore(ctx context.Context, q *gen.Queries, contract domain.ContractRevision) error {
	evidence, err := json.Marshal(contract.EvidenceExpectations)
	if err != nil {
		return err
	}
	authority, err := json.Marshal(contract.AuthorityCeiling)
	if err != nil {
		return err
	}
	stops, err := json.Marshal(contract.StopConditions)
	if err != nil {
		return err
	}
	facets, err := json.Marshal(contract.Facets)
	if err != nil {
		return err
	}
	var temporal sql.NullString
	if contract.TemporalCondition != nil {
		temporal = nullString(*contract.TemporalCondition)
	}
	// The base relation lets its own column default stamp the row, so a
	// revision carrying no explicit time gets the same "now" here rather than
	// a zero year.
	createdAt := contract.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return q.CreateContractRevisionIntakeCore(ctx, gen.CreateContractRevisionIntakeCoreParams{ContractRevisionID: contract.ID.String(), EvidenceExpectations: string(evidence), AuthorityCeiling: string(authority), StopConditions: string(stops), TemporalCondition: temporal, Facets: string(facets), CreatedAt: createdAt})
}

func applyContractIntakeCore(contract *domain.ContractRevision, core gen.ContractRevisionIntakeCore) error {
	fields := []struct {
		data   string
		target any
	}{{core.EvidenceExpectations, &contract.EvidenceExpectations}, {core.AuthorityCeiling, &contract.AuthorityCeiling}, {core.StopConditions, &contract.StopConditions}, {core.Facets, &contract.Facets}}
	for _, field := range fields {
		if err := json.Unmarshal([]byte(field.data), field.target); err != nil {
			return err
		}
	}
	if core.TemporalCondition.Valid {
		value := core.TemporalCondition.String
		contract.TemporalCondition = &value
	}
	return nil
}

// CreateResponsibilityLink atomically creates idempotent explicit lineage.
func (s *Store) CreateResponsibilityLink(ctx context.Context, link domain.ResponsibilityLink, request ports.ResponsibilityLinkIdempotency) (domain.ResponsibilityLink, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if replay, err := s.qw.FindResponsibilityLinkByRequestKey(ctx, request.Key); err == nil {
		if replay.RequestFingerprint != request.Fingerprint {
			return domain.ResponsibilityLink{}, &ports.IntakeIdempotencyConflictError{Key: request.Key}
		}
		return responsibilityLinkFromRow(replay), nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.ResponsibilityLink{}, err
	}
	if err := link.Validate(); err != nil {
		return domain.ResponsibilityLink{}, err
	}
	if existing, err := s.qw.FindActiveResponsibilityLinkPair(ctx, gen.FindActiveResponsibilityLinkPairParams{SourceOpenLoopID: link.SourceOpenLoopID.String(), DestinationOutcomeID: link.DestinationOutcomeID.String()}); err == nil {
		return domain.ResponsibilityLink{}, &ports.ResponsibilityLinkDuplicateError{SourceOpenLoopID: domain.OpenLoopID(existing.SourceOpenLoopID), DestinationOutcomeID: domain.OutcomeID(existing.DestinationOutcomeID)}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.ResponsibilityLink{}, err
	}
	projectID, found, err := s.GetOutcomeProjectID(ctx, link.DestinationOutcomeID)
	if err != nil {
		return domain.ResponsibilityLink{}, err
	}
	if !found {
		return domain.ResponsibilityLink{}, fmt.Errorf("destination outcome %s does not exist", link.DestinationOutcomeID)
	}
	if err := s.qw.CreateResponsibilityLink(ctx, gen.CreateResponsibilityLinkParams{ID: link.ID.String(), ProjectID: string(projectID), SourceOpenLoopID: link.SourceOpenLoopID.String(), DestinationOutcomeID: link.DestinationOutcomeID.String(), Creator: string(link.Creator), Reason: link.Reason, RequestKey: request.Key, RequestFingerprint: request.Fingerprint, CreatedAt: link.CreatedAt}); err != nil {
		return domain.ResponsibilityLink{}, err
	}
	return link, nil
}

// GetResponsibilityLink reads one lineage record.
func (s *Store) GetResponsibilityLink(ctx context.Context, id domain.ResponsibilityLinkID) (domain.ResponsibilityLink, bool, error) {
	row, err := s.qr.GetResponsibilityLink(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ResponsibilityLink{}, false, nil
	}
	if err != nil {
		return domain.ResponsibilityLink{}, false, err
	}
	return responsibilityLinkFromRow(row), true, nil
}

// EndResponsibilityLink records the one allowed lineage end transition.
func (s *Store) EndResponsibilityLink(ctx context.Context, id domain.ResponsibilityLinkID, actor domain.ResponsibilityLinkCreator, reason string, at time.Time) (domain.ResponsibilityLink, bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.EndResponsibilityLink(ctx, gen.EndResponsibilityLinkParams{EndedAt: sql.NullTime{Time: at, Valid: true}, EndedBy: string(actor), EndedReason: reason, ID: id.String()})
	if err != nil {
		return domain.ResponsibilityLink{}, false, err
	}
	if rows == 0 {
		if _, err := s.qw.GetResponsibilityLink(ctx, id.String()); errors.Is(err, sql.ErrNoRows) {
			return domain.ResponsibilityLink{}, false, nil
		}
		return domain.ResponsibilityLink{}, true, &ports.ResponsibilityLinkEndConflictError{ID: id}
	}
	row, err := s.qw.GetResponsibilityLink(ctx, id.String())
	if err != nil {
		return domain.ResponsibilityLink{}, true, err
	}
	return responsibilityLinkFromRow(row), true, nil
}

func responsibilityLinkFromRow(row gen.ResponsibilityLink) domain.ResponsibilityLink {
	link := domain.ResponsibilityLink{ID: domain.ResponsibilityLinkID(row.ID), SourceOpenLoopID: domain.OpenLoopID(row.SourceOpenLoopID), DestinationOutcomeID: domain.OutcomeID(row.DestinationOutcomeID), Creator: domain.ResponsibilityLinkCreator(row.Creator), Reason: row.Reason, CreatedAt: row.CreatedAt, EndedBy: domain.ResponsibilityLinkCreator(row.EndedBy), EndedReason: row.EndedReason}
	if row.EndedAt.Valid {
		value := row.EndedAt.Time
		link.EndedAt = &value
	}
	return link
}

// CreateIntakeAnalysisRequest opens one durable ask for an agent-authored
// Contract proposal. It is written before the agent is spawned: an agent
// holding a token for an unrecorded request would have nowhere to answer.
func (s *Store) CreateIntakeAnalysisRequest(ctx context.Context, request domain.IntakeAnalysisRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.qw.CreateIntakeAnalysisRequest(ctx, gen.CreateIntakeAnalysisRequestParams{
		ID:                       request.ID,
		IntakeID:                 request.IntakeID.String(),
		ExpectedProposalRevision: request.ExpectedProposalRevision,
		Status:                   request.Status,
		CallbackTokenDigest:      request.CallbackTokenDigest,
		SessionID:                request.SessionID,
		Harness:                  request.Harness,
		ExpiresAt:                request.ExpiresAt,
		RawProposal:              request.RawProposal,
		RefusalReason:            request.RefusalReason,
		CreatedAt:                request.CreatedAt,
	})
}

// GetIntakeAnalysisRequest reads one ask.
func (s *Store) GetIntakeAnalysisRequest(ctx context.Context, id domain.IntakeAnalysisRequestID) (domain.IntakeAnalysisRequest, bool, error) {
	row, err := s.qr.GetIntakeAnalysisRequest(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.IntakeAnalysisRequest{}, false, nil
	}
	if err != nil {
		return domain.IntakeAnalysisRequest{}, false, err
	}
	return intakeAnalysisRequestFromRow(row), true, nil
}

// LatestIntakeAnalysisRequest returns an intake's newest ask of any status.
func (s *Store) LatestIntakeAnalysisRequest(ctx context.Context, intakeID domain.IntakeSessionID) (domain.IntakeAnalysisRequest, bool, error) {
	row, err := s.qr.LatestIntakeAnalysisRequest(ctx, intakeID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return domain.IntakeAnalysisRequest{}, false, nil
	}
	if err != nil {
		return domain.IntakeAnalysisRequest{}, false, err
	}
	return intakeAnalysisRequestFromRow(row), true, nil
}

// ListOpenIntakeAnalysisRequests returns every unanswered ask.
func (s *Store) ListOpenIntakeAnalysisRequests(ctx context.Context) ([]domain.IntakeAnalysisRequest, error) {
	rows, err := s.qr.ListOpenIntakeAnalysisRequests(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.IntakeAnalysisRequest, 0, len(rows))
	for _, row := range rows {
		out = append(out, intakeAnalysisRequestFromRow(row))
	}
	return out, nil
}

// BindIntakeAnalysisRequestSession records which session and harness answer.
func (s *Store) BindIntakeAnalysisRequestSession(ctx context.Context, id domain.IntakeAnalysisRequestID, sessionID string, harness domain.AgentHarness) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.BindIntakeAnalysisRequestSession(ctx, gen.BindIntakeAnalysisRequestSessionParams{
		SessionID: sessionID, Harness: harness, ID: id,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ports.ErrIntakeAnalysisRequestClosed
	}
	return nil
}

// AnswerIntakeAnalysisRequest closes an open ask one way, retaining the draft.
// Answering an already-closed ask reports ErrIntakeAnalysisRequestClosed
// rather than overwriting the first answer: that is the single-use guard.
func (s *Store) AnswerIntakeAnalysisRequest(ctx context.Context, answer ports.IntakeAnalysisRequestAnswer) error {
	if !answer.Status.Valid() || answer.Status.Open() {
		return fmt.Errorf("answer intake analysis request %s: %q does not close it", answer.RequestID, answer.Status)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.AnswerIntakeAnalysisRequest(ctx, gen.AnswerIntakeAnalysisRequestParams{
		Status:        answer.Status,
		RawProposal:   answer.RawProposal,
		RefusalReason: answer.RefusalReason,
		AnsweredAt:    sql.NullTime{Time: answer.At, Valid: true},
		ID:            answer.RequestID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ports.ErrIntakeAnalysisRequestClosed
	}
	return nil
}

func intakeAnalysisRequestFromRow(row gen.IntakeAnalysisRequest) domain.IntakeAnalysisRequest {
	request := domain.IntakeAnalysisRequest{
		ID:                       row.ID,
		IntakeID:                 domain.IntakeSessionID(row.IntakeID),
		ExpectedProposalRevision: row.ExpectedProposalRevision,
		Status:                   row.Status,
		CallbackTokenDigest:      row.CallbackTokenDigest,
		SessionID:                row.SessionID,
		Harness:                  row.Harness,
		ExpiresAt:                row.ExpiresAt,
		RawProposal:              row.RawProposal,
		RefusalReason:            row.RefusalReason,
		CreatedAt:                row.CreatedAt,
	}
	if row.AnsweredAt.Valid {
		answered := row.AnsweredAt.Time
		request.AnsweredAt = &answered
	}
	return request
}

// OpenIntakeClarificationRound persists one aggregate transition atomically.
func (s *Store) OpenIntakeClarificationRound(ctx context.Context, id domain.IntakeSessionID, draft domain.IntakeClarificationRoundDraft, at time.Time) (domain.IntakeClarificationRound, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var opened domain.IntakeClarificationRound
	err := s.inTx(ctx, "open intake clarification round", func(q *gen.Queries) error {
		session, err := q.GetIntakeSession(ctx, id.String())
		if err != nil {
			return err
		}
		rounds, err := loadAllIntakeClarificationRounds(ctx, q, id)
		if err != nil {
			return err
		}
		history := domain.IntakeClarificationHistory{IntakeID: id, CurrentProposalRevision: session.CurrentProposalRevision, Rounds: rounds}
		next, err := domain.OpenClarificationRound(history, draft)
		if err != nil {
			return err
		}
		opened = next.Rounds[len(next.Rounds)-1]
		if err := q.CreateIntakeClarificationRound(ctx, gen.CreateIntakeClarificationRoundParams{ID: string(opened.ID), IntakeID: id.String(), Ordinal: opened.Ordinal, Version: string(opened.Version), ExpectedProposalRevision: opened.ExpectedProposalRevision, ExplicitReanalysis: boolInt64(draft.ExplicitReanalysis), CreatedAt: at}); err != nil {
			return err
		}
		for _, question := range opened.Questions {
			alternatives, err := json.Marshal(question.Alternatives)
			if err != nil {
				return err
			}
			if err := q.CreateIntakeClarificationRoundQuestion(ctx, gen.CreateIntakeClarificationRoundQuestionParams{RoundID: string(opened.ID), QuestionID: string(question.ID), Position: question.Position, Question: question.Question, Reason: question.Reason, Recommendation: question.Recommendation, Alternatives: string(alternatives), DeferralConsequence: question.DeferralConsequence}); err != nil {
				return err
			}
		}
		return nil
	})
	return opened, err
}

// AnswerIntakeClarificationRound applies an answer batch atomically.
func (s *Store) AnswerIntakeClarificationRound(ctx context.Context, id domain.IntakeSessionID, batch domain.IntakeClarificationAnswerBatch, at time.Time) (domain.IntakeClarificationRound, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var answered domain.IntakeClarificationRound
	err := s.inTx(ctx, "answer intake clarification round", func(q *gen.Queries) error {
		session, err := q.GetIntakeSession(ctx, id.String())
		if err != nil {
			return err
		}
		rounds, err := loadAllIntakeClarificationRounds(ctx, q, id)
		if err != nil {
			return err
		}
		next, err := domain.AnswerClarificationRound(domain.IntakeClarificationHistory{IntakeID: id, CurrentProposalRevision: session.CurrentProposalRevision, Rounds: rounds}, batch)
		if err != nil {
			return err
		}
		for _, answer := range batch.Answers {
			if err := q.CreateIntakeClarificationRoundAnswer(ctx, gen.CreateIntakeClarificationRoundAnswerParams{RoundID: string(answer.RoundID), QuestionID: string(answer.QuestionID), Answer: answer.Answer, AnsweredAt: at}); err != nil {
				return err
			}
		}
		for _, round := range next.Rounds {
			if round.ID == batch.RoundID {
				answered = round
				break
			}
		}
		return nil
	})
	return answered, err
}

// ListIntakeClarificationHistory returns a stable, bounded page. The first page
// freezes the current maximum ordinal; subsequent pages validate that same bound.
func (s *Store) ListIntakeClarificationHistory(ctx context.Context, id domain.IntakeSessionID, cursor ports.IntakeClarificationHistoryCursor, limit int) (ports.IntakeClarificationHistoryPage, error) {
	if s.intakeCursorCodec == nil {
		return ports.IntakeClarificationHistoryPage{}, fmt.Errorf("intake cursor codec unavailable")
	}
	if limit == 0 {
		limit = ports.IntakeClarificationHistoryDefaultPageLimit
	}
	if limit < 1 || limit > ports.IntakeClarificationHistoryPageLimit {
		return ports.IntakeClarificationHistoryPage{}, fmt.Errorf("clarification history page limit must be between 1 and %d", ports.IntakeClarificationHistoryPageLimit)
	}
	var after, high ports.IntakeClarificationCursorBoundary
	var answerHighWater int64
	if cursor == "" {
		latest, err := s.qr.GetLatestIntakeClarificationRound(ctx, id.String())
		if errors.Is(err, sql.ErrNoRows) {
			return ports.IntakeClarificationHistoryPage{Rounds: []domain.IntakeClarificationRound{}}, nil
		}
		if err != nil {
			return ports.IntakeClarificationHistoryPage{}, err
		}
		high = ports.IntakeClarificationCursorBoundary{Ordinal: latest.Ordinal, RoundID: domain.IntakeClarificationRoundID(latest.ID)}
		answerHighWater, err = s.qr.MaxIntakeClarificationAnswerRowID(ctx, id.String())
		if err != nil {
			return ports.IntakeClarificationHistoryPage{}, err
		}
	} else {
		var err error
		after, high, answerHighWater, err = s.intakeCursorCodec.Parse(cursor, id)
		if err != nil {
			return ports.IntakeClarificationHistoryPage{}, err
		}
		if err := validateIntakeClarificationCursorBoundary(ctx, s.qr, id, high); err != nil {
			return ports.IntakeClarificationHistoryPage{}, err
		}
		if after.Ordinal > 0 {
			if err := validateIntakeClarificationCursorBoundary(ctx, s.qr, id, after); err != nil {
				return ports.IntakeClarificationHistoryPage{}, err
			}
		}
	}
	rows, err := s.qr.ListIntakeClarificationRoundsPage(ctx, gen.ListIntakeClarificationRoundsPageParams{IntakeID: id.String(), AfterOrdinal: after.Ordinal, HighWaterOrdinal: high.Ordinal, PageLimit: int64(limit + 1)})
	if err != nil {
		return ports.IntakeClarificationHistoryPage{}, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	page := ports.IntakeClarificationHistoryPage{Rounds: make([]domain.IntakeClarificationRound, 0, len(rows))}
	for _, row := range rows {
		round, err := intakeClarificationRoundFromRowAtHighWater(ctx, s.qr, row, answerHighWater)
		if err != nil {
			return ports.IntakeClarificationHistoryPage{}, err
		}
		page.Rounds = append(page.Rounds, round)
	}
	if hasMore {
		last := rows[len(rows)-1]
		page.NextCursor, err = s.intakeCursorCodec.New(id,
			ports.IntakeClarificationCursorBoundary{Ordinal: last.Ordinal, RoundID: domain.IntakeClarificationRoundID(last.ID)}, high, answerHighWater)
		if err != nil {
			return ports.IntakeClarificationHistoryPage{}, err
		}
	}
	return page, nil
}

func validateIntakeClarificationCursorBoundary(ctx context.Context, q *gen.Queries, id domain.IntakeSessionID, boundary ports.IntakeClarificationCursorBoundary) error {
	row, err := q.GetIntakeClarificationRound(ctx, gen.GetIntakeClarificationRoundParams{IntakeID: id.String(), ID: string(boundary.RoundID)})
	if err != nil || row.Ordinal != boundary.Ordinal {
		return fmt.Errorf("invalid clarification history cursor boundary")
	}
	return nil
}

func loadAllIntakeClarificationRounds(ctx context.Context, q *gen.Queries, id domain.IntakeSessionID) ([]domain.IntakeClarificationRound, error) {
	highWater, err := q.MaxIntakeClarificationRoundOrdinal(ctx, id.String())
	if err != nil {
		return nil, err
	}
	rows, err := q.ListIntakeClarificationRoundsPage(ctx, gen.ListIntakeClarificationRoundsPageParams{IntakeID: id.String(), AfterOrdinal: 0, HighWaterOrdinal: highWater, PageLimit: highWater + 1})
	if err != nil {
		return nil, err
	}
	result := make([]domain.IntakeClarificationRound, 0, len(rows))
	for _, row := range rows {
		round, err := intakeClarificationRoundFromRow(ctx, q, row)
		if err != nil {
			return nil, err
		}
		result = append(result, round)
	}
	return result, nil
}

func intakeClarificationRoundFromRowAtHighWater(ctx context.Context, q *gen.Queries, row gen.IntakeClarificationRound, answerHighWater int64) (domain.IntakeClarificationRound, error) {
	round := domain.IntakeClarificationRound{Version: domain.IntakeClarificationRoundVersion(row.Version), ID: domain.IntakeClarificationRoundID(row.ID), IntakeID: domain.IntakeSessionID(row.IntakeID), Ordinal: row.Ordinal, ExpectedProposalRevision: row.ExpectedProposalRevision}
	questions, err := q.ListIntakeClarificationRoundQuestions(ctx, row.ID)
	if err != nil {
		return domain.IntakeClarificationRound{}, err
	}
	for _, item := range questions {
		var alternatives []string
		if err := json.Unmarshal([]byte(item.Alternatives), &alternatives); err != nil {
			return domain.IntakeClarificationRound{}, err
		}
		round.Questions = append(round.Questions, domain.IntakeClarificationQuestion{ID: domain.IntakeClarificationQuestionID(item.QuestionID), Position: item.Position, Question: item.Question, Reason: item.Reason, Recommendation: item.Recommendation, Alternatives: alternatives, DeferralConsequence: item.DeferralConsequence})
	}
	answers, err := q.ListIntakeClarificationRoundAnswersAtHighWater(ctx, gen.ListIntakeClarificationRoundAnswersAtHighWaterParams{RoundID: row.ID, AnswerHighWater: answerHighWater})
	if err != nil {
		return domain.IntakeClarificationRound{}, err
	}
	answerByID := make(map[string]gen.ListIntakeClarificationRoundAnswersAtHighWaterRow, len(answers))
	for _, answer := range answers {
		answerByID[answer.QuestionID] = answer
	}
	for _, question := range round.Questions {
		if answer, ok := answerByID[string(question.ID)]; ok {
			round.Answers = append(round.Answers, domain.IntakeClarificationRoundAnswer{RoundID: round.ID, QuestionID: question.ID, Answer: answer.Answer})
		}
	}
	return round, nil
}

func intakeClarificationRoundFromRow(ctx context.Context, q *gen.Queries, row gen.IntakeClarificationRound) (domain.IntakeClarificationRound, error) {
	round := domain.IntakeClarificationRound{Version: domain.IntakeClarificationRoundVersion(row.Version), ID: domain.IntakeClarificationRoundID(row.ID), IntakeID: domain.IntakeSessionID(row.IntakeID), Ordinal: row.Ordinal, ExpectedProposalRevision: row.ExpectedProposalRevision}
	questions, err := q.ListIntakeClarificationRoundQuestions(ctx, row.ID)
	if err != nil {
		return domain.IntakeClarificationRound{}, err
	}
	for _, item := range questions {
		var alternatives []string
		if err := json.Unmarshal([]byte(item.Alternatives), &alternatives); err != nil {
			return domain.IntakeClarificationRound{}, err
		}
		round.Questions = append(round.Questions, domain.IntakeClarificationQuestion{ID: domain.IntakeClarificationQuestionID(item.QuestionID), Position: item.Position, Question: item.Question, Reason: item.Reason, Recommendation: item.Recommendation, Alternatives: alternatives, DeferralConsequence: item.DeferralConsequence})
	}
	answers, err := q.ListIntakeClarificationRoundAnswers(ctx, row.ID)
	if err != nil {
		return domain.IntakeClarificationRound{}, err
	}
	answerByID := make(map[string]gen.IntakeClarificationRoundAnswer, len(answers))
	for _, answer := range answers {
		answerByID[answer.QuestionID] = answer
	}
	for _, question := range round.Questions {
		if answer, ok := answerByID[string(question.ID)]; ok {
			round.Answers = append(round.Answers, domain.IntakeClarificationRoundAnswer{RoundID: round.ID, QuestionID: question.ID, Answer: answer.Answer})
		}
	}
	return round, nil
}

func boolInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

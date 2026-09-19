package ports

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// IntakeIdempotency binds a request key to a normalized input fingerprint.
type IntakeIdempotency struct {
	Key         string
	Fingerprint string
}

// IntakeSnapshot is the complete durable read model for one intake.
type IntakeSnapshot struct {
	Session           domain.IntakeSession
	ConversationRefs  []domain.IntakeConversationRef
	Proposal          *domain.OutcomeContractProposal
	Clarification     *domain.ClarificationRequest
	ConfirmedOutcome  *domain.Outcome
	ConfirmedContract *domain.ContractRevision
}

const (
	IntakeClarificationHistoryDefaultPageLimit = 20
	IntakeClarificationHistoryPageLimit        = 50
)

type IntakeClarificationHistoryCursor string

type IntakeClarificationCursorBoundary struct {
	Ordinal int64
	RoundID domain.IntakeClarificationRoundID
}

type intakeClarificationCursorPayload struct {
	Version          int    `json:"v"`
	IntakeIDHash     string `json:"i"`
	HighWaterOrdinal int64  `json:"ho"`
	HighWaterRoundID string `json:"hr"`
	AfterOrdinal     int64  `json:"ao"`
	AfterRoundID     string `json:"ar"`
	AnswerHighWater  int64  `json:"aw"`
}

type IntakeClarificationCursorCodec struct{ key []byte }

func NewIntakeClarificationCursorCodec(key []byte) (*IntakeClarificationCursorCodec, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("intake cursor MAC key must be 32 bytes")
	}
	return &IntakeClarificationCursorCodec{key: append([]byte(nil), key...)}, nil
}
func (c *IntakeClarificationCursorCodec) New(id domain.IntakeSessionID, after, highWater IntakeClarificationCursorBoundary, answerHighWater int64) (IntakeClarificationHistoryCursor, error) {
	if c == nil || len(c.key) != 32 || id.IsZero() || after.Ordinal < 0 || highWater.Ordinal < 1 || after.Ordinal > highWater.Ordinal || (after.Ordinal == 0) != (after.RoundID == "") || highWater.RoundID == "" || (after.Ordinal == highWater.Ordinal && after.RoundID != highWater.RoundID) || answerHighWater < 0 {
		return "", fmt.Errorf("invalid clarification history cursor bounds")
	}
	p := intakeClarificationCursorPayload{Version: 1, IntakeIDHash: intakeClarificationCursorIntakeHash(id), HighWaterOrdinal: highWater.Ordinal, HighWaterRoundID: string(highWater.RoundID), AfterOrdinal: after.Ordinal, AfterRoundID: string(after.RoundID), AnswerHighWater: answerHighWater}
	payload, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte("kennel.intake-clarification-history.cursor.mac.v1\x00"))
	_, _ = mac.Write(payload)
	return IntakeClarificationHistoryCursor(base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))), nil
}
func (c *IntakeClarificationCursorCodec) Parse(cursor IntakeClarificationHistoryCursor, id domain.IntakeSessionID) (after, highWater IntakeClarificationCursorBoundary, answerHighWater int64, err error) {
	if c == nil || len(c.key) != 32 {
		return after, highWater, 0, fmt.Errorf("intake cursor codec unavailable")
	}
	parts := strings.Split(string(cursor), ".")
	if len(parts) != 2 {
		return after, highWater, 0, fmt.Errorf("invalid clarification history cursor")
	}
	payload, e := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if e != nil {
		return after, highWater, 0, fmt.Errorf("invalid clarification history cursor")
	}
	sig, e := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if e != nil {
		return after, highWater, 0, fmt.Errorf("invalid clarification history cursor")
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte("kennel.intake-clarification-history.cursor.mac.v1\x00"))
	_, _ = mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return after, highWater, 0, fmt.Errorf("invalid clarification history cursor")
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var p intakeClarificationCursorPayload
	if e := dec.Decode(&p); e != nil {
		return after, highWater, 0, fmt.Errorf("invalid clarification history cursor")
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return after, highWater, 0, fmt.Errorf("invalid clarification history cursor")
	}
	canonical, e := json.Marshal(p)
	if e != nil || !bytes.Equal(payload, canonical) || p.Version != 1 || p.IntakeIDHash != intakeClarificationCursorIntakeHash(id) {
		return after, highWater, 0, fmt.Errorf("invalid clarification history cursor")
	}
	after = IntakeClarificationCursorBoundary{Ordinal: p.AfterOrdinal, RoundID: domain.IntakeClarificationRoundID(p.AfterRoundID)}
	highWater = IntakeClarificationCursorBoundary{Ordinal: p.HighWaterOrdinal, RoundID: domain.IntakeClarificationRoundID(p.HighWaterRoundID)}
	if after.Ordinal < 0 || highWater.Ordinal < 1 || after.Ordinal > highWater.Ordinal || (after.Ordinal == 0) != (after.RoundID == "") || highWater.RoundID == "" || (after.Ordinal == highWater.Ordinal && after.RoundID != highWater.RoundID) || p.AnswerHighWater < 0 {
		return IntakeClarificationCursorBoundary{}, IntakeClarificationCursorBoundary{}, 0, fmt.Errorf("invalid clarification history cursor")
	}
	return after, highWater, p.AnswerHighWater, nil
}

func intakeClarificationCursorIntakeHash(id domain.IntakeSessionID) string {
	sum := sha256.Sum256([]byte("kennel.intake-clarification-history.cursor.intake.v1\x00" + id.String()))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type IntakeClarificationHistoryPage struct {
	Rounds     []domain.IntakeClarificationRound
	NextCursor IntakeClarificationHistoryCursor
}

// IntakeRevisionConflictError reports an optimistic concurrency mismatch.
type IntakeRevisionConflictError struct {
	IntakeID         domain.IntakeSessionID
	ExpectedRevision int64
	CurrentRevision  int64
}

func (err *IntakeRevisionConflictError) Error() string {
	return fmt.Sprintf("intake %s proposal revision conflict: expected %d, current %d", err.IntakeID, err.ExpectedRevision, err.CurrentRevision)
}

// IntakeIdempotencyConflictError reports reuse with different input.
type IntakeIdempotencyConflictError struct{ Key string }

func (err *IntakeIdempotencyConflictError) Error() string {
	return fmt.Sprintf("intake idempotency key %q is already bound to different input", err.Key)
}

// IntakeStore persists the shared state machine and immutable proposal
// revisions. Every mutation is guarded by the expected proposal revision.
type IntakeStore interface {
	GetProject(context.Context, string) (domain.ProjectRecord, bool, error)
	CreateIntake(context.Context, domain.IntakeSession, []domain.IntakeConversationRef, IntakeIdempotency) (IntakeSnapshot, error)
	GetIntake(context.Context, domain.IntakeSessionID) (IntakeSnapshot, bool, error)
	BeginIntakeAnalysis(context.Context, domain.IntakeSessionID, int64, time.Time) (IntakeSnapshot, error)
	CompleteIntakeWithProposal(context.Context, domain.IntakeSessionID, int64, domain.OutcomeContractProposal, time.Time) (IntakeSnapshot, error)
	CompleteIntakeWithClarification(context.Context, domain.IntakeSessionID, int64, domain.ClarificationRequest, time.Time) (IntakeSnapshot, error)
	AnswerIntakeClarification(context.Context, domain.IntakeSessionID, int64, string, time.Time) (IntakeSnapshot, error)
	AppendIntakeProposalRevision(context.Context, domain.IntakeSessionID, int64, domain.OutcomeContractProposal, time.Time) (IntakeSnapshot, error)
	EnsureWorkResponsibilitySpace(context.Context, domain.ProjectID) (domain.ResponsibilitySpace, error)
	ConfirmIntakeWithOutcome(context.Context, domain.IntakeSessionID, int64, domain.Outcome, domain.ContractRevision, IntakeIdempotency, time.Time) (IntakeSnapshot, error)
	FailIntakeAnalysis(context.Context, domain.IntakeSessionID, int64, string, time.Time) (IntakeSnapshot, error)
	CancelIntake(context.Context, domain.IntakeSessionID, int64, string, time.Time) (IntakeSnapshot, error)
	RecoverInterruptedIntakeAnalyses(context.Context, time.Time) (int64, error)
	OpenIntakeClarificationRound(context.Context, domain.IntakeSessionID, domain.IntakeClarificationRoundDraft, time.Time) (domain.IntakeClarificationRound, error)
	AnswerIntakeClarificationRound(context.Context, domain.IntakeSessionID, domain.IntakeClarificationAnswerBatch, time.Time) (domain.IntakeClarificationRound, error)
	ListIntakeClarificationHistory(context.Context, domain.IntakeSessionID, IntakeClarificationHistoryCursor, int) (IntakeClarificationHistoryPage, error)

	// CreateIntakeAnalysisRequest opens one durable ask for an agent-authored
	// Contract proposal. It is written BEFORE the agent is spawned.
	CreateIntakeAnalysisRequest(context.Context, domain.IntakeAnalysisRequest) error

	// GetIntakeAnalysisRequest reads one ask; ok=false when absent.
	GetIntakeAnalysisRequest(context.Context, domain.IntakeAnalysisRequestID) (domain.IntakeAnalysisRequest, bool, error)

	// LatestIntakeAnalysisRequest returns an intake's newest ask of any status,
	// which is what the waiting state and the refused-draft view read.
	LatestIntakeAnalysisRequest(context.Context, domain.IntakeSessionID) (domain.IntakeAnalysisRequest, bool, error)

	// AnswerIntakeAnalysisRequest closes an open ask one way, retaining the
	// draft whatever the verdict. Answering a closed ask changes nothing and
	// returns ErrIntakeAnalysisRequestClosed — that guard is what makes the
	// callback single-use.
	AnswerIntakeAnalysisRequest(context.Context, IntakeAnalysisRequestAnswer) error

	// ListOpenIntakeAnalysisRequests returns every unanswered ask so a
	// durable deadline can be enforced at startup and on a timer.
	ListOpenIntakeAnalysisRequests(context.Context) ([]domain.IntakeAnalysisRequest, error)

	// BindIntakeAnalysisRequestSession records which spawned session and
	// harness are answering, so a restart can tell what was working on this
	// and the waiting state can name who.
	BindIntakeAnalysisRequestSession(context.Context, domain.IntakeAnalysisRequestID, string, domain.AgentHarness) error
}

// IntakeAnalysisRequestAnswer closes one ask. RawProposal is retained whatever
// the verdict, so a refused draft stays inspectable.
type IntakeAnalysisRequestAnswer struct {
	RequestID     domain.IntakeAnalysisRequestID
	Status        domain.IntakeAnalysisRequestStatus
	RawProposal   string
	RefusalReason string
	At            time.Time
}

// ErrIntakeAnalysisRequestClosed reports an answer to an ask that is no longer
// open. It is the single-use guard, not an unexpected failure.
var ErrIntakeAnalysisRequestClosed = errors.New("intake analysis request is not open")

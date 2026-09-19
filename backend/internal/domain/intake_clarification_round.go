package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// IntakeClarificationRoundVersion identifies the frozen, additive round contract.
type IntakeClarificationRoundVersion string

const IntakeClarificationRoundV1 IntakeClarificationRoundVersion = "intake.clarification-round.v1"

type IntakeClarificationRoundID string
type IntakeClarificationQuestionID string

// IntakeClarificationQuestion is one immutable question in a canonical round.
type IntakeClarificationQuestion struct {
	ID                  IntakeClarificationQuestionID
	Position            int64
	Question            string
	Reason              string
	Recommendation      string
	Alternatives        []string
	DeferralConsequence string
}

// IntakeClarificationRoundAnswer is an immutable answer identified by round and question.
type IntakeClarificationRoundAnswer struct {
	RoundID    IntakeClarificationRoundID
	QuestionID IntakeClarificationQuestionID
	Answer     string
}

// IntakeClarificationRound is one immutable question set plus append-only answers.
type IntakeClarificationRound struct {
	Version                  IntakeClarificationRoundVersion
	ID                       IntakeClarificationRoundID
	IntakeID                 IntakeSessionID
	Ordinal                  int64
	ExpectedProposalRevision int64
	Questions                []IntakeClarificationQuestion
	Answers                  []IntakeClarificationRoundAnswer
}

// IntakeClarificationHistory is the pure aggregate used to validate round transitions.
// It is not adopted by the runtime in Stage 5A.
type IntakeClarificationHistory struct {
	IntakeID                IntakeSessionID
	CurrentProposalRevision int64
	Rounds                  []IntakeClarificationRound
}

// IntakeClarificationRoundDraft carries analyzer output for a proposed round.
type IntakeClarificationRoundDraft struct {
	Version                  IntakeClarificationRoundVersion
	ID                       IntakeClarificationRoundID
	ExpectedProposalRevision int64
	Questions                []IntakeClarificationQuestion
	ExplicitReanalysis       bool
}

// IntakeClarificationAnswerBatch is one atomic answer transition.
type IntakeClarificationAnswerBatch struct {
	Version                  IntakeClarificationRoundVersion
	RoundID                  IntakeClarificationRoundID
	ExpectedProposalRevision int64
	Answers                  []IntakeClarificationRoundAnswer
}

func exactNonBlank(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not have leading or trailing whitespace", field)
	}
	return nil
}

func validateQuestion(question IntakeClarificationQuestion) error {
	if err := exactNonBlank(string(question.ID), "clarification question id"); err != nil {
		return err
	}
	if question.Position < 1 {
		return fmt.Errorf("clarification question position must be positive")
	}
	if err := exactNonBlank(question.Question, "clarification question"); err != nil {
		return err
	}
	if err := exactNonBlank(question.Reason, "clarification reason"); err != nil {
		return err
	}
	if len(question.Alternatives) > 3 {
		return fmt.Errorf("clarification question permits at most three alternatives")
	}
	seen := map[string]struct{}{}
	for _, alternative := range question.Alternatives {
		if err := exactNonBlank(alternative, "clarification alternative"); err != nil {
			return err
		}
		if _, exists := seen[alternative]; exists {
			return fmt.Errorf("duplicate clarification alternative %q", alternative)
		}
		seen[alternative] = struct{}{}
	}
	return nil
}

func canonicalQuestions(questions []IntakeClarificationQuestion) ([]IntakeClarificationQuestion, error) {
	if len(questions) == 0 {
		return nil, fmt.Errorf("clarification round requires questions")
	}
	result := make([]IntakeClarificationQuestion, len(questions))
	for i, question := range questions {
		question.Alternatives = append([]string(nil), question.Alternatives...)
		if err := validateQuestion(question); err != nil {
			return nil, err
		}
		result[i] = question
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Position < result[j].Position })
	ids := map[IntakeClarificationQuestionID]struct{}{}
	for i, question := range result {
		if question.Position != int64(i+1) {
			return nil, fmt.Errorf("clarification question positions must be contiguous")
		}
		if _, exists := ids[question.ID]; exists {
			return nil, fmt.Errorf("duplicate clarification question id %q", question.ID)
		}
		ids[question.ID] = struct{}{}
	}
	return result, nil
}

func (round IntakeClarificationRound) Complete() bool {
	return len(round.Questions) > 0 && len(round.Answers) == len(round.Questions)
}

func validateRound(round IntakeClarificationRound) error {
	if round.Version != IntakeClarificationRoundV1 {
		return fmt.Errorf("unsupported clarification round version %q", round.Version)
	}
	if err := exactNonBlank(string(round.ID), "clarification round id"); err != nil {
		return err
	}
	if round.IntakeID.IsZero() {
		return fmt.Errorf("clarification round intake id is required")
	}
	if round.Ordinal < 1 {
		return fmt.Errorf("clarification round ordinal must be positive")
	}
	if round.ExpectedProposalRevision < 0 {
		return fmt.Errorf("clarification round proposal revision must not be negative")
	}
	questions, err := canonicalQuestions(round.Questions)
	if err != nil {
		return err
	}
	known := map[IntakeClarificationQuestionID]int{}
	for i, question := range questions {
		known[question.ID] = i
	}
	answered := map[IntakeClarificationQuestionID]struct{}{}
	for _, answer := range round.Answers {
		if answer.RoundID != round.ID {
			return fmt.Errorf("answer belongs to another clarification round")
		}
		if _, exists := known[answer.QuestionID]; !exists {
			return fmt.Errorf("answer belongs to an unknown clarification question")
		}
		if _, exists := answered[answer.QuestionID]; exists {
			return fmt.Errorf("clarification question already answered")
		}
		if err := exactNonBlank(answer.Answer, "clarification answer"); err != nil {
			return err
		}
		answered[answer.QuestionID] = struct{}{}
	}
	return nil
}

func (history IntakeClarificationHistory) validate() error {
	if history.IntakeID.IsZero() {
		return fmt.Errorf("clarification history intake id is required")
	}
	if history.CurrentProposalRevision < 0 {
		return fmt.Errorf("current proposal revision must not be negative")
	}
	ids := map[IntakeClarificationRoundID]struct{}{}
	for i, round := range history.Rounds {
		if err := validateRound(round); err != nil {
			return err
		}
		if round.IntakeID != history.IntakeID {
			return fmt.Errorf("clarification round belongs to another intake")
		}
		if round.Ordinal != int64(i+1) {
			return fmt.Errorf("clarification round ordinals must be contiguous")
		}
		if _, exists := ids[round.ID]; exists {
			return fmt.Errorf("duplicate clarification round id %q", round.ID)
		}
		ids[round.ID] = struct{}{}
		if i < len(history.Rounds)-1 && !round.Complete() {
			return fmt.Errorf("later clarification round requires prior completion")
		}
	}
	return nil
}

func cloneHistory(history IntakeClarificationHistory) IntakeClarificationHistory {
	result := history
	result.Rounds = make([]IntakeClarificationRound, len(history.Rounds))
	for i, round := range history.Rounds {
		result.Rounds[i] = round
		result.Rounds[i].Questions, _ = canonicalQuestions(round.Questions)
		result.Rounds[i].Answers = append([]IntakeClarificationRoundAnswer(nil), round.Answers...)
	}
	return result
}

// OpenClarificationRound validates and returns a new aggregate without mutating input.
func OpenClarificationRound(history IntakeClarificationHistory, draft IntakeClarificationRoundDraft) (IntakeClarificationHistory, error) {
	if err := history.validate(); err != nil {
		return IntakeClarificationHistory{}, err
	}
	if draft.Version != IntakeClarificationRoundV1 {
		return IntakeClarificationHistory{}, fmt.Errorf("unsupported clarification round version %q", draft.Version)
	}
	if err := exactNonBlank(string(draft.ID), "clarification round id"); err != nil {
		return IntakeClarificationHistory{}, err
	}
	for _, round := range history.Rounds {
		if round.ID == draft.ID {
			return IntakeClarificationHistory{}, fmt.Errorf("clarification round id already used")
		}
	}
	if len(history.Rounds) > 0 {
		if !history.Rounds[len(history.Rounds)-1].Complete() {
			return IntakeClarificationHistory{}, fmt.Errorf("cannot open concurrent clarification rounds")
		}
		if !draft.ExplicitReanalysis {
			return IntakeClarificationHistory{}, fmt.Errorf("later clarification round requires explicit reanalysis")
		}
	}
	if draft.ExpectedProposalRevision != history.CurrentProposalRevision {
		return IntakeClarificationHistory{}, fmt.Errorf("stale clarification round proposal revision")
	}
	questions, err := canonicalQuestions(draft.Questions)
	if err != nil {
		return IntakeClarificationHistory{}, err
	}
	result := cloneHistory(history)
	result.Rounds = append(result.Rounds, IntakeClarificationRound{Version: draft.Version, ID: draft.ID, IntakeID: history.IntakeID, Ordinal: int64(len(result.Rounds) + 1), ExpectedProposalRevision: draft.ExpectedProposalRevision, Questions: questions})
	return result, nil
}

// AnswerClarificationRound applies a whole batch or rejects it without a partial result.
func AnswerClarificationRound(history IntakeClarificationHistory, batch IntakeClarificationAnswerBatch) (IntakeClarificationHistory, error) {
	if err := history.validate(); err != nil {
		return IntakeClarificationHistory{}, err
	}
	if batch.Version != IntakeClarificationRoundV1 {
		return IntakeClarificationHistory{}, fmt.Errorf("unsupported clarification round version %q", batch.Version)
	}
	if len(batch.Answers) == 0 {
		return IntakeClarificationHistory{}, fmt.Errorf("clarification answer batch must not be empty")
	}
	if batch.ExpectedProposalRevision != history.CurrentProposalRevision {
		return IntakeClarificationHistory{}, fmt.Errorf("stale clarification answer proposal revision")
	}
	index := -1
	for i, round := range history.Rounds {
		if round.ID == batch.RoundID {
			index = i
			break
		}
	}
	if index < 0 {
		return IntakeClarificationHistory{}, fmt.Errorf("clarification round not found")
	}
	round := history.Rounds[index]
	if round.ExpectedProposalRevision != batch.ExpectedProposalRevision {
		return IntakeClarificationHistory{}, fmt.Errorf("clarification round proposal revision mismatch")
	}
	if round.Complete() {
		return IntakeClarificationHistory{}, fmt.Errorf("completed clarification round cannot be answered")
	}
	known := map[IntakeClarificationQuestionID]int{}
	for i, question := range round.Questions {
		known[question.ID] = i
	}
	seen := map[IntakeClarificationQuestionID]struct{}{}
	for _, existing := range round.Answers {
		seen[existing.QuestionID] = struct{}{}
	}
	additions := make([]IntakeClarificationRoundAnswer, len(batch.Answers))
	for i, answer := range batch.Answers {
		if answer.RoundID != batch.RoundID {
			return IntakeClarificationHistory{}, fmt.Errorf("answer belongs to another clarification round")
		}
		if _, exists := known[answer.QuestionID]; !exists {
			return IntakeClarificationHistory{}, fmt.Errorf("answer belongs to an unknown clarification question")
		}
		if _, exists := seen[answer.QuestionID]; exists {
			return IntakeClarificationHistory{}, fmt.Errorf("clarification question already answered")
		}
		if err := exactNonBlank(answer.Answer, "clarification answer"); err != nil {
			return IntakeClarificationHistory{}, err
		}
		seen[answer.QuestionID] = struct{}{}
		additions[i] = answer
	}
	result := cloneHistory(history)
	result.Rounds[index].Answers = append(result.Rounds[index].Answers, additions...)
	sort.Slice(result.Rounds[index].Answers, func(i, j int) bool {
		return known[result.Rounds[index].Answers[i].QuestionID] < known[result.Rounds[index].Answers[j].QuestionID]
	})
	return result, nil
}

// AdvanceIntakeProposal appends the semantic proposal transition: exactly one revision.
func AdvanceIntakeProposal(history IntakeClarificationHistory, expectedRevision int64) (IntakeClarificationHistory, error) {
	if err := history.validate(); err != nil {
		return IntakeClarificationHistory{}, err
	}
	if expectedRevision != history.CurrentProposalRevision {
		return IntakeClarificationHistory{}, fmt.Errorf("stale proposal revision")
	}
	result := cloneHistory(history)
	result.CurrentProposalRevision++
	return result, nil
}

// ValidateIntakeAnalyzerChoice enforces proposal XOR clarification-round output.
func ValidateIntakeAnalyzerChoice(hasProposal, hasClarificationRound bool) error {
	if hasProposal == hasClarificationRound {
		return fmt.Errorf("analyzer must return exactly one proposal or clarification round")
	}
	return nil
}

// LegacyClarificationRound requires revision evidence because the legacy row did
// not store the proposal revision. Stage 5B must migrate or derive that evidence.
func LegacyClarificationRound(intakeID IntakeSessionID, clarificationID ClarificationRequestID, expectedProposalRevision *int64, question IntakeClarificationQuestion) (IntakeClarificationRound, error) {
	if intakeID.IsZero() || strings.TrimSpace(string(clarificationID)) == "" {
		return IntakeClarificationRound{}, fmt.Errorf("legacy clarification identity is required")
	}
	if expectedProposalRevision == nil {
		return IntakeClarificationRound{}, fmt.Errorf("legacy clarification proposal revision evidence is required")
	}
	if question.ID == "" {
		question.ID = IntakeClarificationQuestionID(clarificationID)
	}
	question.Position = 1
	sum := sha256.Sum256([]byte("intake.clarification-round.legacy.v1\x00" + intakeID.String() + "\x00" + string(clarificationID)))
	round := IntakeClarificationRound{Version: IntakeClarificationRoundV1, ID: IntakeClarificationRoundID("legacy-v1-" + hex.EncodeToString(sum[:16])), IntakeID: intakeID, Ordinal: 1, ExpectedProposalRevision: *expectedProposalRevision, Questions: []IntakeClarificationQuestion{question}}
	if err := validateRound(round); err != nil {
		return IntakeClarificationRound{}, err
	}
	return round, nil
}

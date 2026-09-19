package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// IntakeClarificationRoundQuestionResponse freezes the public question shape.
type IntakeClarificationRoundQuestionResponse struct {
	ID                  string   `json:"id"`
	Position            int64    `json:"position"`
	Question            string   `json:"question"`
	Reason              string   `json:"reason"`
	Recommendation      string   `json:"recommendation"`
	Alternatives        []string `json:"alternatives"`
	DeferralConsequence string   `json:"deferralConsequence"`
}

// IntakeClarificationRoundAnswerResponse freezes the public answer shape.
type IntakeClarificationRoundAnswerResponse struct {
	QuestionID string `json:"questionId"`
	Answer     string `json:"answer"`
}

// IntakeClarificationRoundResponse intentionally omits route-bound intake
// identity, ordinal machinery, and answer timestamps.
type IntakeClarificationRoundResponse struct {
	Version                  string                                     `json:"version"`
	RoundID                  string                                     `json:"roundId"`
	ExpectedProposalRevision int64                                      `json:"expectedProposalRevision"`
	Questions                []IntakeClarificationRoundQuestionResponse `json:"questions"`
	Answers                  []IntakeClarificationRoundAnswerResponse   `json:"answers"`
	Complete                 bool                                       `json:"complete"`
}

// IntakeClarificationRoundAnswerInput has no derived state or route lineage.
type IntakeClarificationRoundAnswerInput struct {
	QuestionID string `json:"questionId"`
	Answer     string `json:"answer"`
}

// IntakeClarificationRoundAnswerBatchRequest freezes the additive wire body.
type IntakeClarificationRoundAnswerBatchRequest struct {
	Version                  string                                `json:"version"`
	RoundID                  string                                `json:"roundId"`
	ExpectedProposalRevision int64                                 `json:"expectedProposalRevision"`
	Answers                  []IntakeClarificationRoundAnswerInput `json:"answers"`
}

// DecodeIntakeClarificationRoundAnswerBatch strictly decodes exactly one JSON
// value. Unknown top-level and nested keys, including legacy answer keys, fail.
func DecodeIntakeClarificationRoundAnswerBatch(body []byte) (IntakeClarificationRoundAnswerBatchRequest, error) {
	var request IntakeClarificationRoundAnswerBatchRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("decode clarification round answer batch: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return request, fmt.Errorf("decode clarification round answer batch: multiple JSON values")
		}
		return request, fmt.Errorf("decode clarification round answer batch: %w", err)
	}
	return request, nil
}

// Domain converts the frozen wire value without repairing or normalizing it.
func (request IntakeClarificationRoundAnswerBatchRequest) Domain() domain.IntakeClarificationAnswerBatch {
	answers := make([]domain.IntakeClarificationRoundAnswer, len(request.Answers))
	for i, answer := range request.Answers {
		answers[i] = domain.IntakeClarificationRoundAnswer{
			RoundID:    domain.IntakeClarificationRoundID(request.RoundID),
			QuestionID: domain.IntakeClarificationQuestionID(answer.QuestionID),
			Answer:     answer.Answer,
		}
	}
	return domain.IntakeClarificationAnswerBatch{
		Version:                  domain.IntakeClarificationRoundVersion(request.Version),
		RoundID:                  domain.IntakeClarificationRoundID(request.RoundID),
		ExpectedProposalRevision: request.ExpectedProposalRevision,
		Answers:                  answers,
	}
}

// NewIntakeClarificationRoundResponse maps canonical domain order without
// adding runtime adoption or endpoint behavior.
func NewIntakeClarificationRoundResponse(round domain.IntakeClarificationRound) IntakeClarificationRoundResponse {
	questions := make([]IntakeClarificationRoundQuestionResponse, len(round.Questions))
	positions := make(map[domain.IntakeClarificationQuestionID]int, len(round.Questions))
	for i, question := range round.Questions {
		positions[question.ID] = i
		questions[i] = IntakeClarificationRoundQuestionResponse{
			ID: string(question.ID), Position: question.Position, Question: question.Question,
			Reason: question.Reason, Recommendation: question.Recommendation,
			Alternatives:        append([]string(nil), question.Alternatives...),
			DeferralConsequence: question.DeferralConsequence,
		}
		if questions[i].Alternatives == nil {
			questions[i].Alternatives = []string{}
		}
	}
	answers := make([]domain.IntakeClarificationRoundAnswer, len(round.Answers))
	copy(answers, round.Answers)
	sortRoundAnswers(answers, positions)
	answerResponses := make([]IntakeClarificationRoundAnswerResponse, len(answers))
	for i, answer := range answers {
		answerResponses[i] = IntakeClarificationRoundAnswerResponse{QuestionID: string(answer.QuestionID), Answer: answer.Answer}
	}
	return IntakeClarificationRoundResponse{
		Version: string(round.Version), RoundID: string(round.ID),
		ExpectedProposalRevision: round.ExpectedProposalRevision,
		Questions:                questions, Answers: answerResponses, Complete: round.Complete(),
	}
}

func sortRoundAnswers(answers []domain.IntakeClarificationRoundAnswer, positions map[domain.IntakeClarificationQuestionID]int) {
	for i := 1; i < len(answers); i++ {
		for j := i; j > 0 && positions[answers[j].QuestionID] < positions[answers[j-1].QuestionID]; j-- {
			answers[j], answers[j-1] = answers[j-1], answers[j]
		}
	}
}

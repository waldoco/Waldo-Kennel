package controllers

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func TestDecodeIntakeClarificationRoundAnswerBatchStrictWireShape(t *testing.T) {
	valid := []byte(`{"version":"intake.clarification-round.v1","roundId":"round-1","expectedProposalRevision":3,"answers":[{"questionId":"q1","answer":"A"}]}`)
	request, err := DecodeIntakeClarificationRoundAnswerBatch(valid)
	if err != nil {
		t.Fatal(err)
	}
	batch := request.Domain()
	if batch.RoundID != "round-1" || batch.Answers[0].RoundID != "round-1" || batch.Answers[0].Answer != "A" {
		t.Fatalf("domain mapping = %+v", batch)
	}

	invalid := [][]byte{
		[]byte(`{"version":"intake.clarification-round.v1","roundId":"round-1","expectedProposalRevision":3,"unknown":true,"answers":[]}`),
		[]byte(`{"version":"intake.clarification-round.v1","roundId":"round-1","expectedProposalRevision":3,"answers":[{"questionId":"q1","answer":"A","unknown":true}]}`),
		[]byte(`{"answer":"legacy","version":"intake.clarification-round.v1","roundId":"round-1","expectedProposalRevision":3,"answers":[]}`),
		[]byte(`{"version":"intake.clarification-round.v1","roundId":"round-1","expectedProposalRevision":3,"answers":[],"answer":"legacy"}`),
		[]byte(`{"version":"intake.clarification-round.v1","roundId":"round-1","expectedProposalRevision":3,"answers":[]} {}`),
	}
	for _, body := range invalid {
		if _, err := DecodeIntakeClarificationRoundAnswerBatch(body); err == nil {
			t.Fatalf("accepted invalid body: %s", body)
		}
	}
}

func TestClarificationRoundAnswerBatchPinsOmittedVersusEmptyAnswers(t *testing.T) {
	for _, test := range []struct {
		body []byte
		want []byte
	}{
		{[]byte(`{"version":"intake.clarification-round.v1","roundId":"r","expectedProposalRevision":0}`), []byte(`{"version":"intake.clarification-round.v1","roundId":"r","expectedProposalRevision":0,"answers":null}`)},
		{[]byte(`{"version":"intake.clarification-round.v1","roundId":"r","expectedProposalRevision":0,"answers":[]}`), []byte(`{"version":"intake.clarification-round.v1","roundId":"r","expectedProposalRevision":0,"answers":[]}`)},
	} {
		request, err := DecodeIntakeClarificationRoundAnswerBatch(test.body)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, test.want) {
			t.Fatalf("bytes = %s, want %s", actual, test.want)
		}
	}
}

func TestClarificationRoundResponseExactShapeAndNoAliasing(t *testing.T) {
	round := domain.IntakeClarificationRound{
		Version: domain.IntakeClarificationRoundV1, ID: "round-1", IntakeID: "private-intake", Ordinal: 7, ExpectedProposalRevision: 3,
		Questions: []domain.IntakeClarificationQuestion{
			{ID: "q1", Position: 1, Question: "First?", Reason: "Reason", Alternatives: nil},
			{ID: "q2", Position: 2, Question: "Second?", Reason: "Reason", Alternatives: []string{"A", "B"}},
		},
		Answers: []domain.IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "q2", Answer: "B"}, {RoundID: "round-1", QuestionID: "q1", Answer: "A"}},
	}
	response := NewIntakeClarificationRoundResponse(round)
	round.Questions[1].Alternatives[0] = "mutated"
	if response.Questions[1].Alternatives[0] != "A" {
		t.Fatal("response aliases domain slices")
	}
	actual, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"version":"intake.clarification-round.v1","roundId":"round-1","expectedProposalRevision":3,"questions":[{"id":"q1","position":1,"question":"First?","reason":"Reason","recommendation":"","alternatives":[],"deferralConsequence":""},{"id":"q2","position":2,"question":"Second?","reason":"Reason","recommendation":"","alternatives":["A","B"],"deferralConsequence":""}],"answers":[{"questionId":"q1","answer":"A"},{"questionId":"q2","answer":"B"}],"complete":true}`)
	if !bytes.Equal(actual, want) {
		t.Fatalf("response = %s\nwant     = %s", actual, want)
	}
	var object map[string]any
	if err := json.Unmarshal(actual, &object); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"intakeId", "ordinal", "answeredAt"} {
		if _, exists := object[forbidden]; exists {
			t.Fatalf("response exposed %s", forbidden)
		}
	}
}

func TestClarificationRoundBatchRequestHasOnlyFrozenFields(t *testing.T) {
	typeOf := reflect.TypeOf(IntakeClarificationRoundAnswerBatchRequest{})
	fields := make([]string, typeOf.NumField())
	for i := range fields {
		fields[i] = typeOf.Field(i).Tag.Get("json")
	}
	want := []string{"version", "roundId", "expectedProposalRevision", "answers"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("request fields = %v", fields)
	}
}

package domain

import (
	"reflect"
	"testing"
)

func testQuestion(id IntakeClarificationQuestionID, position int64) IntakeClarificationQuestion {
	return IntakeClarificationQuestion{ID: id, Position: position, Question: "Choose " + string(id), Reason: "The choice changes the contract."}
}

func testHistory() IntakeClarificationHistory {
	return IntakeClarificationHistory{IntakeID: "intake-1", CurrentProposalRevision: 3}
}

func openTestRound(t *testing.T, history IntakeClarificationHistory, id IntakeClarificationRoundID, explicit bool, questions ...IntakeClarificationQuestion) IntakeClarificationHistory {
	t.Helper()
	result, err := OpenClarificationRound(history, IntakeClarificationRoundDraft{Version: IntakeClarificationRoundV1, ID: id, ExpectedProposalRevision: history.CurrentProposalRevision, Questions: questions, ExplicitReanalysis: explicit})
	if err != nil {
		t.Fatalf("open round: %v", err)
	}
	return result
}

func answerTestRound(t *testing.T, history IntakeClarificationHistory, roundID IntakeClarificationRoundID, answers ...IntakeClarificationRoundAnswer) IntakeClarificationHistory {
	t.Helper()
	result, err := AnswerClarificationRound(history, IntakeClarificationAnswerBatch{Version: IntakeClarificationRoundV1, RoundID: roundID, ExpectedProposalRevision: history.CurrentProposalRevision, Answers: answers})
	if err != nil {
		t.Fatalf("answer round: %v", err)
	}
	return result
}

func TestClarificationRoundCanonicalTransitions(t *testing.T) {
	original := testHistory()
	history := openTestRound(t, original, "round-1", false, testQuestion("q2", 2), testQuestion("q1", 1))
	if len(original.Rounds) != 0 || history.CurrentProposalRevision != 3 {
		t.Fatal("opening round mutated input or proposal revision")
	}
	if history.Rounds[0].Ordinal != 1 || history.Rounds[0].Questions[0].ID != "q1" {
		t.Fatalf("round not canonical: %+v", history.Rounds[0])
	}

	partial := answerTestRound(t, history, "round-1", IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q2", Answer: "B"})
	if partial.Rounds[0].Complete() {
		t.Fatal("partial round reported complete")
	}
	complete := answerTestRound(t, partial, "round-1", IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q1", Answer: "A"})
	if !complete.Rounds[0].Complete() || complete.Rounds[0].Answers[0].QuestionID != "q1" {
		t.Fatalf("answers not complete/canonical: %+v", complete.Rounds[0])
	}
	if history.Rounds[0].Answers != nil || complete.CurrentProposalRevision != 3 {
		t.Fatal("answering mutated input or proposal revision")
	}

	second := openTestRound(t, complete, "round-2", true, testQuestion("q3", 1))
	if second.Rounds[1].Ordinal != 2 {
		t.Fatalf("later ordinal = %d", second.Rounds[1].Ordinal)
	}
	second = answerTestRound(t, second, "round-2", IntakeClarificationRoundAnswer{RoundID: "round-2", QuestionID: "q3", Answer: "C"})
	third := openTestRound(t, second, "round-3", true, testQuestion("q4", 1))
	if third.Rounds[2].ExpectedProposalRevision != 3 {
		t.Fatal("multiple completed rounds at one revision not preserved")
	}
}

func TestClarificationRoundRejectsInvalidOpenTransitions(t *testing.T) {
	history := openTestRound(t, testHistory(), "round-1", false, testQuestion("q1", 1))
	cases := []IntakeClarificationRoundDraft{
		{Version: IntakeClarificationRoundV1, ID: "round-2", ExpectedProposalRevision: 3, ExplicitReanalysis: true, Questions: []IntakeClarificationQuestion{testQuestion("q2", 1)}},
		{Version: IntakeClarificationRoundV1, ID: "round-1", ExpectedProposalRevision: 3, ExplicitReanalysis: true, Questions: []IntakeClarificationQuestion{testQuestion("q2", 1)}},
		{Version: IntakeClarificationRoundV1, ID: "round-2", ExpectedProposalRevision: 2, ExplicitReanalysis: true, Questions: []IntakeClarificationQuestion{testQuestion("q2", 1)}},
	}
	for _, draft := range cases {
		before := cloneHistory(history)
		if _, err := OpenClarificationRound(history, draft); err == nil {
			t.Fatalf("accepted invalid draft: %+v", draft)
		}
		if !reflect.DeepEqual(history, before) {
			t.Fatal("failed open mutated aggregate")
		}
	}
	complete := answerTestRound(t, history, "round-1", IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q1", Answer: "A"})
	if _, err := OpenClarificationRound(complete, IntakeClarificationRoundDraft{Version: IntakeClarificationRoundV1, ID: "round-2", ExpectedProposalRevision: 3, Questions: []IntakeClarificationQuestion{testQuestion("q2", 1)}}); err == nil {
		t.Fatal("later round accepted without explicit reanalysis")
	}

	invalidHistories := []IntakeClarificationHistory{
		{IntakeID: "intake-1", CurrentProposalRevision: 3, Rounds: []IntakeClarificationRound{{Version: IntakeClarificationRoundV1, ID: "r", IntakeID: "intake-1", Ordinal: 2, ExpectedProposalRevision: 3, Questions: []IntakeClarificationQuestion{testQuestion("q", 1)}}}},
		{IntakeID: "intake-1", CurrentProposalRevision: 3, Rounds: append(cloneHistory(complete).Rounds, complete.Rounds[0])},
	}
	for _, invalid := range invalidHistories {
		if _, err := OpenClarificationRound(invalid, IntakeClarificationRoundDraft{}); err == nil {
			t.Fatal("accepted skipped/reused ordinal history")
		}
	}
}

func TestClarificationAnswerBatchIsAtomicStrictAndSingleUse(t *testing.T) {
	history := openTestRound(t, testHistory(), "round-1", false, testQuestion("Alpha", 1), testQuestion("alpha", 2))
	bad := []IntakeClarificationAnswerBatch{
		{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 3},
		{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 2, Answers: []IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "Alpha", Answer: "A"}}},
		{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 3, Answers: []IntakeClarificationRoundAnswer{{RoundID: "foreign", QuestionID: "Alpha", Answer: "A"}}},
		{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 3, Answers: []IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "missing", Answer: "A"}}},
		{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 3, Answers: []IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "Alpha", Answer: " A"}}},
		{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 3, Answers: []IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "Alpha", Answer: "A"}, {RoundID: "round-1", QuestionID: "Alpha", Answer: "A"}}},
	}
	for _, batch := range bad {
		before := cloneHistory(history)
		if _, err := AnswerClarificationRound(history, batch); err == nil {
			t.Fatalf("accepted invalid batch: %+v", batch)
		}
		if !reflect.DeepEqual(history, before) {
			t.Fatal("rejected batch mutated aggregate")
		}
	}

	partial := answerTestRound(t, history, "round-1", IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "Alpha", Answer: "A"})
	for _, answer := range []string{"A", "B"} {
		if _, err := AnswerClarificationRound(partial, IntakeClarificationAnswerBatch{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 3, Answers: []IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "Alpha", Answer: answer}}}); err == nil {
			t.Fatal("accepted second answer")
		}
	}
	complete := answerTestRound(t, partial, "round-1", IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "alpha", Answer: "a"})
	if _, err := AnswerClarificationRound(complete, IntakeClarificationAnswerBatch{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 3, Answers: []IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "alpha", Answer: "again"}}}); err == nil {
		t.Fatal("accepted answer after completion")
	}
}

func TestClarificationQuestionValidationAndNoAliasing(t *testing.T) {
	question := testQuestion("q1", 1)
	question.Alternatives = []string{"A", "a"}
	history := openTestRound(t, testHistory(), "round-1", false, question)
	question.Alternatives[0] = "mutated"
	if history.Rounds[0].Questions[0].Alternatives[0] != "A" {
		t.Fatal("round aliases caller question slices")
	}
	invalid := []IntakeClarificationQuestion{
		{ID: "q", Position: 1, Question: " padded", Reason: "reason"},
		{ID: "q", Position: 1, Question: "question", Reason: "reason", Alternatives: []string{"A", "A"}},
		{ID: "q", Position: 1, Question: "question", Reason: "reason", Alternatives: []string{""}},
	}
	for _, q := range invalid {
		if _, err := OpenClarificationRound(testHistory(), IntakeClarificationRoundDraft{Version: IntakeClarificationRoundV1, ID: "r", ExpectedProposalRevision: 3, Questions: []IntakeClarificationQuestion{q}}); err == nil {
			t.Fatalf("accepted invalid question: %+v", q)
		}
	}
}

func TestProposalRevisionTransitionRequiresCompleteHistoryAndDoesNotRebindRounds(t *testing.T) {
	empty := testHistory()
	emptyAdvanced, err := AdvanceIntakeProposal(empty, 3)
	if err != nil || emptyAdvanced.CurrentProposalRevision != 4 {
		t.Fatalf("advance without rounds: revision=%d err=%v", emptyAdvanced.CurrentProposalRevision, err)
	}

	incomplete := openTestRound(t, testHistory(), "round-1", false, testQuestion("q1", 1))
	before := cloneHistory(incomplete)
	if _, err := AdvanceIntakeProposal(incomplete, 3); err == nil {
		t.Fatal("advanced proposal with incomplete latest round")
	}
	if !reflect.DeepEqual(incomplete, before) {
		t.Fatal("rejected proposal advance mutated history")
	}

	complete := answerTestRound(t, incomplete, "round-1", IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q1", Answer: "A"})
	advanced, err := AdvanceIntakeProposal(complete, 3)
	if err != nil {
		t.Fatal(err)
	}
	if advanced.CurrentProposalRevision != 4 || advanced.Rounds[0].ExpectedProposalRevision != 3 {
		t.Fatal("proposal advance rebound old round")
	}
	if _, err := AdvanceIntakeProposal(complete, 2); err == nil {
		t.Fatal("accepted stale proposal advance")
	}
	if _, err := AnswerClarificationRound(advanced, IntakeClarificationAnswerBatch{Version: IntakeClarificationRoundV1, RoundID: "round-1", ExpectedProposalRevision: 3, Answers: []IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "q1", Answer: "again"}}}); err == nil {
		t.Fatal("accepted stale answer after proposal advance")
	}
}

func TestClarificationHistoryRejectsFutureRegressionPermutationAndTwoIncompleteRounds(t *testing.T) {
	completed := openTestRound(t, testHistory(), "round-1", false, testQuestion("q1", 1))
	completed = answerTestRound(t, completed, "round-1", IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q1", Answer: "A"})
	advanced, err := AdvanceIntakeProposal(completed, 3)
	if err != nil {
		t.Fatal(err)
	}
	second := openTestRound(t, advanced, "round-2", true, testQuestion("q2", 1))

	future := cloneHistory(second)
	future.Rounds[1].ExpectedProposalRevision = 5
	regressing := cloneHistory(second)
	regressing.Rounds[0].ExpectedProposalRevision = 4
	regressing.Rounds[1].ExpectedProposalRevision = 3
	permuted := cloneHistory(second)
	permuted.Rounds[0], permuted.Rounds[1] = permuted.Rounds[1], permuted.Rounds[0]
	twoIncomplete := cloneHistory(second)
	twoIncomplete.Rounds[0].Answers = nil

	for name, malformed := range map[string]IntakeClarificationHistory{
		"future": future, "regressing": regressing, "permuted": permuted, "two-incomplete": twoIncomplete,
	} {
		if _, err := OpenClarificationRound(malformed, IntakeClarificationRoundDraft{}); err == nil {
			t.Fatalf("accepted malformed %s history", name)
		}
	}
}

func TestEquivalentQuestionAndAnswerPermutationsCanonicalizeIdentically(t *testing.T) {
	left := openTestRound(t, testHistory(), "round-1", false, testQuestion("q2", 2), testQuestion("q1", 1))
	right := openTestRound(t, testHistory(), "round-1", false, testQuestion("q1", 1), testQuestion("q2", 2))
	left = answerTestRound(t, left, "round-1",
		IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q2", Answer: "B"},
		IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q1", Answer: "A"})
	right = answerTestRound(t, right, "round-1",
		IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q1", Answer: "A"},
		IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "q2", Answer: "B"})
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("equivalent permutations differ:\nleft=%+v\nright=%+v", left, right)
	}
}

func TestCrossRoundAnswerReplayRejectsRepeatedQuestionID(t *testing.T) {
	history := openTestRound(t, testHistory(), "round-1", false, testQuestion("same-question", 1))
	history = answerTestRound(t, history, "round-1", IntakeClarificationRoundAnswer{RoundID: "round-1", QuestionID: "same-question", Answer: "first"})
	history = openTestRound(t, history, "round-2", true, testQuestion("same-question", 1))
	before := cloneHistory(history)
	_, err := AnswerClarificationRound(history, IntakeClarificationAnswerBatch{
		Version: IntakeClarificationRoundV1, RoundID: "round-2", ExpectedProposalRevision: 3,
		Answers: []IntakeClarificationRoundAnswer{{RoundID: "round-1", QuestionID: "same-question", Answer: "replay"}},
	})
	if err == nil {
		t.Fatal("accepted prior-round composite answer in newer round")
	}
	if !reflect.DeepEqual(history, before) {
		t.Fatal("cross-round replay rejection mutated history")
	}
}

func TestAnalyzerChoiceIsExclusive(t *testing.T) {
	for _, values := range [][3]bool{{false, false, false}, {true, true, false}, {true, false, true}, {false, true, true}} {
		err := ValidateIntakeAnalyzerChoice(values[0], values[1])
		if (err == nil) != values[2] {
			t.Fatalf("choice (%v,%v) error=%v", values[0], values[1], err)
		}
	}
}

func TestLegacyClarificationRoundRequiresRevisionEvidenceAndIsStable(t *testing.T) {
	question := testQuestion("legacy-question", 1)
	question.ID = ""
	if _, err := LegacyClarificationRound("intake-1", "clarification-7", nil, question); err == nil {
		t.Fatal("invented legacy proposal revision")
	}
	revision := int64(4)
	first, err := LegacyClarificationRound("intake-1", "clarification-7", &revision, question)
	if err != nil {
		t.Fatal(err)
	}
	later := int64(12)
	second, err := LegacyClarificationRound("intake-1", "clarification-7", &later, question)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := LegacyClarificationRound("intake-1", "clarification-7", &revision, question)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.ID != restarted.ID || first.Questions[0].ID != "clarification-7" || first.Ordinal != 1 {
		t.Fatalf("legacy identity unstable: %+v %+v", first, second)
	}
	if first.ExpectedProposalRevision != 4 || second.ExpectedProposalRevision != 12 {
		t.Fatal("legacy evidence was not preserved")
	}
}

package store_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func createRoundIntake(t *testing.T, id domain.IntakeSessionID) (*testRoundFixture, context.Context) {
	t.Helper()
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "round-project")
	now := time.Date(2026, 9, 19, 1, 0, 0, 0, time.UTC)
	_, err := s.CreateIntake(ctx, domain.IntakeSession{ID: id, SourceSurface: domain.IntakeSourceWork, Purpose: domain.IntakePurposeOutcome, ProjectID: "round-project", Statement: "clarify", Status: domain.IntakeStatusCaptured, CreatedAt: now, UpdatedAt: now}, nil, ports.IntakeIdempotency{Key: "key-" + id.String(), Fingerprint: "fp"})
	if err != nil {
		t.Fatal(err)
	}
	return &testRoundFixture{store: s, now: now}, ctx
}

type testRoundFixture struct {
	store interface {
		OpenIntakeClarificationRound(context.Context, domain.IntakeSessionID, domain.IntakeClarificationRoundDraft, time.Time) (domain.IntakeClarificationRound, error)
		AnswerIntakeClarificationRound(context.Context, domain.IntakeSessionID, domain.IntakeClarificationAnswerBatch, time.Time) (domain.IntakeClarificationRound, error)
		ListIntakeClarificationHistory(context.Context, domain.IntakeSessionID, ports.IntakeClarificationHistoryCursor, int) (ports.IntakeClarificationHistoryPage, error)
	}
	now time.Time
}

func draft(id string, revision int64, reanalysis bool) domain.IntakeClarificationRoundDraft {
	return domain.IntakeClarificationRoundDraft{Version: domain.IntakeClarificationRoundV1, ID: domain.IntakeClarificationRoundID(id), ExpectedProposalRevision: revision, ExplicitReanalysis: reanalysis, Questions: []domain.IntakeClarificationQuestion{{ID: domain.IntakeClarificationQuestionID("q-" + id), Position: 1, Question: "Question?", Reason: "Reason"}}}
}

func TestIntakeClarificationRoundStoreAtomicHistoryAndCursor(t *testing.T) {
	f, ctx := createRoundIntake(t, "round-intake")
	r1, err := f.store.OpenIntakeClarificationRound(ctx, "round-intake", draft("r1", 0, false), f.now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.AnswerIntakeClarificationRound(ctx, "round-intake", domain.IntakeClarificationAnswerBatch{Version: domain.IntakeClarificationRoundV1, RoundID: r1.ID, ExpectedProposalRevision: 0, Answers: []domain.IntakeClarificationRoundAnswer{{RoundID: r1.ID, QuestionID: r1.Questions[0].ID, Answer: "A1"}}}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := f.store.OpenIntakeClarificationRound(ctx, "round-intake", draft("r2", 0, true), f.now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.AnswerIntakeClarificationRound(ctx, "round-intake", domain.IntakeClarificationAnswerBatch{Version: domain.IntakeClarificationRoundV1, RoundID: r2.ID, ExpectedProposalRevision: 0, Answers: []domain.IntakeClarificationRoundAnswer{{RoundID: r2.ID, QuestionID: r2.Questions[0].ID, Answer: "A2"}}}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.store.ListIntakeClarificationHistory(ctx, "round-intake", "", 1)
	if err != nil || len(first.Rounds) != 1 || first.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	// Later appends cannot move or alter a frozen walk, and the same cursor+limit replays identically.
	if _, err := f.store.OpenIntakeClarificationRound(ctx, "round-intake", draft("r3", 0, true), f.now); err != nil {
		t.Fatal(err)
	}
	replay, err := f.store.ListIntakeClarificationHistory(ctx, "round-intake", first.NextCursor, 1)
	if err != nil || len(replay.Rounds) != 1 || replay.Rounds[0].ID != "r2" || replay.NextCursor != "" {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	// A child answer appended after cursor mint must not change the frozen aggregate replay.
	if _, err := f.store.AnswerIntakeClarificationRound(ctx, "round-intake", domain.IntakeClarificationAnswerBatch{Version: domain.IntakeClarificationRoundV1, RoundID: "r3", ExpectedProposalRevision: 0, Answers: []domain.IntakeClarificationRoundAnswer{{RoundID: "r3", QuestionID: "q-r3", Answer: "late"}}}, f.now); err != nil {
		t.Fatal(err)
	}
	replayAfterAnswer, err := f.store.ListIntakeClarificationHistory(ctx, "round-intake", first.NextCursor, 1)
	if err != nil || !reflect.DeepEqual(replay, replayAfterAnswer) {
		t.Fatalf("frozen replay changed after answer append: before=%+v after=%+v err=%v", replay, replayAfterAnswer, err)
	}
	if _, _, _, err := testCursorCodec(t).Parse(first.NextCursor, "other-intake"); err == nil {
		t.Fatal("cross-intake cursor accepted")
	}
	for name, cursor := range map[string]ports.IntakeClarificationHistoryCursor{
		"malformed":                   "%%%",
		"padding":                     first.NextCursor + "=",
		"bit flip":                    flipCursorBit(t, first.NextCursor),
		"non-canonical JSON":          rewriteCursor(t, first.NextCursor, func(m map[string]any) { m["extra"] = true }),
		"swapped IDs":                 rewriteCursor(t, first.NextCursor, func(m map[string]any) { m["hr"], m["ar"] = m["ar"], m["hr"] }),
		"nonexistent high water":      rewriteCursor(t, first.NextCursor, func(m map[string]any) { m["hr"] = "missing" }),
		"impossible bounds":           rewriteCursor(t, first.NextCursor, func(m map[string]any) { m["ao"] = float64(99) }),
		"answer high water decrement": rewriteCursor(t, first.NextCursor, func(m map[string]any) { m["aw"] = m["aw"].(float64) - 1 }),
		"answer high water increment": rewriteCursor(t, first.NextCursor, func(m map[string]any) { m["aw"] = m["aw"].(float64) + 1 }),
		"answer high water huge":      rewriteCursor(t, first.NextCursor, func(m map[string]any) { m["aw"] = float64(1 << 52) }),
	} {
		if _, err := f.store.ListIntakeClarificationHistory(ctx, "round-intake", cursor, 1); err == nil {
			t.Fatalf("%s cursor accepted", name)
		}
	}
	second, err := f.store.ListIntakeClarificationHistory(ctx, "round-intake", first.NextCursor, 1)
	if err != nil || len(second.Rounds) != 1 || second.Rounds[0].ID != "r2" || second.NextCursor != "" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	defaults, err := f.store.ListIntakeClarificationHistory(ctx, "round-intake", "", 0)
	if err != nil || len(defaults.Rounds) != 3 {
		t.Fatalf("default page=%+v err=%v", defaults, err)
	}
	if _, err := f.store.ListIntakeClarificationHistory(ctx, "round-intake", "", ports.IntakeClarificationHistoryPageLimit+1); err == nil {
		t.Fatal("unbounded page accepted")
	}
}

func testCursorCodec(t *testing.T) *ports.IntakeClarificationCursorCodec {
	t.Helper()
	c, err := ports.NewIntakeClarificationCursorCodec(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func rewriteCursor(t *testing.T, cursor ports.IntakeClarificationHistoryCursor, change func(map[string]any)) ports.IntakeClarificationHistoryCursor {
	t.Helper()
	parts := strings.Split(string(cursor), ".")
	if len(parts) != 2 {
		t.Fatal("bad test cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	change(value)
	raw, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	// Retain the original signature so every payload edit must fail MAC verification.
	return ports.IntakeClarificationHistoryCursor(base64.RawURLEncoding.EncodeToString(raw) + "." + parts[1])
}
func flipCursorBit(t *testing.T, cursor ports.IntakeClarificationHistoryCursor) ports.IntakeClarificationHistoryCursor {
	t.Helper()
	parts := strings.Split(string(cursor), ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 1
	return ports.IntakeClarificationHistoryCursor(base64.RawURLEncoding.EncodeToString(raw) + "." + parts[1])
}

func TestIntakeClarificationRoundStoreConcurrentOpenHasOneWinner(t *testing.T) {
	f, ctx := createRoundIntake(t, "concurrent-intake")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"race-a", "race-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := f.store.OpenIntakeClarificationRound(ctx, "concurrent-intake", draft(id, 0, false), f.now)
			errs <- err
		}(id)
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successes=%d want 1", successes)
	}
	page, err := f.store.ListIntakeClarificationHistory(ctx, "concurrent-intake", "", 10)
	if err != nil || len(page.Rounds) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestIntakeClarificationRoundStoreRejectsPartialAnswerBatch(t *testing.T) {
	f, ctx := createRoundIntake(t, "atomic-intake")
	d := draft("atomic-round", 0, false)
	d.Questions = append(d.Questions, domain.IntakeClarificationQuestion{ID: "q2", Position: 2, Question: "Second?", Reason: "Reason"})
	r, err := f.store.OpenIntakeClarificationRound(ctx, "atomic-intake", d, f.now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.AnswerIntakeClarificationRound(ctx, "atomic-intake", domain.IntakeClarificationAnswerBatch{Version: domain.IntakeClarificationRoundV1, RoundID: r.ID, ExpectedProposalRevision: 0, Answers: []domain.IntakeClarificationRoundAnswer{{RoundID: r.ID, QuestionID: r.Questions[0].ID, Answer: "valid"}, {RoundID: r.ID, QuestionID: "missing", Answer: "bad"}}}, f.now)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("err=%v", err)
	}
	page, _ := f.store.ListIntakeClarificationHistory(ctx, "atomic-intake", "", 10)
	if len(page.Rounds) != 1 || len(page.Rounds[0].Answers) != 0 {
		t.Fatalf("partial answers persisted: %+v", page)
	}
}

func TestIntakeClarificationRoundStoreMultiStoreConcurrentOpenHasOneWinner(t *testing.T) {
	dataDir := t.TempDir()
	first, err := sqlitetest.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	codec := testCursorCodec(t)
	if err := first.SetIntakeClarificationCursorCodec(codec); err != nil {
		t.Fatal(err)
	}
	if err := second.SetIntakeClarificationCursorCodec(codec); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedProject(t, first, "multi-project")
	now := time.Date(2026, 9, 19, 2, 0, 0, 0, time.UTC)
	_, err = first.CreateIntake(ctx, domain.IntakeSession{ID: "multi-intake", SourceSurface: domain.IntakeSourceWork, Purpose: domain.IntakePurposeOutcome, ProjectID: "multi-project", Statement: "race", Status: domain.IntakeStatusCaptured, CreatedAt: now, UpdatedAt: now}, nil, ports.IntakeIdempotency{Key: "multi-key", Fingerprint: "fp"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, st := range []interface {
		OpenIntakeClarificationRound(context.Context, domain.IntakeSessionID, domain.IntakeClarificationRoundDraft, time.Time) (domain.IntakeClarificationRound, error)
	}{first, second} {
		wg.Add(1)
		go func(i int, st interface {
			OpenIntakeClarificationRound(context.Context, domain.IntakeSessionID, domain.IntakeClarificationRoundDraft, time.Time) (domain.IntakeClarificationRound, error)
		}) {
			defer wg.Done()
			<-start
			_, err := st.OpenIntakeClarificationRound(ctx, "multi-intake", draft(string(rune('a'+i)), 0, false), now)
			errs <- err
		}(i, st)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("multi-store successes=%d want 1", successes)
	}
	page, err := first.ListIntakeClarificationHistory(ctx, "multi-intake", "", 10)
	if err != nil || len(page.Rounds) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestIntakeClarificationRoundStoreFailsClosedWithoutCursorCodec(t *testing.T) {
	s, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.ListIntakeClarificationHistory(context.Background(), "missing", "", 1); err == nil || !strings.Contains(err.Error(), "codec unavailable") {
		t.Fatalf("err=%v", err)
	}
}

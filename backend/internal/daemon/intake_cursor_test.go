package daemon

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallIntakeCursorCodecSurvivesRestart(t *testing.T) {
	d := t.TempDir()
	s, e := sqlite.Open(d)
	if e != nil {
		t.Fatal(e)
	}
	if e = installIntakeCursorCodec(s, d); e != nil {
		t.Fatal(e)
	}
	seedCursorIntake(t, s)
	now := time.Now().UTC()
	r1, e := s.OpenIntakeClarificationRound(context.Background(), "i", domain.IntakeClarificationRoundDraft{Version: domain.IntakeClarificationRoundV1, ID: "r1", Questions: []domain.IntakeClarificationQuestion{{ID: "q1", Position: 1, Question: "?", Reason: "why"}}}, now)
	if e != nil {
		t.Fatal(e)
	}
	answered, e := s.AnswerIntakeClarificationRound(context.Background(), "i", domain.IntakeClarificationAnswerBatch{Version: domain.IntakeClarificationRoundV1, RoundID: r1.ID, Answers: []domain.IntakeClarificationRoundAnswer{{RoundID: r1.ID, QuestionID: "q1", Answer: "done"}}}, now)
	if e != nil || len(answered.Answers) != 1 {
		t.Fatalf("answer r1=%+v err=%v", answered, e)
	}
	if _, e = s.OpenIntakeClarificationRound(context.Background(), "i", domain.IntakeClarificationRoundDraft{Version: domain.IntakeClarificationRoundV1, ID: "r2", ExplicitReanalysis: true, Questions: []domain.IntakeClarificationQuestion{{ID: "q2", Position: 1, Question: "?", Reason: "why"}}}, now); e != nil {
		t.Fatal(e)
	}
	page, e := s.ListIntakeClarificationHistory(context.Background(), "i", "", 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = sqlite.Open(d)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = installIntakeCursorCodec(s, d); e != nil {
		t.Fatal(e)
	}
	if len(page.Rounds) != 1 || len(page.Rounds[0].Answers) != 1 || page.Rounds[0].Answers[0].Answer != "done" || page.NextCursor == "" {
		t.Fatalf("missing persisted answer/cursor: %+v", page)
	}
	next, e := s.ListIntakeClarificationHistory(context.Background(), "i", page.NextCursor, 1)
	if e != nil || len(next.Rounds) != 1 || next.Rounds[0].ID != "r2" {
		t.Fatalf("restart page=%+v err=%v", next, e)
	}
}
func TestInstallIntakeCursorCodecFailsClosedOnCorruptKey(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "secrets", "waldo-intake-cursor-hmac-v1")
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte("bad"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := sqlite.Open(d)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = installIntakeCursorCodec(s, d); e == nil {
		t.Fatal("corrupt key accepted")
	}
}
func seedCursorIntake(t *testing.T, s *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if e := s.UpsertProject(ctx, domain.ProjectRecord{ID: "p", Path: "/tmp/p", RegisteredAt: now}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.CreateIntake(ctx, domain.IntakeSession{ID: "i", ProjectID: "p", SourceSurface: domain.IntakeSourceWork, Purpose: domain.IntakePurposeOutcome, Statement: "x", Status: domain.IntakeStatusCaptured, CreatedAt: now, UpdatedAt: now}, nil, ports.IntakeIdempotency{Key: "k", Fingerprint: "f"}); e != nil {
		t.Fatal(e)
	}
}

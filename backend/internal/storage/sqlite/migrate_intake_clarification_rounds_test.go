package sqlite

import (
	"strings"
	"testing"
)

func TestMigration0160AddsAppendOnlyRoundsWithoutBackfill(t *testing.T) {
	db := openContractTestDB(t)
	upTo(t, db, 158)
	seedContractProject(t, db)
	if _, err := db.Exec(`INSERT INTO intake_sessions
		(id, source_surface, purpose, project_id, statement, status, clarification_count, request_key, request_fingerprint)
		VALUES ('legacy-intake','work','outcome','p1','legacy','needs_user',1,'legacy-key','legacy-fp')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO intake_clarifications
		(id,intake_id,question,reason,recommendation,alternatives,deferral_consequence)
		VALUES ('legacy-question','legacy-intake','Q?','R','','[]','')`); err != nil {
		t.Fatal(err)
	}
	upTo(t, db, 160)
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM intake_clarification_rounds`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("strict no-backfill count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM intake_clarifications WHERE id='legacy-question'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy row count=%d err=%v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO intake_clarification_rounds
		(id,intake_id,ordinal,version,expected_proposal_revision,explicit_reanalysis,created_at)
		VALUES ('round-1','legacy-intake',1,'intake.clarification-round.v1',0,0,'2026-09-19T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE intake_clarification_rounds SET ordinal=2 WHERE id='round-1'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("round update error=%v", err)
	}
	if _, err := db.Exec(`INSERT INTO intake_clarification_rounds
		(id,intake_id,ordinal,version,expected_proposal_revision,explicit_reanalysis,created_at)
		VALUES ('round-duplicate','legacy-intake',1,'intake.clarification-round.v1',0,0,'2026-09-19T00:00:00Z')`); err == nil {
		t.Fatal("duplicate ordinal must fail")
	}
}

func TestMigration0160RoundQuestionAnswerGuards(t *testing.T) {
	db := openContractTestDB(t)
	upTo(t, db, 160)
	seedContractProject(t, db)
	if _, err := db.Exec(`INSERT INTO intake_sessions (id,source_surface,purpose,project_id,statement,status,request_key,request_fingerprint) VALUES ('i','work','outcome','p1','s','captured','k','f')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO intake_clarification_rounds VALUES ('r','i',1,'intake.clarification-round.v1',0,0,'2026-09-19T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO intake_clarification_round_questions VALUES ('r','q',1,'Question','Reason','','[]','')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO intake_clarification_round_answers (round_id,question_id,answer,answered_at) VALUES ('r','unknown','answer','2026-09-19T00:01:00Z')`); err == nil {
		t.Fatal("unknown question answer must fail")
	}
	if _, err := db.Exec(`INSERT INTO intake_clarification_round_answers (round_id,question_id,answer,answered_at) VALUES ('r','q','answer','2026-09-19T00:01:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM intake_clarification_round_answers WHERE round_id='r'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("answer delete error=%v", err)
	}
}

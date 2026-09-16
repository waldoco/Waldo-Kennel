package sqlite

import (
	"context"
	"testing"
)

func TestGovernedControlCommandsMigrationAcceptsExactClassesAndRejectsUnknown(t *testing.T) {
	db := openMigratedTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO projects(id,path,registered_at) VALUES('gc-project','/tmp/gc','2026-09-16 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions(id,project_id,num,kind,harness,activity_last_at,created_at,updated_at) VALUES('gc-session','gc-project',1,'worker','codex','2026-09-16','2026-09-16','2026-09-16')`); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO governed_control_commands(id,session_id,idempotency_key,request_fingerprint,command_class,state,controller_generation,expected_revision,capability_fingerprint,provider_conversation_id,client_message_id,provider_turn_id,target_generation,quiescence,created_at,updated_at) VALUES(?,?,?,?,?,'claimed','gen','plan','caps','thread',?,?,?,'not_applicable','2026-09-16','2026-09-16')`
	for _, class := range []string{"steer", "answer", "interrupt"} {
		client, turn, generation := "", "", ""
		if class == "steer" {
			client, turn = "message", "turn"
		}
		if class == "interrupt" {
			turn = "turn"
		}
		if class == "answer" {
			generation = "request"
		}
		if _, err := db.ExecContext(ctx, insert, "gc-"+class, "gc-session", "key-"+class, "fp", class, client, turn, generation); err != nil {
			t.Fatalf("%s: %v", class, err)
		}
	}
	if _, err := db.ExecContext(ctx, insert, "gc-bad", "gc-session", "key-bad", "fp", "replace", "", "", ""); err == nil {
		t.Fatal("unknown control class inserted")
	}
}

package sqlite

import (
	"database/sql"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttemptStartReservationMigrationUpAndTransitionGuards(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(db, "migrations", goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	var sqlText string
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='attempt_start_reservations'`).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	var triggerSQL string
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='trigger' AND name='attempts_status_transition'`).Scan(&triggerSQL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(triggerSQL, "OLD.status='awaiting_authority' AND NEW.status IN ('queued','failed','cancelled')") || strings.Contains(triggerSQL, "OLD.status='awaiting_authority' AND NEW.status IN ('running'") {
		t.Fatalf("unexpected transition guard: %s", triggerSQL)
	}
}

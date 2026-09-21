package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestPlanningReadinessMigrationDetachesReconciledTrashGuards(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kennel.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "migrations", 156, goose.WithAllowMissing()); err != nil {
		t.Fatalf("migrate legacy profile: %v", err)
	}
	if err := installOutcomeDeletionSchema(db); err != nil {
		t.Fatalf("install reconciled trash guards: %v", err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("migrate profile with reconciled trash guards: %v", err)
	}

	for _, name := range []string{"outcome_trash_planning_session_guard", "outcome_trash_planning_turn_guard"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name=?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("trigger %s count=%d, want 1", name, count)
		}
	}
}

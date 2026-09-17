package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigration0139UpgradeRetainsExistingData(t *testing.T) {
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	upTo(t, db, 138)
	seedContractProject(t, db)
	if _, err := db.Exec(`INSERT INTO sessions(id,project_id,num,harness,activity_last_at,created_at,updated_at) VALUES('upgrade-session','p1',1,'codex',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("138->139 migrate: %v", err)
	}
	var path string
	if err := db.QueryRow(`SELECT path FROM projects WHERE id='p1'`).Scan(&path); err != nil {
		t.Fatalf("read retained project: %v", err)
	}
	if path != "/tmp/p1" {
		t.Fatalf("retained path=%q", path)
	}
	var sessions int
	if err := db.QueryRow(`SELECT count(*) FROM sessions WHERE id='upgrade-session'`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("retained session count=%d err=%v", sessions, err)
	}
	var table int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='attempt_replacement_decisions'`).Scan(&table); err != nil || table != 1 {
		t.Fatalf("new table count=%d err=%v", table, err)
	}
}

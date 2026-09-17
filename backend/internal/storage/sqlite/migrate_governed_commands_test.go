package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigration0140UpgradeRetainsExistingDataAndCreatesGovernedCommands(t *testing.T) {
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	upTo(t, db, 139)
	seedContractProject(t, db)
	if _, err := db.Exec(`INSERT INTO sessions(id,project_id,num,harness,activity_last_at,created_at,updated_at) VALUES('upgrade-session','p1',1,'codex',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("139->140 migrate: %v", err)
	}
	var retained int
	if err := db.QueryRow(`SELECT count(*) FROM sessions WHERE id='upgrade-session'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("retained session count=%d err=%v", retained, err)
	}
	var table int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='governed_commands'`).Scan(&table); err != nil || table != 1 {
		t.Fatalf("new table count=%d err=%v", table, err)
	}
}

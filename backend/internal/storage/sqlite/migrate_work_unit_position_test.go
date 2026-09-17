package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestWorkUnitPositionMigrationPreservesLegacyPolicyAndFreezesNewPositions(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "migrations", 152, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	seedContractProject(t, db)
	spaceID := seedWorkSpace(t, db)
	if _, err := db.Exec(`INSERT INTO outcomes(id,space_id,title,current_revision_number) VALUES('position-out',?,'position',1)`, spaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO contract_revisions(id,outcome_id,number,goal,success_criteria,review) VALUES('position-cr','position-out',1,'g',json('["ok"]'),'r')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO plan_revisions(id,outcome_id,number,contract_revision_number,status,summary,run_brief_core_digest) VALUES('position-plan','position-out',1,1,'proposed','s',printf('%064d',0))`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"wu-z", "wu-a"} {
		if _, err := db.Exec(`INSERT INTO work_units(id,plan_revision_id,kind,title,contract_revision_number,output_summary,evidence_checks,verification_requirement,stop_conditions) VALUES(?,'position-plan','direct',?,1,'o',json('["c"]'),'v',json('["s"]'))`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := goose.Up(db, "migrations", goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	var unset int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_units WHERE plan_revision_id='position-plan' AND position IS NULL`).Scan(&unset); err != nil || unset != 2 {
		t.Fatalf("legacy unset positions = %d, err=%v", unset, err)
	}
	if _, err := db.Exec(`UPDATE work_units SET position=1 WHERE id='wu-z'`); err == nil {
		t.Fatal("legacy position mutation should fail")
	}
	if _, err := db.Exec(`UPDATE work_units SET position=9 WHERE id='wu-z'`); err == nil {
		t.Fatal("position mutation should fail")
	}
}

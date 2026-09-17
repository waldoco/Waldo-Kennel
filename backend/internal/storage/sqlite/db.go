// Package sqlite owns SQLite connection setup and goose-managed schema
// migrations. Typed CRUD lives in the store subpackage; this package keeps the
// public Open entrypoint and compatibility aliases for callers.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pressly/goose/v3"

	sqlitestore "github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/store"

	// modernc.org/sqlite is the pure-Go (CGO-free) SQLite driver — chosen so the
	// daemon cross-compiles and ships as a static binary with no libsqlite/CGO
	// toolchain dependency, at the cost of some raw throughput vs a C-backed driver.
	_ "modernc.org/sqlite"
)

// Store is the SQLite-backed persistence layer.
type Store = sqlitestore.Store

//go:embed migrations/*.sql
var migrationsFS embed.FS

// pragmas are applied on every connection open. WAL + NORMAL lets readers run
// concurrently with the writer; busy_timeout absorbs brief writer contention;
// foreign_keys enforces the cascades and the CDC triggers' lookups.
const pragmas = "?_pragma=journal_mode(WAL)" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=foreign_keys(ON)" +
	"&_pragma=synchronous(NORMAL)"

const readOnlyPragmas = "?mode=ro" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=foreign_keys(ON)"

// maxReaders caps the reader pool. WAL allows many concurrent readers.
const maxReaders = 8

// Open opens (creating if absent) the SQLite database under dataDir and returns
// a Store. It uses TWO pools against the same file:
//
//   - a single WRITER connection (writeDB, MaxOpenConns=1): every write goes
//     here, so a write and the CDC triggers' subqueries it fires always see the
//     prior writes on the same connection (read-your-writes). This is required
//     because the pr/pr_checks triggers SELECT from sessions/pr to fill in the
//     event's project_id; a pooled writer could land that read on a connection
//     that hasn't caught up to the commit and read NULL.
//   - a READER pool (readDB, MaxOpenConns=maxReaders): all reads scale across
//     it; WAL readers see the latest committed snapshot.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + filepath.Join(dataDir, "kennel.db") + pragmas

	writeDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite writer: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)
	if err := migrate(writeDB); err != nil {
		_ = writeDB.Close()
		return nil, err
	}

	readDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite reader: %w", err)
	}
	readDB.SetMaxOpenConns(maxReaders)
	readDB.SetMaxIdleConns(maxReaders)

	return sqlitestore.NewStore(writeDB, readDB), nil
}

// OpenReadOnly opens an existing SQLite database under dataDir without creating
// the directory, opening a writable connection, or running migrations.
func OpenReadOnly(ctx context.Context, dataDir string) (*Store, error) {
	dsn := "file:" + filepath.Join(dataDir, "kennel.db") + readOnlyPragmas

	writeDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only writer: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)
	if err := writeDB.PingContext(ctx); err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite read-only writer: %w", err)
	}

	readDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite read-only reader: %w", err)
	}
	readDB.SetMaxOpenConns(maxReaders)
	readDB.SetMaxIdleConns(maxReaders)
	if err := readDB.PingContext(ctx); err != nil {
		_ = readDB.Close()
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite read-only reader: %w", err)
	}

	return sqlitestore.NewStore(writeDB, readDB), nil
}

// gooseMu serialises calls into goose. goose v3 keeps its baseFS / logger /
// dialect as package-level globals (goose.SetBaseFS, goose.SetLogger,
// goose.SetDialect), so two concurrent Open() calls — uncommon in production
// but normal in -race test runs — race on those writes. The cost of holding the
// mutex is one process-startup migration; readers and writers afterwards never
// touch goose.
var gooseMu sync.Mutex

func migrate(db *sql.DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := repairRenumberedChatMigrationHistory(db); err != nil {
		return fmt.Errorf("repair renumbered chat migration history: %w", err)
	}
	if err := prepareAutoInjectReviewMigration(db); err != nil {
		return fmt.Errorf("prepare auto-inject-review migration: %w", err)
	}
	if err := repairRenumberedAgentSwitchMigrationHistory(db); err != nil {
		return fmt.Errorf("repair renumbered agent-switch migration history: %w", err)
	}
	if err := prepareBurnedSchemaRepairs(db); err != nil {
		return fmt.Errorf("prepare burned schema repairs: %w", err)
	}
	if err := prepareReviewPerHarnessMigration(db); err != nil {
		return fmt.Errorf("prepare review per-harness migration: %w", err)
	}
	if err := prepareBrowserVerifierMigration(db); err != nil {
		return fmt.Errorf("prepare browser verifier migration: %w", err)
	}
	if err := prepareQueuedTurnPromotionMigration(db); err != nil {
		return fmt.Errorf("prepare queued-turn promotion migration: %w", err)
	}
	if err := preparePlanReviewContextMigration(db); err != nil {
		return fmt.Errorf("prepare plan-review context migration: %w", err)
	}
	if err := prepareInteractivePlanningMigration(db); err != nil {
		return fmt.Errorf("prepare interactive-planning migration: %w", err)
	}
	if err := prepareWorkUnitPositionMigration(db); err != nil {
		return fmt.Errorf("prepare work-unit-position migration: %w", err)
	}
	if err := prepareWorkUnitOrchestrationMigration(db); err != nil {
		return fmt.Errorf("prepare work-unit-orchestration migration: %w", err)
	}
	// Builds can advance a database past a migration that is added or
	// renumbered later (notably across fast-moving Nightly releases). Apply
	// those embedded migrations instead of permanently wedging daemon startup
	// on goose's out-of-order-history guard.
	if err := goose.Up(db, "migrations", goose.WithAllowMissing()); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	if err := reconcileOutcomeProofSchema(db); err != nil {
		return fmt.Errorf("reconcile outcome proof schema: %w", err)
	}
	// Migration 0099 rebuilds the checked change_log relation and detaches the
	// inherited CDC writers; restore them (and heal degraded profiles whose
	// subject tables arrive through repairs) before anything reads the stream.
	if err := restoreChangeLogWriters(db); err != nil {
		return fmt.Errorf("restore change log writers: %w", err)
	}
	if err := reconcileComposedOutcomesSchema(db); err != nil {
		return fmt.Errorf("reconcile composed outcomes schema: %w", err)
	}
	if err := reconcileProjectChatProjection(db); err != nil {
		return fmt.Errorf("reconcile project chat projection: %w", err)
	}
	if err := reconcileExecutionRoutingSchema(db); err != nil {
		return fmt.Errorf("reconcile execution routing schema: %w", err)
	}
	if err := reconcileWorkUnitPositionSchema(db); err != nil {
		return fmt.Errorf("reconcile work-unit-position schema: %w", err)
	}
	if err := reconcileWorkUnitOrchestrationSchema(db); err != nil {
		return fmt.Errorf("reconcile work-unit-orchestration schema: %w", err)
	}
	if err := reconcilePlanReviewSchema(db); err != nil {
		return fmt.Errorf("reconcile plan-review schema: %w", err)
	}
	if err := reconcileInteractivePlanningSchema(db); err != nil {
		return fmt.Errorf("reconcile interactive-planning schema: %w", err)
	}
	if err := reconcilePlanningPlanImmutability(db); err != nil {
		return fmt.Errorf("reconcile planning Plan immutability: %w", err)
	}
	if err := reconcileAdmissionSchema(db); err != nil {
		return fmt.Errorf("reconcile admission schema: %w", err)
	}
	// A degraded profile can acquire the planning tables only during the
	// reconciliation above, after the first CDC restoration pass.
	if err := restoreChangeLogWriters(db); err != nil {
		return fmt.Errorf("restore reconciled change log writers: %w", err)
	}
	if err := reconcileSchema(db); err != nil {
		return err
	}
	if err := installOutcomeDeletionSchema(db); err != nil {
		return fmt.Errorf("install scoped Outcome deletion guards: %w", err)
	}
	return nil
}

func prepareWorkUnitOrchestrationMigration(db *sql.DB) error {
	var gooseTable, workUnits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='goose_db_version'`).Scan(&gooseTable); err != nil || gooseTable == 0 {
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_units'`).Scan(&workUnits); err != nil || workUnits != 0 {
		return err
	}
	_, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (155, 1)`)
	return err
}

func reconcileWorkUnitOrchestrationSchema(db *sql.DB) error {
	var workUnits, role int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_units'`).Scan(&workUnits); err != nil || workUnits == 0 {
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_units') WHERE name='role'`).Scan(&role); err != nil {
		return err
	}
	if role == 0 {
		if _, err := db.Exec(`ALTER TABLE work_units ADD COLUMN role TEXT`); err != nil {
			return err
		}
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS work_unit_inputs(work_unit_id TEXT NOT NULL REFERENCES work_units(id),from_work_unit_id TEXT NOT NULL REFERENCES work_units(id),required TEXT NOT NULL CHECK(length(trim(required))>0),position INTEGER NOT NULL CHECK(position>0),PRIMARY KEY(work_unit_id,from_work_unit_id),UNIQUE(work_unit_id,position)); CREATE TRIGGER IF NOT EXISTS work_unit_inputs_immutable_update BEFORE UPDATE ON work_unit_inputs BEGIN SELECT RAISE(ABORT,'work unit inputs are immutable'); END; CREATE TRIGGER IF NOT EXISTS work_unit_inputs_immutable_delete BEFORE DELETE ON work_unit_inputs BEGIN SELECT RAISE(ABORT,'work unit inputs are immutable'); END;`)
	return err
}

// prepareWorkUnitPositionMigration lets degraded profiles whose Outcome tables
// were skipped pass Goose. Reconciliation installs the same shape after the
// execution-routing seam has restored work_units.
func prepareWorkUnitPositionMigration(db *sql.DB) error {
	var gooseTable, workUnits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='goose_db_version'`).Scan(&gooseTable); err != nil || gooseTable == 0 {
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_units'`).Scan(&workUnits); err != nil || workUnits != 0 {
		return err
	}
	_, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (154, 1)`)
	return err
}

func reconcileWorkUnitPositionSchema(db *sql.DB) error {
	var workUnits, position int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_units'`).Scan(&workUnits); err != nil || workUnits == 0 {
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_units') WHERE name='position'`).Scan(&position); err != nil {
		return err
	}
	if position == 0 {
		if _, err := db.Exec(`ALTER TABLE work_units ADD COLUMN position INTEGER`); err != nil {
			return err
		}
	}
	_, err := db.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_work_units_plan_position ON work_units(plan_revision_id, position) WHERE position IS NOT NULL;
DROP TRIGGER IF EXISTS work_units_position_immutable_update;
CREATE TRIGGER IF NOT EXISTS work_units_immutable_update BEFORE UPDATE ON work_units BEGIN SELECT RAISE(ABORT, 'work units are immutable'); END;`)
	return err
}

// prepareInteractivePlanningMigration lets a profile whose Outcome migration
// versions were burned complete goose without running 0133 against absent
// authority tables. The migration is recorded as applied and the physical
// shape is installed by reconcileInteractivePlanningSchema once every
// dependency actually exists; no authority or provenance is synthesized.
func prepareInteractivePlanningMigration(db *sql.DB) error {
	var gooseTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return nil
	}
	var applied int
	if err := db.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = 133 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied); err != nil {
		return err
	}
	if applied != 0 {
		return nil
	}
	for _, table := range []string{"projects", "outcomes", "contract_revisions", "plan_revisions", "intelligence_runs"} {
		var present int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			_, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (133, 1)`)
			return err
		}
	}
	return nil
}

// preparePlanReviewContextMigration lets a degraded profile with burned
// Outcome migration versions complete goose without attempting ALTER TABLE on
// an absent plan_revisions table. reconcilePlanReviewSchema below installs the
// same durable shape once the execution-routing repair recreates that table.
func preparePlanReviewContextMigration(db *sql.DB) error {
	var gooseTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return nil
	}
	var applied int
	if err := db.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = 117 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied); err != nil {
		return err
	}
	if applied != 0 {
		return nil
	}
	var planTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'plan_revisions'`,
	).Scan(&planTable); err != nil {
		return err
	}
	if planTable != 0 {
		return nil
	}
	_, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (117, 1)`)
	return err
}

// reconcilePlanReviewSchema is the degraded-profile counterpart to migration
// 0117. SQLite cannot conditionally guard ALTER TABLE inside a goose file, so
// the complete Plan review shape is installed here after execution-routing
// repair has recreated any missing Outcome tables.
func reconcilePlanReviewSchema(db *sql.DB) error {
	var planTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'plan_revisions'`,
	).Scan(&planTable); err != nil {
		return err
	}
	if planTable == 0 {
		return nil
	}
	for _, column := range []struct {
		name string
		ddl  string
	}{
		{name: "assumptions_json", ddl: `ALTER TABLE plan_revisions ADD COLUMN assumptions_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(assumptions_json))`},
		{name: "blockers_json", ddl: `ALTER TABLE plan_revisions ADD COLUMN blockers_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(blockers_json))`},
	} {
		var present int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('plan_revisions') WHERE name = ?`, column.name,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			if _, err := db.Exec(column.ddl); err != nil {
				return err
			}
		}
	}
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS plan_revisions_immutable_update`); err != nil {
		return err
	}
	_, err := db.Exec(`
CREATE TRIGGER plan_revisions_immutable_update
BEFORE UPDATE ON plan_revisions
WHEN OLD.id <> NEW.id
     OR OLD.outcome_id <> NEW.outcome_id
     OR OLD.number <> NEW.number
     OR OLD.contract_revision_number <> NEW.contract_revision_number
     OR OLD.summary <> NEW.summary
     OR OLD.assumptions_json <> NEW.assumptions_json
     OR OLD.blockers_json <> NEW.blockers_json
     OR OLD.run_brief_core_digest <> NEW.run_brief_core_digest
     OR OLD.run_brief_compiled_digest IS NOT NEW.run_brief_compiled_digest
     OR OLD.routing_decisions_json IS NOT NEW.routing_decisions_json
     OR OLD.created_at <> NEW.created_at
BEGIN
    SELECT RAISE(ABORT, 'plan revisions are immutable');
END`)
	return err
}

// reconcileInteractivePlanningSchema is the degraded-profile counterpart to
// migration 0133. It installs the same Contract-bound conversation shape only
// after every referenced authority/provenance table exists. Existing rows are
// left untouched, so a repair cannot invent a planning grant or Plan source.
func reconcileInteractivePlanningSchema(db *sql.DB) error {
	for _, table := range []string{"projects", "outcomes", "contract_revisions", "plan_revisions", "intelligence_runs"} {
		var present int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS planning_sessions (
    id                        TEXT PRIMARY KEY,
    outcome_id                TEXT NOT NULL REFERENCES outcomes (id),
    project_id                TEXT NOT NULL REFERENCES projects (id),
    contract_revision_id      TEXT NOT NULL REFERENCES contract_revisions (id),
    contract_revision_number  INTEGER NOT NULL CHECK (contract_revision_number >= 1),
    revision                  INTEGER NOT NULL CHECK (revision >= 1),
    latest_turn_sequence      INTEGER NOT NULL DEFAULT 0 CHECK (latest_turn_sequence >= 0),
    status                    TEXT NOT NULL CHECK (status IN ('active','proposal_ready','superseded','cancelled')),
    waiting_on                TEXT NOT NULL CHECK (waiting_on IN ('owner','provider','none')),
    mode                      TEXT NOT NULL CHECK (mode IN ('direct_api','native_harness')),
    requested_provider        TEXT NOT NULL,
    model_selection           TEXT NOT NULL CHECK (model_selection IN ('provider_default','explicit')),
    requested_model           TEXT NOT NULL DEFAULT '',
    requested_effort          TEXT NOT NULL DEFAULT '',
    context_mode              TEXT NOT NULL CHECK (context_mode IN ('repository_read','supplied_packet')),
    planning_grant_digest     TEXT NOT NULL CHECK (length(planning_grant_digest) = 64 AND planning_grant_digest NOT GLOB '*[^0-9a-f]*'),
    context_digest            TEXT NOT NULL CHECK (length(context_digest) = 64 AND context_digest NOT GLOB '*[^0-9a-f]*'),
    context_snapshot_json     TEXT NOT NULL CHECK (json_valid(context_snapshot_json)),
    effective_provider        TEXT NOT NULL DEFAULT '',
    effective_model           TEXT NOT NULL DEFAULT '',
    native_conversation_ref   TEXT NOT NULL DEFAULT '',
    proposed_plan_revision_id TEXT REFERENCES plan_revisions (id),
    last_failure_code         TEXT NOT NULL DEFAULT '',
    last_failure_detail       TEXT NOT NULL DEFAULT '',
    request_key               TEXT NOT NULL UNIQUE,
    request_fingerprint       TEXT NOT NULL CHECK (length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
    created_at                TIMESTAMP NOT NULL,
    updated_at                TIMESTAMP NOT NULL,
    closed_at                 TIMESTAMP,
    CHECK ((model_selection = 'provider_default' AND requested_model = '') OR (model_selection = 'explicit' AND requested_model <> '')),
    CHECK (effective_provider <> '' OR (effective_model = '' AND native_conversation_ref = '')),
    CHECK ((status = 'active' AND waiting_on <> 'none' AND closed_at IS NULL AND proposed_plan_revision_id IS NULL)
        OR (status <> 'active' AND waiting_on = 'none' AND closed_at IS NOT NULL)),
    CHECK ((status = 'proposal_ready') = (proposed_plan_revision_id IS NOT NULL)),
    UNIQUE (id, outcome_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_planning_sessions_one_active_outcome
    ON planning_sessions (outcome_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_planning_sessions_outcome_created
    ON planning_sessions (outcome_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS planning_turns (
    id                      TEXT PRIMARY KEY,
    planning_session_id     TEXT NOT NULL REFERENCES planning_sessions (id),
    sequence                INTEGER NOT NULL CHECK (sequence >= 1),
    reply_to_turn_id        TEXT REFERENCES planning_turns (id),
    role                    TEXT NOT NULL CHECK (role IN ('owner','planner')),
    kind                    TEXT NOT NULL CHECK (kind IN ('message','finalize_request','clarification','contract_change_proposal','plan_proposal')),
    text                    TEXT NOT NULL,
    structured_payload_json TEXT CHECK (structured_payload_json IS NULL OR json_valid(structured_payload_json)),
    intelligence_run_id     TEXT REFERENCES intelligence_runs (id),
    request_key             TEXT,
    request_fingerprint     TEXT CHECK (request_fingerprint IS NULL OR (length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*')),
    created_at              TIMESTAMP NOT NULL,
    CHECK ((role = 'owner' AND reply_to_turn_id IS NULL AND intelligence_run_id IS NULL AND request_key IS NOT NULL AND request_fingerprint IS NOT NULL AND kind IN ('message','finalize_request'))
        OR (role = 'planner' AND reply_to_turn_id IS NOT NULL AND intelligence_run_id IS NOT NULL AND request_key IS NULL AND request_fingerprint IS NULL AND kind IN ('clarification','contract_change_proposal','plan_proposal'))),
    UNIQUE (planning_session_id, sequence),
    UNIQUE (planning_session_id, request_key)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_planning_turns_reply
    ON planning_turns (reply_to_turn_id) WHERE reply_to_turn_id IS NOT NULL;
`); err != nil {
		return err
	}

	for _, column := range []struct {
		name string
		ddl  string
	}{
		{name: "planning_session_id", ddl: `ALTER TABLE plan_revisions ADD COLUMN planning_session_id TEXT REFERENCES planning_sessions (id)`},
		{name: "source_intelligence_run_id", ddl: `ALTER TABLE plan_revisions ADD COLUMN source_intelligence_run_id TEXT REFERENCES intelligence_runs (id)`},
	} {
		var present int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('plan_revisions') WHERE name = ?`, column.name,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			if _, err := tx.Exec(column.ddl); err != nil {
				return err
			}
		}
	}

	if _, err := tx.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_revisions_planning_session
    ON plan_revisions (planning_session_id) WHERE planning_session_id IS NOT NULL;

DROP TRIGGER IF EXISTS plan_revisions_planning_source_guard;
CREATE TRIGGER plan_revisions_planning_source_guard
BEFORE INSERT ON plan_revisions
WHEN NEW.planning_session_id IS NOT NULL OR NEW.source_intelligence_run_id IS NOT NULL
BEGIN
    SELECT CASE WHEN NEW.planning_session_id IS NULL OR NEW.source_intelligence_run_id IS NULL
        THEN RAISE(ABORT, 'planning Plan provenance must be complete') END;
    SELECT CASE WHEN NOT EXISTS (
        SELECT 1 FROM planning_sessions s
        JOIN planning_turns t ON t.planning_session_id = s.id
        JOIN contract_revisions cr
          ON cr.id = s.contract_revision_id
         AND cr.outcome_id = s.outcome_id
         AND cr.number = s.contract_revision_number
        JOIN outcomes o
          ON o.id = s.outcome_id
         AND o.current_revision_number = s.contract_revision_number
        WHERE s.id = NEW.planning_session_id
          AND s.outcome_id = NEW.outcome_id
          AND s.contract_revision_number = NEW.contract_revision_number
          AND s.status = 'active'
          AND t.intelligence_run_id = NEW.source_intelligence_run_id
          AND t.kind = 'plan_proposal'
    ) THEN RAISE(ABORT, 'planning Plan provenance does not match active session') END;
END;

DROP TRIGGER IF EXISTS planning_sessions_update_guard;
CREATE TRIGGER planning_sessions_update_guard
BEFORE UPDATE ON planning_sessions
WHEN OLD.id <> NEW.id
     OR OLD.outcome_id <> NEW.outcome_id
     OR OLD.project_id <> NEW.project_id
     OR OLD.contract_revision_id <> NEW.contract_revision_id
     OR OLD.contract_revision_number <> NEW.contract_revision_number
     OR OLD.mode <> NEW.mode
     OR OLD.requested_provider <> NEW.requested_provider
     OR OLD.model_selection <> NEW.model_selection
     OR OLD.requested_model <> NEW.requested_model
     OR OLD.requested_effort <> NEW.requested_effort
     OR OLD.context_mode <> NEW.context_mode
     OR OLD.planning_grant_digest <> NEW.planning_grant_digest
     OR OLD.context_digest <> NEW.context_digest
     OR OLD.context_snapshot_json <> NEW.context_snapshot_json
     OR OLD.request_key <> NEW.request_key
     OR OLD.request_fingerprint <> NEW.request_fingerprint
     OR OLD.created_at <> NEW.created_at
     OR NEW.revision <> OLD.revision + 1
     OR NEW.latest_turn_sequence < OLD.latest_turn_sequence
     OR (OLD.status <> 'active' AND NEW.status <> OLD.status)
     OR (OLD.effective_provider <> '' AND NEW.effective_provider <> OLD.effective_provider)
     OR (OLD.effective_model <> '' AND NEW.effective_model <> OLD.effective_model)
     OR (OLD.native_conversation_ref <> '' AND NEW.native_conversation_ref <> OLD.native_conversation_ref)
     OR (OLD.proposed_plan_revision_id IS NOT NULL AND NEW.proposed_plan_revision_id IS NOT OLD.proposed_plan_revision_id)
BEGIN
    SELECT RAISE(ABORT, 'planning session binding, lineage, and terminal state are immutable');
END;

DROP TRIGGER IF EXISTS planning_turns_immutable_update;
CREATE TRIGGER planning_turns_immutable_update
BEFORE UPDATE ON planning_turns BEGIN
    SELECT RAISE(ABORT, 'planning turns are immutable');
END;
DROP TRIGGER IF EXISTS planning_turns_immutable_delete;
CREATE TRIGGER planning_turns_immutable_delete
BEFORE DELETE ON planning_turns BEGIN
    SELECT RAISE(ABORT, 'planning turns are immutable');
END;
`); err != nil {
		return err
	}
	return tx.Commit()
}

// reconcilePlanningPlanImmutability restores the newest trigger after the
// older plan-review repair has rebuilt its historical shape. It is conditional
// so a degraded profile that has not acquired the planning columns does not
// reference columns it does not have.
func reconcilePlanningPlanImmutability(db *sql.DB) error {
	for _, column := range []string{"planning_session_id", "source_intelligence_run_id"} {
		var present int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('plan_revisions') WHERE name = ?`, column).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
	}
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS plan_revisions_immutable_update`); err != nil {
		return err
	}
	_, err := db.Exec(`
CREATE TRIGGER plan_revisions_immutable_update
BEFORE UPDATE ON plan_revisions
WHEN OLD.id <> NEW.id
     OR OLD.outcome_id <> NEW.outcome_id
     OR OLD.number <> NEW.number
     OR OLD.contract_revision_number <> NEW.contract_revision_number
     OR OLD.summary <> NEW.summary
     OR OLD.assumptions_json <> NEW.assumptions_json
     OR OLD.blockers_json <> NEW.blockers_json
     OR OLD.run_brief_core_digest <> NEW.run_brief_core_digest
     OR OLD.run_brief_compiled_digest IS NOT NEW.run_brief_compiled_digest
     OR OLD.routing_decisions_json IS NOT NEW.routing_decisions_json
     OR OLD.planning_session_id IS NOT NEW.planning_session_id
     OR OLD.source_intelligence_run_id IS NOT NEW.source_intelligence_run_id
     OR OLD.created_at <> NEW.created_at
BEGIN
    SELECT RAISE(ABORT, 'plan revisions are immutable');
END`)
	return err
}

// executionRoutingDDL is the WT3 routing and WorkUnit-graph schema, shared
// verbatim with sqlc so generated code and the running database cannot
// disagree.
//
//go:embed schema/execution_routing.sql
var executionRoutingDDL string

// reconcileExecutionRoutingSchema installs the schema migration 0114
// deliberately does not.
//
// It exists for the same reason reconcileComposedOutcomesSchema does: a burned
// 0099/0100 ledger entry marks those versions applied while leaving
// contract_revisions, plan_revisions and work_units physically absent, and
// ALTER TABLE cannot be made conditional inside migration SQL. On a complete
// profile this adds the execution-preference, routing-decision and model
// binding columns and the WorkUnit graph tables; on a degraded one it defers
// without inventing routing state.
//
// Every statement is idempotent, so a repaired profile heals on the next start.
func reconcileExecutionRoutingSchema(db *sql.DB) error {
	for _, table := range []string{
		"contract_revisions", "plan_revisions", "work_units",
		"contract_criteria", "work_unit_provider_bindings",
	} {
		var present int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
	}

	columns := []struct {
		table  string
		column string
		ddl    string
	}{
		{"contract_revisions", "execution_preference_json",
			`ALTER TABLE contract_revisions ADD COLUMN execution_preference_json TEXT
                CHECK (execution_preference_json IS NULL OR json_valid(execution_preference_json))`},
		{"plan_revisions", "routing_decisions_json",
			`ALTER TABLE plan_revisions ADD COLUMN routing_decisions_json TEXT
                CHECK (routing_decisions_json IS NULL OR json_valid(routing_decisions_json))`},
		{"work_unit_provider_bindings", "model_selection",
			`ALTER TABLE work_unit_provider_bindings ADD COLUMN model_selection TEXT
                CHECK (model_selection IS NULL OR model_selection IN ('explicit', 'provider_default'))`},
		{"work_unit_provider_bindings", "model",
			`ALTER TABLE work_unit_provider_bindings ADD COLUMN model TEXT`},
	}
	for _, column := range columns {
		var present int
		if err := db.QueryRow(
			fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?`, column.table),
			column.column,
		).Scan(&present); err != nil {
			return err
		}
		if present > 0 {
			continue
		}
		if _, err := db.Exec(column.ddl); err != nil {
			return err
		}
	}

	_, err := db.Exec(executionRoutingDDL)
	return err
}

// composedOutcomesDDL is the composition schema, shared verbatim with sqlc so
// generated code and the running database cannot disagree.
//
//go:embed schema/composed_outcomes.sql
var composedOutcomesDDL string

// reconcileComposedOutcomesSchema installs the composition schema migration
// 0106 deliberately does not (ADR 0007).
//
// It lives here for the same reason reconcileOutcomeProofSchema does: a burned
// 0099 ledger entry marks the version applied while leaving `outcomes` and
// `contract_criteria` physically absent, and ALTER TABLE cannot be made
// conditional inside migration SQL. On a complete profile this adds the parent
// column, the contribution_links table, and the fail-closed guards; on a
// degraded one it defers without inventing composition state.
//
// Every statement is idempotent, so a repaired profile heals on the next start.
func reconcileComposedOutcomesSchema(db *sql.DB) error {
	for _, table := range []string{"outcomes", "contract_revisions", "contract_criteria"} {
		var present int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
	}

	var hasParent int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('outcomes') WHERE name = 'parent_outcome_id'`,
	).Scan(&hasParent); err != nil {
		return err
	}
	if hasParent == 0 {
		// Default NULL is required: SQLite forbids ADD COLUMN with a
		// REFERENCES clause unless the default is NULL. It is also the correct
		// default — an existing Outcome contributes to nothing.
		if _, err := db.Exec(`ALTER TABLE outcomes ADD COLUMN parent_outcome_id TEXT REFERENCES outcomes (id)`); err != nil {
			return err
		}
	}

	_, err := db.Exec(composedOutcomesDDL)
	return err
}

// reconcileOutcomeProofSchema performs the one conditional data-copy SQLite
// migration SQL cannot express safely when a burned 0099 ledger entry leaves
// contract_revisions absent. On complete profiles it backfills stable
// criterion identity and installs cross-column lineage guards; on degraded
// profiles it defers without inventing Outcome state.
func reconcileOutcomeProofSchema(db *sql.DB) error {
	for _, table := range []string{"outcomes", "contract_revisions", "contract_criteria", "evidence_items", "verification_runs", "acceptance_decisions", "outcome_corrections"} {
		var present int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
	}

	_, err := db.Exec(`
INSERT OR IGNORE INTO contract_criteria (id, contract_revision_id, position, text)
SELECT 'crit-' || cr.id || '-' || printf('%04d', CAST(j.key AS INTEGER) + 1),
       cr.id,
       CAST(j.key AS INTEGER) + 1,
       CAST(j.value AS TEXT)
FROM contract_revisions cr, json_each(cr.success_criteria) j
ORDER BY cr.id, CAST(j.key AS INTEGER);

DROP TRIGGER IF EXISTS evidence_items_contract_lineage;
CREATE TRIGGER evidence_items_contract_lineage
BEFORE INSERT ON evidence_items
WHEN NOT EXISTS (
    SELECT 1 FROM contract_revisions
    WHERE id = NEW.contract_revision_id AND outcome_id = NEW.outcome_id
)
BEGIN SELECT RAISE(ABORT, 'evidence contract lineage mismatch'); END;

DROP TRIGGER IF EXISTS verification_runs_contract_lineage;
CREATE TRIGGER verification_runs_contract_lineage
BEFORE INSERT ON verification_runs
WHEN NOT EXISTS (
    SELECT 1 FROM contract_revisions
    WHERE id = NEW.contract_revision_id AND outcome_id = NEW.outcome_id
)
BEGIN SELECT RAISE(ABORT, 'verification contract lineage mismatch'); END;

DROP TRIGGER IF EXISTS acceptance_decisions_contract_lineage;
CREATE TRIGGER acceptance_decisions_contract_lineage
BEFORE INSERT ON acceptance_decisions
WHEN NOT EXISTS (
    SELECT 1 FROM contract_revisions
    WHERE id = NEW.contract_revision_id AND outcome_id = NEW.outcome_id
)
BEGIN SELECT RAISE(ABORT, 'acceptance contract lineage mismatch'); END;

DROP TRIGGER IF EXISTS outcome_corrections_decision_lineage;
CREATE TRIGGER outcome_corrections_decision_lineage
BEFORE INSERT ON outcome_corrections
WHEN NOT EXISTS (
    SELECT 1 FROM acceptance_decisions
    WHERE id = NEW.decision_id
      AND outcome_id = NEW.outcome_id
      AND contract_revision_id = NEW.contract_revision_id
)
BEGIN SELECT RAISE(ABORT, 'correction decision lineage mismatch'); END;
`)
	return err
}

// reconcileProjectChatProjection repairs the one known cross-repository Goose
// version collision. Kennel merged its project-chat projection as 0098 before
// Kennel assigned the same number to session_native_identity_generation. An
// Kennel-derived database can therefore legitimately contain version 98 without
// these triggers, causing Goose to skip Kennel's file. The physical trigger
// seam is unambiguous and CREATE TRIGGER IF NOT EXISTS is idempotent, so startup
// reconciles the missing behavior without rewriting either shipped ledger.
func reconcileProjectChatProjection(db *sql.DB) error {
	for table, columns := range map[string][]string{
		"sessions":              {"latest_assistant_update", "session_mode"},
		"conversations":         {"current_session_id"},
		"conversation_messages": {"role", "streaming", "text", "updated_at"},
	} {
		for _, column := range columns {
			var present int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column,
			).Scan(&present); err != nil {
				return err
			}
			if present == 0 {
				return fmt.Errorf("required column %s.%s is missing", table, column)
			}
		}
	}

	_, err := db.Exec(`
CREATE TRIGGER IF NOT EXISTS conversation_assistant_insert_session_projection
AFTER INSERT ON conversation_messages
WHEN NEW.role = 'assistant' AND NEW.streaming = 0
BEGIN
    UPDATE sessions
    SET latest_assistant_update = NEW.text,
        updated_at = CASE WHEN updated_at < NEW.updated_at THEN NEW.updated_at ELSE updated_at END
    WHERE id = (
        SELECT current_session_id
        FROM conversations
        WHERE id = NEW.conversation_id
    )
      AND session_mode = 'chat'
      AND is_terminated = 0;
END;

CREATE TRIGGER IF NOT EXISTS conversation_assistant_settle_session_projection
AFTER UPDATE OF text, streaming ON conversation_messages
WHEN NEW.role = 'assistant'
 AND NEW.streaming = 0
 AND (OLD.streaming <> 0 OR OLD.text <> NEW.text)
BEGIN
    UPDATE sessions
    SET latest_assistant_update = NEW.text,
        updated_at = CASE WHEN updated_at < NEW.updated_at THEN NEW.updated_at ELSE updated_at END
    WHERE id = (
        SELECT current_session_id
        FROM conversations
        WHERE id = NEW.conversation_id
    )
      AND session_mode = 'chat'
      AND is_terminated = 0;
END;
`)
	return err
}

// prepareAutoInjectReviewMigration preserves development databases whose
// physical review-injection schema exists without the corresponding goose
// ledger entry. Without this repair, goose replays 0084 and startup stops on
// the first duplicate column. Fresh schemas still run the migration normally;
// only the complete four-table shape is accepted as already applied.
func prepareAutoInjectReviewMigration(db *sql.DB) error {
	var gooseTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return nil
	}

	var applied int
	if err := db.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = 84 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied); err != nil {
		return err
	}
	if applied != 0 {
		return nil
	}

	for _, table := range []string{"sessions", "review_run", "pr_reviews", "pr_comment"} {
		var present int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'auto_inject_review'`, table,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
	}

	_, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (84, 1)`)
	return err
}

// prepareBrowserVerifierMigration preserves development databases that ran an
// earlier version of this branch where the verifier was mistakenly added to
// shipped migration 0048. The restored 0048 no longer owns the column, so mark
// the new 0081 migration applied when its exact schema effect already exists;
// otherwise goose applies 0081 normally.
func prepareBrowserVerifierMigration(db *sql.DB) error {
	var gooseTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return nil
	}
	var applied int
	if err := db.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = 81 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied); err != nil {
		return err
	}
	if applied != 0 {
		return nil
	}
	var verifierColumn int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'browser_capability_verifier'`,
	).Scan(&verifierColumn); err != nil {
		return err
	}
	if verifierColumn == 0 {
		return nil
	}
	_, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (81, 1)`)
	return err
}

// prepareQueuedTurnPromotionMigration preserves development databases that ran
// the promotion migration while it still used version 86 or 88. Upstream now
// owns both numbers, so record the promotion schema as version 89. If version 88
// came from the old promotion migration, remove that ledger entry so goose can
// apply upstream's auto-inject-CI migration at its canonical version.
func prepareQueuedTurnPromotionMigration(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var gooseTable int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return tx.Commit()
	}

	var promotionColumns int
	if err := tx.QueryRow(`
SELECT COUNT(*) FROM pragma_table_info('conversation_turns')
WHERE name IN ('promotion_started_at', 'promoted_to_turn_id')`).Scan(&promotionColumns); err != nil {
		return err
	}
	if promotionColumns != 2 {
		return tx.Commit()
	}

	var applied89 int
	if err := tx.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = 89 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied89); err != nil {
		return err
	}
	if applied89 == 0 {
		if _, err := tx.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (89, 1)`); err != nil {
			return err
		}
	}

	var applied88, autoInjectCIColumns int
	if err := tx.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = 88 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied88); err != nil {
		return err
	}
	if applied88 != 0 {
		if err := tx.QueryRow(`
SELECT (SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'auto_inject_ci')
     + (SELECT COUNT(*) FROM pragma_table_info('pr') WHERE name = 'auto_inject_ci')`).Scan(&autoInjectCIColumns); err != nil {
			return err
		}
		if autoInjectCIColumns == 0 {
			if _, err := tx.Exec(`DELETE FROM goose_db_version WHERE version_id = 88`); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func prepareBurnedSchemaRepairs(db *sql.DB) error {
	var gooseTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return nil
	}
	for _, rc := range schemaRepairs {
		var applied int
		if err := db.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = ? ORDER BY id DESC LIMIT 1
), 0)`, rc.version).Scan(&applied); err != nil {
			return err
		}
		if applied == 0 {
			continue
		}
		var tableCount int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, rc.table,
		).Scan(&tableCount); err != nil {
			return err
		}
		if tableCount == 0 {
			continue
		}
		var columnCount int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, rc.table, rc.column,
		).Scan(&columnCount); err != nil {
			return err
		}
		if columnCount > 0 {
			continue
		}
		if _, err := db.Exec(rc.addDDL); err != nil {
			return err
		}
		for _, stmt := range rc.postAdd {
			if _, err := db.Exec(stmt); err != nil {
				return err
			}
		}
	}
	return nil
}

// prepareReviewPerHarnessMigration makes the fresh 0080 rebuild executable on
// field databases that already recorded 0048/0049 as applied without actually
// running their SQL. Once 0080 has applied, it still repairs the physical review
// table if it is missing columns or constraints expected by current queries, but
// does not recreate review_session after the rebuild drops it.
func prepareReviewPerHarnessMigration(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var gooseTable int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return tx.Commit()
	}
	var applied80 int
	if err := tx.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = 80 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied80); err != nil {
		return err
	}
	var applied48 int
	if err := tx.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = 48 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied48); err != nil {
		return err
	}
	if applied48 == 0 {
		return tx.Commit()
	}
	var reviewTable int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'review'`,
	).Scan(&reviewTable); err != nil {
		return err
	}
	if reviewTable == 0 {
		return tx.Commit()
	}
	var agentSessionColumn int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('review') WHERE name = 'agent_session_id'`,
	).Scan(&agentSessionColumn); err != nil {
		return err
	}
	if agentSessionColumn == 0 {
		if _, err := tx.Exec(`ALTER TABLE review ADD COLUMN agent_session_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if applied80 != 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		return repairReviewPerHarnessShape(db)
	}
	var reviewSessionTable int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'review_session'`,
	).Scan(&reviewSessionTable); err != nil {
		return err
	}
	if reviewSessionTable == 0 {
		if _, err := tx.Exec(`
CREATE TABLE review_session (
    session_id         TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    project_id         TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    harness            TEXT NOT NULL,
    reviewer_handle_id TEXT NOT NULL DEFAULT '',
    agent_session_id   TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (session_id, harness)
)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func repairReviewPerHarnessShape(db *sql.DB) error {
	ok, err := reviewHasSessionHarnessUnique(db)
	if err != nil || ok {
		return err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer func() { _, _ = db.Exec(`PRAGMA foreign_keys=ON`) }()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
CREATE TABLE review_repair (
    id                 TEXT PRIMARY KEY,
    session_id         TEXT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    project_id         TEXT NOT NULL REFERENCES projects (id),
    harness            TEXT NOT NULL,
    pr_url             TEXT NOT NULL DEFAULT '',
    reviewer_handle_id TEXT NOT NULL DEFAULT '',
    agent_session_id   TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    UNIQUE(session_id, harness)
)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
INSERT INTO review_repair (
    id, session_id, project_id, harness, pr_url, reviewer_handle_id,
    agent_session_id, created_at, updated_at
)
SELECT
    id, session_id, project_id, harness, pr_url, reviewer_handle_id,
    agent_session_id, created_at, updated_at
FROM review`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE review`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE review_repair RENAME TO review`); err != nil {
		return err
	}
	return tx.Commit()
}

func reviewHasSessionHarnessUnique(db *sql.DB) (bool, error) {
	rows, err := db.Query(`PRAGMA index_list('review')`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	uniqueIndexes := []string{}
	for rows.Next() {
		var seq int
		var name, origin string
		var unique, partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return false, err
		}
		if unique == 0 {
			continue
		}
		uniqueIndexes = append(uniqueIndexes, name)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	for _, name := range uniqueIndexes {
		if indexColumnsMatch(db, name, []string{"session_id", "harness"}) {
			return true, nil
		}
	}
	return false, nil
}

func indexColumnsMatch(db *sql.DB, indexName string, want []string) bool {
	rows, err := db.Query(`PRAGMA index_info(` + quoteSQLiteIdent(indexName) + `)`)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	got := make([]string, 0, len(want))
	for rows.Next() {
		var seqno, cid int
		var name string
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return false
		}
		got = append(got, name)
	}
	if rows.Err() != nil || len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func quoteSQLiteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// repairRenumberedChatMigrationHistory preserves databases opened by this
// feature branch before its Chat migrations moved from 0052-0065 to 0066-0079.
// The files are byte-for-byte identical after the rename, so recording the new
// numbers is safer than replaying their ALTER/CREATE statements over an already
// upgraded schema. Version 0052 now belongs to model usage on main; if that
// physical schema is absent, release the burned ledger entry so goose can apply
// the real 0052 migration below.
func repairRenumberedChatMigrationHistory(db *sql.DB) error {
	// Probe on a read-only query first. A fresh database (the common import
	// path and the cli import test) has no goose_db_version table yet, so there
	// is nothing to repair; entering a write transaction would needlessly
	// create WAL/SHM files and is the exact I/O surface that surfaced as a
	// transient SQLITE_IOERR_WRITE on macOS CI runners.
	var gooseTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return nil
	}

	var chatColumn, conversationsTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'session_mode'`,
	).Scan(&chatColumn); err != nil {
		return err
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'conversations'`,
	).Scan(&conversationsTable); err != nil {
		return err
	}
	if chatColumn == 0 || conversationsTable == 0 {
		return nil
	}

	// Only enter a write transaction when there is actual repair work to do.
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	legacyApplied := false
	for oldVersion := int64(52); oldVersion <= 65; oldVersion++ {
		var applied int
		if err := tx.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = ? ORDER BY id DESC LIMIT 1
), 0)`, oldVersion).Scan(&applied); err != nil {
			return err
		}
		if applied == 0 {
			continue
		}
		legacyApplied = true
		newVersion := oldVersion + 14
		var alreadyMapped int
		if err := tx.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
    WHERE version_id = ? ORDER BY id DESC LIMIT 1
), 0)`, newVersion).Scan(&alreadyMapped); err != nil {
			return err
		}
		if alreadyMapped == 0 {
			if _, err := tx.Exec(
				`INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, 1)`,
				newVersion,
			); err != nil {
				return err
			}
		}
	}
	if !legacyApplied {
		return tx.Commit()
	}

	var modelUsageTable int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'model_usage_events'`,
	).Scan(&modelUsageTable); err != nil {
		return err
	}
	if modelUsageTable == 0 {
		if _, err := tx.Exec(`DELETE FROM goose_db_version WHERE version_id = 52`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// repairRenumberedAgentSwitchMigrationHistory preserves databases opened by
// earlier revisions of this feature branch. Agent switching first occupied
// 0080/0081, later 0081/0082, and briefly 0083/0084; main now owns 0080
// through 0084. Remap the physically present switching schema to the
// consolidated 0085, then release only the main migration numbers whose
// schema effects are still absent.
func repairRenumberedAgentSwitchMigrationHistory(db *sql.DB) error {
	var gooseTable, agentSwitchTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&gooseTable); err != nil {
		return err
	}
	if gooseTable == 0 {
		return nil
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'agent_switches'`,
	).Scan(&agentSwitchTable); err != nil {
		return err
	}
	if agentSwitchTable == 0 {
		return nil
	}

	reviewUpgraded, err := reviewHasSessionHarnessUnique(db)
	if err != nil {
		return err
	}

	var browserVerifierColumn, primeHarnessShape, reconciledKimchiPrimeHarnessShape, autoInjectReviewColumn int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'browser_capability_verifier'`,
	).Scan(&browserVerifierColumn); err != nil {
		return err
	}
	if err := db.QueryRow(`
SELECT COUNT(*)
FROM sqlite_master
WHERE type = 'table'
  AND name = 'sessions'
  AND instr(COALESCE(sql, ''), '''prime-agent''') > 0`).Scan(&primeHarnessShape); err != nil {
		return err
	}
	if err := db.QueryRow(`
SELECT COUNT(*)
FROM sqlite_master
WHERE type = 'table'
  AND name = 'sessions'
  AND instr(COALESCE(sql, ''), '''kimchi''') > 0
  AND instr(COALESCE(sql, ''), '''prime-agent''') > 0`).Scan(&reconciledKimchiPrimeHarnessShape); err != nil {
		return err
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'auto_inject_review'`,
	).Scan(&autoInjectReviewColumn); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Earlier feature builds split finalized-handoff storage into 0084, while
	// still earlier builds only had the base switching table. Repair either
	// shape before recording the now-consolidated 0085 as applied. The status is
	// deliberately not inferred from retained paths: old rows remain unknown.
	for _, column := range []struct {
		name string
		ddl  string
	}{
		{name: "final_handoff_path", ddl: `ALTER TABLE agent_switches ADD COLUMN final_handoff_path TEXT NOT NULL DEFAULT ''`},
		{name: "final_handoff_hash", ddl: `ALTER TABLE agent_switches ADD COLUMN final_handoff_hash TEXT NOT NULL DEFAULT ''`},
		{name: "source_transcript_status", ddl: `ALTER TABLE agent_switches ADD COLUMN source_transcript_status TEXT NOT NULL DEFAULT 'not_attempted' CHECK (source_transcript_status IN ('not_attempted', 'available', 'unavailable'))`},
		{name: "semantic_handoff_included", ddl: `ALTER TABLE agent_switches ADD COLUMN semantic_handoff_included INTEGER NOT NULL DEFAULT 0 CHECK (semantic_handoff_included IN (0, 1))`},
	} {
		var present int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('agent_switches') WHERE name = ?`, column.name,
		).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			if _, err := tx.Exec(column.ddl); err != nil {
				return err
			}
		}
	}

	var applied85 int
	if err := tx.QueryRow(`
SELECT COALESCE((
    SELECT is_applied FROM goose_db_version
	WHERE version_id = 85 ORDER BY id DESC LIMIT 1
), 0)`).Scan(&applied85); err != nil {
		return err
	}
	if applied85 == 0 {
		if _, err := tx.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (85, 1)`); err != nil {
			return err
		}
	}

	for version, mainEffectPresent := range map[int64]bool{
		80: reviewUpgraded,
		81: browserVerifierColumn != 0,
		82: primeHarnessShape != 0,
		83: reconciledKimchiPrimeHarnessShape != 0,
		84: autoInjectReviewColumn != 0,
	} {
		if mainEffectPresent {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM goose_db_version WHERE version_id = ?`, version); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// schemaRepairs lists the column-level effects of migrations that real
// installs are known to skip. Issue #3475/#3476: profiles exist whose
// goose_db_version already records versions 40 through 46 (written by a
// foreign build), so goose silently skips the real migrations carrying those
// numbers and the generated queries then fail with "no such column" — every
// session list 500s while /healthz stays green. A versioned repair migration
// cannot fix this class, because a burned version number is exactly what
// caused it; instead the physical schema is verified on every startup.
//
// Each entry keys on one column. postAdd statements replay the rest of the
// skipped migration's effects (backfills, index swaps) and run ONLY when the
// column was just added, so healthy databases — where those statements would
// clobber live data — are never touched.
//
// Any migration whose schema the generated queries depend on MUST add an entry
// here when a burned field profile can skip it, or the session list can regress
// to the 500s this exists to prevent.
var schemaRepairs = []struct {
	version int64
	table   string
	column  string
	addDDL  string
	postAdd []string
}{
	// 0040_add_session_diff_base.sql
	{version: 40, table: "sessions", column: "diff_base_sha",
		addDDL: `ALTER TABLE sessions ADD COLUMN diff_base_sha TEXT NOT NULL DEFAULT ''`},
	{version: 40, table: "sessions", column: "diff_base_ref",
		addDDL: `ALTER TABLE sessions ADD COLUMN diff_base_ref TEXT NOT NULL DEFAULT ''`},
	// 0041_notification_resolution.sql
	{version: 41, table: "notifications", column: "resolved_at",
		addDDL: `ALTER TABLE notifications ADD COLUMN resolved_at TIMESTAMP`,
		postAdd: []string{
			`UPDATE notifications SET resolved_at = created_at WHERE status = 'read'`,
			`DROP INDEX IF EXISTS idx_notifications_unread_dedupe`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_open_dedupe
    ON notifications(session_id, type, pr_url)
    WHERE status = 'unread' OR resolved_at IS NULL`,
			`CREATE INDEX IF NOT EXISTS idx_notifications_unresolved
    ON notifications(resolved_at, created_at DESC, id DESC)`,
		}},
	// 0042_review_run_unique_per_harness.sql
	{version: 42, table: "sessions", column: "reviewer_harness",
		addDDL: `ALTER TABLE sessions ADD COLUMN reviewer_harness TEXT NOT NULL DEFAULT ''`,
		postAdd: []string{
			`DROP INDEX IF EXISTS idx_review_run_session_pr_sha`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_review_run_session_pr_sha_harness
    ON review_run (session_id, pr_url, target_sha, harness)
    WHERE target_sha != ''
        AND status NOT IN ('failed', 'cancelled')
        AND (status = 'running' OR verdict NOT IN ('', 'changes_requested'))`,
		}},
	// 0043_add_session_pinned.sql. The trigger replay hangs off pinned_at, the
	// second of the two columns: it references both, and SQLite resolves a
	// trigger body at CREATE time, so it cannot run until both exist.
	{version: 43, table: "sessions", column: "is_pinned",
		addDDL: `ALTER TABLE sessions ADD COLUMN is_pinned BOOLEAN NOT NULL DEFAULT 0`},
	{version: 43, table: "sessions", column: "pinned_at",
		addDDL: `ALTER TABLE sessions ADD COLUMN pinned_at DATETIME`,
		postAdd: []string{
			`DROP TRIGGER IF EXISTS sessions_cdc_update`,
			`CREATE TRIGGER sessions_cdc_update
AFTER UPDATE ON sessions
WHEN OLD.activity_state <> NEW.activity_state
    OR OLD.is_terminated <> NEW.is_terminated
    OR (OLD.first_signal_at IS NULL AND NEW.first_signal_at IS NOT NULL)
    OR OLD.preview_url <> NEW.preview_url
    OR OLD.preview_revision <> NEW.preview_revision
    OR OLD.display_name <> NEW.display_name
    OR OLD.terminate_on_pr_merge <> NEW.terminate_on_pr_merge
    OR OLD.is_pinned <> NEW.is_pinned
    OR OLD.pinned_at <> NEW.pinned_at
    OR (OLD.pinned_at IS NULL AND NEW.pinned_at IS NOT NULL)
    OR (OLD.pinned_at IS NOT NULL AND NEW.pinned_at IS NULL)
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES (NEW.project_id, NEW.id, 'session_updated',
        json_object(
            'id', NEW.id,
            'activity', NEW.activity_state,
            'isTerminated', json(CASE WHEN NEW.is_terminated THEN 'true' ELSE 'false' END),
            'terminateOnPrMerge', json(CASE WHEN NEW.terminate_on_pr_merge THEN 'true' ELSE 'false' END),
            'previewUrl', NEW.preview_url,
            'previewRevision', NEW.preview_revision,
            'isPinned', json(CASE WHEN NEW.is_pinned THEN 'true' ELSE 'false' END)
        ),
        NEW.updated_at);
			END`,
		}},
	// 0081_browser_capability_verifier.sql. Keep the generated session queries
	// healthy even if a field database has already burned this migration number.
	{version: 81, table: "sessions", column: "browser_capability_verifier",
		addDDL: `ALTER TABLE sessions ADD COLUMN browser_capability_verifier TEXT NOT NULL DEFAULT ''`},
	// A pre-renumbered chat-mode branch created conversations before the
	// current_session_id controller binding existed, then later builds recorded
	// 0052 as applied. Generated chat queries require the column on startup.
	{version: 66, table: "conversations", column: "current_session_id",
		addDDL: `ALTER TABLE conversations ADD COLUMN current_session_id TEXT REFERENCES sessions(id) ON DELETE SET NULL`,
		postAdd: []string{
			`UPDATE conversations SET current_session_id = session_id WHERE current_session_id IS NULL AND session_id IS NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_conversations_current_session ON conversations(current_session_id)
    WHERE current_session_id IS NOT NULL`,
		}},
	// Version 86 was briefly used by queued-turn promotion on a development
	// branch. Ensure those databases also receive upstream's version-86 effect.
	{version: 86, table: "workspace_repos", column: "default_branch",
		addDDL: `ALTER TABLE workspace_repos ADD COLUMN default_branch TEXT NOT NULL DEFAULT ''`},
	// These columns are referenced by every generated conversation turn/message
	// query, so verify their physical presence on every startup.
	{version: 89, table: "conversation_turns", column: "promotion_started_at",
		addDDL: `ALTER TABLE conversation_turns ADD COLUMN promotion_started_at TIMESTAMP`},
	{version: 89, table: "conversation_turns", column: "promoted_to_turn_id",
		addDDL: `ALTER TABLE conversation_turns ADD COLUMN promoted_to_turn_id TEXT REFERENCES conversation_turns(id) ON DELETE SET NULL`},
}

// reconcileSchema verifies that the columns in schemaRepairs physically exist
// and replays the skipped migration's effects for any that are missing. It is
// idempotent: a healthy database (migrations applied normally, or one already
// repaired by hand or a previous startup) is left untouched. Failures surface
// as a specific, actionable startup error instead of an opaque INTERNAL_ERROR
// on the first session list.
func reconcileSchema(db *sql.DB) error {
	for _, rc := range schemaRepairs {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, rc.table, rc.column,
		).Scan(&count); err != nil {
			return fmt.Errorf("schema verification: inspect %s.%s: %w", rc.table, rc.column, err)
		}
		if count > 0 {
			continue
		}
		if _, err := db.Exec(rc.addDDL); err != nil {
			return fmt.Errorf(
				"schema repair: %s.%s is missing (a burned goose version skipped the migration that adds it, see #3475) and could not be added: %w",
				rc.table, rc.column, err,
			)
		}
		for _, stmt := range rc.postAdd {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("schema repair: replay skipped migration effects for %s.%s: %w", rc.table, rc.column, err)
			}
		}
	}
	if err := reconcileHarnessConstraint(db); err != nil {
		return err
	}
	return nil
}

const (
	sessionsHarnessCheckWithoutMuse                   = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'kiro', 'kilocode', 'vibe', 'pi', 'autohand', 'fake'))`
	sessionsHarnessCheckWithMuse                      = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'autohand', 'fake'))`
	sessionsHarnessCheckWithoutMuseQM                 = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'kiro', 'kilocode', 'vibe', 'pi', 'autohand', 'qm', 'fake'))`
	sessionsHarnessCheckWithMuseQM                    = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'autohand', 'qm', 'fake'))`
	sessionsHarnessCheckWithMuseKimchi                = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'kimchi', 'autohand', 'fake'))`
	sessionsHarnessCheckWithMuseQMKimchi              = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'kimchi', 'autohand', 'qm', 'fake'))`
	sessionsHarnessCheckWithMusePrimeAgent            = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'prime-agent', 'autohand', 'fake'))`
	sessionsHarnessCheckWithMuseQMPrimeAgent          = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'prime-agent', 'autohand', 'qm', 'fake'))`
	sessionsHarnessCheckWithMuseQMLegacyPrimeAgent    = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'autohand', 'qm', 'prime-agent', 'fake'))`
	sessionsHarnessCheckWithMuseKimchiPrimeAgent      = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'kimchi', 'prime-agent', 'autohand', 'fake'))`
	sessionsHarnessCheckWithMuseQMKimchiPrimeAgent    = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'kimchi', 'prime-agent', 'autohand', 'qm', 'fake'))`
	sessionsHarnessCheckWithMuseKimchiPrimeAgentOMP   = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'kimchi', 'prime-agent', 'autohand', 'omp', 'fake'))`
	sessionsHarnessCheckWithMuseQMKimchiPrimeAgentOMP = `CHECK (harness IN ('', 'claude-code', 'codex', 'aider', 'opencode', 'grok', 'droid', 'amp', 'agy', 'crush', 'cursor', 'qwen', 'copilot', 'goose', 'auggie', 'continue', 'devin', 'cline', 'kimi', 'muse', 'kiro', 'kilocode', 'vibe', 'pi', 'kimchi', 'prime-agent', 'autohand', 'omp', 'qm', 'fake'))`
)

func reconcileHarnessConstraint(db *sql.DB) error {
	var schema string
	if err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'sessions'`,
	).Scan(&schema); err != nil {
		return fmt.Errorf("schema verification: inspect sessions harness constraint: %w", err)
	}
	needsMuse := !strings.Contains(schema, "'muse'")
	needsKimchi := !strings.Contains(schema, "'kimchi'")
	needsPrimeAgent := !strings.Contains(schema, "'prime-agent'")
	needsOMP := !strings.Contains(schema, "'omp'")
	if !needsMuse && !needsKimchi && !needsPrimeAgent && !needsOMP {
		return nil
	}
	if _, err := db.Exec(`PRAGMA writable_schema = ON`); err != nil {
		return fmt.Errorf("schema repair: enable writable_schema for sessions harness constraint: %w", err)
	}
	type replacement struct {
		old string
		new string
	}
	var repairs []replacement
	if needsMuse {
		repairs = append(repairs,
			replacement{sessionsHarnessCheckWithoutMuse, sessionsHarnessCheckWithMuse},
			replacement{sessionsHarnessCheckWithoutMuseQM, sessionsHarnessCheckWithMuseQM},
		)
	}
	if needsKimchi {
		// After the Muse repair (if any), the constraint will be in one of
		// these known states — each gets Kimchi inserted in the same
		// position as migration 0054 does.
		repairs = append(repairs,
			replacement{sessionsHarnessCheckWithMuse, sessionsHarnessCheckWithMuseKimchi},
			replacement{sessionsHarnessCheckWithMuseQM, sessionsHarnessCheckWithMuseQMKimchi},
			replacement{sessionsHarnessCheckWithMusePrimeAgent, sessionsHarnessCheckWithMuseKimchiPrimeAgent},
			replacement{sessionsHarnessCheckWithMuseQMPrimeAgent, sessionsHarnessCheckWithMuseQMKimchiPrimeAgent},
			replacement{sessionsHarnessCheckWithMuseQMLegacyPrimeAgent, sessionsHarnessCheckWithMuseQMKimchiPrimeAgent},
		)
	}
	if needsPrimeAgent {
		repairs = append(repairs,
			replacement{sessionsHarnessCheckWithMuseKimchi, sessionsHarnessCheckWithMuseKimchiPrimeAgent},
			replacement{sessionsHarnessCheckWithMuseQMKimchi, sessionsHarnessCheckWithMuseQMKimchiPrimeAgent},
		)
	}
	if needsOMP {
		repairs = append(repairs,
			replacement{sessionsHarnessCheckWithMuseKimchiPrimeAgent, sessionsHarnessCheckWithMuseKimchiPrimeAgentOMP},
			replacement{sessionsHarnessCheckWithMuseQMKimchiPrimeAgent, sessionsHarnessCheckWithMuseQMKimchiPrimeAgentOMP},
		)
	}
	for _, r := range repairs {
		if _, err := db.Exec(
			`UPDATE sqlite_master
SET sql = replace(sql, ?, ?)
WHERE type = 'table' AND name = 'sessions'`,
			r.old,
			r.new,
		); err != nil {
			return fmt.Errorf("schema repair: widen sessions harness constraint: %w", err)
		}
	}
	if _, err := db.Exec(`PRAGMA writable_schema = RESET`); err != nil {
		return fmt.Errorf("schema repair: reparse sessions harness constraint: %w", err)
	}
	if err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'sessions'`,
	).Scan(&schema); err != nil {
		return fmt.Errorf("schema verification: inspect repaired sessions harness constraint: %w", err)
	}
	if !strings.Contains(schema, "'muse'") {
		return fmt.Errorf("schema repair: sessions harness constraint is missing Muse and did not match known pre-Muse schema")
	}
	if !strings.Contains(schema, "'kimchi'") {
		return fmt.Errorf("schema repair: sessions harness constraint is missing Kimchi and did not match known pre-Kimchi schema")
	}
	if !strings.Contains(schema, "'prime-agent'") {
		return fmt.Errorf("schema repair: sessions harness constraint is missing Prime Agent and did not match known pre-Prime-Agent schema")
	}
	if !strings.Contains(schema, "'omp'") {
		return fmt.Errorf("schema repair: sessions harness constraint is missing OMP and did not match known pre-OMP schema")
	}
	return nil
}

//go:embed schema/outcome_deletion_guards.sql
var outcomeDeletionGuardsDDL string

//go:embed schema/admission_packets.sql
var admissionPacketsDDL string

// reconcileAdmissionSchema conditionally installs admission evidence after
// burned-ledger repairs. It defers rather than inventing missing Outcome state.
func reconcileAdmissionSchema(db *sql.DB) error {
	for _, table := range []string{"outcomes", "plan_revisions", "work_units", "attempts"} {
		var present int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			return nil
		}
	}
	var budgetColumn int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_units') WHERE name='execution_budget_json'`).Scan(&budgetColumn); err != nil {
		return err
	}
	if budgetColumn == 0 {
		if _, err := db.Exec(`ALTER TABLE work_units ADD COLUMN execution_budget_json TEXT CHECK (execution_budget_json IS NULL OR json_valid(execution_budget_json))`); err != nil {
			return err
		}
	}
	var intentColumn int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_units') WHERE name='intent'`).Scan(&intentColumn); err != nil {
		return err
	}
	if intentColumn == 0 {
		if _, err := db.Exec(`ALTER TABLE work_units ADD COLUMN intent TEXT NOT NULL DEFAULT 'legacy_unknown'`); err != nil {
			return fmt.Errorf("add work unit intent: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS attempt_execution_usage (
		attempt_id TEXT NOT NULL REFERENCES attempts(id), provider TEXT NOT NULL, session_id TEXT NOT NULL,
		sequence INTEGER NOT NULL CHECK(sequence >= 1), cumulative_input_tokens INTEGER NOT NULL CHECK(cumulative_input_tokens >= 0),
		cumulative_output_tokens INTEGER NOT NULL CHECK(cumulative_output_tokens >= 0), input_delta INTEGER NOT NULL CHECK(input_delta >= 0),
		output_delta INTEGER NOT NULL CHECK(output_delta >= 0), created_at TIMESTAMP NOT NULL,
		PRIMARY KEY(attempt_id, provider, session_id, sequence))`); err != nil {
		return fmt.Errorf("create attempt execution usage ledger: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS attempt_budget_stops (
		attempt_id TEXT PRIMARY KEY REFERENCES attempts(id), session_id TEXT NOT NULL,
		reason_code TEXT NOT NULL CHECK(reason_code IN ('retry_budget_exhausted','token_budget_exhausted','wall_time_budget_exhausted')),
		measured_usage TEXT NOT NULL CHECK(json_valid(measured_usage)), claimed_at TIMESTAMP NOT NULL,
		provider_stopped_at TIMESTAMP, machine_result TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(machine_result)))`); err != nil {
		return fmt.Errorf("create attempt budget stop ledger: %w", err)
	}
	_, err := db.Exec(admissionPacketsDDL)
	return err
}

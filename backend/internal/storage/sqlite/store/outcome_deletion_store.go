package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// Scope follows database foreign keys from the selected responsibility. Tables
// holding project-wide knowledge are never erased as an incidental dependency.
func deletionTableAllowed(name string) bool {
	// New tables are not implicitly granted owner-erasure authority. Their FKs
	// block deletion until their ownership is reviewed and added here.
	const owned = " outcomes contract_revisions plan_revisions planning_sessions planning_turns work_units capability_grants attempts attempt_sessions attempt_observations attempt_execution_usage attempt_budget_stops attempt_fences attempt_recovery_receipts contract_criteria evidence_items verification_runs acceptance_decisions outcome_corrections intake_sessions intake_conversation_refs intake_clarifications intake_clarification_answers intake_proposal_revisions intake_confirmations contract_revision_intake_core intake_analysis_requests responsibility_links work_unit_provider_bindings intelligence_runs attempt_receipts attempt_artifact_files work_unit_checks attempt_check_runs outcome_run_intents outcome_document_contexts outcome_document_sources outcome_deliveries contribution_links decomposition_revisions decomposition_contributions decomposition_retained_criteria contribution_dependencies contribution_dependency_waivers decomposition_requests work_unit_dependencies work_unit_criterion_bindings work_unit_required_capabilities outcome_trash sessions session_worktrees session_cleanup_facts shell_terminals notifications conversations conversation_turns conversation_messages conversation_provider_events conversation_activities conversation_branches session_interface_transitions session_interface_transition_messages agent_switches usage_bindings usage_sources model_usage_events review_run review telemetry_event "
	return strings.Contains(owned, " "+name+" ")
}
func quoteDeletionName(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

type deletionFK struct {
	child, parent string
	from, to      []string
}

func deletionRelations(ctx context.Context, tx *sql.Tx) ([]deletionFK, error) {
	tables, err := scopeStrings(ctx, tx, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return nil, err
	}
	var links []deletionFK
	for _, table := range tables {
		if strings.HasPrefix(table, "sqlite_") || strings.HasPrefix(table, "goose_") {
			continue
		}
		tableLinks, err := deletionTableRelations(ctx, tx, table)
		if err != nil {
			return nil, err
		}
		links = append(links, tableLinks...)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].child+links[i].parent < links[j].child+links[j].parent })
	return links, nil
}
func deletionTableRelations(ctx context.Context, tx *sql.Tx, table string) ([]deletionFK, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_list("+quoteDeletionName(table)+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	grouped := map[int]*deletionFK{}
	for rows.Next() {
		var id, seq int
		var parent, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return nil, err
		}
		f := grouped[id]
		if f == nil {
			f = &deletionFK{child: table, parent: parent}
			grouped[id] = f
		}
		f.from = append(f.from, from)
		f.to = append(f.to, to)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	links := make([]deletionFK, 0, len(grouped))
	for _, f := range grouped {
		links = append(links, *f)
	}
	return links, nil
}
func expandDeletionScope(ctx context.Context, tx *sql.Tx, links []deletionFK) error {
	for pass := 0; pass < 256; pass++ {
		changed := int64(0)
		for _, f := range links {
			if !deletionTableAllowed(f.child) || !deletionTableAllowed(f.parent) {
				continue
			}
			var joins []string
			for i := range f.from {
				if f.to[i] == "" {
					return fmt.Errorf("unsupported implicit deletion key on %s", f.child)
				}
				joins = append(joins, "c."+quoteDeletionName(f.from[i])+"=p."+quoteDeletionName(f.to[i]))
			}
			// #nosec G202 -- identifiers are quoted schema names restricted by the explicit ownership allowlist; values use bindings.
			q := "INSERT OR IGNORE INTO outcome_purge_scope(table_name,row_id) SELECT ?,c.rowid FROM " + quoteDeletionName(f.child) + " c JOIN " + quoteDeletionName(f.parent) + " p ON " + strings.Join(joins, " AND ") + " JOIN outcome_purge_scope s ON s.table_name=? AND s.row_id=p.rowid"
			result, err := tx.ExecContext(ctx, q, f.child, f.parent)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			changed += n
		}
		if changed == 0 {
			return nil
		}
	}
	return fmt.Errorf("outcome deletion scope exceeded its traversal bound")
}
func scopeStrings(ctx context.Context, tx *sql.Tx, query string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) deletionScope(ctx context.Context, tx *sql.Tx, id domain.OutcomeID) (ports.OutcomeDeletionPreview, error) {
	p := ports.OutcomeDeletionPreview{OutcomeID: string(id), SessionIDs: []string{}, AttemptIDs: []string{}, WorkspacePaths: []string{}, Blockers: []string{}}
	var parent sql.NullString
	err := tx.QueryRowContext(ctx, "SELECT title,current_revision_number,parent_outcome_id,EXISTS(SELECT 1 FROM outcome_trash WHERE outcome_id=outcomes.id) FROM outcomes WHERE id=?", id).Scan(&p.Title, &p.Revision, &parent, &p.Trashed)
	if errors.Is(err, sql.ErrNoRows) {
		return p, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	if err != nil {
		return p, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT r.project_id,COALESCE(t.erasing,0) FROM outcomes o JOIN responsibility_spaces r ON r.id=o.space_id LEFT JOIN outcome_trash t ON t.outcome_id=o.id WHERE o.id=?", id).Scan(&p.ProjectID, &p.Erasing); err != nil {
		return p, err
	}
	if parent.Valid {
		p.Blockers = append(p.Blockers, "Delete contributing Outcomes from their parent Outcome.")
	}
	var existing int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM outcome_purge_scope").Scan(&existing); err != nil {
		return p, err
	}
	if existing != 0 {
		return p, fmt.Errorf("unexpected durable deletion scope; refusing erasure")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO outcome_purge_scope SELECT 'outcomes',rowid FROM outcomes WHERE id=?", id); err != nil {
		return p, err
	}
	links, err := deletionRelations(ctx, tx)
	if err != nil {
		return p, err
	}
	if err := expandDeletionScope(ctx, tx, links); err != nil {
		return p, err
	}
	// The confirmed intake belongs to this Outcome. Unconfirmed/shared
	// conversations are not swept into the erasure scope.
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO outcome_purge_scope SELECT 'intake_sessions',i.rowid FROM intake_sessions i JOIN intake_confirmations c ON c.intake_id=i.id JOIN outcome_purge_scope s ON s.table_name='intake_confirmations' AND s.row_id=c.rowid"); err != nil {
		return p, err
	}
	if err := expandDeletionScope(ctx, tx, links); err != nil {
		return p, err
	}
	// Session references are an ownership edge expressed as provider identity,
	// rather than a SQLite FK. Refuse shared sessions before adding their rows.
	p.SessionIDs, err = scopeStrings(ctx, tx, "SELECT DISTINCT a.session_id FROM attempt_sessions a JOIN outcome_purge_scope s ON s.table_name='attempt_sessions' AND s.row_id=a.rowid ORDER BY a.session_id")
	if err != nil {
		return p, err
	}
	for _, sid := range p.SessionIDs {
		var shared int
		err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM attempt_sessions a WHERE a.session_id=? AND NOT EXISTS (SELECT 1 FROM outcome_purge_scope s WHERE s.table_name='attempt_sessions' AND s.row_id=a.rowid)", sid).Scan(&shared)
		if err != nil {
			return p, err
		}
		var projectHistory int
		if err := tx.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM conversations WHERE (scope='project' OR session_id<>?) AND current_session_id=?) + (SELECT COUNT(*) FROM conversation_turns t JOIN conversations c ON c.id=t.conversation_id WHERE (c.scope='project' OR c.session_id<>?) AND t.handled_by_session_id=?) + (SELECT COUNT(*) FROM conversation_branches b JOIN conversations c ON c.id=b.conversation_id WHERE (c.scope='project' OR c.session_id<>?) AND b.session_id=?)", sid, sid, sid, sid, sid, sid).Scan(&projectHistory); err != nil {
			return p, err
		}
		shared += projectHistory
		if shared > 0 {
			p.Blockers = append(p.Blockers, "A session is shared with another Outcome; preserve it before deleting.")
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO outcome_purge_scope SELECT 'sessions',rowid FROM sessions WHERE id=?", sid); err != nil {
			return p, err
		}
	}
	if err := expandDeletionScope(ctx, tx, links); err != nil {
		return p, err
	}
	// Only concrete session rows need lifecycle cleanup. Historical references
	// may outlive session garbage collection.
	p.SessionIDs, err = scopeStrings(ctx, tx, "SELECT a.id FROM sessions a JOIN outcome_purge_scope s ON s.table_name='sessions' AND s.row_id=a.rowid ORDER BY a.id")
	if err != nil {
		return p, err
	}
	// A shared/project-wide row pointing into the scope must not be erased
	// incidentally. Surface it before any filesystem cleanup can start.
	for _, f := range links {
		if !deletionTableAllowed(f.parent) || deletionTableAllowed(f.child) {
			continue
		}
		joins := []string{}
		for i := range f.from {
			joins = append(joins, "c."+quoteDeletionName(f.from[i])+"=p."+quoteDeletionName(f.to[i]))
		}
		var n int
		q := "SELECT COUNT(*) FROM " + quoteDeletionName(f.child) + " c JOIN " + quoteDeletionName(f.parent) + " p ON " + strings.Join(joins, " AND ") + " JOIN outcome_purge_scope s ON s.table_name=? AND s.row_id=p.rowid"
		// Session CDC references are detached while audit payloads are retained.
		if f.child == "change_log" {
			continue
		}
		if err := tx.QueryRowContext(ctx, q, f.parent).Scan(&n); err != nil {
			return p, err
		}
		if n > 0 {
			p.Blockers = append(p.Blockers, "Shared records in "+f.child+" still reference this Outcome; preserve or detach them first.")
		}
	}
	p.AttemptIDs, err = scopeStrings(ctx, tx, "SELECT a.id FROM attempts a JOIN outcome_purge_scope s ON s.table_name='attempts' AND s.row_id=a.rowid ORDER BY a.id")
	if err != nil {
		return p, err
	}
	p.WorkspacePaths, err = scopeStrings(ctx, tx, "SELECT DISTINCT workspace_path FROM sessions a JOIN outcome_purge_scope s ON s.table_name='sessions' AND s.row_id=a.rowid WHERE workspace_path<>''")
	if err != nil {
		return p, err
	}
	var active int
	for _, q := range []string{
		"SELECT COUNT(*) FROM attempt_fences a JOIN outcome_purge_scope s ON s.table_name='attempt_fences' AND s.row_id=a.rowid WHERE a.released_at IS NULL",
		"SELECT COUNT(*) FROM attempts a JOIN outcome_purge_scope s ON s.table_name='attempts' AND s.row_id=a.rowid WHERE a.status IN ('queued','running','paused','lost')",
		"SELECT COUNT(*) FROM sessions a JOIN outcome_purge_scope s ON s.table_name='sessions' AND s.row_id=a.rowid WHERE a.is_terminated=0",
		"SELECT COUNT(*) FROM outcome_deliveries a JOIN outcome_purge_scope s ON s.table_name='outcome_deliveries' AND s.row_id=a.rowid WHERE a.state='pending'",
		"SELECT COUNT(*) FROM outcome_run_intents a JOIN outcome_purge_scope s ON s.table_name='outcome_run_intents' AND s.row_id=a.rowid WHERE a.desired='running' AND a.generation=(SELECT MAX(g.generation) FROM outcome_run_intents g WHERE g.outcome_id=a.outcome_id)",
	} {
		var n int
		if err := tx.QueryRowContext(ctx, q).Scan(&n); err != nil {
			return p, err
		}
		active += n
	}
	if active > 0 {
		p.Blockers = append(p.Blockers, "Stop or reconcile active sessions, run authorization and pending deliveries before deletion.")
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM outcome_purge_scope WHERE table_name='outcomes'").Scan(&p.OutcomeCount); err != nil {
		return p, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM outcome_purge_scope").Scan(&p.RecordCount); err != nil {
		return p, err
	}
	p.DocumentContextIDs, err = scopeStrings(ctx, tx, "SELECT a.id FROM outcome_document_contexts a JOIN outcome_purge_scope s ON s.table_name='outcome_document_contexts' AND s.row_id=a.rowid")
	if err != nil {
		return p, err
	}
	return p, nil
}

// PreviewOutcomeDeletion loads the owned scope without committing erasure authority.
func (s *Store) PreviewOutcomeDeletion(ctx context.Context, id domain.OutcomeID) (ports.OutcomeDeletionPreview, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return ports.OutcomeDeletionPreview{}, err
	}
	defer func() { _ = tx.Rollback() }()
	p, err := s.deletionScope(ctx, tx, id)
	return p, err
}

// ChangeOutcomeTrash hides or restores an inactive responsibility tree.
func (s *Store) ChangeOutcomeTrash(ctx context.Context, id domain.OutcomeID, revision int64, trash bool) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	p, err := s.deletionScope(ctx, tx, id)
	if err != nil {
		return err
	}
	if p.Revision != revision {
		return apierr.Invalid("OUTCOME_DELETION_STALE", "The Contract changed; refresh the deletion preview", nil)
	}
	if !trash && p.Erasing {
		return apierr.Invalid("OUTCOME_ERASURE_STARTED", "Permanent deletion has started; retry cleanup instead of restoring", nil)
	}
	if trash && len(p.Blockers) > 0 {
		return apierr.Invalid("OUTCOME_DELETION_BLOCKED", strings.Join(p.Blockers, " "), nil)
	}
	if trash {
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO outcome_trash(outcome_id,trash_root_id) SELECT o.id,? FROM outcomes o JOIN outcome_purge_scope s ON s.table_name='outcomes' AND s.row_id=o.rowid", id)
	} else {
		_, err = tx.ExecContext(ctx, "DELETE FROM outcome_trash WHERE trash_root_id=?", id)
	}
	if err != nil {
		return err
	}
	// Existing Outcome CDC emits the removal/restoration through the normal feed.
	if _, err = tx.ExecContext(ctx, "UPDATE outcomes SET updated_at=datetime('now') WHERE rowid IN (SELECT row_id FROM outcome_purge_scope WHERE table_name='outcomes')"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM outcome_purge_scope"); err != nil {
		return err
	}
	return tx.Commit()
}

// ListTrashedOutcomes lists recoverable roots and pending cleanup.
func (s *Store) ListTrashedOutcomes(ctx context.Context, project domain.ProjectID) ([]ports.OutcomeTrashEntry, error) {
	rows, err := s.readDB.QueryContext(ctx, "SELECT o.id,o.title,o.current_revision_number,t.erasing FROM outcomes o JOIN responsibility_spaces r ON r.id=o.space_id JOIN outcome_trash t ON t.outcome_id=o.id AND t.trash_root_id=o.id WHERE r.project_id=? ORDER BY t.deleted_at DESC", project)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ports.OutcomeTrashEntry{}
	for rows.Next() {
		p := ports.OutcomeTrashEntry{Trashed: true}
		if err := rows.Scan(&p.OutcomeID, &p.Title, &p.Revision, &p.Erasing); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PurgeOutcomeRecords erases only the previewed, inactive responsibility. The
// transaction rolls back every deletion and its row authority on any failure.
func (s *Store) PurgeOutcomeRecords(ctx context.Context, id domain.OutcomeID, revision int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	p, err := s.deletionScope(ctx, tx, id)
	if err != nil {
		return err
	}
	if !p.Trashed || !p.Erasing || p.Revision != revision {
		return apierr.Invalid("OUTCOME_DELETION_STALE", "Move this Outcome to Trash and refresh before permanent deletion", nil)
	}
	if len(p.Blockers) > 0 {
		return apierr.Invalid("OUTCOME_DELETION_BLOCKED", strings.Join(p.Blockers, " "), nil)
	}
	// Workspace teardown belongs to the session manager. Never recursively
	// remove a workspace from a path read out of an Outcome or provider record.
	for _, path := range p.WorkspacePaths {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return apierr.Invalid("OUTCOME_WORKSPACE_REMAINS", "Clean up the session workspace before permanent deletion: "+path, nil)
		}
	}
	if _, err = tx.ExecContext(ctx, "PRAGMA defer_foreign_keys=ON"); err != nil {
		return err
	}
	// Preserve audit payloads while detaching their FK from erased sessions.
	// The Outcome delete trigger emits a tombstone.
	if _, err = tx.ExecContext(ctx, "UPDATE change_log SET session_id=NULL WHERE session_id IN (SELECT a.id FROM sessions a JOIN outcome_purge_scope s ON s.table_name='sessions' AND s.row_id=a.rowid)"); err != nil {
		return err
	}
	tables, err := scopeStrings(ctx, tx, "SELECT DISTINCT table_name FROM outcome_purge_scope ORDER BY table_name")
	if err != nil {
		return err
	}
	// RESTRICT constraints can require child-first order even in a deferred
	// transaction. Retry only tables whose dependencies were removed this pass.
	for len(tables) > 0 {
		remaining := []string{}
		var lastErr error
		for _, table := range tables {
			// #nosec G202 -- table is a quoted identifier from the private, allowlisted scope, never request data.
			_, err = tx.ExecContext(ctx, "DELETE FROM "+quoteDeletionName(table)+" WHERE rowid IN (SELECT row_id FROM outcome_purge_scope WHERE table_name=?)", table)
			if err != nil {
				remaining = append(remaining, table)
				lastErr = err
			}
		}
		if len(remaining) == len(tables) {
			return fmt.Errorf("outcome history cannot be erased because another record still protects it: %w", lastErr)
		}
		tables = remaining
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM outcome_purge_scope"); err != nil {
		return err
	}
	return tx.Commit()
}

// BeginOutcomePurge marks irreversible cleanup durably before removing bytes.
func (s *Store) BeginOutcomePurge(ctx context.Context, id domain.OutcomeID, revision int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	p, err := s.deletionScope(ctx, tx, id)
	if err != nil {
		return err
	}
	if !p.Trashed || p.Revision != revision {
		return apierr.Invalid("OUTCOME_DELETION_STALE", "Move to Trash and refresh before permanent deletion", nil)
	}
	if len(p.Blockers) > 0 {
		return apierr.Invalid("OUTCOME_DELETION_BLOCKED", strings.Join(p.Blockers, " "), nil)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE outcome_trash SET erasing=1 WHERE trash_root_id=?", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM outcome_purge_scope"); err != nil {
		return err
	}
	return tx.Commit()
}

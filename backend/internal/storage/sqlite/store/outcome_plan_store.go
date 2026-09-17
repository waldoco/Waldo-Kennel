package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

const insertPlanRevisionCanonicalSQL = `
INSERT INTO plan_revisions (
    id, outcome_id, number, contract_revision_number, status, summary,
    assumptions_json, blockers_json, run_brief_core_digest, run_brief_compiled_digest,
    planning_session_id, source_intelligence_run_id, routing_decisions_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) `

const insertWorkUnitExecutionBindingCanonicalSQL = `
INSERT INTO work_unit_provider_bindings (work_unit_id, provider, model_selection, model)
VALUES (?, ?, ?, ?)`

// AppendPlanRevision is the one production writer for new Plan revisions. The
// entire immutable graph, exact execution bindings, criterion coverage,
// required capabilities, routing provenance, and grants land atomically.
func (s *Store) AppendPlanRevision(ctx context.Context, outcomeID domain.OutcomeID, plan domain.PlanRevision) (domain.PlanRevision, error) {
	if plan.Status != domain.PlanStatusProposed {
		return domain.PlanRevision{}, fmt.Errorf("append plan for %s: only proposed plans are created", outcomeID)
	}
	plan.OutcomeID = outcomeID
	// Pre-position callers are legacy in-process producers. Freeze their
	// reviewed slice order once at append; new planner output arrives already
	// positioned by the canonical compiler. Partial/malformed positions still
	// fail domain validation.
	positioned := 0
	for i := range plan.WorkUnits {
		if plan.WorkUnits[i].Position > 0 {
			positioned++
		}
	}
	if positioned == 0 {
		for i := range plan.WorkUnits {
			plan.WorkUnits[i].Position = int64(i + 1)
		}
	}
	if err := validateCanonicalPlanPersistence(plan); err != nil {
		return domain.PlanRevision{}, err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return domain.PlanRevision{}, fmt.Errorf("begin append plan for %s: %w", outcomeID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)

	maxNum, err := txq.MaxPlanRevisionNumber(ctx, outcomeID)
	if err != nil {
		return domain.PlanRevision{}, fmt.Errorf("max plan number for %s: %w", outcomeID, err)
	}
	switch value := maxNum.(type) {
	case int64:
		plan.Number = value + 1
	default:
		return domain.PlanRevision{}, fmt.Errorf("max plan number for %s: unexpected type %T", outcomeID, maxNum)
	}
	if err := plan.Validate(); err != nil {
		return domain.PlanRevision{}, err
	}

	routingJSON, err := json.Marshal(plan.RoutingDecisions)
	if err != nil {
		return domain.PlanRevision{}, fmt.Errorf("encode routing decisions for plan %s: %w", plan.ID, err)
	}
	assumptionsJSON, err := marshalJSONStrings(plan.Assumptions)
	if err != nil {
		return domain.PlanRevision{}, fmt.Errorf("encode assumptions for plan %s: %w", plan.ID, err)
	}
	blockersJSON, err := marshalJSONStrings(plan.Blockers)
	if err != nil {
		return domain.PlanRevision{}, fmt.Errorf("encode blockers for plan %s: %w", plan.ID, err)
	}
	if _, err := tx.ExecContext(ctx, insertPlanRevisionCanonicalSQL,
		plan.ID, plan.OutcomeID, plan.Number, plan.ContractRevisionNumber, string(plan.Status), plan.Summary,
		assumptionsJSON, blockersJSON, plan.RunBriefCoreDigest, plan.RunBriefCompiledDigest,
		nullString(plan.PlanningSessionID.String()), nullString(string(plan.SourceIntelligenceRunID)), string(routingJSON),
	); err != nil {
		return domain.PlanRevision{}, fmt.Errorf("create plan revision %s: %w", plan.ID, err)
	}

	var contractRevisionID string
	if err := tx.QueryRowContext(ctx, `
SELECT id FROM contract_revisions
WHERE outcome_id = ? AND number = ?`, plan.OutcomeID, plan.ContractRevisionNumber).Scan(&contractRevisionID); err != nil {
		return domain.PlanRevision{}, fmt.Errorf("resolve contract revision for plan %s: %w", plan.ID, err)
	}

	for _, unit := range plan.WorkUnits {
		checks, err := marshalJSONStrings(unit.EvidenceChecks)
		if err != nil {
			return domain.PlanRevision{}, fmt.Errorf("plan %s work unit %s evidence checks: %w", plan.ID, unit.ID, err)
		}
		stops, err := marshalJSONStrings(unit.StopConditions)
		if err != nil {
			return domain.PlanRevision{}, fmt.Errorf("plan %s work unit %s stop conditions: %w", plan.ID, unit.ID, err)
		}
		budgetJSON, err := json.Marshal(unit.ExecutionBudget)
		if err != nil {
			return domain.PlanRevision{}, fmt.Errorf("plan %s work unit %s budget: %w", plan.ID, unit.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_units (id,plan_revision_id,kind,title,position,contract_revision_number,output_summary,evidence_checks,verification_requirement,stop_conditions,execution_budget_json,intent) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, unit.ID, plan.ID, string(unit.Kind), unit.Title, unit.Position, unit.ContractRevisionNumber, unit.OutputSummary, checks, unit.VerificationRequirement, stops, string(budgetJSON), string(unit.Intent)); err != nil {
			return domain.PlanRevision{}, fmt.Errorf("create work unit %s: %w", unit.ID, err)
		}

		binding, err := unit.ExecutionBindingForNewWork()
		if err != nil {
			return domain.PlanRevision{}, fmt.Errorf("work unit %s execution binding: %w", unit.ID, err)
		}
		var model any
		if binding.ModelSelection == domain.ExecutionBindingModelExplicit {
			model = binding.Model
		}
		if _, err := tx.ExecContext(ctx, insertWorkUnitExecutionBindingCanonicalSQL,
			unit.ID, string(binding.Provider), string(binding.ModelSelection), model,
		); err != nil {
			return domain.PlanRevision{}, fmt.Errorf("persist work unit %s execution binding: %w", unit.ID, err)
		}

		for _, criterionID := range unit.CriterionIDs {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO work_unit_criterion_bindings (work_unit_id, contract_revision_id, criterion_id)
VALUES (?, ?, ?)`, unit.ID, contractRevisionID, criterionID); err != nil {
				return domain.PlanRevision{}, fmt.Errorf("bind work unit %s to criterion %s: %w", unit.ID, criterionID, err)
			}
		}
		for _, capability := range unit.RequiredCapabilities {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO work_unit_required_capabilities (work_unit_id, capability)
VALUES (?, ?)`, unit.ID, capability); err != nil {
				return domain.PlanRevision{}, fmt.Errorf("persist work unit %s required capability %s: %w", unit.ID, capability, err)
			}
		}
		// Position is stored so the approved order is the executed order: a
		// build check that runs after the test that depends on it is a
		// different check set than the one reviewed.
		for position, check := range unit.Checks {
			argv, err := marshalJSONStrings(check.Argv)
			if err != nil {
				return domain.PlanRevision{}, fmt.Errorf("plan %s check %s argv: %w", plan.ID, check.ID, err)
			}
			if err := txq.CreateWorkUnitCheck(ctx, gen.CreateWorkUnitCheckParams{
				ID: string(check.ID), WorkUnitID: string(unit.ID), CriterionID: string(check.CriterionID),
				Position: int64(position), Argv: argv, TimeoutSeconds: check.TimeoutSeconds,
			}); err != nil {
				return domain.PlanRevision{}, fmt.Errorf("persist work unit %s approved check %s: %w", unit.ID, check.ID, err)
			}
		}
	}

	// Dependencies are inserted after every WorkUnit exists, so forward edges
	// remain valid graph truth rather than depending on serialization order.
	for _, unit := range plan.WorkUnits {
		for _, dependency := range unit.DependsOn {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO work_unit_dependencies (work_unit_id, depends_on_work_unit_id)
VALUES (?, ?)`, unit.ID, dependency); err != nil {
				return domain.PlanRevision{}, fmt.Errorf("persist work unit %s dependency %s: %w", unit.ID, dependency, err)
			}
		}
	}

	for _, grant := range plan.Grants {
		if err := txq.CreateCapabilityGrant(ctx, gen.CreateCapabilityGrantParams{
			ID: grant.ID, PlanRevisionID: plan.ID, Name: grant.Name, Scope: grant.Scope,
		}); err != nil {
			return domain.PlanRevision{}, fmt.Errorf("create capability grant %s: %w", grant.ID, err)
		}
	}
	if !plan.PlanningSessionID.IsZero() {
		planningSession, err := txq.GetPlanningSessionByID(ctx, plan.PlanningSessionID.String())
		if err != nil {
			return domain.PlanRevision{}, fmt.Errorf("read planning session %s before finalization: %w", plan.PlanningSessionID, err)
		}
		now := time.Now().UTC()
		changed, err := txq.LinkPlanningSessionPlan(ctx, gen.LinkPlanningSessionPlanParams{
			ProposedPlanRevisionID: nullableString(string(plan.ID)),
			UpdatedAt:              now,
			ClosedAt:               sql.NullTime{Time: now, Valid: true},
			ID:                     plan.PlanningSessionID.String(),
			// The update itself reads and increments the current session
			// revision inside this transaction. The write lock serializes all
			// canonical writers, while the SQL also rechecks exact Contract
			// identity/currentness and Plan provenance.
			Revision:                planningSession.Revision,
			ID_2:                    plan.ID,
			SourceIntelligenceRunID: nullableString(string(plan.SourceIntelligenceRunID)),
		})
		if err != nil {
			return domain.PlanRevision{}, fmt.Errorf("link planning session %s to plan %s: %w", plan.PlanningSessionID, plan.ID, err)
		}
		if changed != 1 {
			return domain.PlanRevision{}, &ports.PlanningFinalizeConflictError{SessionID: plan.PlanningSessionID}
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.PlanRevision{}, fmt.Errorf("commit append plan for %s: %w", outcomeID, err)
	}
	return plan, nil
}

func validateCanonicalPlanPersistence(plan domain.PlanRevision) error {
	if err := plan.Validate(); err != nil {
		// Number is assigned in the transaction, so permit only that missing
		// storage-owned field before persistence.
		planCopy := plan
		planCopy.Number = 1
		if err := planCopy.Validate(); err != nil {
			return err
		}
	}
	if err := domain.ValidateExactPlanCapabilityGrants(plan.Grants, plan.WorkUnits); err != nil {
		return err
	}
	decisions := make(map[domain.WorkUnitID]domain.WorkUnitRoutingDecision, len(plan.RoutingDecisions))
	for _, decision := range plan.RoutingDecisions {
		if _, duplicate := decisions[decision.WorkUnitID]; duplicate {
			return fmt.Errorf("duplicate routing decision for work unit %s", decision.WorkUnitID)
		}
		decisions[decision.WorkUnitID] = decision
	}
	for _, unit := range plan.WorkUnits {
		if _, err := unit.ExecutionBindingForNewWork(); err != nil {
			return fmt.Errorf("work unit %s is not executable: %w", unit.ID, err)
		}
		decision, ok := decisions[unit.ID]
		if !ok {
			return fmt.Errorf("work unit %s has no routing decision", unit.ID)
		}
		if err := decision.ValidateAgainst(unit); err != nil {
			return err
		}
	}
	if len(decisions) != len(plan.WorkUnits) {
		return fmt.Errorf("plan has routing decisions for unknown work units")
	}
	return nil
}

// LatestProposedPlanRevision loads the latest proposal for a contract revision.
func (s *Store) LatestProposedPlanRevision(ctx context.Context, outcomeID domain.OutcomeID, contractRevision int64) (domain.PlanRevision, bool, error) {
	row, err := s.qr.LatestProposedPlanRevision(ctx, gen.LatestProposedPlanRevisionParams{
		OutcomeID: outcomeID, ContractRevisionNumber: contractRevision,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlanRevision{}, false, nil
	}
	if err != nil {
		return domain.PlanRevision{}, false, fmt.Errorf("latest proposed plan for %s at r%d: %w", outcomeID, contractRevision, err)
	}
	return s.planFromRow(ctx, gen.PlanRevision{
		ID: row.ID, OutcomeID: row.OutcomeID, Number: row.Number, ContractRevisionNumber: row.ContractRevisionNumber,
		Status: row.Status, Summary: row.Summary, AssumptionsJson: row.AssumptionsJson, BlockersJson: row.BlockersJson,
		RunBriefCoreDigest: row.RunBriefCoreDigest, RunBriefCompiledDigest: row.RunBriefCompiledDigest,
		CreatedAt: row.CreatedAt, PlanningSessionID: row.PlanningSessionID,
		SourceIntelligenceRunID: row.SourceIntelligenceRunID, RoutingDecisionsJson: row.RoutingDecisionsJson,
	})
}

// GetPlanRevision loads one plan revision scoped to its Outcome.
func (s *Store) GetPlanRevision(ctx context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID) (domain.PlanRevision, bool, error) {
	row, err := s.qr.GetPlanRevision(ctx, gen.GetPlanRevisionParams{ID: planID, OutcomeID: outcomeID})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlanRevision{}, false, nil
	}
	if err != nil {
		return domain.PlanRevision{}, false, fmt.Errorf("get plan %s: %w", planID, err)
	}
	return s.planFromRow(ctx, gen.PlanRevision{
		ID: row.ID, OutcomeID: row.OutcomeID, Number: row.Number, ContractRevisionNumber: row.ContractRevisionNumber,
		Status: row.Status, Summary: row.Summary, AssumptionsJson: row.AssumptionsJson, BlockersJson: row.BlockersJson,
		RunBriefCoreDigest: row.RunBriefCoreDigest, RunBriefCompiledDigest: row.RunBriefCompiledDigest,
		CreatedAt: row.CreatedAt, PlanningSessionID: row.PlanningSessionID,
		SourceIntelligenceRunID: row.SourceIntelligenceRunID, RoutingDecisionsJson: row.RoutingDecisionsJson,
	})
}

// GetLatestPlanRevision loads the latest plan revision for an Outcome.
func (s *Store) GetLatestPlanRevision(ctx context.Context, outcomeID domain.OutcomeID) (domain.PlanRevision, bool, error) {
	row, err := s.qr.GetLatestPlanRevision(ctx, outcomeID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlanRevision{}, false, nil
	}
	if err != nil {
		return domain.PlanRevision{}, false, fmt.Errorf("latest plan for %s: %w", outcomeID, err)
	}
	return s.planFromRow(ctx, gen.PlanRevision{
		ID: row.ID, OutcomeID: row.OutcomeID, Number: row.Number, ContractRevisionNumber: row.ContractRevisionNumber,
		Status: row.Status, Summary: row.Summary, AssumptionsJson: row.AssumptionsJson, BlockersJson: row.BlockersJson,
		RunBriefCoreDigest: row.RunBriefCoreDigest, RunBriefCompiledDigest: row.RunBriefCompiledDigest,
		CreatedAt: row.CreatedAt, PlanningSessionID: row.PlanningSessionID,
		SourceIntelligenceRunID: row.SourceIntelligenceRunID, RoutingDecisionsJson: row.RoutingDecisionsJson,
	})
}

// ApprovePlanRevision marks a proposed plan revision as owner-approved.
func (s *Store) ApprovePlanRevision(ctx context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID) (domain.PlanRevision, bool, error) {
	s.writeMu.Lock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		s.writeMu.Unlock()
		return domain.PlanRevision{}, false, fmt.Errorf("begin approve plan %s: %w", planID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)

	rows, err := txq.ApprovePlanRevision(ctx, gen.ApprovePlanRevisionParams{ID: planID, OutcomeID: outcomeID})
	if err != nil {
		s.writeMu.Unlock()
		return domain.PlanRevision{}, false, fmt.Errorf("approve plan %s: %w", planID, err)
	}
	if rows == 0 {
		row, getErr := txq.GetPlanRevision(ctx, gen.GetPlanRevisionParams{ID: planID, OutcomeID: outcomeID})
		if errors.Is(getErr, sql.ErrNoRows) {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, false, nil
		}
		if getErr != nil {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, false, fmt.Errorf("re-read plan %s after guard miss: %w", planID, getErr)
		}
		if domain.PlanStatus(row.Status) != domain.PlanStatusApproved {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, true, fmt.Errorf("plan %s is not approvable from status %s", planID, row.Status)
		}
	}
	if err := tx.Commit(); err != nil {
		s.writeMu.Unlock()
		return domain.PlanRevision{}, true, fmt.Errorf("commit approve plan %s: %w", planID, err)
	}
	s.writeMu.Unlock()
	return s.GetPlanRevision(ctx, outcomeID, planID)
}

func (s *Store) planFromRow(ctx context.Context, row gen.PlanRevision) (domain.PlanRevision, bool, error) {
	grants, err := s.qr.ListCapabilityGrantsForPlan(ctx, row.ID)
	if err != nil {
		return domain.PlanRevision{}, true, fmt.Errorf("list grants for %s: %w", row.ID, err)
	}
	units, err := s.qr.ListWorkUnitsForPlan(ctx, row.ID)
	if err != nil {
		return domain.PlanRevision{}, true, fmt.Errorf("list work units for %s: %w", row.ID, err)
	}
	plan, err := planFromParts(row, units, grants)
	if err != nil {
		return domain.PlanRevision{}, true, err
	}
	if err := s.hydrateCanonicalPlanMetadata(ctx, &plan); err != nil {
		return domain.PlanRevision{}, true, err
	}
	if err := plan.Validate(); err != nil {
		return domain.PlanRevision{}, true, fmt.Errorf("plan %s failed readback validation: %w", row.ID, err)
	}
	return plan, true, nil
}

func planFromParts(row gen.PlanRevision, units []gen.WorkUnit, grants []gen.CapabilityGrant) (domain.PlanRevision, error) {
	assumptions, err := unmarshalJSONStrings(row.AssumptionsJson)
	if err != nil {
		return domain.PlanRevision{}, fmt.Errorf("plan %s assumptions: %w", row.ID, err)
	}
	blockers, err := unmarshalJSONStrings(row.BlockersJson)
	if err != nil {
		return domain.PlanRevision{}, fmt.Errorf("plan %s blockers: %w", row.ID, err)
	}
	plan := domain.PlanRevision{
		ID: row.ID, OutcomeID: row.OutcomeID, Number: row.Number,
		ContractRevisionNumber: row.ContractRevisionNumber, Status: domain.PlanStatus(row.Status),
		Summary: row.Summary, Assumptions: assumptions, Blockers: blockers, RunBriefCoreDigest: row.RunBriefCoreDigest,
		RunBriefCompiledDigest: row.RunBriefCompiledDigest, PlanningSessionID: domain.PlanningSessionID(row.PlanningSessionID.String),
		SourceIntelligenceRunID: domain.IntelligenceRunID(row.SourceIntelligenceRunID.String), CreatedAt: row.CreatedAt,
	}
	for _, item := range units {
		checks, err := unmarshalJSONStrings(item.EvidenceChecks)
		if err != nil {
			return domain.PlanRevision{}, fmt.Errorf("work unit %s evidence checks: %w", item.ID, err)
		}
		stops, err := unmarshalJSONStrings(item.StopConditions)
		if err != nil {
			return domain.PlanRevision{}, fmt.Errorf("work unit %s stop conditions: %w", item.ID, err)
		}
		plan.WorkUnits = append(plan.WorkUnits, domain.WorkUnit{
			ID: item.ID, Kind: domain.WorkUnitKind(item.Kind), Title: item.Title,
			ContractRevisionNumber: item.ContractRevisionNumber, OutputSummary: item.OutputSummary,
			EvidenceChecks: checks, VerificationRequirement: item.VerificationRequirement, StopConditions: stops,
		})
	}
	for _, grant := range grants {
		plan.Grants = append(plan.Grants, domain.CapabilityGrant{ID: grant.ID, Name: grant.Name, Scope: grant.Scope})
	}
	return plan, nil
}

func (s *Store) hydrateCanonicalPlanMetadata(ctx context.Context, plan *domain.PlanRevision) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}

	var routingJSON sql.NullString
	err := s.readDB.QueryRowContext(ctx, `
SELECT routing_decisions_json FROM plan_revisions WHERE id = ?`, plan.ID).Scan(&routingJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("plan %s disappeared during hydration", plan.ID)
	}
	if err != nil {
		return fmt.Errorf("get routing decisions for plan %s: %w", plan.ID, err)
	}
	if routingJSON.Valid && routingJSON.String != "" {
		if err := json.Unmarshal([]byte(routingJSON.String), &plan.RoutingDecisions); err != nil {
			return fmt.Errorf("decode routing decisions for plan %s: %w", plan.ID, err)
		}
	}

	for index := range plan.WorkUnits {
		unit := &plan.WorkUnits[index]

		var budgetJSON sql.NullString
		var intent string
		if err := s.readDB.QueryRowContext(ctx, `SELECT execution_budget_json, intent, COALESCE(position, 0) FROM work_units WHERE id=? AND plan_revision_id=?`, unit.ID, plan.ID).Scan(&budgetJSON, &intent, &unit.Position); err != nil {
			return fmt.Errorf("get execution budget for work unit %s: %w", unit.ID, err)
		}
		unit.Intent = domain.WorkUnitIntent(intent)
		if budgetJSON.Valid && budgetJSON.String != "" {
			if err := json.Unmarshal([]byte(budgetJSON.String), &unit.ExecutionBudget); err != nil {
				return fmt.Errorf("decode execution budget for work unit %s: %w", unit.ID, err)
			}
		}

		var provider string
		var modelSelection sql.NullString
		var model sql.NullString
		err := s.readDB.QueryRowContext(ctx, `
SELECT provider, model_selection, model
FROM work_unit_provider_bindings
WHERE work_unit_id = ?`, unit.ID).Scan(&provider, &modelSelection, &model)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// Provider-less history remains readable and non-executable.
		case err != nil:
			return fmt.Errorf("get execution binding for work unit %s: %w", unit.ID, err)
		default:
			unit.Provider = domain.AgentHarness(provider)
			if modelSelection.Valid {
				unit.ModelSelection = domain.ExecutionBindingModelSelection(modelSelection.String)
				if model.Valid {
					unit.Model = model.String
				}
			}
		}

		dependencies, err := queryWorkUnitStrings(ctx, s.readDB, `
SELECT depends_on_work_unit_id
FROM work_unit_dependencies
WHERE work_unit_id = ?
	ORDER BY depends_on_work_unit_id`, unit.ID)
		if err != nil {
			return fmt.Errorf("list dependencies for work unit %s: %w", unit.ID, err)
		}
		for _, dependency := range dependencies {
			unit.DependsOn = append(unit.DependsOn, domain.WorkUnitID(dependency))
		}

		criteria, err := queryWorkUnitStrings(ctx, s.readDB, `
SELECT criterion_id
FROM work_unit_criterion_bindings
WHERE work_unit_id = ?
	ORDER BY criterion_id`, unit.ID)
		if err != nil {
			return fmt.Errorf("list criteria for work unit %s: %w", unit.ID, err)
		}
		for _, criterion := range criteria {
			unit.CriterionIDs = append(unit.CriterionIDs, domain.CriterionID(criterion))
		}

		capabilities, err := queryWorkUnitStrings(ctx, s.readDB, `
SELECT capability
FROM work_unit_required_capabilities
WHERE work_unit_id = ?
	ORDER BY capability`, unit.ID)
		if err != nil {
			return fmt.Errorf("list required capabilities for work unit %s: %w", unit.ID, err)
		}
		unit.RequiredCapabilities = append(unit.RequiredCapabilities, capabilities...)

		checkRows, err := s.qr.ListWorkUnitChecksForWorkUnit(ctx, string(unit.ID))
		if err != nil {
			return fmt.Errorf("list approved checks for work unit %s: %w", unit.ID, err)
		}
		for _, row := range checkRows {
			argv, err := unmarshalJSONStrings(row.Argv)
			if err != nil {
				return fmt.Errorf("approved check %s argv: %w", row.ID, err)
			}
			unit.Checks = append(unit.Checks, domain.ApprovedCheck{
				ID: domain.ApprovedCheckID(row.ID), CriterionID: domain.CriterionID(row.CriterionID),
				Argv: argv, TimeoutSeconds: row.TimeoutSeconds,
			})
		}
	}

	// Keep every projection aligned with the frozen serial Plan order. Legacy
	// Plans have no positions and retain the historical lexical opaque-ID
	// policy. New Plans are fully positioned by validation.
	legacy := len(plan.WorkUnits) > 0
	for i := range plan.WorkUnits {
		if plan.WorkUnits[i].Position != 0 {
			legacy = false
			break
		}
	}
	if legacy {
		sort.Slice(plan.WorkUnits, func(i, j int) bool { return plan.WorkUnits[i].ID < plan.WorkUnits[j].ID })
	} else {
		sort.Slice(plan.WorkUnits, func(i, j int) bool { return plan.WorkUnits[i].Position < plan.WorkUnits[j].Position })
	}
	return nil
}

func queryWorkUnitStrings(ctx context.Context, db *sql.DB, query string, workUnitID domain.WorkUnitID) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, workUnitID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

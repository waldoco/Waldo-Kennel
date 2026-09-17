// Recovery and liveness for Act & Observe (#31).
//
// Containment marks suspicion without inventing facts; reconcile decides from
// durable evidence only. CUSTODY LAW: a fence may be released ONLY once the
// bound provider's stop is proven — a durably terminated session, or an
// explicit owner assertion that is recorded as its own containment
// observation. Stored status alone is never proof. Unproven liveness
// escalates as needs_attention; it NEVER releases custody, so a replacement
// can never write beside a possibly-live original provider. Stale
// observations remain inspectable but can never mutate current truth.
package outcome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// RecoverAttempt applies one owner-directed recovery verb:
//
//   - contain: mark suspicion; custody stays held; nothing is decided.
//   - reconcile: auto-verdict from heartbeat evidence — proven-alive records
//     `resumed`; otherwise the attempt becomes lost, its fence releases with
//     a reason, and a replacement_attempt receipt lands.
//   - replace: force the lost+release+receipt verdict so a new attempt may
//     acquire the fence.
//   - attention: record needs_attention without deciding custody.
//
// There is deliberately no resume verb: nothing in #31 can command a provider
// to resume, so proving an already-running provider alive is reconcile's job.
func (s *Service) RecoverAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID, in RecoveryInput) (RecoveryView, error) {
	if !in.Action.Valid() {
		return RecoveryView{}, apierr.Invalid("RECOVERY_ACTION_INVALID",
			"Recovery action must be contain, reconcile, replace, or attention", nil)
	}
	attempt, _, err := s.requireAttempt(ctx, outcomeID, attemptID)
	if err != nil {
		return RecoveryView{}, err
	}
	launchState, err := s.launchRecoveryState(ctx, attempt)
	if err != nil {
		return RecoveryView{}, apierr.Conflict(CodeAttemptCustodyUnproven, "Launch evidence is inconsistent; custody remains held", map[string]any{"detail": err.Error()})
	}
	// A packet without a matching bound/running session proves only that the
	// crash boundary was crossed. Provider launch is unconfirmed, so custody
	// stays held unless later machine evidence or explicit owner containment
	// proves it safe to release.
	switch in.Action {
	case RecoveryActionContain:
		return s.containAttempt(ctx, attempt, launchState)
	case RecoveryActionReconcile:
		return s.reconcileAttempt(ctx, in, attempt, launchState)
	case RecoveryActionReplace:
		return s.recoveryReplace(ctx, in, attempt, launchState)
	case RecoveryActionAttention:
		return s.recoveryAttention(ctx, attempt)
	}
	return RecoveryView{}, apierr.Invalid("RECOVERY_ACTION_INVALID", "Unknown recovery action", nil)
}

// containAttempt records suspicion. It mutates NO stored status: containment
// is an observation, and the open fence already blocks duplicate admission.
func (s *Service) containAttempt(ctx context.Context, attempt domain.Attempt, launchState LaunchRecoveryState) (RecoveryView, error) {
	switch attempt.Status {
	case domain.AttemptQueued, domain.AttemptRunning, domain.AttemptPaused:
	default:
		return RecoveryView{}, apierr.Conflict("ATTEMPT_ALREADY_ENDED",
			fmt.Sprintf("Attempt already ended as %s; nothing to contain", attempt.Status),
			map[string]any{"status": string(attempt.Status)})
	}
	payload := mustJSON(map[string]any{"reason": "liveness suspect", "launchState": launchState})
	if _, err := s.store.AppendAttemptObservation(ctx, attempt.ID, domain.ObservationAttemptContained, payload, s.clock()); err != nil {
		return RecoveryView{}, err
	}
	view, err := s.GetAttempt(ctx, attempt.OutcomeID, attempt.ID)
	if err != nil {
		return RecoveryView{}, err
	}
	return RecoveryView{Attempt: view}, nil
}

// custodyProof states WHY releasing a fence is safe. Custody may move only
// once the bound provider's stop is proven — the anti-duplicate-writer
// contract — or when the owner explicitly asserts containment (invariant 5).
type custodyProof struct {
	reason  string
	proven  bool
	byOwner bool
}

// proveProviderStopped resolves the release gate from durable facts:
//
//   - terminated bound session -> machine proof
//   - explicit owner assertion -> accepted and recorded as containment
//
// Stored status alone — including `failed` — is NEVER proof: a spawn
// failure's true point-of-failure is not knowable after the fact, so only
// termination facts or the owner's recorded assertion unlock custody.
func (s *Service) proveProviderStopped(facts attemptFacts, ownerConfirmed bool) custodyProof {
	switch {
	case facts.facts.Present && facts.facts.IsTerminated:
		return custodyProof{reason: "bound session durably terminated", proven: true}
	case ownerConfirmed:
		return custodyProof{reason: "owner asserted the provider is stopped", proven: true, byOwner: true}
	default:
		return custodyProof{}
	}
}

// refuseUnprovenCustody records the escalation receipt and refuses release.
func (s *Service) refuseUnprovenCustody(ctx context.Context, attempt domain.Attempt, launchState LaunchRecoveryState) error {
	payload := mustJSON(map[string]any{"launchState": launchState, "evidence": "provider stop unproven"})
	if _, err := s.store.AppendAttemptObservation(ctx, attempt.ID, domain.ObservationRecoveryAttention, payload, s.clock()); err != nil {
		return err
	}
	receipt, rErr := s.recordReceipt(ctx, attempt.ID, domain.RecoveryNeedsAttention, map[string]any{
		"evidence": "cannot prove the bound provider stopped — terminate it or confirm containment", "launchState": launchState,
	})
	if rErr != nil {
		return fmt.Errorf("record unproven-custody receipt for %s: %w", attempt.ID, rErr)
	}
	return apierr.Conflict(CodeAttemptCustodyUnproven,
		"Custody stays held: the bound provider's stop is not proven. Terminate it, then reconcile — or replace with explicit confirmation",
		map[string]any{"attemptId": string(attempt.ID), "receiptId": receipt.ID})
}

// maybeRecordOwnerContainment makes an owner stop-assertion inspectable.
func (s *Service) maybeRecordOwnerContainment(ctx context.Context, attempt domain.Attempt, proof custodyProof) error {
	if !proof.byOwner {
		return nil
	}
	payload := mustJSON(map[string]any{"assertion": "owner asserts the bound provider is stopped"})
	_, err := s.store.AppendAttemptObservation(ctx, attempt.ID, domain.ObservationOwnerContained, payload, s.clock())
	return err
}

// reconcileAttempt auto-verdicts from durable evidence. Every custody release
// requires proved provider stop; anything else escalates without deciding.
func (s *Service) reconcileAttempt(ctx context.Context, in RecoveryInput, attempt domain.Attempt, launchState LaunchRecoveryState) (RecoveryView, error) {
	facts, err := s.heartbeatFacts(ctx, attempt.ID)
	if err != nil {
		return RecoveryView{}, err
	}
	if attempt.Status == domain.AttemptSucceeded {
		if launchState == LaunchLegacy && !facts.terminated() {
			return RecoveryView{}, s.refuseUnprovenCustody(ctx, attempt, launchState)
		}
		return s.accountSucceededCustody(ctx, attempt)
	}
	if (launchState == LaunchRunning || launchState == LaunchLegacy) && attempt.Status == domain.AttemptRunning && facts.alive() {
		receipt, err := s.recordReceipt(ctx, attempt.ID, domain.RecoveryResumed, map[string]any{
			"evidence": "bound session present, signalled, not terminated",
		})
		if err != nil {
			return RecoveryView{}, err
		}
		view, err := s.GetAttempt(ctx, attempt.OutcomeID, attempt.ID)
		if err != nil {
			return RecoveryView{}, err
		}
		return RecoveryView{Attempt: view, Receipt: receipt}, nil
	}
	unknownCheck, err := s.resolveGovernedCheckTerminationUnknown(ctx, attempt.ID, domain.SessionID(facts.sessionID))
	if err != nil {
		return RecoveryView{}, err
	}
	ownerConfirmed := in.ConfirmProviderStopped && launchState != LaunchLegacy
	proof := s.proveProviderStopped(facts, ownerConfirmed)
	if launchState == LaunchPrepared {
		proof = custodyProof{reason: "launch packet absent; provider boundary not crossed", proven: true}
	}
	if unknownCheck {
		// Provider termination cannot prove that a detached check process tree
		// ended. Only the owner's explicit containment assertion reconciles it.
		proof = s.proveProviderStopped(attemptFacts{}, ownerConfirmed)
	}
	if !proof.proven {
		return RecoveryView{}, s.refuseUnprovenCustody(ctx, attempt, launchState)
	}
	if err := s.maybeRecordOwnerContainment(ctx, attempt, proof); err != nil {
		return RecoveryView{}, err
	}
	switch attempt.Status {
	case domain.AttemptFailed, domain.AttemptCancelled, domain.AttemptReconciled:
		// Terminal by truthful record: never rewrite to lost. Account for the
		// predecessor and hand custody to the replacement lineage.
		return s.accountTerminalCustody(ctx, attempt, proof.reason)
	default:
		return s.forceLost(ctx, attempt, domain.RecoveryReplacement,
			"reconcile could not prove liveness; provider stop "+proof.reason)
	}
}

// accountSucceededCustody repairs the only legitimate post-success recovery
// case: an older process classified and froze the result but crashed before
// releasing its fence. Success itself is already terminal and never becomes a
// replacement authorization; this method only accounts for custody.
func (s *Service) accountSucceededCustody(ctx context.Context, attempt domain.Attempt) (RecoveryView, error) {
	if s.receipts == nil {
		return RecoveryView{}, apierr.Conflict(CodeAttemptCustodyUnproven,
			"Succeeded Attempt custody cannot be repaired because receipt storage is unavailable", nil)
	}
	receipt, ok, err := s.receipts.GetAttemptReceipt(ctx, attempt.ID)
	if err != nil {
		return RecoveryView{}, err
	}
	if !ok || !receipt.Frozen() || !receipt.RetentionState.Complete() {
		return RecoveryView{}, apierr.Conflict(CodeAttemptCustodyUnproven,
			"Succeeded Attempt custody stays held until its complete retained result is present and frozen", map[string]any{"attemptId": string(attempt.ID)})
	}
	if err := s.releaseCustody(ctx, attempt.ID, "succeeded_attempt_reconciled"); err != nil {
		return RecoveryView{}, err
	}
	view, err := s.GetAttempt(ctx, attempt.OutcomeID, attempt.ID)
	if err != nil {
		return RecoveryView{}, err
	}
	return RecoveryView{Attempt: view}, nil
}

// accountTerminalCustody releases the fence a TERMINAL predecessor still
// holds (failed-before-spawn, owner-cancelled, ended-unclassified) without
// mutating its immutable record, then stamps the replacement receipt. This is
// the reconcile -> release(old) -> issue(new) path D4 guarantees; without it a
// cancelled or failed attempt would hold worktree custody forever.
func (s *Service) accountTerminalCustody(ctx context.Context, attempt domain.Attempt, reason string) (RecoveryView, error) {
	if err := s.releaseCustody(ctx, attempt.ID, "reconciled_terminal_"+string(attempt.Status)); err != nil {
		return RecoveryView{}, err
	}
	receipt, err := s.recordReceipt(ctx, attempt.ID, domain.RecoveryReplacement, map[string]any{
		"evidence":       "terminal predecessor accounted for (" + reason + "); custody handed to replacement",
		"previousStatus": string(attempt.Status),
	})
	if err != nil {
		return RecoveryView{}, err
	}
	view, err := s.GetAttempt(ctx, attempt.OutcomeID, attempt.ID)
	if err != nil {
		return RecoveryView{}, err
	}
	return RecoveryView{Attempt: view, Receipt: receipt}, nil
}

// recoveryReplace forces the lost verdict and hands custody back so the next
// StartAttempt may issue a fresh fence. Replacement is always a NEW row.
func (s *Service) recoveryReplace(ctx context.Context, in RecoveryInput, attempt domain.Attempt, launchState LaunchRecoveryState) (RecoveryView, error) {
	facts, fErr := s.heartbeatFacts(ctx, attempt.ID)
	if fErr != nil {
		return RecoveryView{}, fErr
	}
	if attempt.Status == domain.AttemptSucceeded {
		if launchState == LaunchLegacy && !facts.terminated() {
			return RecoveryView{}, s.refuseUnprovenCustody(ctx, attempt, launchState)
		}
		return s.accountSucceededCustody(ctx, attempt)
	}
	unknownCheck, err := s.resolveGovernedCheckTerminationUnknown(ctx, attempt.ID, domain.SessionID(facts.sessionID))
	if err != nil {
		return RecoveryView{}, err
	}
	ownerConfirmed := in.ConfirmProviderStopped && launchState != LaunchLegacy
	proof := s.proveProviderStopped(facts, ownerConfirmed)
	if launchState == LaunchPrepared {
		proof = custodyProof{reason: "launch packet absent; provider boundary not crossed", proven: true}
	}
	if unknownCheck {
		proof = s.proveProviderStopped(attemptFacts{}, ownerConfirmed)
	}
	if !proof.proven {
		return RecoveryView{}, s.refuseUnprovenCustody(ctx, attempt, launchState)
	}
	if err := s.maybeRecordOwnerContainment(ctx, attempt, proof); err != nil {
		return RecoveryView{}, err
	}
	switch attempt.Status {
	case domain.AttemptQueued, domain.AttemptRunning, domain.AttemptPaused:
		return s.forceLost(ctx, attempt, domain.RecoveryReplacement,
			"owner directed replacement; provider stop "+proof.reason)
	case domain.AttemptLost, domain.AttemptFailed, domain.AttemptCancelled, domain.AttemptReconciled:
		// Custody may already be released by reconcile; make sure it is, then
		// stamp the replacement receipt idempotently. Terminal records are
		// never rewritten — only their custody moves.
		if err := s.releaseCustody(ctx, attempt.ID, "replacement_attempt"); err != nil {
			return RecoveryView{}, err
		}
		receipt, err := s.recordReceipt(ctx, attempt.ID, domain.RecoveryReplacement, map[string]any{
			"evidence":       "predecessor accounted for (" + proof.reason + "); custody handed to replacement",
			"previousStatus": string(attempt.Status),
		})
		if err != nil {
			return RecoveryView{}, err
		}
		view, err := s.GetAttempt(ctx, attempt.OutcomeID, attempt.ID)
		if err != nil {
			return RecoveryView{}, err
		}
		return RecoveryView{Attempt: view, Receipt: receipt}, nil
	case domain.AttemptSucceeded:
		return s.accountSucceededCustody(ctx, attempt)
	default:
		return RecoveryView{}, apierr.Conflict("ATTEMPT_ALREADY_ENDED",
			fmt.Sprintf("Attempt already ended as %s", attempt.Status),
			map[string]any{"status": string(attempt.Status)})
	}
}

// recoveryAttention escalates to the owner without mutating status or custody.
func (s *Service) recoveryAttention(ctx context.Context, attempt domain.Attempt) (RecoveryView, error) {
	payload := mustJSON(map[string]any{"reason": "owner escalation"})
	if _, err := s.store.AppendAttemptObservation(ctx, attempt.ID, domain.ObservationRecoveryAttention, payload, s.clock()); err != nil {
		return RecoveryView{}, err
	}
	receipt, err := s.recordReceipt(ctx, attempt.ID, domain.RecoveryNeedsAttention, map[string]any{
		"evidence": "owner escalation recorded",
	})
	if err != nil {
		return RecoveryView{}, err
	}
	view, err := s.GetAttempt(ctx, attempt.OutcomeID, attempt.ID)
	if err != nil {
		return RecoveryView{}, err
	}
	return RecoveryView{Attempt: view, Receipt: receipt}, nil
}

// EvaluateAttemptLiveness is the daemon reconcile-loop hook. For every
// RUNNING attempt whose bound provider session is durably terminated, it
// records the exit as an ordered observation and moves the attempt to
// `reconciled`: ended and accounted for, result unclassified. Missing or
// stale heartbeats mutate NOTHING here — they derive as unconfirmed at read
// time and wait for explicit contain/reconcile.
func (s *Service) EvaluateAttemptLiveness(ctx context.Context) error {
	running, err := s.store.ListAttemptsByStatus(ctx, domain.AttemptRunning)
	if err != nil {
		return fmt.Errorf("list running attempts: %w", err)
	}
	var failures []error
	stopClaims := map[domain.AttemptID]domain.AttemptBudgetStop{}
	if stops, ok := s.store.(ports.AttemptBudgetStopStore); ok {
		pending, stopErr := stops.ListUnfinishedAttemptBudgetStops(ctx)
		if stopErr != nil {
			return fmt.Errorf("list budget stops: %w", stopErr)
		}
		for _, claim := range pending {
			stopClaims[claim.AttemptID] = claim
		}
	}
	for _, attempt := range running {
		if claim, claimed := stopClaims[attempt.ID]; claimed {
			if claim.ProviderStopped() {
				if err := s.finalizeBudgetStop(ctx, attempt, claim); err != nil {
					failures = append(failures, err)
				}
				continue
			}
			ref, bound, refErr := s.store.LatestAttemptSessionRef(ctx, attempt.ID)
			if refErr != nil || !bound || ref.SessionID != claim.SessionID {
				failures = append(failures, fmt.Errorf("attempt %s budget claim session mismatch", attempt.ID))
				continue
			}
			var measured map[string]any
			_ = json.Unmarshal([]byte(claim.MeasuredUsage), &measured)
			if err := s.failRunningAttemptForBudget(ctx, attempt, ref, claim.Reason, measured); err != nil {
				failures = append(failures, err)
			}
			continue
		}
		plan, found, planErr := s.store.GetPlanRevision(ctx, attempt.OutcomeID, attempt.PlanRevisionID)
		if planErr != nil {
			failures = append(failures, fmt.Errorf("attempt %s budget plan: %w", attempt.ID, planErr))
			continue
		}
		if found {
			if unit, ok := workUnitByID(plan, attempt.WorkUnitID); ok && unit.ExecutionBudget.WallTimeLimit > 0 && !s.clock().Before(attempt.CreatedAt.Add(unit.ExecutionBudget.WallTimeLimit)) {
				ref, bound, refErr := s.store.LatestAttemptSessionRef(ctx, attempt.ID)
				if refErr != nil {
					failures = append(failures, fmt.Errorf("attempt %s budget session: %w", attempt.ID, refErr))
					continue
				}
				if !bound {
					failures = append(failures, fmt.Errorf("attempt %s wall budget expired without a bound session", attempt.ID))
					continue
				}
				if stopErr := s.failRunningAttemptForBudget(ctx, attempt, ref, domain.RuntimeWallTimeBudgetExhausted, map[string]any{"elapsedNanos": s.clock().Sub(attempt.CreatedAt).Nanoseconds(), "wallTimeLimitNanos": unit.ExecutionBudget.WallTimeLimit.Nanoseconds()}); stopErr != nil {
					failures = append(failures, fmt.Errorf("attempt %s wall budget: %w", attempt.ID, stopErr))
				}
				continue
			}
		}
		facts, err := s.heartbeatFacts(ctx, attempt.ID)
		if err != nil {
			failures = append(failures, fmt.Errorf("attempt %s: %w", attempt.ID, err))
			continue
		}
		// Health-gated lease: only PROVABLY alive custodians renew their
		// fence stamp. Unhealthy/unproven attempts keep their OLD stamp — a
		// stale renewal is exactly how custody that may outlive its provider
		// becomes visible.
		if facts.alive() {
			if _, rErr := s.store.RenewFenceForAttempt(ctx, attempt.ID, s.clock()); rErr != nil {
				failures = append(failures, fmt.Errorf("renew fence for %s: %w", attempt.ID, rErr))
			}
		}
		if !facts.terminated() {
			continue
		}
		if blocked, checkErr := s.importGovernedCheckUncertainty(ctx, attempt.ID, domain.SessionID(facts.sessionID)); checkErr != nil {
			failures = append(failures, fmt.Errorf("attempt %s: %w", attempt.ID, checkErr))
			continue
		} else if blocked {
			// Unknown process-tree termination is stronger than a provider exit:
			// completion and custody release remain forbidden until explicit
			// owner recovery reconciles the possibly-surviving effect.
			continue
		}
		target := domain.AttemptReconciled
		outcome := "provider session ended; result unclassified"
		releaseReason := ""
		if facts.completionBoundary == domain.AttemptCompletionProcessExit &&
			!domain.SupervisedExitSucceeded(facts.exitCode, facts.exitReason) {
			target = domain.AttemptFailed
			outcome = "governed provider process exited unsuccessfully"
			releaseReason = "governed_provider_process_failed"
		}
		payload := mustJSON(map[string]any{
			"sessionId":  facts.sessionID,
			"outcome":    outcome,
			"exitCode":   facts.exitCode,
			"exitReason": facts.exitReason,
		})
		// The transition, its observation, and (for a failure) the custody
		// release commit atomically. A crash or write failure between them
		// would otherwise leave a terminal Attempt holding its workspace fence
		// forever: normal liveness scanning only revisits Running attempts, so
		// nothing else would ever converge that cleanup.
		// applied is false only when the Attempt moved off Running concurrently
		// (a competing reconciler already committed); nothing else follows in
		// this iteration either way, so the next tick simply sees the truth.
		if _, _, err := s.store.TerminateRunningAttemptWithObservation(ctx, ports.AttemptRunningTermination{
			OutcomeID: attempt.OutcomeID, AttemptID: attempt.ID, TargetStatus: target,
			ObservationKind: domain.ObservationProviderExit, ObservationPayload: payload,
			ReleaseReason: releaseReason, At: s.clock(),
		}); err != nil {
			failures = append(failures, fmt.Errorf("attempt %s: %w", attempt.ID, err))
		}
	}
	switch len(failures) {
	case 0:
		return nil
	case 1:
		return failures[0]
	default:
		return fmt.Errorf("attempt liveness evaluation: %d failures: %w", len(failures), errors.Join(failures...))
	}
}

func (s *Service) importGovernedCheckUncertainty(ctx context.Context, attemptID domain.AttemptID, sessionID domain.SessionID) (bool, error) {
	if s.governedCheckUncertainty == nil || strings.TrimSpace(string(sessionID)) == "" {
		return false, nil
	}
	fact, found, err := s.governedCheckUncertainty.GovernedCheckUncertainty(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("read governed check uncertainty for session %s: %w", sessionID, err)
	}
	if !found || !fact.TerminationUnknown {
		return false, nil
	}
	observations, err := s.store.ListAttemptObservations(ctx, attemptID)
	if err != nil {
		return false, fmt.Errorf("list observations before importing governed check uncertainty: %w", err)
	}
	for _, observation := range observations {
		if observation.Kind == domain.ObservationGovernedCheckTerminationUnknown {
			return true, nil
		}
	}
	payload := mustJSON(map[string]any{
		"sessionId":          fact.SessionID,
		"checkId":            fact.CheckID,
		"terminationUnknown": fact.TerminationUnknown,
		"timedOut":           fact.TimedOut,
		"cancelled":          fact.Cancelled,
		"enforcedBy":         fact.EnforcedBy,
		"observedAt":         fact.ObservedAt,
		"outcome":            "approved check termination unknown; completion and custody remain blocked",
	})
	if _, err := s.store.AppendAttemptObservation(ctx, attemptID, domain.ObservationGovernedCheckTerminationUnknown, payload, s.clock()); err != nil {
		return false, fmt.Errorf("record governed check uncertainty: %w", err)
	}
	return true, nil
}

func (s *Service) hasGovernedCheckTerminationUnknown(ctx context.Context, attemptID domain.AttemptID) (bool, error) {
	observations, err := s.store.ListAttemptObservations(ctx, attemptID)
	if err != nil {
		return false, err
	}
	for _, observation := range observations {
		if observation.Kind == domain.ObservationGovernedCheckTerminationUnknown {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) resolveGovernedCheckTerminationUnknown(ctx context.Context, attemptID domain.AttemptID, sessionID domain.SessionID) (bool, error) {
	// Recovery can run before the periodic liveness tick. Consult the durable
	// marker synchronously so that provider termination alone can never race
	// ahead of unknown child-process custody evidence.
	if blocked, err := s.importGovernedCheckUncertainty(ctx, attemptID, sessionID); err != nil || blocked {
		return blocked, err
	}
	return s.hasGovernedCheckTerminationUnknown(ctx, attemptID)
}

// attemptFacts pairs derived heartbeat facts with the binding they came from.
type attemptFacts struct {
	sessionID          string
	facts              domain.SessionHeartbeatFacts
	completionBoundary domain.AttemptCompletionBoundary
	exitCode           *int
	exitReason         string
}

// alive reports PROVABLE current liveness: present, signalled, not
// terminated, and durably active inside the staleness window (sticky states
// exempt). A session that vanished long ago is NOT alive forever.
func (f attemptFacts) alive() bool {
	if !f.facts.Present || f.facts.IsTerminated || f.facts.FirstSignalAt.IsZero() {
		return false
	}
	return f.facts.RecentlyActive(domain.LivenessPolicy{
		Now:                 time.Now().UTC(),
		StaleHeartbeatAfter: domain.DefaultStaleHeartbeatWindow,
	})
}

func (f attemptFacts) terminated() bool {
	return f.facts.Present && f.facts.IsTerminated
}

// heartbeatFacts resolves the bound session's heartbeat facts for an attempt.
func (s *Service) heartbeatFacts(ctx context.Context, attemptID domain.AttemptID) (attemptFacts, error) {
	ref, ok, err := s.store.LatestAttemptSessionRef(ctx, attemptID)
	if err != nil {
		return attemptFacts{}, err
	}
	if !ok || s.heartbeats == nil {
		return attemptFacts{}, nil
	}
	if s.admission != nil {
		if packet, found, err := s.admission.GetWorkspaceBoundLaunchPacket(ctx, attemptID); err != nil {
			return attemptFacts{}, err
		} else if found && packet.SessionID != ref.SessionID {
			return attemptFacts{}, fmt.Errorf("launch packet/session identity mismatch")
		}
	}
	rec, present, err := s.heartbeats.GetSession(ctx, domain.SessionID(ref.SessionID))
	if err != nil {
		return attemptFacts{}, err
	}
	if !present {
		return attemptFacts{sessionID: ref.SessionID}, nil
	}
	var completionBoundary domain.AttemptCompletionBoundary
	if domain.LooksLikeGovernedAdmissionSnapshot(ref.AdmissionSnapshot) {
		snapshot, err := domain.ParseAdmissionSnapshot(ref.AdmissionSnapshot)
		if err != nil {
			return attemptFacts{}, fmt.Errorf("parse governed admission snapshot: %w", err)
		}
		completionBoundary = snapshot.CompletionBoundary
	}
	return attemptFacts{
		sessionID: ref.SessionID,
		facts: domain.SessionHeartbeatFacts{
			Present:        true,
			ActivityState:  rec.Activity.State,
			FirstSignalAt:  rec.FirstSignalAt,
			LastActivityAt: rec.Activity.LastActivityAt,
			IsTerminated:   rec.IsTerminated,
		},
		completionBoundary: completionBoundary,
		exitCode:           rec.Metadata.SupervisedProcessExitCode,
		exitReason:         rec.Metadata.SupervisedProcessExitReason,
	}, nil
}

// forceLost walks the lost verdict: guarded transition to lost (legal from
// queued/running/paused), custody release with a reason, and the receipt.
func (s *Service) forceLost(ctx context.Context, attempt domain.Attempt, resolution domain.RecoveryResolution, evidence string) (RecoveryView, error) {
	rows, err := s.store.TransitionAttemptStatus(ctx, attempt.OutcomeID, attempt.ID, attempt.Status, domain.AttemptLost, s.clock())
	if err != nil {
		return RecoveryView{}, err
	}
	if rows == 0 && attempt.Status != domain.AttemptLost {
		return RecoveryView{}, apierr.Conflict("ATTEMPT_STATUS_MOVED", "The attempt changed state concurrently; reload and retry", nil)
	}
	if err := s.releaseCustody(ctx, attempt.ID, "reconciled_lost"); err != nil {
		return RecoveryView{}, err
	}
	receipt, err := s.recordReceipt(ctx, attempt.ID, resolution, map[string]any{"evidence": evidence})
	if err != nil {
		return RecoveryView{}, err
	}
	view, err := s.GetAttempt(ctx, attempt.OutcomeID, attempt.ID)
	if err != nil {
		return RecoveryView{}, err
	}
	return RecoveryView{Attempt: view, Receipt: receipt}, nil
}

// releaseCustody releases the attempt's open fence; releasing without holding
// one is a tolerated no-op so recovery verbs stay idempotent.
func (s *Service) releaseCustody(ctx context.Context, attemptID domain.AttemptID, reason string) error {
	_, err := s.store.ReleaseFenceForAttempt(ctx, attemptID, reason, s.clock())
	return err
}

func (s *Service) recordReceipt(ctx context.Context, attemptID domain.AttemptID, resolution domain.RecoveryResolution, detail map[string]any) (*domain.AttemptRecoveryReceipt, error) {
	receipt := domain.AttemptRecoveryReceipt{
		ID:         "rcpt-" + uuid.NewString(),
		AttemptID:  attemptID,
		Resolution: resolution,
		Detail:     mustJSON(detail),
		CreatedAt:  s.clock(),
	}
	if err := s.store.CreateRecoveryReceipt(ctx, receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

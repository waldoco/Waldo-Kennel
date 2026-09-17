package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/artifactstore"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/governedcheck"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// attemptCheckRunner executes an Attempt's approved checks in the workspace
// that Attempt actually produced, under the Attempt's own frozen policy.
//
// Running them in the producing workspace — rather than a reconstruction — is
// what makes the result meaningful: a build check needs the base checkout the
// change was made against, and the daemon owns that workspace until teardown.
// The cost is that a check can mutate what it checks, so the workspace is
// re-measured afterwards and a mismatch is reported rather than ignored.
type attemptCheckRunner struct {
	sessions    attemptSessionControl
	refs        attemptRetentionSource
	artifacts   *artifactstore.Store
	clock       func() time.Time
	escalations ports.CapabilityEscalationStore
}

var _ ports.AttemptCheckRunner = (*attemptCheckRunner)(nil)

func (r *attemptCheckRunner) now() time.Time {
	if r.clock != nil {
		return r.clock()
	}
	return time.Now().UTC()
}

func (r *attemptCheckRunner) RunAttemptChecks(ctx context.Context, req ports.AttemptCheckRequest) (ports.AttemptCheckResult, error) {
	if r == nil || r.sessions == nil || r.refs == nil || r.artifacts == nil {
		return ports.AttemptCheckResult{}, fmt.Errorf("attempt check runner is not fully wired")
	}
	if len(req.Checks) == 0 {
		return ports.AttemptCheckResult{}, nil
	}
	workspace, kind, err := r.attemptWorkspace(ctx, req.Attempt)
	if err != nil {
		return ports.AttemptCheckResult{}, err
	}

	result := ports.AttemptCheckResult{Observations: make([]ports.AttemptCheckObservation, 0, len(req.Checks))}
	for _, check := range req.Checks {
		result.Observations = append(result.Observations, r.runOne(ctx, req, check, workspace))
	}

	// Re-measure before returning. Bytes observed before a check ran are not
	// evidence about the bytes that exist after it.
	result.ObservedArtifactVersion, result.MeasurementError = r.measure(ctx, req, workspace, kind)
	return result, nil
}

// measure re-reads the workspace manifest after the checks ran.
//
// A failure here is reported rather than raised: the checks themselves did
// happen, and the caller needs their observations. What it must not do is let
// an unmeasurable workspace look intact, so the reason is returned and treated
// as "changed" by AttemptCheckResult.
func (r *attemptCheckRunner) measure(ctx context.Context, req ports.AttemptCheckRequest, workspace string, kind domain.WorkspaceKind) (version, failure string) {
	observed, err := r.artifacts.ObserveVersion(ctx, artifactstore.Input{
		AttemptID: req.Attempt.ID, OutcomeID: req.Attempt.OutcomeID, PlanRevisionID: req.Attempt.PlanRevisionID,
		WorkUnitID: req.Attempt.WorkUnitID, ContractRevisionNumber: req.Attempt.ContractRevisionNumber,
		WorkspaceKind: kind, WorkspacePath: workspace,
		RepositoryPath: req.Receipt.RepositoryPath, BaseRevision: req.Receipt.BaseRevision,
	})
	if err != nil {
		return "", err.Error()
	}
	return observed, ""
}

// runBaseline runs the same command against a pristine empty workspace, under
// the same frozen policy, to establish whether its result depends on the work
// at all.
//
// A zero exit proves the criterion only if a non-zero exit was possible. A
// command that passes with none of the work present — the recorded live failure
// printed the expected string and exited zero — cannot be evidence that the
// work happened. The baseline is what makes that detectable without a denylist
// of commands that "do not count", which would be folklore and wrong for the
// next command nobody listed.
//
// It establishes workspace dependence: necessary for the check to be evidence,
// not sufficient. A check that fails here could still be testing the wrong
// thing; owner-supplied fixtures are what answer that.
//
// The run is confined to a temporary directory that is removed afterwards, so
// it cannot touch the result it is about. A baseline that cannot be established
// is reported as unestablished rather than assumed either way.
func (r *attemptCheckRunner) runBaseline(ctx context.Context, req ports.AttemptCheckRequest, check domain.ApprovedCheck, observation *ports.AttemptCheckObservation) {
	root, err := os.MkdirTemp("", "kennel-check-baseline-")
	if err != nil {
		observation.BaselineDetail = "baseline workspace could not be created: " + err.Error()
		return
	}
	defer func() { _ = os.RemoveAll(root) }()

	run, err := governedcheck.Run(ctx, governedcheck.Request{
		Policy:        req.Policy,
		WorkspaceRoot: root,
		Argv:          append([]string(nil), check.Argv...),
		Timeout:       time.Duration(check.TimeoutSeconds) * time.Second,
	})
	switch {
	case err != nil && run.EnforcedBy == "":
		observation.BaselineDetail = "baseline could not be run: " + err.Error()
	case run.TerminationUnknown, run.TimedOut, run.Cancelled:
		// An interrupted baseline is not a failing baseline. Reading it as one
		// would let a command that merely hung count as having discriminated.
		observation.BaselineDetail = "baseline did not finish, so nothing was established"
	default:
		observation.BaselineRan = true
		observation.BaselinePassed = err == nil
	}
}

func (r *attemptCheckRunner) runOne(ctx context.Context, req ports.AttemptCheckRequest, check domain.ApprovedCheck, workspace string) ports.AttemptCheckObservation {
	observation := ports.AttemptCheckObservation{
		Check: check, ArtifactVersion: req.Receipt.ArtifactVersion, StartedAt: r.now(),
	}
	// The baseline runs first and in its own directory, so it can never observe
	// or disturb the retained result the real run is about.
	r.runBaseline(ctx, req, check, &observation)
	run, err := governedcheck.Run(ctx, governedcheck.Request{
		Policy:        req.Policy,
		WorkspaceRoot: workspace,
		Argv:          append([]string(nil), check.Argv...),
		Timeout:       time.Duration(check.TimeoutSeconds) * time.Second,
	})
	observation.EndedAt = r.now()
	observation.EnforcedBy = run.EnforcedBy
	observation.Output = run.Output
	observation.OutputTruncated = run.OutputTruncated
	observation.TimedOut = run.TimedOut
	observation.Cancelled = run.Cancelled
	observation.TerminationUnknown = run.TerminationUnknown
	observation.ExitCode = run.ExitCode

	switch {
	case errors.Is(err, governedcheck.ErrCapabilityDenied):
		observation.Unavailable = err.Error()
		r.recordCapabilityDenial(ctx, req, check)
		return observation
	case errors.Is(err, governedcheck.ErrEnforcementUnavailable),
		errors.Is(err, governedcheck.ErrInvalidCommand):
		// The check never launched. That is not a failing check, and calling
		// it one would let a host without a sandbox look like a host whose
		// checks are red.
		observation.Unavailable = err.Error()
		return observation
	case err == nil:
		observation.Ran = true
		observation.Passed = !run.TerminationUnknown
		return observation
	default:
		observation.Ran = run.EnforcedBy != ""
		if !observation.Ran {
			observation.Unavailable = err.Error()
			return observation
		}
		observation.Passed = false
		if observation.Output == "" {
			observation.Output = err.Error()
		}
		return observation
	}
}

// attemptWorkspace resolves the daemon-owned workspace of an ended Attempt.
// It never accepts a path from a client or from provider prose.
func (r *attemptCheckRunner) attemptWorkspace(ctx context.Context, attempt domain.Attempt) (string, domain.WorkspaceKind, error) {
	ref, found, err := r.refs.LatestAttemptSessionRef(ctx, attempt.ID)
	if err != nil {
		return "", "", fmt.Errorf("read session binding: %w", err)
	}
	if !found || strings.TrimSpace(ref.SessionID) == "" {
		return "", "", fmt.Errorf("attempt %s has no daemon-owned session binding", attempt.ID)
	}
	session, err := r.sessions.Get(ctx, domain.SessionID(ref.SessionID))
	if err != nil {
		return "", "", fmt.Errorf("read session %s: %w", ref.SessionID, err)
	}
	kind := domain.WorkspaceStagedFolder
	if strings.TrimSpace(session.Metadata.DiffBaseSHA) != "" || strings.TrimSpace(session.Metadata.WorkspaceRepoPath) != "" {
		kind = domain.WorkspaceGitWorktree
	}
	if strings.TrimSpace(session.Metadata.WorkspacePath) == "" {
		return "", "", fmt.Errorf("attempt %s has no workspace to check", attempt.ID)
	}
	return session.Metadata.WorkspacePath, kind, nil
}

func (r *attemptCheckRunner) recordCapabilityDenial(ctx context.Context, req ports.AttemptCheckRequest, check domain.ApprovedCheck) {
	if r.escalations == nil {
		return
	}
	ref, found, err := r.refs.LatestAttemptSessionRef(ctx, req.Attempt.ID)
	if err != nil || !found {
		return
	}
	policyDigest, err := req.Policy.Digest()
	if err != nil {
		return
	}
	raw, err := json.Marshal(struct {
		CheckID domain.ApprovedCheckID `json:"checkId"`
		Argv    []string               `json:"argv"`
	}{check.ID, check.Argv})
	if err != nil {
		return
	}
	sum := sha256.Sum256(raw)
	operationDigest := hex.EncodeToString(sum[:])
	qsum := sha256.Sum256([]byte("capability-question\x00" + operationDigest + "\x00" + string(req.Attempt.ID) + "\x00" + fmt.Sprint(req.Attempt.Number)))
	questionGeneration := "ceq-" + hex.EncodeToString(qsum[:16])
	within := false
	if revisions, readErr := r.escalations.ListContractRevisions(ctx, req.Attempt.OutcomeID); readErr == nil {
		for _, revision := range revisions {
			if revision.Number == req.Attempt.ContractRevisionNumber {
				within = domain.ContractAllowsCapability(revision, domain.CapabilityWorktreeExec)
				break
			}
		}
	}
	e := domain.CapabilityEscalation{Version: domain.CapabilityEscalationVersion, ExecutorKind: "governed_check", OutcomeID: req.Attempt.OutcomeID, ContractRevisionNumber: req.Attempt.ContractRevisionNumber, PlanRevisionID: req.Attempt.PlanRevisionID, WorkUnitID: req.Attempt.WorkUnitID, AttemptID: req.Attempt.ID, AttemptGeneration: req.Attempt.Number, AttemptSessionRefID: ref.ID, SessionID: domain.SessionID(ref.SessionID), SessionGeneration: ref.Seq, PolicyDigest: policyDigest, ArtifactVersion: req.Receipt.ArtifactVersion, CheckID: check.ID, RequestedCapability: domain.CapabilityWorktreeExec, DenialSource: "governed_check", GrantFingerprint: policyDigest, OperationID: "check:" + string(check.ID), RequestFingerprint: operationDigest, QuestionGeneration: questionGeneration, WithinContractCeiling: within}
	e.Digest, err = e.ComputedDigest()
	if err != nil {
		return
	}
	_, _, _ = r.escalations.CreateCapabilityEscalation(ctx, e)
}

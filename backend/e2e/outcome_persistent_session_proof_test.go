//go:build !windows

package e2e

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	_ "modernc.org/sqlite"
)

// TestOutcomeLaunchCutPersistentSessionProof drives only production HTTP and
// daemon restart paths. It is intentionally behind KENNEL_CHAT_E2E: passing
// requires a signed-in real Codex provider, not a fake completion.
func TestOutcomeLaunchCutPersistentSessionProof(t *testing.T) {
	requireE2E(t)
	dataDir := t.TempDir()
	d := startDaemon(t, dataDir)
	requireReasoningProvider(t, d)
	project := seedProject(t, d, "outcome-persistent")
	// Execution intent is explicit fixture wiring, never inferred from the
	// provider env: with no worker override, routing lexically picks
	// claude-code (correct production behavior), whose adapter has no governed
	// execution-policy mapping. Native Codex TUI has the only production one.
	d.mustCall("PUT", "/projects/"+project+"/config", http.StatusOK, map[string]any{
		"config": map[string]any{"defaultBranch": "main", "worker": map[string]any{"agent": "codex"}},
	}, nil)
	// The governed Attempt proof runs in TUI mode: chat mode (the Codex App
	// Server driver) deliberately rejects governed Attempt policies because
	// governed repo-tool injection is not implemented there.
	d.mustCall("PATCH", "/settings/session-interface", http.StatusOK, map[string]any{"defaultSessionMode": "tui"}, nil)

	var created struct {
		Outcome struct {
			ID              string `json:"id"`
			CurrentRevision struct {
				ID       string           `json:"id"`
				Number   int64            `json:"number"`
				Criteria []proofCriterion `json:"criteria"`
			} `json:"currentRevision"`
		} `json:"outcome"`
	}
	// The review line names the fixture's natural executable check and the
	// planner is expected to attach it: governed Codex admission fails closed
	// without an approved check vector, so this journey cannot launch
	// checkless. That check is also the only truthful success path after the
	// owner kill below - the guard there asserts its recorded provenance.
	d.mustCall("POST", "/projects/"+project+"/outcomes", http.StatusCreated, map[string]any{
		"title": "Persistent governed session fixture", "goal": "Create durable.txt containing PERSISTENT and keep the same governed Codex session steerable.",
		"successCriteria": []string{"durable.txt contains exactly PERSISTENT"}, "review": "Run test -f durable.txt and grep -Fx PERSISTENT durable.txt.",
		"authorityCeiling": map[string]any{"readWorkspace": true, "writeWorkspace": true, "executeLocal": true}, "requestKey": "b4-create",
	}, &created)
	out := created.Outcome.ID
	var plan struct {
		Plan struct {
			ID, Status string
			WorkUnits  []struct {
				ID             string              `json:"id"`
				Provider       string              `json:"provider"`
				ApprovedChecks []approvedCheckView `json:"approvedChecks"`
			} `json:"workUnits"`
		} `json:"plan"`
	}
	d.mustCall("POST", "/outcomes/"+out+"/plans", http.StatusCreated, map[string]any{"expectedContractRevision": 1}, &plan)
	if plan.Plan.Status != "proposed" || len(plan.Plan.WorkUnits) != 1 {
		t.Fatalf("proposal=%+v", plan.Plan)
	}
	if len(plan.Plan.WorkUnits[0].ApprovedChecks) == 0 {
		t.Fatalf("proposal carried no approved checks; the governed Codex launch requires one: %+v", plan.Plan)
	}
	if plan.Plan.WorkUnits[0].Provider != "codex" {
		t.Fatalf("frozen plan provider=%q, want codex", plan.Plan.WorkUnits[0].Provider)
	}
	assertFrozenBudgetMatchesFixture(t, dataDir, plan.Plan.ID)

	// Approval is durable authority, not execution: no Attempt/session may exist.
	d.mustCall("POST", "/outcomes/"+out+"/plans/"+plan.Plan.ID+"/approval", http.StatusOK, map[string]any{"expectedContractRevision": 1}, nil)
	assertLaunchRows(t, dataDir, out, 0, 0)

	start := startOutcomeAttempt(t, d, out, plan.Plan.ID, plan.Plan.WorkUnits[0].ID, "b4-start")
	if start.Status != "running" || len(start.Sessions) != 1 || start.Sessions[0].Mode != "tui" {
		t.Fatalf("started attempt=%+v", start)
	}
	session := start.Sessions[0].SessionID
	assertLaunchRows(t, dataDir, out, 1, 1)

	// Exact retry converges before any further work or restart.
	replay := startOutcomeAttempt(t, d, out, plan.Plan.ID, plan.Plan.WorkUnits[0].ID, "b4-start")
	if replay.ID != start.ID || replay.Sessions[0].SessionID != session {
		t.Fatalf("retry changed lineage: %+v -> %+v", start, replay)
	}
	assertLaunchRows(t, dataDir, out, 1, 1)

	// Steer the same live native session through the session-input seam
	// (`kennel send`'s endpoint). TUI input is send-keys into the one
	// persistent pane: there is no controller turn to re-id, so
	// not-interrupt-and-respawn is proven by the same runtime handle answering
	// the steered instruction.
	handle := sessionRuntimeHandle(t, dataDir, session)
	d.mustCall("POST", "/sessions/"+session+"/send", http.StatusOK, map[string]any{"message": "Run this shell command: for i in 1 2 3 4 5 6; do echo b4-$i; sleep 3; done"}, nil)
	awaitPane(t, handle, 2*time.Minute, "governed command running", func(out string) bool { return strings.Contains(out, "b4-1") })
	d.mustCall("POST", "/sessions/"+session+"/send", http.StatusOK, map[string]any{"message": "Stop that loop and ensure durable.txt contains exactly PERSISTENT, then reply B4-STEERED."}, nil)
	awaitPane(t, handle, 4*time.Minute, "steered work complete", func(out string) bool { return strings.Contains(out, "B4-STEERED") })

	// Crash/restart: tmux is the persistence layer and outlives the daemon, so
	// the same pane must still answer input while the exact
	// Attempt -> AttemptSessionRef -> session lineage rereads unchanged.
	d.kill()
	restarted := startDaemon(t, dataDir)
	awaitPane(t, handle, 90*time.Second, "pane surviving daemon restart", func(out string) bool { return strings.Contains(out, "B4-STEERED") })
	restarted.mustCall("POST", "/sessions/"+session+"/send", http.StatusOK, map[string]any{"message": "Reply with exactly: B4-PERSISTENT"}, nil)
	awaitPane(t, handle, 3*time.Minute, "post-restart reply in the same pane", func(out string) bool { return strings.Contains(out, "B4-PERSISTENT") })
	reread := getOutcomeAttempt(t, restarted, out, start.ID)
	if reread.ID != start.ID || len(reread.Sessions) != 1 || reread.Sessions[0].SessionID != session {
		t.Fatalf("restart changed lineage: %+v", reread)
	}

	// A provider/session exit is only execution-end evidence; it can never
	// earn success by itself. The one truthful success path here is the
	// plan's approved deterministic check, which the daemon runs itself
	// against the retained bytes during the reconcile classification.
	restarted.mustCall("POST", "/sessions/"+session+"/kill", http.StatusOK, nil, nil)
	// The status flip is visible the instant the liveness half of a reconcile
	// tick commits; the retained-artifact receipt lands in the classification
	// half that immediately follows in the same tick. A direct read can
	// legitimately land between the two, so wait for the row before
	// asserting on it.
	awaitAttemptReceipt(t, dataDir, out, start.ID)
	artifactVersion, artifactDigest := retainedAttemptArtifact(t, dataDir, out, start.ID)
	criterionID := created.Outcome.CurrentRevision.Criteria[0].CriterionID
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		reread = getOutcomeAttempt(t, restarted, out, start.ID)
		if reread.Status == "succeeded" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if reread.Status != "succeeded" {
		t.Fatalf("post-exit status=%s, want success earned by the approved check", reread.Status)
	}
	assertLaunchRows(t, dataDir, out, 1, 1)
	// Success with zero proof rows is exit-fabricated and fatal. Success here
	// must be backed by the daemon's own check observation, bound to this
	// exact Attempt and retained artifact version.
	assertCheckEarnedSuccess(t, restarted, out, criterionID, start.ID, artifactVersion, plan.Plan.WorkUnits[0].ApprovedChecks)

	// The manual journey still runs on top: the owner posts independently
	// recorded evidence and verification bound to this exact Attempt and
	// retained artifact version through the review API. The attempt is
	// already succeeded on the check's observation; these rows must
	// accumulate alongside it.
	restarted.mustCall("POST", "/outcomes/"+out+"/evidence", http.StatusCreated, map[string]any{
		"expectedContractRevision": 1, "contractRevisionId": created.Outcome.CurrentRevision.ID,
		"criterionId": criterionID, "subjectType": "attempt", "subjectId": start.ID,
		"subjectRevision": artifactVersion, "kind": "supporting", "sourceType": "deterministic_check",
		"sourceRef": "grep-durable-txt", "producerType": "tool", "producerRef": "b4-real-daemon-e2e",
		"summary": "retained durable.txt matched PERSISTENT", "contentDigest": artifactDigest, "requestKey": "b4-evidence",
	}, nil)
	pv := getProof(t, restarted, out)
	criterion := criterionByID(pv, criterionID)
	evidenceID := ""
	if criterion != nil {
		for _, item := range criterion.Evidence {
			if item["subjectId"] == start.ID && item["subjectRevision"] == artifactVersion &&
				item["producerRef"] == "b4-real-daemon-e2e" && item["sourceRef"] == "grep-durable-txt" {
				evidenceID, _ = item["id"].(string)
			}
		}
	}
	if evidenceID == "" {
		t.Fatalf("owner-posted evidence missing alongside the check observation: %+v", pv)
	}
	restarted.mustCall("POST", "/outcomes/"+out+"/verifications", http.StatusCreated, map[string]any{
		"expectedContractRevision": 1, "contractRevisionId": created.Outcome.CurrentRevision.ID,
		"criterionId": criterionID, "subjectType": "attempt", "subjectId": start.ID,
		"subjectRevision": artifactVersion, "evidenceItemIds": []string{evidenceID},
		"method": "grep -Fx PERSISTENT durable.txt", "independenceClass": "separate_session", "result": "passed",
		"producerRef": "b4-real-daemon-e2e", "verifierRef": "b4-independent-check", "requestKey": "b4-verification",
	}, nil)
	// The attempt is already succeeded on the check's observation, so the
	// final poll cannot prove this owner verification persisted; re-read the
	// proof and require the exact owner-provenance row bound to the owner
	// evidence selected above.
	pv = getProof(t, restarted, out)
	criterion = criterionByID(pv, criterionID)
	ownerVerified := false
	if criterion != nil {
		for _, verification := range criterion.Verifications {
			if verification["subjectId"] == start.ID && verification["subjectRevision"] == artifactVersion &&
				verification["method"] == "grep -Fx PERSISTENT durable.txt" && verification["independenceClass"] == "separate_session" &&
				verification["result"] == "passed" && verification["producerRef"] == "b4-real-daemon-e2e" &&
				verification["verifierRef"] == "b4-independent-check" && stringListContains(verification["evidenceItemIds"], evidenceID) {
				ownerVerified = true
			}
		}
	}
	if !ownerVerified {
		t.Fatalf("owner-posted verification missing from proof: %+v", pv)
	}
	deadline = time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		reread = getOutcomeAttempt(t, restarted, out, start.ID)
		if reread.Status == "succeeded" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if reread.Status != "succeeded" {
		t.Fatalf("durably proved attempt status=%s", reread.Status)
	}
	assertSucceededEvidence(t, dataDir, out, start.ID, artifactVersion)
}

// assertFrozenBudgetMatchesFixture proves the proposal's frozen WorkUnit
// budget is exactly the test-only fixture policy's default — control-plane
// provenance, not planner output and not an invented production default.
func assertFrozenBudgetMatchesFixture(t *testing.T, dataDir, planRevisionID string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var raw string
	if err := db.QueryRow(`SELECT execution_budget_json FROM work_units WHERE plan_revision_id=?`, planRevisionID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var budget domain.ExecutionBudget
	if err := json.Unmarshal([]byte(raw), &budget); err != nil {
		t.Fatalf("frozen budget does not decode: %v", err)
	}
	if want := e2eAdmissionPolicy(t).Default; budget != want {
		t.Fatalf("frozen WorkUnit budget=%+v, want fixture policy default %+v", budget, want)
	}
}

// awaitAttemptReceipt waits for the classification half of the reconcile
// tick to persist the attempt's artifact receipt. Two reconcile intervals
// (the tick is 15s) cover a retried first pass; anything longer is a real
// retention failure, surfaced by the receipt assertions that follow.
func awaitAttemptReceipt(t *testing.T, dataDir, outcomeID, attemptID string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		var n int
		scanErr := db.QueryRow(`SELECT count(*) FROM attempt_receipts WHERE outcome_id=? AND attempt_id=?`, outcomeID, attemptID).Scan(&n)
		_ = db.Close()
		if scanErr != nil {
			t.Fatal(scanErr)
		}
		if n > 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("attempt receipt never appeared for the reconciled attempt")
}

// retainedAttemptArtifact proves the retained bytes are the steered work
// product. The criterion is "durable.txt contains exactly PERSISTENT" and the
// check is grep -Fx, both of which accept a missing trailing newline, so the
// helper pins the two exact byte forms that satisfy it and returns the digest
// actually retained - attesting a hardcoded digest the steer instruction never
// pinned would be a fixture flake, not a product fact.
func retainedAttemptArtifact(t *testing.T, dataDir, outcomeID, attemptID string) (version, fileDigest string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var state string
	if err := db.QueryRow(`SELECT artifact_version, retention_state FROM attempt_receipts WHERE outcome_id=? AND attempt_id=?`, outcomeID, attemptID).Scan(&version, &state); err != nil {
		t.Fatal(err)
	}
	if version == "" || state != "retained" {
		t.Fatalf("receipt version/state=%q/%q", version, state)
	}
	if err := db.QueryRow(`SELECT content_digest FROM attempt_artifact_files WHERE attempt_id=? AND relative_path='durable.txt' AND change_kind IN ('added','modified','untracked')`, attemptID).Scan(&fileDigest); err != nil {
		t.Fatal(err)
	}
	if fileDigest != digest("PERSISTENT\n") && fileDigest != digest("PERSISTENT") {
		t.Fatalf("durable.txt digest=%q", fileDigest)
	}
	return version, fileDigest
}

type approvedCheckView struct {
	ID          string   `json:"id"`
	CriterionID string   `json:"criterionId"`
	Argv        []string `json:"argv"`
}

// assertCheckEarnedSuccess proves a succeeded attempt earned the success: the
// proof holds the daemon's own approved-check observation - evidence plus a
// deterministic, passed verification, both naming one of the plan's approved
// check argv vectors and bound to this exact Attempt and retained artifact
// version. Success without those rows is exit-fabricated and fails the test.
func assertCheckEarnedSuccess(t *testing.T, d *daemon, outcomeID, criterionID, attemptID, artifactVersion string, checks []approvedCheckView) {
	t.Helper()
	criterion := criterionByID(getProof(t, d, outcomeID), criterionID)
	if criterion == nil {
		t.Fatalf("criterion %s missing from proof", criterionID)
	}
	for _, check := range checks {
		if check.CriterionID != criterionID || len(check.Argv) == 0 {
			continue
		}
		argv := strings.Join(check.Argv, " ")
		for _, item := range criterion.Evidence {
			if item["subjectId"] != attemptID || item["subjectRevision"] != artifactVersion ||
				item["sourceType"] != "deterministic_check" || item["sourceRef"] != argv || item["producerRef"] != attemptID {
				continue
			}
			evidenceID, _ := item["id"].(string)
			if evidenceID == "" {
				continue
			}
			// The verification must name this exact evidence row: a detached
			// passed verification with the same method proves nothing about
			// this check's observation.
			for _, verification := range criterion.Verifications {
				if verification["subjectId"] == attemptID && verification["subjectRevision"] == artifactVersion &&
					verification["method"] == argv && verification["independenceClass"] == "deterministic" &&
					verification["result"] == "passed" && verification["producerRef"] == attemptID &&
					stringListContains(verification["evidenceItemIds"], evidenceID) {
					return
				}
			}
		}
	}
	t.Fatalf("session exit fabricated success: succeeded with no approved-check observation bound to attempt %s artifact %s: %+v", attemptID, artifactVersion, criterion)
}

func stringListContains(v any, want string) bool {
	items, _ := v.([]any)
	for _, item := range items {
		if s, _ := item.(string); s == want {
			return true
		}
	}
	return false
}

func assertSucceededEvidence(t *testing.T, dataDir, outcomeID, attemptID, artifactVersion string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var frozen int
	if err := db.QueryRow(`SELECT count(*) FROM attempt_receipts WHERE outcome_id=? AND attempt_id=? AND artifact_version=? AND frozen_at IS NOT NULL`, outcomeID, attemptID, artifactVersion).Scan(&frozen); err != nil {
		t.Fatal(err)
	}
	var openFence int
	if err := db.QueryRow(`SELECT count(*) FROM attempt_fences WHERE attempt_id=? AND released_at IS NULL`, attemptID).Scan(&openFence); err != nil {
		t.Fatal(err)
	}
	if frozen != 1 || openFence != 0 {
		t.Fatalf("durable success receipt/fence=%d/%d", frozen, openFence)
	}
}

// sessionRuntimeHandle reads the durable tmux handle the TUI session is bound
// to; the pane it names is the persistence layer this proof steers and
// crash-tests through.
func sessionRuntimeHandle(t *testing.T, dataDir, sessionID string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var handle string
	if err := db.QueryRow(`SELECT runtime_handle_id FROM sessions WHERE id=?`, sessionID).Scan(&handle); err != nil {
		t.Fatal(err)
	}
	if handle == "" {
		t.Fatalf("session %s has no runtime handle", sessionID)
	}
	return handle
}

// awaitPane polls the live tmux pane until pred holds of its captured text.
func awaitPane(t *testing.T, handle string, timeout time.Duration, what string, pred func(string) bool) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var out []byte
	for time.Now().Before(deadline) {
		var err error
		out, err = exec.Command("tmux", "capture-pane", "-t", handle, "-p", "-S", "-300").CombinedOutput()
		if err == nil && pred(string(out)) {
			return string(out)
		}
		time.Sleep(2 * time.Second)
	}
	preserveE2EArtifact(t, fmt.Sprintf("pane-%s.txt", handle), out)
	t.Fatalf("timed out waiting for %s in pane %s:\n%s", what, handle, out)
	return ""
}

type b4Attempt struct {
	ID, Status string
	Sessions   []struct{ SessionID, Mode string } `json:"sessions"`
}

func startOutcomeAttempt(t *testing.T, d *daemon, out, plan, unit, key string) b4Attempt {
	t.Helper()
	var env struct {
		Attempt b4Attempt `json:"attempt"`
	}
	d.mustCall("POST", "/outcomes/"+out+"/attempts", http.StatusCreated, map[string]any{"planRevisionId": plan, "workUnitId": unit, "requestKey": key}, &env)
	return env.Attempt
}
func getOutcomeAttempt(t *testing.T, d *daemon, out, attempt string) b4Attempt {
	t.Helper()
	var env struct {
		Attempt b4Attempt `json:"attempt"`
	}
	d.mustCall("GET", "/outcomes/"+out+"/attempts/"+attempt, http.StatusOK, nil, &env)
	return env.Attempt
}
func assertLaunchRows(t *testing.T, dataDir, out string, wantAttempts, wantRefs int) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var attempts, refs int
	if err = db.QueryRow(`SELECT count(*) FROM attempts WHERE outcome_id=?`, out).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM attempt_sessions s JOIN attempts a ON a.id=s.attempt_id WHERE a.outcome_id=?`, out).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if attempts != wantAttempts || refs != wantRefs {
		t.Fatalf("durable rows attempts=%d refs=%d want %d/%d", attempts, refs, wantAttempts, wantRefs)
	}
	if wantRefs > 0 {
		var bad int
		if err = db.QueryRow(`SELECT count(*) FROM attempt_sessions s JOIN attempts a ON a.id=s.attempt_id JOIN sessions x ON x.id=s.session_id WHERE a.outcome_id=? AND (x.session_mode<>'tui' OR x.runtime_handle_id='')`, out).Scan(&bad); err != nil {
			t.Fatal(err)
		}
		if bad != 0 {
			t.Fatalf("%d session bindings lack persistent runtime proof", bad)
		}
	}
}

func TestOutcomeLaunchCutRejectsStaleAuthorizationWithoutCustody(t *testing.T) {
	requireE2E(t)
	dataDir := t.TempDir()
	d := startDaemon(t, dataDir)
	requireReasoningProvider(t, d)
	project := seedProject(t, d, "outcome-stale")
	var created struct {
		Outcome struct {
			ID string `json:"id"`
		} `json:"outcome"`
	}
	d.mustCall("POST", "/projects/"+project+"/outcomes", 201, map[string]any{"title": "stale fixture", "goal": "write durable state", "successCriteria": []string{"durable state file exists"}, "review": "run test -f durable.txt to confirm the state file exists", "authorityCeiling": map[string]any{"readWorkspace": true, "writeWorkspace": true, "executeLocal": true}, "requestKey": "b4-stale-create"}, &created)
	var plan struct {
		Plan struct {
			ID        string `json:"id"`
			WorkUnits []struct {
				ID string `json:"id"`
			} `json:"workUnits"`
		} `json:"plan"`
	}
	d.mustCall("POST", "/outcomes/"+created.Outcome.ID+"/plans", 201, map[string]any{"expectedContractRevision": 1}, &plan)
	if len(plan.Plan.WorkUnits) == 0 {
		t.Fatalf("plans returned 201 with no work units (plan id %q) - a readiness packet is not a plan", plan.Plan.ID)
	}

	// Starting an unapproved plan is an authority rejection, not an implicit
	// approval. In particular it may not acquire custody or create a session.
	status, e := d.callExpectingError("POST", "/outcomes/"+created.Outcome.ID+"/attempts", map[string]any{"planRevisionId": plan.Plan.ID, "workUnitId": plan.Plan.WorkUnits[0].ID, "requestKey": "b4-unauthorized-start"})
	if status != http.StatusConflict || e.Code != "PLAN_NOT_APPROVED" {
		t.Fatalf("unapproved start=%d/%s", status, e.Code)
	}
	assertLaunchRows(t, dataDir, created.Outcome.ID, 0, 0)

	// A stale expected contract revision may not authorize the plan either.
	status, e = d.callExpectingError("POST", "/outcomes/"+created.Outcome.ID+"/plans/"+plan.Plan.ID+"/approval", map[string]any{"expectedContractRevision": 2})
	if status != http.StatusConflict || e.Code != "PLAN_CONTRACT_STALE" {
		t.Fatalf("stale approval=%d/%s", status, e.Code)
	}
	assertLaunchRows(t, dataDir, created.Outcome.ID, 0, 0)
}

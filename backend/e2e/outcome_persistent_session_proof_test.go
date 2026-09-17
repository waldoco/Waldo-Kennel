//go:build !windows

package e2e

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestOutcomeLaunchCutPersistentSessionProof drives only production HTTP and
// daemon restart paths. It is intentionally behind KENNEL_CHAT_E2E: passing
// requires a signed-in real Codex provider, not a fake completion.
func TestOutcomeLaunchCutPersistentSessionProof(t *testing.T) {
	requireE2E(t)
	dataDir := t.TempDir()
	d := startDaemon(t, dataDir)
	project := seedProject(t, d, "outcome-persistent")
	d.mustCall("PATCH", "/settings/session-interface", http.StatusOK, map[string]any{"defaultSessionMode": "chat"}, nil)

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
				ID string `json:"id"`
			} `json:"workUnits"`
		} `json:"plan"`
	}
	d.mustCall("POST", "/outcomes/"+out+"/plans", http.StatusCreated, map[string]any{"expectedContractRevision": 1}, &plan)
	if plan.Plan.Status != "proposed" || len(plan.Plan.WorkUnits) != 1 {
		t.Fatalf("proposal=%+v", plan.Plan)
	}

	// Approval is durable authority, not execution: no Attempt/session may exist.
	d.mustCall("POST", "/outcomes/"+out+"/plans/"+plan.Plan.ID+"/approval", http.StatusOK, map[string]any{"expectedContractRevision": 1}, nil)
	assertLaunchRows(t, dataDir, out, 0, 0)

	start := startOutcomeAttempt(t, d, out, plan.Plan.ID, plan.Plan.WorkUnits[0].ID, "b4-start")
	if start.Status != "running" || len(start.Sessions) != 1 || start.Sessions[0].Mode != "chat" {
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

	// Steer a real active provider turn. The provider turn and execution session
	// both remain the same; this is not interrupt-and-respawn.
	send(t, d, session, "Run this shell command: for i in 1 2 3 4 5 6; do echo b4-$i; sleep 3; done", "b4-long")
	before := d.awaitConversation(session, 2*time.Minute, "governed turn running", func(s snapshot) bool { _, ok := runningTurn(s); return ok })
	running, _ := runningTurn(before)
	var steered steerAccepted
	d.mustCall("POST", "/sessions/"+session+"/conversation/steer", http.StatusAccepted, map[string]any{"text": "Stop that loop and ensure durable.txt contains exactly PERSISTENT, then reply B4-STEERED.", "clientMessageId": "b4-steer"}, &steered)
	if steered.ProviderTurnID != running.ProviderTurnID {
		t.Fatalf("steer moved turn %s -> %s", running.ProviderTurnID, steered.ProviderTurnID)
	}
	d.awaitConversation(session, 4*time.Minute, "steered work complete", func(s snapshot) bool { turn, ok := s.turnByID(running.ID); return ok && terminal(turn.State) })

	// Crash/restart reattaches the same durable provider conversation and keeps
	// the exact Attempt -> AttemptSessionRef -> session lineage.
	convBefore := d.conversation(session)
	d.kill()
	restarted := startDaemon(t, dataDir)
	convAfter := restarted.awaitLiveController(session, 90*time.Second)
	if convAfter.ConversationID != convBefore.ConversationID {
		t.Fatalf("conversation changed %s -> %s", convBefore.ConversationID, convAfter.ConversationID)
	}
	reread := getOutcomeAttempt(t, restarted, out, start.ID)
	if reread.ID != start.ID || len(reread.Sessions) != 1 || reread.Sessions[0].SessionID != session {
		t.Fatalf("restart changed lineage: %+v", reread)
	}

	// A provider/session exit is only execution-end evidence. With no durable
	// criterion proof yet the Attempt must settle reconciled, never succeeded.
	restarted.mustCall("POST", "/sessions/"+session+"/kill", http.StatusOK, nil, nil)
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		reread = getOutcomeAttempt(t, restarted, out, start.ID)
		if reread.Status == "reconciled" {
			break
		}
		if reread.Status == "succeeded" {
			t.Fatal("session exit fabricated success")
		}
		time.Sleep(500 * time.Millisecond)
	}
	if reread.Status != "reconciled" {
		t.Fatalf("post-exit status=%s, want reconciled proof failure", reread.Status)
	}
	assertLaunchRows(t, dataDir, out, 1, 1)
	if proof := getProof(t, restarted, out); proof.Status == "ready_for_acceptance" || proof.Status == "accepted" {
		t.Fatalf("provider exit without criterion evidence yielded proof status %q", proof.Status)
	}

	// Now bind independently recorded evidence and verification to this exact
	// Attempt and retained artifact version. A later reconcile may classify the
	// Attempt, but only from these durable facts.
	artifactVersion := retainedAttemptArtifact(t, dataDir, out, start.ID)
	criterionID := created.Outcome.CurrentRevision.Criteria[0].CriterionID
	restarted.mustCall("POST", "/outcomes/"+out+"/evidence", http.StatusCreated, map[string]any{
		"expectedContractRevision": 1, "contractRevisionId": created.Outcome.CurrentRevision.ID,
		"criterionId": criterionID, "subjectType": "attempt", "subjectId": start.ID,
		"subjectRevision": artifactVersion, "kind": "supporting", "sourceType": "deterministic_check",
		"sourceRef": "grep-durable-txt", "producerType": "tool", "producerRef": "b4-real-daemon-e2e",
		"summary": "retained durable.txt matched PERSISTENT", "contentDigest": digest("PERSISTENT\n"), "requestKey": "b4-evidence",
	}, nil)
	pv := getProof(t, restarted, out)
	criterion := criterionByID(pv, criterionID)
	if criterion == nil || len(criterion.Evidence) != 1 {
		t.Fatalf("attempt evidence missing: %+v", pv)
	}
	evidenceID, _ := criterion.Evidence[0]["id"].(string)
	restarted.mustCall("POST", "/outcomes/"+out+"/verifications", http.StatusCreated, map[string]any{
		"expectedContractRevision": 1, "contractRevisionId": created.Outcome.CurrentRevision.ID,
		"criterionId": criterionID, "subjectType": "attempt", "subjectId": start.ID,
		"subjectRevision": artifactVersion, "evidenceItemIds": []string{evidenceID},
		"method": "grep -Fx PERSISTENT durable.txt", "independenceClass": "separate_session", "result": "passed",
		"producerRef": "b4-real-daemon-e2e", "verifierRef": "b4-independent-check", "requestKey": "b4-verification",
	}, nil)
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

func retainedAttemptArtifact(t *testing.T, dataDir, outcomeID, attemptID string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, state string
	if err := db.QueryRow(`SELECT artifact_version, retention_state FROM attempt_receipts WHERE outcome_id=? AND attempt_id=?`, outcomeID, attemptID).Scan(&version, &state); err != nil {
		t.Fatal(err)
	}
	if version == "" || state != "retained" {
		t.Fatalf("receipt version/state=%q/%q", version, state)
	}
	var fileDigest string
	if err := db.QueryRow(`SELECT content_digest FROM attempt_artifact_files WHERE attempt_id=? AND relative_path='durable.txt' AND change_kind IN ('added','modified','untracked')`, attemptID).Scan(&fileDigest); err != nil {
		t.Fatal(err)
	}
	if fileDigest != digest("PERSISTENT\n") {
		t.Fatalf("durable.txt digest=%q", fileDigest)
	}
	return version
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
		if err = db.QueryRow(`SELECT count(*) FROM attempt_sessions s JOIN attempts a ON a.id=s.attempt_id JOIN sessions x ON x.id=s.session_id WHERE a.outcome_id=? AND (x.session_mode<>'chat' OR x.provider_conversation_id='' OR x.controller_generation='')`, out).Scan(&bad); err != nil {
			t.Fatal(err)
		}
		if bad != 0 {
			t.Fatalf("%d session bindings lack persistent controller proof", bad)
		}
	}
}

func TestOutcomeLaunchCutRejectsStaleAuthorizationWithoutCustody(t *testing.T) {
	requireE2E(t)
	dataDir := t.TempDir()
	d := startDaemon(t, dataDir)
	project := seedProject(t, d, "outcome-stale")
	var created struct {
		Outcome struct {
			ID string `json:"id"`
		} `json:"outcome"`
	}
	d.mustCall("POST", "/projects/"+project+"/outcomes", 201, map[string]any{"title": "stale fixture", "goal": "write durable state", "successCriteria": []string{"state exists"}, "review": "inspect state", "authorityCeiling": map[string]any{"readWorkspace": true, "writeWorkspace": true}, "requestKey": "b4-stale-create"}, &created)
	var plan struct {
		Plan struct {
			ID        string `json:"id"`
			WorkUnits []struct {
				ID string `json:"id"`
			} `json:"workUnits"`
		} `json:"plan"`
	}
	d.mustCall("POST", "/outcomes/"+created.Outcome.ID+"/plans", 201, map[string]any{"expectedContractRevision": 1}, &plan)

	// Starting an unapproved plan is an authority rejection, not an implicit
	// approval. In particular it may not acquire custody or create a session.
	status, e := d.callExpectingError("POST", "/outcomes/"+created.Outcome.ID+"/attempts", map[string]any{"planRevisionId": plan.Plan.ID, "workUnitId": plan.Plan.WorkUnits[0].ID, "requestKey": "b4-unauthorized-start"})
	if status != http.StatusConflict || e.Code != "PLAN_NOT_APPROVED" {
		t.Fatalf("unapproved start=%d/%s", status, e.Code)
	}
	assertLaunchRows(t, dataDir, created.Outcome.ID, 0, 0)

	// A stale expected contract revision may not authorize the plan either.
	status, e = d.callExpectingError("POST", "/outcomes/"+created.Outcome.ID+"/plans/"+plan.Plan.ID+"/approval", map[string]any{"expectedContractRevision": 2})
	if status != http.StatusConflict || !strings.Contains(e.Code, "REVISION") {
		t.Fatalf("stale approval=%d/%s", status, e.Code)
	}
	assertLaunchRows(t, dataDir, created.Outcome.ID, 0, 0)
}

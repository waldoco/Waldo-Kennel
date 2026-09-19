package domain

import "time"

// These ID types are distinct string types so they can't be swapped at a call
// site by accident.
type (
	// SessionID identifies a session.
	SessionID string
	// ProjectID identifies a project.
	ProjectID string
	// IssueID identifies a tracker issue.
	IssueID string
)

// SessionKind distinguishes a worker session from an orchestrator session.
type SessionKind string

// Session kinds.
const (
	KindWorker       SessionKind = "worker"
	KindOrchestrator SessionKind = "orchestrator"
)

// SessionMetadata is the typed, off-status metadata for a session: operational
// handles and seed inputs used by Session Manager and reaper.
type SessionMetadata struct {
	Branch            string `json:"branch,omitempty"`
	WorkspacePath     string `json:"workspacePath,omitempty"`
	WorkspaceRepoPath string `json:"workspaceRepoPath,omitempty"`
	DiffBaseSHA       string `json:"diffBaseSha,omitempty"`
	DiffBaseRef       string `json:"diffBaseRef,omitempty"`
	RuntimeHandleID string                `json:"runtimeHandleId,omitempty"`
	RuntimeLaunchID string                `json:"runtimeLaunchId,omitempty"`
	AgentSessionID  string                `json:"agentSessionId,omitempty"`
	Prompt          string                `json:"prompt,omitempty"`
	// LatestUserPrompt is the latest real user-authored task direction observed
	// for this Kennel session. Internal Kennel coordination messages (for example an
	// agent-switch handoff request) must not replace it.
	LatestUserPrompt string `json:"latestUserPrompt,omitempty"`
	// LatestAssistantUpdate is the latest user-facing assistant update observed
	// before any internal agent-switch coordination turn.
	LatestAssistantUpdate string `json:"latestAssistantUpdate,omitempty"`
	// NativeTranscriptPath is the read-only transcript path for the currently
	// active native agent session when its provider exposes one. Retained
	// provider-specific paths also live on AgentNativeSession records.
	NativeTranscriptPath string `json:"nativeTranscriptPath,omitempty"`
	// ProviderConversationID is the opaque handle a Chat driver needs to resume
	// this session's provider conversation after a restart (a Codex thread id
	// today). Normally empty for TUI sessions. It remains a distinct field from
	// AgentSessionID because most harnesses do not prove those protocol identities
	// interchangeable; the interface-transition coordinator copies one value into
	// both only after the adapter explicitly declares that equivalence.
	ProviderConversationID string `json:"providerConversationId,omitempty"`
	// ControllerGeneration is rotated each time a Chat controller is started for
	// this session. Events carrying an older generation are rejected, so a
	// controller that is dying cannot mutate the session that replaced it. Not
	// the same fence as RuntimeLaunchID, which covers terminal runtimes.
	ControllerGeneration string `json:"controllerGeneration,omitempty"`
	// PreviewURL is the browser preview target the desktop app opens for this
	// session. Set via `kennel preview` (POST /sessions/{id}/preview); persisted so
	// it survives a daemon restart. Empty means no preview has been requested.
	PreviewURL string `json:"previewUrl,omitempty"`
	// PreviewRevision is a monotonic counter bumped on every `kennel preview` call,
	// even when PreviewURL is unchanged. The desktop browser panel keys
	// navigation on it so a repeated `kennel preview <same-url>` still refreshes.
	PreviewRevision int64 `json:"previewRevision,omitempty"`
	// BrowserCapabilityVerifier is a one-way verifier for the random browser
	// capability held by this session's worker process. The bearer token itself
	// is never persisted, so reading the database cannot grant access to another
	// session. Keeping the verifier durable lets a surviving worker authenticate
	// after the desktop app or daemon restarts.
	BrowserCapabilityVerifier string `json:"-"`
	// GovernedExecutionPolicyDigest marks a session created for an admitted
	// Attempt. It is only a recovery gate: the Attempt session-reference
	// admission snapshot remains the authority for the frozen binding and
	// policy. A non-empty marker with missing or invalid evidence must block
	// recovery rather than inherit mutable Project preferences.
	GovernedExecutionPolicyDigest string `json:"governedExecutionPolicyDigest,omitempty"`
	// SupervisorCapabilityVerifier authenticates completion reports from the
	// exact supervised worker generation. The bearer capability is never
	// persisted or exposed to the provider process.
	SupervisorCapabilityVerifier string `json:"-"`
	// SupervisedProcessExitCode and SupervisedProcessExitReason are trusted
	// supervisor observations for the current RuntimeLaunchID. Nil means no
	// authenticated exit report was recorded; zero is a successful process exit,
	// not Outcome success or owner Acceptance.
	SupervisedProcessExitCode   *int   `json:"-"`
	SupervisedProcessExitReason string `json:"-"`
}

// SupervisedExitReasonExited is the only reason value a successful supervised
// process exit may report.
const SupervisedExitReasonExited = "exited"

// SupervisedExitReasonOwnerKilled marks a proven owner-initiated termination
// of a governed session. The daemon itself records it — the supervisor never
// reports it and the wire never accepts it — so an intentional, non-success
// end stays distinguishable from a provider crash: the attempt settles
// reconciled (result unclassified), never failed. It is written exactly
// once, atomically with the session's termination, by the owner-kill path
// only, and only when no authenticated exit report exists — observed crash
// facts always win. The same write closes the supervisor reporting channel
// for the dead launch generation, so no late report can overwrite the origin
// after the attempt settles.
const SupervisedExitReasonOwnerKilled = "owner_killed"

// SupervisedExitFactsConsistent reports whether an (exitCode, reason) pair is
// internally consistent: a zero exit code and the "exited" reason must agree
// in both directions. This rejects "zero exit code plus a failure reason" and
// "exited reason plus a missing or nonzero exit code" as the same kind of
// contradiction, without needing to enumerate every non-exited reason string.
func SupervisedExitFactsConsistent(exitCode *int, reason string) bool {
	zero := exitCode != nil && *exitCode == 0
	exited := reason == SupervisedExitReasonExited
	return zero == exited
}

// SupervisedExitSucceeded is the single definition of a successful supervised
// process exit: exit code zero AND reason "exited", nothing else. Callers
// must use this instead of re-deriving success from either fact alone, so a
// contradictory or partial report (nil/nonzero code, a non-exited reason, or
// a mismatched combination) never reads as success.
func SupervisedExitSucceeded(exitCode *int, reason string) bool {
	return exitCode != nil && *exitCode == 0 && reason == SupervisedExitReasonExited
}

// SessionRecord is the persistence shape. It intentionally stores only durable
// facts: identity, agent harness, activity_state, is_terminated, and operational
// metadata. The user-facing Status is derived from these facts plus PR facts.
type SessionRecord struct {
	ID        SessionID    `json:"id"`
	ProjectID ProjectID    `json:"projectId"`
	IssueID   IssueID      `json:"issueId,omitempty"`
	Kind      SessionKind  `json:"kind"`
	Harness   AgentHarness `json:"harness,omitempty"`
	// ReviewerHarness is this session's preferred reviewer. Empty delegates to
	// the project configuration.
	ReviewerHarness   ReviewerHarness `json:"reviewerHarness,omitempty" enum:"claude-code,codex,copilot,cursor,kilocode,opencode,kiro,pi,qwen,agy,continue,goose,vibe,devin,droid,kimi,kimchi,muse,amp,aider,grok,crush,auggie,cline,autohand"`
	AutoReviewEnabled bool            `json:"autoReviewEnabled"`
	DisplayName       string          `json:"displayName,omitempty"`
	// Mode is the session's currently committed conversation controller. Every
	// send, restore, kill, and reaper decision dispatches from it. Only the
	// durable interface-transition coordinator may change it; the daemon default
	// never changes an existing session. Rows written before Chat mode existed
	// read back as SessionModeTUI.
	Mode     SessionMode `json:"mode" enum:"chat,tui"`
	Activity Activity    `json:"activity"`
	// FirstSignalAt is when the FIRST agent hook callback arrived for the
	// current spawn/restore: raw signal receipt, independent of the derived
	// activity state. Zero means no hook has ever reported, which deriveStatus
	// surfaces as StatusNoSignal after a grace period. Internal fact, not part
	// of the API read model.
	FirstSignalAt time.Time `json:"-"`
	IsTerminated  bool      `json:"isTerminated"`
	// TerminateOnPRMerge is a user-controlled lifecycle policy. When enabled,
	// completing the session's PR set through a merge tears down the session.
	TerminateOnPRMerge bool            `json:"terminateOnPrMerge"`
	AutoInjectReview   bool            `json:"autoInjectReview"`
	AutoInjectCI       bool            `json:"autoInjectCI"`
	Metadata           SessionMetadata `json:"-"`
	// CleanupGeneration is a monotonic counter bumped each time the session is
	// un-terminated (spawn/restore). The terminal-resource reconciler stamps its
	// durable cleanup facts with the generation they were written for so a
	// finalize started under an earlier terminal episode cannot satisfy a later
	// one. Internal fact, not part of the API read model.
	CleanupGeneration int64      `json:"-"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	IsPinned          bool       `json:"isPinned"`
	PinnedAt          *time.Time `json:"pinnedAt,omitempty"`
}

// Session is the read-model returned across the API boundary: a SessionRecord
// plus derived display facts. Neither Status nor SCMStatus is persisted.
type Session struct {
	SessionRecord
	Status            SessionStatus `json:"status" enum:"working,pr_open,draft,ci_failed,review_pending,changes_requested,approved,mergeable,merged,needs_input,exited,idle,terminated,no_signal"`
	SCMStatus         SessionStatus `json:"scmStatus,omitempty" enum:"pr_open,draft,ci_failed,review_pending,changes_requested,approved,mergeable,merged"`
	TerminalHandleID  string        `json:"terminalHandleId,omitempty"`
	ActiveAgentSwitch *AgentSwitch  `json:"-"`
	// PRs are the session's attributed pull requests (one session can own many).
	// They feed status derivation and are surfaced on the API read model. Not
	// serialized here: the HTTP boundary maps them to the curated wire shape.
	PRs []PRFacts `json:"-"`
}

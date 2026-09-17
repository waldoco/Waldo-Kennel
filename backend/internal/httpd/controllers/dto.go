package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devimport"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	agentsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/agent"
	projectsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/project"
	sessionsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/session"

	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// HTTP response envelopes for the projects surface — the SINGLE definition of
// each wire shape. The handlers encode these (envelope.WriteJSON), and
// apispec.Build reflects these same types into openapi.yaml, so the served
// contract and the generated spec can't disagree. The request side needs no
// wrappers: handlers decode the body straight into the project commands
// (projectsvc.AddInput), which apispec also reflects.

// ProjectIDParam is the {id} path parameter shared by the /projects/{id}
// routes. Handlers read it via chi.URLParam (see projectID); it is declared here
// so every wire input/output shape has one home, and apispec.Build reflects it
// as the path parameter.
type ProjectIDParam struct {
	ID string `path:"id" description:"Project identifier (registry key)."`
}

// AgentIDParam is the {agent} path parameter for one-agent catalog probes.
type AgentIDParam struct {
	Agent string `path:"agent" description:"Agent adapter identifier."`
}

// ListProjectsResponse is the body of GET /api/v1/projects.
type ListProjectsResponse struct {
	Projects []projectsvc.Summary `json:"projects"`
}

// ProjectResponse is the { project } body shared by POST /projects (201).
type ProjectResponse struct {
	Project projectsvc.Project `json:"project"`
}

// GetProjectResponse is the { status, project } body of GET /projects/{id},
// where project is oneOf Project|Degraded discriminated by status.
type GetProjectResponse struct {
	Status  string            `json:"status" enum:"ok,degraded"`
	Project ProjectOrDegraded `json:"project"`
}

// ProjectOrDegraded is the discriminated `project` field: exactly one of
// Project/Degraded is set. It marshals as whichever is present (so the handler
// emits the right object) and exposes the oneOf variants to the spec reflector
// (so apispec.Build emits `oneOf: [Project, Degraded]`) — one type, both jobs.
type ProjectOrDegraded struct {
	Project  *projectsvc.Project
	Degraded *projectsvc.Degraded
}

// MarshalJSON encodes whichever variant is set (Project or Degraded).
func (p ProjectOrDegraded) MarshalJSON() ([]byte, error) {
	switch {
	case p.Degraded != nil:
		return json.Marshal(p.Degraded)
	case p.Project != nil:
		return json.Marshal(p.Project)
	default:
		// Unreachable in practice: the handler validates the GetResult via
		// newGetProjectResponse and writes a 500 before committing the 200
		// status, so this never encodes. Kept as a last-resort backstop —
		// erroring is still better than emitting a contract-breaking `null`,
		// though by here the status is already sent, so the real guard is
		// upstream.
		return nil, errEmptyProjectOrDegraded
	}
}

// errEmptyProjectOrDegraded marks a GetResult that set neither variant — a
// Manager-contract violation. newGetProjectResponse returns it so the handler
// can map it to a 500 before any response bytes are written.
var errEmptyProjectOrDegraded = errors.New("controllers: GetResult has neither Project nor Degraded set")

// ResolvedMissionRolesResponse is the { roles } body of
// GET /projects/{id}/resolved-mission-roles: the daemon-resolved Mission-role
// proposal for one project (stored preferences enriched with live adapter
// admission). Advisory for future Missions only; it never rewrites historical
// sessions or approved Plans.
type ResolvedMissionRolesResponse struct {
	Roles domain.ResolvedMissionRoles `json:"roles"`
}

// JSONSchemaOneOf is read by swaggest's reflector (apispec.Build) to emit the
// oneOf for this field; it is not used at runtime.
func (ProjectOrDegraded) JSONSchemaOneOf() []interface{} {
	return []interface{}{projectsvc.Project{}, projectsvc.Degraded{}}
}

// newGetProjectResponse maps the internal GetResult onto the wire envelope —
// the explicit project→httpd boundary the result type exists for. It errors
// when the result sets neither variant, so the handler can return a clean 500
// BEFORE writing the 200 status rather than flushing a truncated body.
func newGetProjectResponse(res projectsvc.GetResult) (GetProjectResponse, error) {
	if res.Project == nil && res.Degraded == nil {
		return GetProjectResponse{}, errEmptyProjectOrDegraded
	}
	return GetProjectResponse{
		Status:  res.Status,
		Project: ProjectOrDegraded{Project: res.Project, Degraded: res.Degraded},
	}, nil
}

// SessionIDParam is the {sessionId} path parameter shared by session routes.
type SessionIDParam struct {
	SessionID string `path:"sessionId" description:"Session identifier, e.g. project-1."`
}

// AgentSwitchIDParam is the {switchId} path parameter for one durable switch saga.
type AgentSwitchIDParam struct {
	SwitchID string `path:"switchId" description:"Durable agent-switch identifier."`
}

// ListSessionsQuery is the query string accepted by GET /api/v1/sessions.
type ListSessionsQuery struct {
	Project          string `query:"project,omitempty" description:"Project id filter."`
	Active           *bool  `query:"active,omitempty" description:"When true, return non-terminated sessions; when false, return terminated sessions."`
	OrchestratorOnly *bool  `query:"orchestratorOnly,omitempty" description:"When true, return only orchestrator sessions."`
	Fresh            *bool  `query:"fresh,omitempty" description:"When true, return only fresh non-terminated sessions."`
}

// CleanupSessionsQuery is the query string accepted by POST /api/v1/sessions/cleanup.
type CleanupSessionsQuery struct {
	Project string `query:"project,omitempty" description:"Project id filter. When omitted, clean terminated sessions across all projects."`
}

// WorkspaceFileQuery is the query string accepted by GET /api/v1/sessions/{sessionId}/workspace/file.
type WorkspaceFileQuery struct {
	Path string `query:"path" description:"Session-worktree-relative file path."`
}

// SessionView is the session wire shape: the domain read model plus the
// display-safe branch name and the session's attributed pull requests in the
// curated SessionPRFacts shape. One session can own many PRs (e.g. a stack), so
// prs is a list. The embedded domain.Session.Metadata and domain.Session.PRs
// fields are json:"-"; these curated fields are what serialize.
type SessionView struct {
	domain.Session
	Branch string `json:"branch,omitempty"`
	// PreviewURL is the browser preview target the desktop app opens for this
	// session, set via POST /sessions/{sessionId}/preview. Empty (omitted) when
	// no preview has been requested. Pulled from the json:"-" domain Metadata.
	PreviewURL string `json:"previewUrl,omitempty"`
	// PreviewRevision bumps on every `kennel preview` call (even when previewUrl is
	// unchanged) so the desktop browser panel can re-navigate / refresh on a
	// repeated preview of the same target. Pulled from the json:"-" domain
	// Metadata.
	PreviewRevision   int64            `json:"previewRevision,omitempty"`
	PRs               []SessionPRFacts `json:"prs"`
	ActiveAgentSwitch *AgentSwitchView `json:"activeAgentSwitch,omitempty"`
}

// ListSessionsResponse is the body of GET /api/v1/sessions.
type ListSessionsResponse struct {
	Sessions []SessionView `json:"sessions"`
}

// SpawnSessionRequest is the body of POST /api/v1/sessions.
type SpawnSessionRequest struct {
	ProjectID       domain.ProjectID       `json:"projectId"`
	IssueID         domain.IssueID         `json:"issueId,omitempty"`
	TrackerProvider domain.TrackerProvider `json:"trackerProvider,omitempty" enum:"github,gitlab"`
	Kind            domain.SessionKind     `json:"kind,omitempty" enum:"worker,orchestrator"`
	Harness         domain.AgentHarness    `json:"harness,omitempty" enum:"codex,deepseek-harness"`
	Branch          string                 `json:"branch,omitempty"`
	// Mode picks the conversation controller: chat talks to the agent over a
	// structured connection, tui opens the agent's native terminal interface.
	// Omitted resolves to the daemon default (tui), which is why an upgrade
	// changes nothing. Compatible sessions may later switch through the durable
	// interface-transition endpoint; the default never mutates existing sessions
	// automatically. An unsupported explicit request fails rather than quietly
	// producing the other kind of session.
	Mode   domain.SessionMode `json:"mode,omitempty" enum:"chat,tui"`
	Prompt string             `json:"prompt,omitempty" maxLength:"4096"`

	// DisplayName is the sidebar label for the session, capped at 20 characters.
	// `kennel spawn --name` always sets it; other clients (e.g. the desktop new-task
	// dialog) may omit it and fall back to the session id in the read model.
	DisplayName string `json:"displayName,omitempty" maxLength:"20"`
	// Attachments are files pasted or dropped into the task brief. Each carries
	// its bytes as standard base64 (no data: URL prefix). The daemon writes them
	// into the session worktree and appends path references to the prompt.
	Attachments []AttachmentInput `json:"attachments,omitempty"`
}

// AttachmentInput is one file attached to a spawn, delegate, stage, or send
// request.
type AttachmentInput struct {
	// MimeType is the browser-reported content type (e.g. "image/png"). Used to
	// derive the on-disk file extension. Explicitly blocked types are rejected.
	MimeType string `json:"mimeType,omitempty"`
	// Data is the raw file bytes, standard base64-encoded, without any
	// "data:...;base64," prefix.
	Data string `json:"data"`
}

// SessionResponse is the { session } body shared by session reads and updates.
type SessionResponse struct {
	Session SessionView `json:"session"`
}

// SpawnSessionResponse includes ephemeral measurements of the final assembled
// prompt texts. The fields are required so a measured zero remains distinct
// from a response that never measured prompt sizes.
type SpawnSessionResponse struct {
	Session           SessionView `json:"session"`
	PromptBytes       int         `json:"promptBytes"`
	SystemPromptBytes int         `json:"systemPromptBytes"`
}

// SwitchAgentRequest is the body of POST /api/v1/sessions/{sessionId}/switch-agent.
type SwitchAgentRequest struct {
	TargetHarness  domain.AgentHarness `json:"targetHarness" enum:"codex" description:"Agent harness to continue the logical Kennel session with. Only continuation-capable harnesses are admitted; worker-only harnesses fail closed."`
	Model          string              `json:"model,omitempty" maxLength:"256" description:"Optional model override for the target agent launch or resume."`
	IdempotencyKey string              `json:"idempotencyKey,omitempty" maxLength:"128" description:"Optional retry key. Reusing it with a different request is rejected."`
}

// AgentSwitchView is the deliberately small public projection of a durable
// switch saga. Provider context, local artifact paths, retry keys, generation
// fences, and raw failure details remain daemon-private.
type AgentSwitchView struct {
	ID                      domain.AgentSwitchID                     `json:"id"`
	SessionID               domain.SessionID                         `json:"sessionId"`
	FromHarness             domain.AgentHarness                      `json:"fromHarness"`
	TargetHarness           domain.AgentHarness                      `json:"targetHarness"`
	TargetStartMode         domain.AgentSwitchTargetStartMode        `json:"targetStartMode,omitempty" enum:"fresh,resumed"`
	State                   domain.AgentSwitchState                  `json:"state" enum:"preparing_handoff,stopping_source,source_stopped,starting_target,target_ready,delivering_context,completed,failed"`
	AgentHandoffStatus      domain.AgentHandoffStatus                `json:"agentHandoffStatus" enum:"not_attempted,requested,received,unavailable,timed_out,failed,rejected"`
	SemanticHandoffIncluded bool                                     `json:"semanticHandoffIncluded"`
	SourceTranscriptStatus  domain.AgentSwitchSourceTranscriptStatus `json:"sourceTranscriptStatus,omitempty" enum:"not_attempted,available,unavailable"`
	ErrorCode               domain.AgentSwitchErrorCode              `json:"errorCode,omitempty" enum:"daemon_restart_pre_stop,daemon_restart_post_stop,daemon_restart_unrecoverable_target,daemon_restart_before_delivery,delivery_unconfirmed,source_session_terminated,source_stop_unconfirmed,target_binary_missing,target_agent_unauthorized,target_start_unconfirmed,source_restore_unconfirmed,request_cancelled,source_blocked,failed_pre_stop,failed_post_stop,target_ready_failed,delivery_failed,switch_failed"`
	RequestedAt             time.Time                                `json:"requestedAt"`
	UpdatedAt               time.Time                                `json:"updatedAt"`
}

// AgentSwitchResponse is the body returned by switch creation, reads, and
// generation-fenced handoff submission.
type AgentSwitchResponse struct {
	Switch AgentSwitchView `json:"switch"`
}

// ListAgentSwitchesResponse is the body of
// GET /api/v1/sessions/{sessionId}/agent-switches.
type ListAgentSwitchesResponse struct {
	Switches []AgentSwitchView `json:"switches"`
}

// SubmitAgentHandoffRequest is the body of
// POST /api/v1/sessions/{sessionId}/agent-switches/{switchId}/handoff.
// Handoff remains provider-neutral JSON and is accepted only from the source
// generation recorded by the durable switch.
type SubmitAgentHandoffRequest struct {
	SourceGenerationID domain.AgentGenerationID `json:"sourceGenerationId" description:"Source invocation generation that authored this handoff."`
	// RawMessage deliberately preserves the source object's original token
	// stream. Decoding into a map here would silently collapse duplicate keys
	// before the semantic validator can reject them.
	Handoff json.RawMessage `json:"handoff" description:"Structured, source-agent-authored handoff enrichment."`
}

// StageSessionAttachmentsRequest attaches files to a session that is already
// running, for a caller that will name the returned paths in its next message.
type StageSessionAttachmentsRequest struct {
	// Attachments each carry their bytes as standard base64 (no data: URL prefix).
	// The same count, size, and blocked-type rules as spawn apply.
	Attachments []AttachmentInput `json:"attachments"`
}

// StageSessionAttachmentsResponse is where the files were written.
type StageSessionAttachmentsResponse struct {
	SessionID domain.SessionID `json:"sessionId"`
	// Paths are worktree-relative and forward-slashed, in the order submitted. They
	// are what the agent can actually open, so a client must send these verbatim
	// rather than a display form of them.
	Paths []string `json:"paths"`
}

// ListWorkspaceFilesResponse is the body of GET /api/v1/sessions/{sessionId}/workspace/files.
type ListWorkspaceFilesResponse struct {
	SessionID      domain.SessionID                `json:"sessionId"`
	CompareBaseSHA string                          `json:"compareBaseSha,omitempty"`
	CompareBaseRef string                          `json:"compareBaseRef,omitempty"`
	CompareMode    sessionsvc.WorkspaceCompareMode `json:"compareMode,omitempty" enum:"base,head_fallback"`
	Files          []WorkspaceFileSummary          `json:"files"`
	Truncated      bool                            `json:"truncated"`
}

// WorkspaceFileSummary is one file row in the session workspace browser.
type WorkspaceFileSummary struct {
	Path         string                         `json:"path"`
	PreviousPath string                         `json:"previousPath,omitempty"`
	Status       sessionsvc.WorkspaceFileStatus `json:"status" enum:"unmodified,modified,added,deleted,renamed"`
	Additions    int                            `json:"additions"`
	Deletions    int                            `json:"deletions"`
	Size         int64                          `json:"size"`
	Binary       bool                           `json:"binary"`
}

// WorkspaceFileResponse is the body of GET /api/v1/sessions/{sessionId}/workspace/file.
type WorkspaceFileResponse struct {
	SessionID        domain.SessionID                `json:"sessionId"`
	Path             string                          `json:"path"`
	PreviousPath     string                          `json:"previousPath,omitempty"`
	Status           sessionsvc.WorkspaceFileStatus  `json:"status" enum:"unmodified,modified,added,deleted,renamed"`
	Additions        int                             `json:"additions"`
	Deletions        int                             `json:"deletions"`
	Size             int64                           `json:"size"`
	Binary           bool                            `json:"binary"`
	Deleted          bool                            `json:"deleted"`
	Content          string                          `json:"content"`
	ContentTruncated bool                            `json:"contentTruncated"`
	Diff             string                          `json:"diff"`
	DiffTruncated    bool                            `json:"diffTruncated"`
	CompareBaseSHA   string                          `json:"compareBaseSha,omitempty"`
	CompareBaseRef   string                          `json:"compareBaseRef,omitempty"`
	CompareMode      sessionsvc.WorkspaceCompareMode `json:"compareMode,omitempty" enum:"base,head_fallback"`
}

// SessionPreviewResponse is the body of GET /api/v1/sessions/{sessionId}/preview.
type SessionPreviewResponse struct {
	SessionID  domain.SessionID `json:"sessionId"`
	PreviewURL string           `json:"previewUrl,omitempty"`
	Entry      string           `json:"entry,omitempty"`
}

// RenameSessionRequest is the body of PATCH /api/v1/sessions/{sessionId}.
type RenameSessionRequest struct {
	DisplayName string `json:"displayName" minLength:"1"`
}

// SetSessionReviewerRequest sets the durable reviewer preference for a session.
// Empty clears the preference and falls back to project configuration.
type SetSessionReviewerRequest struct {
	Harness domain.ReviewerHarness `json:"harness,omitempty" enum:"codex"`
}

// SetSessionAutoReviewRequest configures daemon-side review automation.
type SetSessionAutoReviewRequest struct {
	Enabled        bool `json:"enabled"`
	enabledPresent bool
}

// UnmarshalJSON distinguishes an omitted required boolean from an explicit
// false without making the generated API schema nullable.
func (r *SetSessionAutoReviewRequest) UnmarshalJSON(data []byte) error {
	var wire struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	r.enabledPresent = wire.Enabled != nil
	if wire.Enabled != nil {
		r.Enabled = *wire.Enabled
	}
	return nil
}

// SetSessionPreviewRequest is the body of POST /api/v1/sessions/{sessionId}/preview.
// An empty url asks the daemon to autodetect a static entry point in the
// session workspace; a non-empty url is used verbatim as the preview target.
type SetSessionPreviewRequest struct {
	URL string `json:"url,omitempty" description:"Preview target URL. When empty, the daemon autodetects a static entry point in the session workspace."`
}

// StartPreviewServerRequest selects one named entry from .kennel/launch.json. The
// name may be omitted when the file contains exactly one configuration.
type StartPreviewServerRequest struct {
	Configuration string `json:"configuration,omitempty" description:"Named preview configuration. Optional when exactly one configuration exists."`
}

// PreviewServerStatusResponse reports the deterministic server Kennel owns for one
// session. Logs are bounded to the latest lines and never contain global
// process or port discovery.
type PreviewServerStatusResponse struct {
	SessionID     domain.SessionID `json:"sessionId"`
	State         string           `json:"state" enum:"stopped,starting,ready,stopping,failed"`
	Configuration string           `json:"configuration,omitempty"`
	TargetKind    string           `json:"targetKind,omitempty" enum:"app,api"`
	URL           string           `json:"url,omitempty"`
	Port          int              `json:"port,omitempty"`
	StartedAt     time.Time        `json:"startedAt,omitempty"`
	Error         string           `json:"error,omitempty"`
	Logs          []string         `json:"logs"`
}

// BrowserStatusQuery selects the session whose logical browser is inspected.
type BrowserStatusQuery struct {
	SessionID domain.SessionID `query:"sessionId" description:"Kennel session identifier."`
}

// BrowserCapabilityHeader proves that the caller owns the target session.
type BrowserCapabilityHeader struct {
	Capability string `header:"X-Kennel-Browser-Capability" description:"Opaque browser capability injected into the owning Kennel worker."`
}

// BrowserStatusResponse reports whether the desktop-owned browser transport is
// ready. A connected runtime can create the session target while its panel is
// hidden; panel visibility is intentionally not part of this state.
type BrowserStatusResponse struct {
	SessionID   domain.SessionID `json:"sessionId"`
	Connected   bool             `json:"connected"`
	ConnectedAt time.Time        `json:"connectedAt,omitempty"`
	Transport   string           `json:"transport"`
}

// BrowserCommandRequest is the stable daemon-facing command envelope. Action
// arguments remain action-specific JSON so new target-scoped operations do not
// require a new transport or Electron IPC surface.
type BrowserCommandRequest struct {
	SessionID domain.SessionID       `json:"sessionId"`
	Action    string                 `json:"action"`
	Args      map[string]interface{} `json:"args,omitempty"`
}

// BrowserCommandResponse returns a correlated result from the browser runtime.
type BrowserCommandResponse struct {
	RequestID string           `json:"requestId"`
	SessionID domain.SessionID `json:"sessionId"`
	Action    string           `json:"action"`
	Result    interface{}      `json:"result"`
}

// SetSessionMergePolicyRequest is the body of PATCH /api/v1/sessions/{sessionId}/merge-policy.
type SetSessionMergePolicyRequest struct {
	TerminateOnPRMerge bool `json:"terminateOnPrMerge"`
}

// RenameSessionResponse is the body of PATCH /api/v1/sessions/{sessionId}.
type RenameSessionResponse struct {
	OK          bool             `json:"ok"`
	SessionID   domain.SessionID `json:"sessionId"`
	DisplayName string           `json:"displayName"`
}

// SetSessionMergePolicyResponse is the body of PATCH /api/v1/sessions/{sessionId}/merge-policy.
type SetSessionMergePolicyResponse struct {
	OK                 bool             `json:"ok"`
	SessionID          domain.SessionID `json:"sessionId"`
	TerminateOnPRMerge bool             `json:"terminateOnPrMerge"`
	Session            SessionView      `json:"session"`
}

// SetSessionAutoInjectReviewRequest is the body of PATCH /api/v1/sessions/{sessionId}/auto-inject-review.
type SetSessionAutoInjectReviewRequest struct {
	AutoInjectReview bool `json:"autoInjectReview"`
}

// SetSessionAutoInjectReviewResponse is the response from updating a session's automatic review-injection policy.
type SetSessionAutoInjectReviewResponse struct {
	OK               bool             `json:"ok"`
	SessionID        domain.SessionID `json:"sessionId"`
	AutoInjectReview bool             `json:"autoInjectReview"`
	Session          SessionView      `json:"session"`
}

// SetSessionAutoInjectCIRequest updates the default automatic CI delivery
// policy captured by PRs created after the change.
type SetSessionAutoInjectCIRequest struct {
	AutoInjectCI bool `json:"autoInjectCI"`
}

// SetSessionAutoInjectCIResponse confirms the persisted session default.
type SetSessionAutoInjectCIResponse struct {
	OK           bool             `json:"ok"`
	SessionID    domain.SessionID `json:"sessionId"`
	AutoInjectCI bool             `json:"autoInjectCI"`
	Session      SessionView      `json:"session"`
}

// RestoreSessionResponse is the body of POST /api/v1/sessions/{sessionId}/restore.
type RestoreSessionResponse struct {
	OK          bool                       `json:"ok"`
	SessionID   domain.SessionID           `json:"sessionId"`
	RestoreMode sessionsvc.RestoreModeView `json:"restoreMode" enum:"native,saved_prompt,fresh"`
	Session     SessionView                `json:"session"`
}

// ResumeAgentResponse is the body of POST /api/v1/sessions/{sessionId}/resume-agent.
type ResumeAgentResponse struct {
	OK         bool                       `json:"ok"`
	SessionID  domain.SessionID           `json:"sessionId"`
	ResumeMode sessionsvc.RestoreModeView `json:"resumeMode" enum:"native,saved_prompt,fresh"`
	Session    SessionView                `json:"session"`
}

// StartSessionInterfaceTransitionRequest is the body of POST
// /api/v1/sessions/{sessionId}/interface-transition.
type StartSessionInterfaceTransitionRequest struct {
	TargetMode domain.SessionMode                      `json:"targetMode" enum:"chat,tui"`
	Policy     domain.SessionInterfaceTransitionPolicy `json:"policy" enum:"drain,interrupt"`
}

// SessionInterfaceTransitionView is the client-facing progress record. The
// provider-native conversation id is intentionally not exposed: clients need
// controller state, not an adapter implementation detail.
type SessionInterfaceTransitionView struct {
	ID          string                                  `json:"id"`
	SessionID   domain.SessionID                        `json:"sessionId"`
	SourceMode  domain.SessionMode                      `json:"sourceMode" enum:"chat,tui"`
	TargetMode  domain.SessionMode                      `json:"targetMode" enum:"chat,tui"`
	Policy      domain.SessionInterfaceTransitionPolicy `json:"policy" enum:"drain,interrupt"`
	Phase       domain.SessionInterfaceTransitionPhase  `json:"phase" enum:"requested,preflighting,draining,source_stopping,source_stopped,target_starting,activating,completed,failed,cancelled,recovery_required"`
	ErrorCode   string                                  `json:"errorCode,omitempty"`
	ErrorDetail string                                  `json:"errorDetail,omitempty"`
	CreatedAt   time.Time                               `json:"createdAt"`
	UpdatedAt   time.Time                               `json:"updatedAt"`
	CompletedAt *time.Time                              `json:"completedAt,omitempty"`
}

// SessionInterfaceTransitionStatusResponse is the body of GET
// /api/v1/sessions/{sessionId}/interface-transition.
type SessionInterfaceTransitionStatusResponse struct {
	Supported  bool                            `json:"supported"`
	TargetMode domain.SessionMode              `json:"targetMode" enum:"chat,tui"`
	ReasonCode string                          `json:"reasonCode,omitempty"`
	Reason     string                          `json:"reason,omitempty"`
	Transition *SessionInterfaceTransitionView `json:"transition,omitempty"`
}

// StartSessionInterfaceTransitionResponse acknowledges an asynchronous handoff.
type StartSessionInterfaceTransitionResponse struct {
	OK         bool                           `json:"ok"`
	SessionID  domain.SessionID               `json:"sessionId"`
	Transition SessionInterfaceTransitionView `json:"transition"`
}

// CancelSessionInterfaceTransitionResponse acknowledges cancellation.
type CancelSessionInterfaceTransitionResponse struct {
	OK        bool             `json:"ok"`
	SessionID domain.SessionID `json:"sessionId"`
}

// KillSessionResponse is the body of POST /api/v1/sessions/{sessionId}/kill.
type KillSessionResponse struct {
	OK        bool             `json:"ok"`
	SessionID domain.SessionID `json:"sessionId"`
	Freed     bool             `json:"freed,omitempty"`
}

// RollbackSessionResponse is the body of POST /api/v1/sessions/{sessionId}/rollback.
// Exactly one of Deleted/Killed is true on a successful rollback; both are
// false when the session was already absent or already terminated (benign).
type RollbackSessionResponse struct {
	OK        bool             `json:"ok"`
	SessionID domain.SessionID `json:"sessionId"`
	Deleted   bool             `json:"deleted,omitempty"`
	Killed    bool             `json:"killed,omitempty"`
}

// CleanupSkippedSession is one terminal session whose workspace cleanup
// preserved rather than reclaimed (a dirty worktree is never force-deleted),
// with the user-facing reason.
type CleanupSkippedSession struct {
	SessionID domain.SessionID `json:"sessionId"`
	Reason    string           `json:"reason"`
}

// CleanupSessionsResponse is the body of POST /api/v1/sessions/cleanup.
type CleanupSessionsResponse struct {
	OK      bool                    `json:"ok"`
	Cleaned []domain.SessionID      `json:"cleaned"`
	Skipped []CleanupSkippedSession `json:"skipped"`
}

// SendSessionMessageRequest is the body of POST /api/v1/sessions/{sessionId}/send.
type SendSessionMessageRequest struct {
	Message string `json:"message" minLength:"1" maxLength:"4096"`
	// Attachment is an optional inline image (e.g. a browser-annotation
	// snapshot) delivered alongside the message. The daemon writes it into the
	// session worktree and appends a path reference to the message.
	Attachment *AttachmentInput `json:"attachment,omitempty"`
}

// SendSessionMessageResponse is the body of POST /api/v1/sessions/{sessionId}/send.
type SendSessionMessageResponse struct {
	OK        bool             `json:"ok"`
	SessionID domain.SessionID `json:"sessionId"`
	Message   string           `json:"message"`
}

// DelegateTaskRequest is the body of POST /api/v1/orchestrators/delegate.
// An omitted agent tells the orchestrator to use the project's worker default.
type DelegateTaskRequest struct {
	ProjectID domain.ProjectID `json:"projectId"`
	Brief     string           `json:"brief" maxLength:"4096"`
	// Outcome routes the brief through Kennel's orchestrator intake and approval
	// gate instead of immediately spawning an implementation worker.
	Outcome bool                `json:"outcome,omitempty"`
	Agent   domain.AgentHarness `json:"agent,omitempty" enum:"codex,deepseek-harness"`
	Model   string              `json:"model,omitempty" maxLength:"256"`
	// Mode is omitted for the daemon-owned default. The UI sends tui only when
	// the user explicitly accepts the fallback after Chat preflight fails.
	Mode domain.SessionMode `json:"mode,omitempty" enum:"tui,chat"`
	// Attachments are files pasted, dropped, or picked into the delegated task
	// brief. Each carries bytes as standard base64 (no data: URL prefix). The
	// daemon writes them into the spawned worker worktree and appends path
	// references to the worker prompt.
	Attachments []AttachmentInput `json:"attachments,omitempty"`
}

// DelegateTaskResponse confirms which worker was spawned and, when available,
// which orchestrator received the follow-up title request.
type DelegateTaskResponse struct {
	OK             bool             `json:"ok"`
	WorkerID       domain.SessionID `json:"workerId"`
	OrchestratorID domain.SessionID `json:"orchestratorId,omitempty"`
}

// SessionPRFacts is the pull-request read shape returned under session PR routes.
type SessionPRFacts struct {
	URL            string                `json:"url"`
	Number         int                   `json:"number"`
	State          string                `json:"state" enum:"draft,open,merged,closed"`
	CI             domain.CIState        `json:"ci" enum:"unknown,pending,passing,failing"`
	Review         domain.ReviewDecision `json:"review" enum:"none,approved,changes_requested,review_required"`
	Mergeability   domain.Mergeability   `json:"mergeability" enum:"unknown,mergeable,conflicting,blocked,unstable"`
	ReviewComments bool                  `json:"reviewComments"`
	UpdatedAt      time.Time             `json:"updatedAt"`
}

// SessionPRSummary is the concise desktop SCM read model returned by GET
// /sessions/{sessionId}/pr. It intentionally omits CI log tails and review
// comment bodies.
type SessionPRSummary struct {
	URL              string                       `json:"url"`
	HTMLURL          string                       `json:"htmlUrl,omitempty"`
	Number           int                          `json:"number"`
	Title            string                       `json:"title"`
	State            domain.PRState               `json:"state" enum:"draft,open,merged,closed"`
	Provider         string                       `json:"provider" enum:"github,gitlab"`
	Repo             string                       `json:"repo"`
	Author           string                       `json:"author"`
	SourceBranch     string                       `json:"sourceBranch"`
	TargetBranch     string                       `json:"targetBranch"`
	HeadSHA          string                       `json:"headSha"`
	Additions        int                          `json:"additions"`
	Deletions        int                          `json:"deletions"`
	ChangedFiles     int                          `json:"changedFiles"`
	CI               SessionPRCISummary           `json:"ci"`
	Review           SessionPRReviewSummary       `json:"review"`
	Mergeability     SessionPRMergeabilitySummary `json:"mergeability"`
	StateChangedAt   *time.Time                   `json:"stateChangedAt,omitempty"`
	CreatedAt        *time.Time                   `json:"createdAt,omitempty"`
	UpdatedAt        time.Time                    `json:"updatedAt"`
	ObservedAt       time.Time                    `json:"observedAt,omitempty"`
	CIObservedAt     time.Time                    `json:"ciObservedAt,omitempty"`
	ReviewObservedAt time.Time                    `json:"reviewObservedAt,omitempty"`
}

// SessionPRCISummary is the CI status block for a session PR summary.
type SessionPRCISummary struct {
	State         domain.CIState          `json:"state" enum:"unknown,pending,passing,failing"`
	FailingChecks []SessionPRFailingCheck `json:"failingChecks"`
	AutoInjectCI  bool                    `json:"autoInjectCI"`
}

// SessionPRFailingCheck is one failed or cancelled CI check for a PR.
type SessionPRFailingCheck struct {
	Name       string               `json:"name"`
	Status     domain.PRCheckStatus `json:"status" enum:"failed,cancelled"`
	Conclusion string               `json:"conclusion"`
	URL        string               `json:"url,omitempty"`
}

// SessionPRReviewSummary is the review state block for a session PR summary.
type SessionPRReviewSummary struct {
	Decision                   domain.ReviewDecision         `json:"decision" enum:"none,approved,changes_requested,review_required"`
	HasUnresolvedHumanComments bool                          `json:"hasUnresolvedHumanComments"`
	UnresolvedBy               []SessionPRUnresolvedReviewer `json:"unresolvedBy"`
	Reviews                    []SessionPRReviewEntry        `json:"reviews,omitempty"`
}

// SessionPRReviewEntry is one submitted provider review summary: a reviewer's
// decisive verdict and the summary body they submitted with it.
type SessionPRReviewEntry struct {
	ReviewerID       string                `json:"reviewerId"`
	Verdict          domain.ReviewDecision `json:"verdict" enum:"none,approved,changes_requested,review_required"`
	Body             string                `json:"body,omitempty"`
	ReviewURL        string                `json:"reviewUrl,omitempty"`
	SubmittedAt      time.Time             `json:"submittedAt"`
	IsBot            bool                  `json:"isBot,omitempty"`
	AutoInjectReview bool                  `json:"autoInjectReview"`
}

// SessionPRUnresolvedReviewer groups unresolved human comments by reviewer.
type SessionPRUnresolvedReviewer struct {
	ReviewerID string                       `json:"reviewerId"`
	Count      int                          `json:"count"`
	Links      []SessionPRReviewCommentLink `json:"links"`
	ReviewURL  string                       `json:"reviewUrl,omitempty"`
	IsBot      bool                         `json:"isBot,omitempty"`
}

// SessionPRReviewCommentLink points to one unresolved review comment.
type SessionPRReviewCommentLink struct {
	URL              string `json:"url,omitempty"`
	File             string `json:"file,omitempty"`
	Line             int    `json:"line,omitempty"`
	Body             string `json:"body,omitempty"`
	AutoInjectReview bool   `json:"autoInjectReview"`
}

// SessionPRMergeabilitySummary is the mergeability block for a session PR summary.
type SessionPRMergeabilitySummary struct {
	State         domain.Mergeability     `json:"state" enum:"unknown,mergeable,conflicting,blocked,unstable"`
	Reasons       []string                `json:"reasons"`
	PRURL         string                  `json:"prUrl"`
	ConflictFiles []SessionPRConflictFile `json:"conflictFiles,omitempty"`
}

// SessionPRConflictFile is one file involved in a PR merge conflict.
type SessionPRConflictFile struct {
	Path string `json:"path"`
	URL  string `json:"url,omitempty"`
}

// ListSessionPRsResponse is the body of GET /sessions/{sessionId}/pr.
type ListSessionPRsResponse struct {
	SessionID domain.SessionID   `json:"sessionId"`
	PRs       []SessionPRSummary `json:"prs"`
}

// NewSessionPRSummary maps the service PR summary model to its HTTP DTO.
func NewSessionPRSummary(in sessionsvc.PRSummary) SessionPRSummary {
	return SessionPRSummary{
		URL:              in.URL,
		HTMLURL:          in.HTMLURL,
		Number:           in.Number,
		Title:            in.Title,
		State:            in.State,
		Provider:         in.Provider,
		Repo:             in.Repo,
		Author:           in.Author,
		SourceBranch:     in.SourceBranch,
		TargetBranch:     in.TargetBranch,
		HeadSHA:          in.HeadSHA,
		Additions:        in.Additions,
		Deletions:        in.Deletions,
		ChangedFiles:     in.ChangedFiles,
		CI:               newSessionPRCISummary(in.CI),
		Review:           newSessionPRReviewSummary(in.Review),
		Mergeability:     newSessionPRMergeabilitySummary(in.Mergeability),
		StateChangedAt:   optionalTime(in.StateChangedAt),
		CreatedAt:        optionalTime(in.CreatedAt),
		UpdatedAt:        in.UpdatedAt,
		ObservedAt:       in.ObservedAt,
		CIObservedAt:     in.CIObservedAt,
		ReviewObservedAt: in.ReviewObservedAt,
	}
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func newSessionPRCISummary(in sessionsvc.PRCISummary) SessionPRCISummary {
	checks := make([]SessionPRFailingCheck, 0, len(in.FailingChecks))
	for _, ch := range in.FailingChecks {
		checks = append(checks, SessionPRFailingCheck{Name: ch.Name, Status: ch.Status, Conclusion: ch.Conclusion, URL: ch.URL})
	}
	return SessionPRCISummary{State: in.State, FailingChecks: checks, AutoInjectCI: in.AutoInjectCI}
}

func newSessionPRReviewSummary(in sessionsvc.PRReviewSummary) SessionPRReviewSummary {
	reviewers := make([]SessionPRUnresolvedReviewer, 0, len(in.UnresolvedBy))
	for _, reviewer := range in.UnresolvedBy {
		links := make([]SessionPRReviewCommentLink, 0, len(reviewer.Links))
		for _, link := range reviewer.Links {
			links = append(links, SessionPRReviewCommentLink{URL: link.URL, File: link.File, Line: link.Line, Body: link.Body, AutoInjectReview: link.AutoInjectReview})
		}
		reviewers = append(reviewers, SessionPRUnresolvedReviewer{ReviewerID: reviewer.ReviewerID, Count: reviewer.Count, Links: links, ReviewURL: reviewer.ReviewURL, IsBot: reviewer.IsBot})
	}
	entries := make([]SessionPRReviewEntry, 0, len(in.Reviews))
	for _, review := range in.Reviews {
		entries = append(entries, SessionPRReviewEntry{
			ReviewerID:       review.Reviewer,
			Verdict:          review.Verdict,
			Body:             review.Body,
			ReviewURL:        review.URL,
			SubmittedAt:      review.SubmittedAt,
			IsBot:            review.IsBot,
			AutoInjectReview: review.AutoInjectReview,
		})
	}
	return SessionPRReviewSummary{Decision: in.Decision, HasUnresolvedHumanComments: in.HasUnresolvedHumanComments, UnresolvedBy: reviewers, Reviews: entries}
}

func newSessionPRMergeabilitySummary(in sessionsvc.PRMergeabilitySummary) SessionPRMergeabilitySummary {
	files := make([]SessionPRConflictFile, 0, len(in.ConflictFiles))
	for _, file := range in.ConflictFiles {
		files = append(files, SessionPRConflictFile{Path: file.Path, URL: file.URL})
	}
	return SessionPRMergeabilitySummary{State: in.State, Reasons: in.Reasons, PRURL: in.PRURL, ConflictFiles: files}
}

// ClaimPRRequest is the body of POST /sessions/{sessionId}/pr/claim.
type ClaimPRRequest struct {
	PR            string `json:"pr" minLength:"1"`
	AllowTakeover *bool  `json:"allowTakeover,omitempty"`
}

// ClaimPRResponse is the body of POST /sessions/{sessionId}/pr/claim.
type ClaimPRResponse struct {
	OK            bool               `json:"ok"`
	SessionID     domain.SessionID   `json:"sessionId"`
	PRs           []SessionPRFacts   `json:"prs"`
	BranchChanged bool               `json:"branchChanged"`
	TakenOverFrom []domain.SessionID `json:"takenOverFrom"`
}

// SupervisedProcessExitRequest is the authenticated process exit fact a
// Kennel-supervised provider process wrapper reports for the current runtime
// generation.
type SupervisedProcessExitRequest struct {
	ExitCode *int   `json:"exitCode" description:"Exact provider process exit code when available."`
	Reason   string `json:"reason" description:"Supervisor exit reason."`
}

// SetActivityRequest is the body of POST /api/v1/sessions/{sessionId}/activity.
// Event/ToolName/ToolUseID are optional correlation facts: which Kennel hook
// sub-command produced the state and, for tool-use hooks, which tool call it
// concerns. Lifecycle uses them to clear a stale blocked state only when the
// specific approved tool finishes. Absent on old CLIs and on adapters whose
// payloads carry no tool identity — the signal then keeps its plain
// state-only semantics.
// AgentSessionID may arrive without State on metadata-only SessionStart hooks.
type SetActivityRequest struct {
	State                 string                        `json:"state,omitempty" enum:"active,idle,waiting_input,blocked,exited" description:"Agent activity state reported by an agent hook. Optional for metadata-only hooks."`
	Event                 string                        `json:"event,omitempty" description:"Kennel hook sub-command that produced this state (e.g. post-tool-use)."`
	ToolName              string                        `json:"toolName,omitempty" description:"Native tool name, for tool-use hook events."`
	ToolUseID             string                        `json:"toolUseId,omitempty" description:"Native tool-use id, for tool-use hook events."`
	AgentSessionID        string                        `json:"agentSessionId,omitempty" description:"Native agent session identifier used to resume its transcript."`
	LatestUserPrompt      string                        `json:"latestUserPrompt,omitempty" maxLength:"16384" description:"Latest real user prompt exposed by the provider hook."`
	LatestAssistantUpdate string                        `json:"latestAssistantUpdate,omitempty" maxLength:"16384" description:"Latest assistant update exposed by the provider hook."`
	TranscriptPath        string                        `json:"transcriptPath,omitempty" maxLength:"4096" description:"Read-only provider-native transcript path exposed by the hook."`
	LaunchID              string                        `json:"launchId,omitempty" description:"Kennel process generation that produced the signal."`
	ProcessExit           *SupervisedProcessExitRequest `json:"processExit,omitempty" description:"Authenticated process exit observation from Kennels supervisor."`
	Usage                 *UsageHookMetadata            `json:"usage,omitempty" description:"Provider transcript metadata used by the local usage pipeline."`
}

// UsageHookMetadata is the transcript metadata carried by supported Claude
// Code and Codex hooks. It contains paths and identifiers only, never prompt or
// response content.
type UsageHookMetadata struct {
	Harness                domain.AgentHarness `json:"harness" enum:"claude-code,codex"`
	TranscriptPath         string              `json:"transcriptPath,omitempty"`
	ModelID                string              `json:"modelId,omitempty"`
	SubagentID             string              `json:"subagentId,omitempty"`
	SubagentTranscriptPath string              `json:"subagentTranscriptPath,omitempty"`
}

// SetActivityResponse is the body of POST /api/v1/sessions/{sessionId}/activity.
type SetActivityResponse struct {
	OK        bool             `json:"ok"`
	SessionID domain.SessionID `json:"sessionId"`
	State     string           `json:"state"`
}

// SetReviewActivityRequest is the body of POST /api/v1/reviews/{reviewSessionID}/activity.
// Reviewer activity does not currently feed worker/Kanban session state.
// AgentSessionID is the native reviewer conversation id used for reviewer
// restore.
type SetReviewActivityRequest struct {
	State          string `json:"state,omitempty" enum:"active,idle,waiting_input,blocked,exited" description:"Reviewer activity state reported by a hook. Accepted for forward compatibility, not used for session display state."`
	Event          string `json:"event,omitempty" description:"Kennel hook sub-command that produced this signal."`
	AgentSessionID string `json:"agentSessionId,omitempty" description:"Native reviewer session identifier used to resume its transcript."`
	LaunchID       string `json:"launchId,omitempty" description:"Kennel process generation that produced the signal."`
}

// SetReviewActivityResponse is the body of POST /api/v1/reviews/{reviewSessionID}/activity.
type SetReviewActivityResponse struct {
	OK              bool   `json:"ok"`
	ReviewSessionID string `json:"reviewSessionId"`
}

// OrchestratorIDParam is the {id} path parameter for orchestrator routes.
type OrchestratorIDParam struct {
	ID string `path:"id" description:"Orchestrator session identifier, e.g. project-orchestrator."`
}

// ReviewSessionIDParam is the {reviewSessionID} path parameter for reviewer-owned routes.
type ReviewSessionIDParam struct {
	ID string `path:"reviewSessionID" description:"Reviewer session identifier, currently the per-harness review row id."`
}

// SpawnOrchestratorRequest is the body of POST /api/v1/orchestrators.
type SpawnOrchestratorRequest struct {
	ProjectID domain.ProjectID `json:"projectId"`
	Clean     bool             `json:"clean,omitempty"`
	// Mode applies only when this request creates a project orchestrator. An
	// idempotent ensure returns the existing orchestrator unchanged, and a clean
	// replacement inherits the existing orchestrator's currently committed mode.
	Mode domain.SessionMode `json:"mode,omitempty" enum:"chat,tui"`
}

// SpawnOrchestratorResponse is the body of POST /api/v1/orchestrators.
type SpawnOrchestratorResponse struct {
	Orchestrator OrchestratorResponse `json:"orchestrator"`
}

// OrchestratorResponse is the minimal orchestrator read model returned after spawn.
type OrchestratorResponse struct {
	ID          domain.SessionID `json:"id"`
	ProjectID   domain.ProjectID `json:"projectId"`
	ProjectName string           `json:"projectName,omitempty"`
}

// ListAgentsResponse is the body of GET /api/v1/agents.
type ListAgentsResponse = agentsvc.Inventory

// RefreshAgentsResponse is the body of POST /api/v1/agents/refresh.
type RefreshAgentsResponse = agentsvc.Inventory

// ProbeAgentResponse is the body of POST /api/v1/agents/{agent}/probe.
type ProbeAgentResponse = agentsvc.ProbeResult

// AgentModelsQuery scopes a model catalog to a project where providers may be
// configured per workspace.
type AgentModelsQuery struct {
	ProjectID string `query:"projectId,omitempty" description:"Optional project identifier used as the model-catalog cache scope."`
}

// AgentModelsRefreshQuery controls forced refresh versus cheap background
// revalidation for a project-scoped model catalog.
type AgentModelsRefreshQuery struct {
	ProjectID  string `query:"projectId,omitempty" description:"Optional project identifier used as the model-catalog cache scope."`
	Revalidate bool   `query:"revalidate,omitempty" description:"When true, compare executable and config metadata before running discovery."`
}

// AgentModelsResponse is the normalized model picker for one agent.
type AgentModelsResponse = ports.AgentModelCatalog

// AgentModelInfo is one selectable model or agent-owned mode.
type AgentModelInfo = ports.AgentModelInfo

// AgentInfo is one supported or installed agent entry.
type AgentInfo = agentsvc.Info

// ListUsageSessionsQuery is the query string accepted by GET
// /api/v1/usage/sessions.
type ListUsageSessionsQuery struct {
	ProjectID domain.ProjectID `query:"projectId,omitempty" description:"Optional project id filter for dashboard cards."`
}

// CompactSessionUsageResponse is one session card's token-only usage summary.
type CompactSessionUsageResponse struct {
	SessionID   domain.SessionID `json:"sessionId"`
	TotalTokens int64            `json:"totalTokens" minimum:"0"`
	Incomplete  bool             `json:"incomplete"`
}

// ListCompactSessionUsageResponse is the batch dashboard usage response.
type ListCompactSessionUsageResponse struct {
	Sessions []CompactSessionUsageResponse `json:"sessions"`
}

// UsageTotalsResponse is the normalized telemetry aggregate for one scope.
type UsageTotalsResponse struct {
	InputTokens         *int64 `json:"inputTokens"`
	UncachedInputTokens *int64 `json:"uncachedInputTokens"`
	CacheReadTokens     *int64 `json:"cacheReadTokens"`
	CacheWriteTokens    *int64 `json:"cacheWriteTokens"`
	OutputTokens        *int64 `json:"outputTokens"`
	ReasoningTokens     *int64 `json:"reasoningTokens"`
}

// UsageModelResponse is telemetry grouped by exact model id.
type UsageModelResponse struct {
	ModelID string              `json:"modelId"`
	Totals  UsageTotalsResponse `json:"totals"`
}

// UsageHarnessResponse groups model telemetry under one Kennel harness.
type UsageHarnessResponse struct {
	Harness string               `json:"harness"`
	Totals  UsageTotalsResponse  `json:"totals"`
	Models  []UsageModelResponse `json:"models"`
}

// SessionUsageResponse is detailed telemetry for the session inspector.
type SessionUsageResponse struct {
	SessionID  domain.SessionID       `json:"sessionId"`
	Incomplete bool                   `json:"incomplete"`
	Totals     UsageTotalsResponse    `json:"totals"`
	Harnesses  []UsageHarnessResponse `json:"harnesses"`
}

// ListNotificationsQuery is the query string accepted by GET /api/v1/notifications.
type ListNotificationsQuery struct {
	Status string `query:"status,omitempty" enum:"unread,all,unresolved" description:"Notification filter. Defaults to unread (unseen); unresolved returns notifications whose underlying issue is still open; all includes read history."`
	Limit  int    `query:"limit,omitempty" minimum:"1" maximum:"100" description:"Maximum notifications to return. Defaults to 100."`
	Cursor string `query:"cursor,omitempty" description:"Opaque cursor returned by the previous page."`
}

// NotificationStreamQuery is the query string accepted by GET /api/v1/notifications/stream.
type NotificationStreamQuery struct {
	ProjectID string `query:"projectId,omitempty" description:"Optional project id filter for live notifications."`
}

// NotificationIDParam is the {id} path parameter shared by notification routes.
type NotificationIDParam struct {
	ID string `path:"id" description:"Notification identifier."`
}

// NotificationTarget is the dashboard navigation target for a notification.
type NotificationTarget struct {
	Kind      string `json:"kind" enum:"session,pr"`
	SessionID string `json:"sessionId"`
	PRURL     string `json:"prUrl,omitempty"`
}

// NotificationResponse is one stored notification returned by the API.
type NotificationResponse struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	ProjectID string    `json:"projectId"`
	PRURL     string    `json:"prUrl"`
	Type      string    `json:"type" enum:"needs_input,ready_to_merge,pr_merged,pr_closed_unmerged"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Status    string    `json:"status" enum:"unread,read" description:"Seen state. unread means the user has not opened the notification panel since it arrived."`
	CreatedAt time.Time `json:"createdAt"`
	// ResolvedAt is set by Kennel when the underlying issue goes away (the session
	// received its input, the PR stopped waiting on a merge). Absent means the
	// issue is still open. There is no user-facing action that sets it.
	ResolvedAt *time.Time         `json:"resolvedAt,omitempty"`
	Target     NotificationTarget `json:"target"`
}

// ListNotificationsResponse is one history page from GET /api/v1/notifications.
type ListNotificationsResponse struct {
	Notifications   []NotificationResponse `json:"notifications"`
	NextCursor      string                 `json:"nextCursor,omitempty"`
	UnreadCount     int                    `json:"unreadCount"`
	UnresolvedCount int                    `json:"unresolvedCount"`
}

// MarkNotificationReadRequest is the body of PATCH /api/v1/notifications/{id}.
type MarkNotificationReadRequest struct {
	Status string `json:"status" enum:"read" description:"V1 supports only marking an unread notification read."`
}

// NotificationEnvelope is the { notification } response body for notification mutations.
type NotificationEnvelope struct {
	Notification NotificationResponse `json:"notification"`
}

// ShellTerminalHandleIDParam is the {handleId} path parameter for shell
// terminal routes. It is the runtime handle the terminal mux attaches to, not
// a session id.
type ShellTerminalHandleIDParam struct {
	HandleID string `path:"handleId" description:"Shell terminal runtime handle identifier."`
}

// OpenShellTerminalRequest is the body of POST /api/v1/shell-terminals.
type OpenShellTerminalRequest struct {
	ProjectID string `json:"projectId,omitempty" description:"Project whose root the shell starts in. Omitted opens the shell in the daemon data dir."`
	SessionID string `json:"sessionId,omitempty" description:"Agent session the shell is scoped to, so it appears only in that session's tab strip. Omitted makes it a standalone shell."`
}

// UpdateShellTerminalRequest is the body of PATCH /api/v1/shell-terminals/{handleId}.
type UpdateShellTerminalRequest struct {
	Title string `json:"title" description:"New tab title for the shell terminal. Trimmed; must be non-empty."`
}

// ShellTerminalResponse is one standalone shell terminal. HandleID is what the
// client opens on the terminal mux, exactly as it would a session's pane.
type ShellTerminalResponse struct {
	HandleID   string    `json:"handleId"`
	ProjectID  string    `json:"projectId,omitempty"`
	SessionID  string    `json:"sessionId,omitempty"`
	WorkingDir string    `json:"workingDir"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ListShellTerminalsResponse is the body of GET /api/v1/shell-terminals.
type ListShellTerminalsResponse struct {
	ShellTerminals []ShellTerminalResponse `json:"shellTerminals"`
}

// ShellTerminalEnvelope is the { shellTerminal } response body for shell
// terminal mutations.
type ShellTerminalEnvelope struct {
	ShellTerminal ShellTerminalResponse `json:"shellTerminal"`
}

// MarkAllNotificationsReadRequest is the optional body of
// POST /api/v1/notifications/read-all.
type MarkAllNotificationsReadRequest struct {
	IDs []string `json:"ids,omitempty" description:"Acknowledge exactly these notifications. Omit to acknowledge every unread notification; paginating clients should send the ids they actually rendered so later pages stay unread."`
}

// MarkAllNotificationsReadResponse is the body of POST /api/v1/notifications/read-all.
type MarkAllNotificationsReadResponse struct {
	Notifications []NotificationResponse `json:"notifications" description:"Deprecated compatibility field. Always empty so mark-all responses stay bounded."`
	UpdatedCount  int64                  `json:"updatedCount" description:"Number of notifications changed from unread to read."`
}

// DevImportProjectsRequest is the body of POST /api/v1/dev/import-projects.
type DevImportProjectsRequest struct {
	SourceDataDir string `json:"sourceDataDir" minLength:"1"`
	DryRun        bool   `json:"dryRun"`
}

// DevImportProjectsResponse is the body of POST /api/v1/dev/import-projects.
type DevImportProjectsResponse struct {
	Report devimport.Report `json:"report"`
}

// PRIDParam is the {id} path parameter shared by the /prs/{id} routes.
type PRIDParam struct {
	ID string `path:"id" description:"PR number."`
}

// MergePRRequest is the body of POST /api/v1/prs/{id}/merge.
type MergePRRequest struct {
	PRURL           string `json:"prUrl" minLength:"1"`
	ExpectedHeadSHA string `json:"expectedHeadSha" minLength:"40"`
}

// MergePRResponse is the body of POST /api/v1/prs/{id}/merge (200).
type MergePRResponse struct {
	OK       bool   `json:"ok"`
	PRNumber int    `json:"prNumber"`
	Method   string `json:"method"`
}

// ResolveCommentsRequest is the optional body of POST /api/v1/prs/{id}/resolve-comments.
type ResolveCommentsRequest struct {
	CommentIDs []string `json:"commentIds,omitempty"`
}

// ResolveCommentsResponse is the body of POST /api/v1/prs/{id}/resolve-comments (200).
type ResolveCommentsResponse struct {
	OK       bool `json:"ok"`
	Resolved int  `json:"resolved"`
}

// MobileStatusResponse is the body of the Connect Mobile status/enable/disable/
// regenerate endpoints. Password is populated only transiently, on enable and
// regenerate responses (empty otherwise) — it is never persisted in plaintext.
type MobileStatusResponse struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	// TailscaleHost is this machine's 100.64.0.0/10 Tailscale address, or "" when
	// Tailscale is not up. The renderer encodes it into the pairing QR when the
	// user selects the Tailscale tab, and shows a hint instead when it is empty.
	TailscaleHost string              `json:"tailscaleHost"`
	Port          int                 `json:"port"`
	Password      string              `json:"password"`
	Warning       string              `json:"warning"`
	SecurePairing SecurePairingStatus `json:"securePairing"`
}

// SecurePairingStatus describes the optional TLS-over-Tailscale pairing mode,
// in which `tailscale serve` fronts the bridge with a real certificate so iOS
// can pair by scanning (App Transport Security blocks cleartext to Tailscale's
// 100.64.0.0/10 range).
type SecurePairingStatus struct {
	Enabled   bool   `json:"enabled"`   // the user turned the mode on
	Available bool   `json:"available"` // CLI present, MagicDNS name known, certs enabled
	Active    bool   `json:"active"`    // proxy verified pointing at the live bridge port
	Host      string `json:"host"`      // MagicDNS name; "" when unknown
	Port      int    `json:"port"`      // 443 when active, else 0
	// Reason is a fixed enum the renderer maps to localized setup steps:
	// no_cli, no_magicdns, no_certs, serve_failed, port_mismatch, clear_failed.
	// Empty when Available. Never a raw error string — those are untranslated
	// and can leak paths.
	Reason string `json:"reason"`
}

// SetSecurePairingRequest is the body of POST /api/v1/mobile/secure-pairing.
type SetSecurePairingRequest struct {
	Enabled bool `json:"enabled"`
}

// PushPairingIDParam is the {id} path parameter for the unpair route. It accepts
// either the phone's install ID or, from builds that predate install IDs, its
// push token.
type PushPairingIDParam struct {
	ID string `path:"id" description:"The phone's install id, or its push token for older builds."`
}

// PushDeviceTokenParam is the {token} path parameter for push-device routes.
type PushDeviceTokenParam struct {
	Token string `path:"token" description:"Expo push token (URL-encoded) identifying the device."`
}

// RegisterPushDeviceRequest is the body of POST /api/v1/push/devices. The phone
// sends its Expo push token plus a bit of descriptive metadata; the daemon keys
// the registry on the install ID (the token is an attribute and is now optional)
// and re-registering is an idempotent upsert.
type RegisterPushDeviceRequest struct {
	// Optional so the published contract matches what the daemon actually
	// accepts: app builds predating install IDs send none, and the handler
	// synthesizes a legacy one rather than rejecting them. Marking it required
	// would generate clients unable to express a request the server handles.
	InstallID string `json:"installId,omitempty" description:"Stable per-install device id, keying the registry so a rotated push token updates the same row. Optional: older app builds omit it and the daemon synthesizes one."`
	// Optional: a row represents a paired phone, not a push registration. Omitted
	// (or empty) when the phone is only announcing its identity — permission not
	// yet granted, or a build that can't mint a token. When present it must still
	// be a well-formed Expo push token.
	Token      string `json:"token,omitempty" description:"Expo push token, e.g. ExponentPushToken[...]. Optional: omitted when the phone has no push token yet."`
	Platform   string `json:"platform,omitempty" enum:"ios,android" description:"Device platform."`
	DeviceName string `json:"deviceName,omitempty" description:"Human-friendly device label."`
}

// PushDeviceResponse is the stored view of a registered push device.
type PushDeviceResponse struct {
	Token      string    `json:"token,omitempty"`
	Platform   string    `json:"platform,omitempty"`
	DeviceName string    `json:"deviceName,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

// PushDeviceEnvelope is the { device } response body for a registered push device.
type PushDeviceEnvelope struct {
	Device PushDeviceResponse `json:"device"`
}

// UnregisterPushDeviceResponse is the body of DELETE /api/v1/push/devices/{token} (200).
type UnregisterPushDeviceResponse struct {
	Token   string `json:"token"`
	Deleted bool   `json:"deleted"`
}

/* ---- chat conversations ------------------------------------------------ */

// SendConversationMessageRequest is a message for a Chat session's agent.
type SendConversationMessageRequest struct {
	Text string `json:"text"`
	// ClientMessageID makes delivery idempotent. A retry carrying the same value
	// must not produce a second provider turn.
	ClientMessageID string                               `json:"clientMessageId,omitempty"`
	Attachments     []ConversationImageContentRequest    `json:"attachments,omitempty"`
	Resources       []ConversationResourceContentRequest `json:"resources,omitempty"`
}

// ConversationImageContentRequest is a native raster image prompt block.
type ConversationImageContentRequest struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

// ConversationResourceContentRequest is a resource link, or embedded text when
// Text is present and the provider negotiated embedded context.
type ConversationResourceContentRequest struct {
	URI      string  `json:"uri"`
	Name     string  `json:"name"`
	MIMEType string  `json:"mimeType,omitempty"`
	Text     *string `json:"text,omitempty"`
}

// SendConversationMessageResponse reports what the send did.
type SendConversationMessageResponse struct {
	TurnID         string `json:"turnId,omitempty"`
	ProviderTurnID string `json:"providerTurnId,omitempty"`
	// State is `running` when the agent picked the message up immediately and
	// `queued` when it arrived mid-turn and will be sent once the turn ends. A
	// client that only reads turnId cannot tell those apart, and "accepted" is not
	// the same claim as "delivered".
	State domain.TurnState `json:"state,omitempty" enum:"queued,running,completed,interrupted,failed"`
	// Duplicate is true when this client message id was already delivered, so a
	// retrying client can stop instead of assuming a new turn began.
	Duplicate bool `json:"duplicate"`
}

// EditConversationMessageRequest changes the readable text of one durable human
// prompt. Structured content is intentionally absent: the service reuses the
// server-side blocks recorded with the original message.
type EditConversationMessageRequest struct {
	Text            string `json:"text"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
}

// ConversationContentSummaryResponse is a lightweight attachment/resource chip.
// Image bytes and embedded resource text never leave the durable server record.
type ConversationContentSummaryResponse struct {
	Type     string `json:"type"`
	MIMEType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
	Name     string `json:"name,omitempty"`
}

// EditConversationMessageResponse identifies the newly selected branch and its
// replacement turn.
type EditConversationMessageResponse struct {
	SourceBranchID string           `json:"sourceBranchId"`
	ActiveBranchID string           `json:"activeBranchId"`
	TurnID         string           `json:"turnId,omitempty"`
	ProviderTurnID string           `json:"providerTurnId,omitempty"`
	State          domain.TurnState `json:"state,omitempty" enum:"queued,running,completed,interrupted,failed"`
}

// ActivateConversationBranchResponse reports the durable head after switching.
type ActivateConversationBranchResponse struct {
	ActiveBranchID string `json:"activeBranchId"`
}

// ConversationModelsResponse is the provider's model catalog plus what is selected.
type ConversationModelsResponse struct {
	Models   []ConversationModelResponse     `json:"models"`
	Selected ConversationTurnSettingsPayload `json:"selected"`
}

// ConversationConfigOptionsResponse is the provider's complete live session
// configuration catalog. Clients replace their cached list after a mutation:
// model changes can add or remove dependent controls.
type ConversationConfigOptionsResponse struct {
	Options []ConversationConfigOptionResponse `json:"options"`
}

// ConversationConfigOptionResponse is one provider-advertised session control.
type ConversationConfigOptionResponse struct {
	ID             string                             `json:"id"`
	Name           string                             `json:"name"`
	Description    string                             `json:"description,omitempty"`
	Category       string                             `json:"category,omitempty"`
	Type           string                             `json:"type" enum:"select,boolean"`
	CurrentValue   string                             `json:"currentValue,omitempty"`
	CurrentBoolean *bool                              `json:"currentBoolean,omitempty"`
	Choices        []ConversationConfigChoiceResponse `json:"choices,omitempty"`
}

// ConversationConfigChoiceResponse is one value in a provider select.
type ConversationConfigChoiceResponse struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty"`
	GroupName   string `json:"groupName,omitempty"`
}

// SetConversationConfigOptionRequest selects one provider-advertised value.
// Selects use Value; booleans use Enabled. Exactly one must be present.
type SetConversationConfigOptionRequest struct {
	Value   string `json:"value,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// ConversationModelResponse is one model the provider offers.
type ConversationModelResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	// Default marks the model the provider would pick on its own, so a client can
	// label it rather than inventing its own idea of a default.
	Default bool `json:"default"`
	// Efforts are the reasoning levels this model supports, in the provider's
	// order. Empty means the model does not take one.
	Efforts       []string `json:"efforts,omitempty"`
	DefaultEffort string   `json:"defaultEffort,omitempty"`
}

// ConversationSkillsResponse is the named skills the provider will let this
// session invoke.
//
// An empty list is a real answer, not a failure: it means this agent offers no
// skills, and a client must render that as "no commands" rather than as an error.
type ConversationSkillsResponse struct {
	Skills []ConversationSkillResponse `json:"skills"`
}

// ConversationSkillResponse is one skill a user can invoke by name.
type ConversationSkillResponse struct {
	// Name is the invocable identifier. It is what a client puts in the message
	// text; DisplayName is only a label.
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	// InputHint is the provider's short placeholder for command arguments.
	InputHint string `json:"inputHint,omitempty"`
	// Source is where the skill came from (the provider's scope: user, repo,
	// system, admin), so a user can tell a repo skill from one of their own.
	Source string `json:"source,omitempty"`
}

// ConversationTurnSettingsPayload is the provider choices for the next turn. It is
// both the request body for changing them and the echo of what is now stored.
//
// Every field is optional and an empty value means "use the provider's default",
// so clearing a choice and never making one are the same thing.
type ConversationTurnSettingsPayload struct {
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	ApprovalMode    string `json:"approvalMode,omitempty" enum:"default,accept-edits,auto,bypass-permissions"`
}

// ResolveConversationApprovalRequest answers a pending approval. DecisionID must
// be one the provider offered for that request; Kennel does not invent options.
type ResolveConversationApprovalRequest struct {
	DecisionID string `json:"decisionId"`
}

// ResolveConversationInputRequest answers a structured form or URL-consent
// request. Content is meaningful only for accept.
type ResolveConversationInputRequest struct {
	Action  string         `json:"action" enum:"accept,decline,cancel"`
	Content map[string]any `json:"content,omitempty"`
}

// CompactConversationResponse reports a compaction the provider accepted.
//
// Accepted, not finished. The provider takes the request and does the work as its
// own turn over the following seconds, so this says what is about to be reclaimed
// and the settled figures arrive on the timeline as a compaction entry. A client
// that wants the outcome reads the timeline, which is where it belongs anyway:
// the reclaim is durable history, not the answer to one request.
type CompactConversationResponse struct {
	// TokensBefore is the conversation's context position when compaction was
	// requested. Zero means the provider has not reported one yet, in which case
	// Kennel deliberately claims no figure rather than guessing at one.
	TokensBefore int64 `json:"tokensBefore,omitempty"`
	// TokensAfter is only set by a provider that compacts synchronously. Zero means
	// the reclaim is still in flight.
	TokensAfter int64 `json:"tokensAfter,omitempty"`
}

// ConversationTurnResponse is one request and the work that followed it.
type ConversationTurnResponse struct {
	ID             string  `json:"id"`
	State          string  `json:"state" enum:"queued,running,completed,interrupted,failed"`
	ProviderTurnID string  `json:"providerTurnId,omitempty"`
	ErrorMessage   string  `json:"errorMessage,omitempty"`
	RequestedAt    string  `json:"requestedAt"`
	StartedAt      *string `json:"startedAt,omitempty"`
	CompletedAt    *string `json:"completedAt,omitempty"`
	// RolledBack marks a turn an undo discarded. Its messages and activities are
	// absent from this snapshot because the agent no longer remembers them; the turn
	// is still reported so a client can say what was taken back rather than letting
	// the timeline quietly shrink.
	RolledBack bool `json:"rolledBack,omitempty"`
	// Diff is what this turn has changed on disk. Absent when the provider has
	// reported nothing, which is not a claim that nothing changed: an agent
	// without diff support never reports at all.
	//
	// Carried on the snapshot the client already polls rather than behind its own
	// route. A dedicated route would be a second request, on the same cadence, for
	// data this read has already loaded -- the diff belongs to a turn, and the turn
	// list is right here. It also keeps the changed-file view and the timeline from
	// disagreeing, which two independently-timed fetches would eventually do.
	Diff *ConversationTurnDiffResponse `json:"diff,omitempty"`
	// Plan is the agent's plan for this turn, or absent when it made none. The
	// provider re-sends the whole plan on every change, so this is the current answer
	// rather than a history of one: the earlier versions are the same plan with fewer
	// steps ticked off.
	Plan *ConversationPlanResponse `json:"plan,omitempty"`
	// DispatchBlockedSince is when this turn's own governed claim last moved, present
	// only while that claim still owns the command effect (claimed, dispatching, or
	// delivery_unknown). Absent means this turn's own claim is not the reason a client
	// cannot dispatch again -- it does NOT mean nothing in the session is blocking: a
	// stuck steer/answer/interrupt claim on this same session blocks every turn's
	// dispatch without living on any turn. See the snapshot's governedTurnBlocks and
	// governedControlBlocks for that session-wide truth.
	DispatchBlockedSince *string `json:"dispatchBlockedSince,omitempty"`
	// DispatchBlockedState is this turn's own claim state whenever
	// DispatchBlockedSince is set. Never claimed/dispatching mislabeled as
	// delivery_unknown -- it is always the claim's real, current state.
	DispatchBlockedState string `json:"dispatchBlockedState,omitempty" enum:"claimed,dispatching,delivery_unknown"`
}

// ConversationPlanResponse is the agent's plan for one turn.
type ConversationPlanResponse struct {
	// Explanation is the agent's note about the plan as a whole, when it gives one.
	Explanation string                         `json:"explanation,omitempty"`
	Steps       []ConversationPlanStepResponse `json:"steps"`
}

// ConversationPlanStepResponse is one step of a plan.
//
// Structured, not prose. The per-step status is the whole point -- it is where the
// agent is up to -- and a client that wants a sentence can join the steps, while one
// that wants checkboxes cannot recover them from a sentence.
type ConversationPlanStepResponse struct {
	Text   string `json:"text"`
	Status string `json:"status" enum:"pending,in_progress,completed"`
}

// ConversationTurnDiffResponse is a turn's changed-file summary.
type ConversationTurnDiffResponse struct {
	Files []ConversationDiffFileResponse `json:"files"`
	// Truncated reports that the file list was cut at the daemon's cap, so a client
	// does not present a partial list as the whole change.
	Truncated bool `json:"truncated,omitempty"`
}

// ConversationDiffFileResponse is one changed path.
//
// No patch text. The turn view answers "what did this touch, and by how much";
// carrying every hunk would put the full diff into a body polled once a second,
// and Kennel already has a diff surface for reading the change itself.
type ConversationDiffFileResponse struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status" enum:"added,modified,deleted,renamed"`
	// OldPath is set only for a rename.
	OldPath string `json:"oldPath,omitempty"`
	// RolledBack marks a turn an undo discarded. Its messages and activities are
	// absent from this snapshot because the agent no longer remembers them; the turn
	// is still reported so a client can say what was taken back rather than letting
	// the timeline quietly shrink.
	RolledBack bool `json:"rolledBack,omitempty"`
}

// ConversationMessageResponse is one readable block of text.
type ConversationMessageResponse struct {
	Kind          string                               `json:"kind" enum:"message"`
	ID            string                               `json:"id"`
	TurnID        string                               `json:"turnId,omitempty"`
	Sequence      int64                                `json:"sequence"`
	Revision      int64                                `json:"revision"`
	Role          string                               `json:"role" enum:"user,assistant"`
	Origin        string                               `json:"origin" enum:"human,automation,daemon,provider"`
	Text          string                               `json:"text"`
	Content       []ConversationContentSummaryResponse `json:"content,omitempty"`
	EditAvailable bool                                 `json:"editAvailable"`
	// Streaming is true while more deltas are expected for this message.
	Streaming bool   `json:"streaming"`
	CreatedAt string `json:"createdAt"`
}

// ConversationActivityResponse is one non-message timeline entry.
type ConversationActivityResponse struct {
	Kind     string `json:"kind" enum:"activity"`
	ID       string `json:"id"`
	TurnID   string `json:"turnId,omitempty"`
	Sequence int64  `json:"sequence"`
	Revision int64  `json:"revision"`
	// ActivityKind discriminates the payload in Detail.
	//
	// mcp_tool is not command: an MCP call has a server, a tool name, structured
	// arguments and a structured result, and rendering it as a shell command claimed
	// the agent had run something in the worktree. auto_review is not approval: an
	// approval is a question waiting on a person, while an auto-review is a decision
	// the provider already made on their behalf, and those are opposites.
	ActivityKind string `json:"activityKind" enum:"command,file_change,plan,reasoning,approval,usage,error,system,mcp_tool,auto_review,user_input"`
	Status       string `json:"status" enum:"running,completed,failed,cancelled,pending,resolved"`
	Summary      string `json:"summary"`
	// Detail is the provider-neutral typed payload for this kind. For an approval
	// it carries the provider's own offered decisions, which is what the client
	// renders buttons from.
	//
	// The keys that depend on the kind:
	//
	//   command      command, rawCommand, cwd, exitCode, durationMs, processId,
	//                output (+ outputSource, outputMayBePartial, outputTruncated),
	//                terminalInput -- the keystrokes the agent sent to the PTY, kept
	//                out of output because the PTY echoes them
	//   file_change  files[] with path, oldPath, status, additions, deletions, patch
	//   reasoning    text -- streamed while the model works, replaced by the
	//                provider's settled summary when the item completes
	//   mcp_tool     server, toolName, namespace, arguments, result, error, success,
	//                progress
	//   plan         event "plan", explanation, steps[] with text and status
	//   auto_review  reviewId, targetItemId, actionType, command, riskLevel,
	//                rationale, decisionSource, status, durationMs
	//   user_input   inputMode, message, schema, url, elicitationId
	//   system       event -- "compaction", "model.rerouted" or
	//                "auth.reauth_required" -- plus that event's own fields
	Detail    map[string]any `json:"detail,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
	// ProviderItemID is the stable parent key used by nested ACP transcripts.
	ProviderItemID string `json:"providerItemId,omitempty"`
	CreatedAt      string `json:"createdAt"`
}

// ConversationSnapshotResponse is the durable read model a client bootstraps from.
type ConversationSnapshotResponse struct {
	ConversationID             string `json:"conversationId"`
	ActiveBranchID             string `json:"activeBranchId,omitempty"`
	BranchedFromEarlierMessage bool   `json:"branchedFromEarlierMessage"`
	SessionID                  string `json:"sessionId"`
	Harness                    string `json:"harness,omitempty"`
	Mode                       string `json:"mode" enum:"chat,tui"`
	// Controller is reported separately from history so a client can tell "no
	// messages yet" apart from "the agent is not running".
	Controller     string                            `json:"controller" enum:"connecting,ready,busy,recovering,stopped"`
	LatestSequence int64                             `json:"latestSequence"`
	OldestSequence int64                             `json:"oldestSequence,omitempty"`
	HasMoreBefore  bool                              `json:"hasMoreBefore"`
	Turns          []ConversationTurnResponse        `json:"turns"`
	Messages       []ConversationMessageResponse     `json:"messages"`
	Activities     []ConversationActivityResponse    `json:"activities"`
	BranchPoints   []ConversationBranchPointResponse `json:"branchPoints,omitempty"`
	// Settings are the provider choices for the next turn. Carried on the snapshot
	// the client already polls so the composer can label itself without a second
	// request, and so a choice made on another client shows up here.
	Settings ConversationTurnSettingsPayload `json:"settings"`
	// Title is the name the provider currently gives this thread. Empty means it has
	// not named one, which is not the same as an empty name.
	Title string `json:"title,omitempty"`
	// Usage is how full this conversation is. Omitted until the provider reports,
	// so a client can tell "not known yet" from a conversation using nothing.
	Usage *ConversationUsagePayload `json:"usage,omitempty"`
	// RateLimits is where the account stands. Omitted until the provider reports.
	RateLimits *ConversationRateLimitsPayload `json:"rateLimits,omitempty"`
	// CompactedAt is when history was last summarized to reclaim context, or absent
	// if never. On the snapshot rather than derived from the timeline so a client can
	// label the control without scanning every activity.
	CompactedAt *string `json:"compactedAt,omitempty"`
	// ModelReroute is present when the provider answered with a model other than the
	// one that was asked for. A client MUST prefer this over the selected model when
	// naming what produced the answers: without it the composer keeps advertising a
	// model that is not replying.
	ModelReroute *ConversationModelReroutePayload `json:"modelReroute,omitempty"`
	// Account is the provider account this conversation runs under. Omitted until the
	// provider says anything about it.
	Account *ConversationAccountPayload `json:"account,omitempty"`
	// ThreadState is the provider's own lifecycle view of the thread. It is NOT the
	// session's status, which stays derived from durable facts and is served on the
	// session resource; this is one more such fact.
	ThreadState *ConversationThreadStatePayload `json:"threadState,omitempty"`
	// MCPServers is the startup state of the tool servers this conversation can
	// reach. Empty means none are configured or none has reported. It answers a
	// question the timeline cannot: a tool call that never happened because its
	// server failed to start reads, from the timeline alone, as the agent choosing
	// not to use it.
	MCPServers []ConversationMCPServerPayload `json:"mcpServers,omitempty"`
	// Capabilities names what this session's provider can do, so a client gates a
	// control before drawing it. Sorted, and only the abilities the provider
	// actually has are listed.
	//
	// An open list rather than a fixed set of booleans: drivers gain abilities, and
	// a client that checks for membership keeps working against a daemon that knows
	// about more of them than it does. Absent until a controller is live, because an
	// unstarted session's abilities are not yet known — and a client must treat
	// absent as "do not offer yet" rather than as "cannot".
	Capabilities []string `json:"capabilities,omitempty"`
	// GovernedTurnBlocks and GovernedControlBlocks are every unsettled governed claim
	// in this session that still blocks a later dispatch -- across the turn table and
	// the steer/answer/interrupt control table alike. Either list being non-empty
	// means Send() is blocked for this session right now, not merely that a claim was
	// once made and left unresolved.
	GovernedTurnBlocks    []GovernedTurnBlockResponse    `json:"governedTurnBlocks,omitempty"`
	GovernedControlBlocks []GovernedControlBlockResponse `json:"governedControlBlocks,omitempty"`
}

// GovernedTurnBlockResponse is one turn-dispatch claim currently blocking
// conflicting dispatch in this session.
type GovernedTurnBlockResponse struct {
	Kind                  string `json:"kind" enum:"turn"`
	TurnID                string `json:"turnId"`
	State                 string `json:"state" enum:"claimed,dispatching,delivery_unknown"`
	Quiescence            string `json:"quiescence" enum:"not_applicable,pending,codex_process_tree_verified"`
	QuiescenceEvidenceRef string `json:"quiescenceEvidenceRef,omitempty"`
	Since                 string `json:"since"`
}

// GovernedControlBlockResponse is one steer/answer/interrupt control claim
// currently blocking conflicting dispatch in this session. ProviderTurnID is
// present only for steer/interrupt rows; RequestInstanceID is present only for
// answer rows. A containment-failed interrupt reports here as
// kind=interrupt, state=delivery_unknown, quiescence=pending -- that shape
// falls out of passing the claim's real fields through, not a special case.
type GovernedControlBlockResponse struct {
	Kind                  string `json:"kind" enum:"steer,answer,interrupt"`
	ID                    string `json:"id"`
	State                 string `json:"state" enum:"claimed,dispatching,delivery_unknown"`
	Quiescence            string `json:"quiescence" enum:"not_applicable,pending,codex_process_tree_verified"`
	QuiescenceEvidenceRef string `json:"quiescenceEvidenceRef,omitempty"`
	ProviderTurnID        string `json:"providerTurnId,omitempty"`
	RequestInstanceID     string `json:"requestInstanceId,omitempty"`
	Since                 string `json:"since"`
}

// ConversationBranchPointResponse describes sibling continuations at one prompt.
type ConversationBranchPointResponse struct {
	TurnID           string `json:"turnId"`
	Position         int    `json:"position"`
	Total            int    `json:"total"`
	PreviousBranchID string `json:"previousBranchId,omitempty"`
	NextBranchID     string `json:"nextBranchId,omitempty"`
}

// ConversationModelReroutePayload is the provider answering with a model other than
// the one that was asked for.
type ConversationModelReroutePayload struct {
	FromModel string `json:"fromModel,omitempty"`
	ToModel   string `json:"toModel"`
	// Reason is the provider's own word for why, carried verbatim rather than
	// translated: Kennel cannot improve on the provider's account of its own policy.
	Reason string `json:"reason,omitempty"`
	// ProviderTurnID is the turn it happened on, so a client can point at the
	// exchange rather than only at the conversation.
	ProviderTurnID string `json:"providerTurnId,omitempty"`
	At             string `json:"at"`
}

// ConversationAccountPayload is what the provider says about the account behind a
// conversation.
type ConversationAccountPayload struct {
	AuthMode  string `json:"authMode,omitempty"`
	PlanLabel string `json:"planLabel,omitempty"`
	// ReauthRequiredAt is when the provider last asked for credentials the daemon
	// does not hold. Present means the session has stopped working for a reason no
	// retry will fix and the user has to sign in again.
	ReauthRequiredAt *string `json:"reauthRequiredAt,omitempty"`
	ReauthReason     string  `json:"reauthReason,omitempty"`
}

// ConversationThreadStatePayload is the provider's lifecycle view of the thread.
type ConversationThreadStatePayload struct {
	Status string `json:"status,omitempty" enum:"active,idle,not_loaded,system_error,closed"`
	// WaitingOn are the provider's active flags. A thread can be active AND blocked
	// on a person, and those are different states.
	WaitingOn []string `json:"waitingOn,omitempty"`
	// ArchivedAt is present while the provider considers the thread archived.
	// Archiving is reversible, so this returns to absent on unarchive.
	ArchivedAt *string `json:"archivedAt,omitempty"`
	// ClosedAt is when the provider dropped the thread. Recorded rather than acted
	// on: the daemon has never observed this, so tearing a controller down on the
	// strength of it would be a guess.
	ClosedAt *string `json:"closedAt,omitempty"`
}

// ConversationMCPServerPayload is one tool server's startup state.
type ConversationMCPServerPayload struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	// Error is the provider's failure text; FailureReason is its classification,
	// which is actionable in a way a message is not.
	Error         string `json:"error,omitempty"`
	FailureReason string `json:"failureReason,omitempty"`
}

// ReloadConversationMCPServersResponse reports the tool servers after a reload.
//
// The list is what the provider reported when asked. Its own startup notifications
// remain the authoritative account and land on the conversation regardless, so a
// client that polls the snapshot will converge on the same answer.
type ReloadConversationMCPServersResponse struct {
	Servers []ConversationMCPServerPayload `json:"servers"`
}

// ConversationUsagePayload is the conversation's token position.
//
// Current state on the snapshot rather than timeline entries: the provider reports
// this after every tool call, and one row per report is what buried the
// conversation before.
type ConversationUsagePayload struct {
	// ContextUsed and ContextWindow are what let a client draw a meter instead of
	// printing a bare number. ContextWindow is 0 when the provider would not state
	// one, and a client must then show the tokens without a fullness claim.
	ContextUsed   int64 `json:"contextUsed"`
	ContextWindow int64 `json:"contextWindow"`
	// The conversation's cumulative spend, which is a different question from
	// fullness: it grows without bound while context rises and falls.
	InputTokens  int64    `json:"inputTokens"`
	OutputTokens int64    `json:"outputTokens"`
	CachedTokens int64    `json:"cachedTokens"`
	TotalTokens  int64    `json:"totalTokens"`
	Cost         *float64 `json:"cost,omitempty"`
	Currency     string   `json:"currency,omitempty"`
}

// ConversationRateLimitsPayload is the account's quota position, which is why a
// turn can fail for reasons that have nothing to do with the request.
type ConversationRateLimitsPayload struct {
	// Percentages in 0..100. Negative means the provider did not report that
	// window, which is not the same as reporting it empty.
	PrimaryUsedPercent   float64 `json:"primaryUsedPercent"`
	SecondaryUsedPercent float64 `json:"secondaryUsedPercent"`
	// Seconds remaining, not the absolute reset instant: a duration cannot read as
	// already-refilled once the snapshot is a few minutes old.
	PrimaryResetsInSeconds   int64  `json:"primaryResetsInSeconds,omitempty"`
	SecondaryResetsInSeconds int64  `json:"secondaryResetsInSeconds,omitempty"`
	PlanLabel                string `json:"planLabel,omitempty"`
	// Title is the name the provider currently gives this thread. Empty means it has
	// none, which is the normal state until something names it.
	Title string `json:"title,omitempty"`
}

// ConversationRequestIDParam is the provider's approval request id. Resolving
// matches on it, so a card left on screen cannot answer a newer request.
type ConversationRequestIDParam struct {
	RequestID string `path:"requestId" description:"Provider approval request identifier. Zero is a legitimate value."`
}

// ConversationConfigIDParam names one provider-advertised session option.
type ConversationConfigIDParam struct {
	ConfigID string `path:"configId" description:"Provider session configuration option identifier."`
}

// ConversationTurnIDParam names one turn in a session's conversation.
type ConversationTurnIDParam struct {
	TurnID string `path:"turnId" description:"Kennel conversation turn identifier, from the snapshot's turns array."`
}

// ConversationBranchIDParam names one durable provider-thread branch.
type ConversationBranchIDParam struct {
	BranchID string `path:"branchId" description:"Conversation branch identifier, from a snapshot branch navigation point."`
}

// RollbackConversationResponse reports what an undo discarded.
type RollbackConversationResponse struct {
	// TurnsDiscarded counts the turns the agent no longer remembers, including the
	// one the caller named. A client can say how much was taken back instead of
	// leaving the user to notice the timeline is shorter.
	TurnsDiscarded int `json:"turnsDiscarded"`
}

// SetConversationTitleRequest names the provider's thread.
type SetConversationTitleRequest struct {
	Title string `json:"title"`
}

// SetConversationTitleResponse echoes the normalized title.
//
// Accepted rather than applied: the provider confirms the name and then reports it
// back on its own event, and that report is what updates Kennel's rows. So this is the
// title Kennel asked for, which is not yet proof the session label has moved.
type SetConversationTitleResponse struct {
	Title string `json:"title"`
}

/* ---- settings ---------------------------------------------------------- */

// SettingsResponse is the daemon-owned preference set.
type SettingsResponse struct {
	// DefaultSessionMode applies to sessions created from now on. Changing it
	// never alters an existing session; only an explicit interface transition can.
	DefaultSessionMode string `json:"defaultSessionMode" enum:"chat,tui"`
	// ChatHarnesses are the agents that can run in chat mode today. Empty means
	// chat cannot be used yet, which a client should say plainly.
	ChatHarnesses     []string                        `json:"chatHarnesses"`
	Reasoning         ReasoningResponse               `json:"reasoning"`
	RepositoryContext RepositoryContextLimitsResponse `json:"repositoryContext"`
}

// ReasoningResponse reports reasoning readiness without returning a secret.
type ReasoningResponse struct {
	// Mode is "direct_api" for explicit Anthropic/OpenAI API credentials or
	// "codex_harness" for the signed-in Codex app-server path.
	Mode       string `json:"mode"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
	Configured bool   `json:"configured"`
	// Ready means only that a call can be attempted: a provider is selected and
	// a matching credential is present. It is not a claim that reasoning works.
	Ready bool `json:"ready"`
	// KeyConfigured means a credential exists for the selected provider. It may
	// still be revoked or mistyped.
	KeyConfigured bool `json:"keyConfigured"`
	// Verified means an actual probe succeeded for exactly the provider and
	// model selected now. It drops back to false when either changes.
	Verified   bool    `json:"verified"`
	VerifiedAt *string `json:"verifiedAt,omitempty"`
	ErrorCode  string  `json:"errorCode,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// UpdateReasoningRequest changes daemon-owned reasoning selection and secret state.
type UpdateReasoningRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
	ClearKey bool   `json:"clearKey,omitempty"`
}

// UpdateSessionInterfaceRequest changes the default interface for new sessions.
type UpdateSessionInterfaceRequest struct {
	DefaultSessionMode string `json:"defaultSessionMode" enum:"chat,tui"`
}

// RepositoryContextLimitsResponse reports the owner's configured bounds for
// Waldo's bounded repository-context packet (intake analysis, planning) and
// the effective values actually in force once Kennel's built-in defaults are
// substituted for anything unconfigured.
type RepositoryContextLimitsResponse struct {
	// MaxFiles/MaxBytes/MaxVisited are the owner's raw override, or null when
	// Kennel's built-in default is in force for that bound.
	MaxFiles   *int64 `json:"maxFiles"`
	MaxBytes   *int64 `json:"maxBytes"`
	MaxVisited *int64 `json:"maxVisited"`
	// EffectiveMaxFiles/EffectiveMaxBytes/EffectiveMaxVisited are what actually
	// governs the next repository-context build: the override above, or
	// Kennel's built-in default when unset. Zero means uncapped.
	EffectiveMaxFiles   int64 `json:"effectiveMaxFiles"`
	EffectiveMaxBytes   int64 `json:"effectiveMaxBytes"`
	EffectiveMaxVisited int64 `json:"effectiveMaxVisited"`
}

// UpdateRepositoryContextLimitsRequest patches the owner's repository-context
// bounds. Omitted fields are preserved, null clears an override back to the
// built-in default, zero explicitly uncaps a bound, and a positive value sets
// the cap. The Set flags are decoder-only presence metadata.
type UpdateRepositoryContextLimitsRequest struct {
	MaxFiles      *int64 `json:"maxFiles,omitempty"`
	MaxBytes      *int64 `json:"maxBytes,omitempty"`
	MaxVisited    *int64 `json:"maxVisited,omitempty"`
	MaxFilesSet   bool   `json:"-"`
	MaxBytesSet   bool   `json:"-"`
	MaxVisitedSet bool   `json:"-"`
}

// UnmarshalJSON retains field presence because encoding/json otherwise maps
// both an omitted pointer and an explicit null to nil. A top-level null is not
// a PATCH object, and unknown fields are rejected instead of being ignored.
func (r *UpdateRepositoryContextLimitsRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("repository-context patch must be a JSON object")
	}
	*r = UpdateRepositoryContextLimitsRequest{}
	for name, raw := range fields {
		var target **int64
		var present *bool
		switch name {
		case "maxFiles":
			target, present = &r.MaxFiles, &r.MaxFilesSet
		case "maxBytes":
			target, present = &r.MaxBytes, &r.MaxBytesSet
		case "maxVisited":
			target, present = &r.MaxVisited, &r.MaxVisitedSet
		default:
			return fmt.Errorf("unknown repository-context field %q", name)
		}
		*present = true
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			*target = nil
			continue
		}
		var value int64
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%s must be an integer or null: %w", name, err)
		}
		*target = &value
	}
	return nil
}

// capabilityNames lists the abilities a provider has, sorted so a client sees a
// stable list rather than Go's map order. Only true entries are named: a
// capability the driver reports as false is one it cannot do, which is the same
// answer as not naming it, and listing both states would invite a client to read
// presence rather than value.
func capabilityNames(caps ports.ChatCapabilities) []string {
	if len(caps) == 0 {
		return nil
	}
	names := make([]string, 0, len(caps))
	for name, has := range caps {
		if has {
			names = append(names, string(name))
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return names
}

// TriggerReviewRequest is the optional body of the review trigger route. An
// empty harness keeps the project's configured reviewer; setting one overrides
// it for this pass only, without editing project config, so one session's choice
// cannot change what another session in the project runs.
type TriggerReviewRequest struct {
	Harness domain.ReviewerHarness `json:"harness,omitempty" enum:"codex"`
}

// ResolveReviewCommentRequest is the body of POST /api/v1/sessions/{sessionId}/reviews/comments/resolve.
type ResolveReviewCommentRequest struct {
	PullRequestURL string `json:"pullRequestUrl,omitempty" description:"Tracked pull request URL. Required when the session has multiple PRs."`
	CommentURL     string `json:"commentUrl" description:"Provider URL of the unresolved review comment to resolve."`
}

// ResolveReviewCommentResponse is returned after Kennel resolves a provider review thread.
type ResolveReviewCommentResponse struct {
	OK bool `json:"ok"`
}

// RequestRereviewRequest is the body of POST /api/v1/sessions/{sessionId}/reviews/rerequest.
type RequestRereviewRequest struct {
	PullRequestURL string `json:"pullRequestUrl,omitempty" description:"Tracked pull request URL. Required when the session has multiple PRs."`
	ReviewerID     string `json:"reviewerId" description:"Provider login of the reviewer to ask for another review."`
}

// RequestRereviewResponse is returned after Kennel asks the SCM provider for another review.
type RequestRereviewResponse struct {
	OK bool `json:"ok"`
}

// MobileDeviceResponse is one row of the desktop's mobile-device roster: the
// stored registration plus whether that phone is running the app right now.
type MobileDeviceResponse struct {
	InstallID            string    `json:"installId"`
	Token                string    `json:"token,omitempty"`
	Platform             string    `json:"platform,omitempty" enum:"ios,android"`
	DeviceName           string    `json:"deviceName,omitempty"`
	Muted                bool      `json:"muted"`
	Live                 bool      `json:"live" description:"True when the phone's app is open and polling."`
	NotificationsEnabled bool      `json:"notificationsEnabled" description:"True when this device has a push token registered."`
	CreatedAt            time.Time `json:"createdAt"`
	LastSeenAt           time.Time `json:"lastSeenAt"`
}

// MobileDevicesResponse is the { devices } envelope for the roster.
type MobileDevicesResponse struct {
	Devices []MobileDeviceResponse `json:"devices"`
}

// MuteDeviceRequest is the body of PATCH /api/v1/mobile/devices/{installId}.
type MuteDeviceRequest struct {
	Muted bool `json:"muted" description:"True to stop sending push notifications to this device."`
}

// InstallIDParam is the {installId} path parameter for mobile-device roster
// routes.
type InstallIDParam struct {
	InstallID string `path:"installId" description:"The device's stable install id."`
}

// OutcomeIDParam is the {outcomeId} path parameter shared by the /outcomes routes.
type OutcomeIDParam struct {
	OutcomeID string `path:"outcomeId" description:"Outcome identifier, e.g. out-<uuid>."`
}

// DecompositionIDParam is the {decompositionId} path parameter on the
// decomposition authorization route.
type DecompositionIDParam struct {
	DecompositionID string `path:"decompositionId" description:"Decomposition revision identifier, e.g. dec-<uuid>."`
}

// AttemptIDParam is the {attemptId} path parameter shared by the attempt routes.
type AttemptIDParam struct {
	AttemptID string `path:"attemptId" description:"Attempt identifier, e.g. att-<uuid>."`
}

// CreateOutcomeRequest is the body for POST /projects/{id}/outcomes.
type CreateOutcomeRequest struct {
	Title           string   `json:"title"`
	Goal            string   `json:"goal"`
	SuccessCriteria []string `json:"successCriteria"`
	Review          string   `json:"review"`
	Constraints     []string `json:"constraints,omitempty"`
	NonGoals        []string `json:"nonGoals,omitempty"`
	Clarification   string   `json:"clarification,omitempty"`
	// AuthorityCeiling is the maximum authority any WorkUnit under this
	// Contract may ever be granted. Omitting it creates an Outcome that can
	// never be planned, because planning derives the least capabilities each
	// unit needs and then checks them against this ceiling.
	AuthorityCeiling IntakeAuthority `json:"authorityCeiling,omitempty"`
	// StopConditions name the moments a human must decide before work
	// continues.
	StopConditions []string `json:"stopConditions,omitempty"`
	RequestKey     string   `json:"requestKey"`
}

// ReviseOutcomeContractRequest is the body for POST
// /outcomes/{outcomeId}/revisions. ExpectedRevision must name the current
// contract revision; anything else is a 409 conflict.
type ReviseOutcomeContractRequest struct {
	ExpectedRevision int64    `json:"expectedRevision"`
	Goal             string   `json:"goal"`
	SuccessCriteria  []string `json:"successCriteria"`
	Review           string   `json:"review"`
	Constraints      []string `json:"constraints,omitempty"`
	NonGoals         []string `json:"nonGoals,omitempty"`
	Clarification    string   `json:"clarification,omitempty"`
	// AuthorityCeiling must be restated on every revision. A revision is a new
	// immutable Contract, not a patch: omitting the ceiling here would grant
	// the successor no authority at all and quietly make the Outcome
	// unplannable.
	AuthorityCeiling IntakeAuthority `json:"authorityCeiling,omitempty"`
	StopConditions   []string        `json:"stopConditions,omitempty"`
	// CriterionEvidence binds by position to the newly assigned criterion IDs.
	CriterionEvidence [][]string    `json:"criterionEvidence,omitempty"`
	TemporalCondition *string       `json:"temporalCondition,omitempty"`
	Facets            []IntakeFacet `json:"facets,omitempty"`
}

// ContractRevisionResponse is one immutable contract revision.
type ContractRevisionResponse struct {
	ID                   string                              `json:"id"`
	Number               int64                               `json:"number"`
	Goal                 string                              `json:"goal"`
	Criteria             []ContractCriterionResponse         `json:"criteria"`
	SuccessCriteria      []string                            `json:"successCriteria"`
	Review               string                              `json:"review"`
	Constraints          []string                            `json:"constraints"`
	NonGoals             []string                            `json:"nonGoals"`
	Clarification        string                              `json:"clarification,omitempty"`
	EvidenceExpectations []IntakeEvidenceExpectationResponse `json:"evidenceExpectations,omitempty"`
	AuthorityCeiling     IntakeAuthority                     `json:"authorityCeiling,omitempty"`
	StopConditions       []string                            `json:"stopConditions,omitempty"`
	TemporalCondition    *string                             `json:"temporalCondition,omitempty"`
	Facets               []IntakeFacet                       `json:"facets,omitempty"`
	CreatedAt            time.Time                           `json:"createdAt"`
}

// ContractCriterionResponse exposes stable criterion identity. Proof binds
// (contractRevisionId, criterionId), never mutable display text alone.
type ContractCriterionResponse struct {
	CriterionID        string `json:"criterionId"`
	ContractRevisionID string `json:"contractRevisionId"`
	Position           int64  `json:"position"`
	Text               string `json:"text"`
}

// OutcomeResponse is the canonical Outcome read model: durable facts plus the
// full immutable revision history. Project listings also include the latest
// durable plan fact so callers can derive stage without transcript inspection.
type OutcomeResponse struct {
	ID      string `json:"id"`
	SpaceID string `json:"spaceId"`
	// ParentID names the Outcome this one contributes to, absent for a
	// Project-level Outcome (ADR 0007).
	ParentID              string                     `json:"parentId,omitempty"`
	Title                 string                     `json:"title"`
	CurrentRevisionNumber int64                      `json:"currentRevisionNumber"`
	Current               ContractRevisionResponse   `json:"currentRevision"`
	History               []ContractRevisionResponse `json:"history"`
	LatestPlan            *PlanRevisionResponse      `json:"latestPlan,omitempty"`
	CreatedAt             time.Time                  `json:"createdAt"`
	UpdatedAt             time.Time                  `json:"updatedAt"`
}

// OutcomeEnvelope is the { outcome } response body for Outcome reads and writes.
type OutcomeEnvelope struct {
	Outcome OutcomeResponse `json:"outcome"`
}

// OutcomesEnvelope is the stable project-scoped collection response. Each
// entry carries its current immutable contract so dashboard re-entry never
// depends on provider transcripts.
type OutcomesEnvelope struct {
	Outcomes []OutcomeResponse `json:"outcomes"`
}

func contractRevisionResponse(rev domain.ContractRevision) ContractRevisionResponse {
	criteria := make([]ContractCriterionResponse, 0, len(rev.Criteria))
	for _, criterion := range rev.Criteria {
		criteria = append(criteria, contractCriterionResponse(criterion))
	}
	evidence := make([]IntakeEvidenceExpectationResponse, 0, len(rev.EvidenceExpectations))
	for _, expectation := range rev.EvidenceExpectations {
		evidence = append(evidence, IntakeEvidenceExpectationResponse{CriterionID: string(expectation.CriterionID), Descriptions: expectation.Descriptions})
	}
	return ContractRevisionResponse{
		ID:                   string(rev.ID),
		Number:               rev.Number,
		Goal:                 rev.Goal,
		Criteria:             criteria,
		SuccessCriteria:      rev.SuccessCriteria,
		Review:               rev.Review,
		Constraints:          rev.Constraints,
		NonGoals:             rev.NonGoals,
		Clarification:        rev.Clarification,
		EvidenceExpectations: evidence,
		AuthorityCeiling:     intakeAuthority(rev.AuthorityCeiling), StopConditions: rev.StopConditions,
		TemporalCondition: rev.TemporalCondition, Facets: intakeFacets(rev.Facets),
		CreatedAt: rev.CreatedAt,
	}
}

// CreateIntakeRequest captures one statement and identifier-only provenance.
// Transcript bodies never cross this persistence contract.
type CreateIntakeRequest struct {
	SourceSurface    string                       `json:"sourceSurface" enum:"home,work"`
	Statement        string                       `json:"statement" maxLength:"4096"`
	SourceOpenLoopID string                       `json:"sourceOpenLoopId,omitempty"`
	ConversationRefs []IntakeConversationRefInput `json:"conversationRefs,omitempty"`
	RequestKey       string                       `json:"requestKey"`
}

// IntakeConversationRefInput references an owning episode and turn.
type IntakeConversationRefInput struct {
	EpisodeID string `json:"episodeId"`
	TurnID    string `json:"turnId"`
	Position  int64  `json:"position"`
}

// AnalyzeIntakeRequest guards analysis with an expected proposal revision.
type AnalyzeIntakeRequest struct {
	ExpectedProposalRevision int64 `json:"expectedProposalRevision"`
	// RepositoryToolUse explicitly authorizes the selected native reasoner to
	// inspect the registered repository read-only for this call.
	RepositoryToolUse bool `json:"repositoryToolUse,omitempty"`
	// Offline runs the deterministic baseline instead of asking an agent. It
	// is how a person stops waiting and takes the proposal that is always
	// available, and it asks no agent anything.
	Offline bool `json:"offline,omitempty"`
}

// AnswerIntakeClarificationRequest answers the single material question.
type AnswerIntakeClarificationRequest struct {
	ExpectedProposalRevision int64  `json:"expectedProposalRevision"`
	Answer                   string `json:"answer"`
	RepositoryToolUse        bool   `json:"repositoryToolUse,omitempty"`
}

// ReviseIntakeProposalRequest appends a reviewed immutable proposal.
type ReviseIntakeProposalRequest struct {
	ExpectedProposalRevision int64               `json:"expectedProposalRevision"`
	Proposal                 IntakeProposalInput `json:"proposal"`
}

// ConfirmIntakeRequest explicitly confirms the current proposal revision.
type ConfirmIntakeRequest struct {
	ExpectedProposalRevision int64  `json:"expectedProposalRevision"`
	RequestKey               string `json:"requestKey"`
}

// CancelIntakeRequest consciously releases an unconfirmed intake.
type CancelIntakeRequest struct {
	ExpectedProposalRevision int64  `json:"expectedProposalRevision"`
	Reason                   string `json:"reason"`
}

// IntakeProposalInput is the editable typed Contract proposal stable core.
type IntakeProposalInput struct {
	Title              string                 `json:"title"`
	DesiredState       string                 `json:"desiredState"`
	Criteria           []IntakeCriterionInput `json:"criteria"`
	ReviewMethod       string                 `json:"reviewMethod"`
	Constraints        []string               `json:"constraints,omitempty"`
	NonGoals           []string               `json:"nonGoals,omitempty"`
	AuthorityCeiling   IntakeAuthority        `json:"authorityCeiling"`
	StopConditions     []string               `json:"stopConditions"`
	ClarificationNotes []string               `json:"clarificationNotes,omitempty"`
	TemporalCondition  *string                `json:"temporalCondition,omitempty"`
	Facets             []IntakeFacet          `json:"facets"`
}

// IntakeCriterionInput carries stable identity and expected evidence.
type IntakeCriterionInput struct {
	ID               string   `json:"id,omitempty"`
	Text             string   `json:"text"`
	EvidenceExpected []string `json:"evidenceExpected"`
}

// IntakeAuthority is the proposed least-privilege ceiling.
type IntakeAuthority struct {
	ReadWorkspace  bool `json:"readWorkspace"`
	WriteWorkspace bool `json:"writeWorkspace"`
	ExecuteLocal   bool `json:"executeLocal"`
	UseNetwork     bool `json:"useNetwork"`
	CommitLocal    bool `json:"commitLocal"`
	CreatePR       bool `json:"createPr"`
	Deploy         bool `json:"deploy"`
	ExternalEffect bool `json:"externalEffect"`
}

// IntakeFacet is one adaptive, typed extension of the stable core.
type IntakeFacet struct {
	Kind         string   `json:"kind" enum:"software,research,design,documentation,investigation,evaluation,operations"`
	Summary      string   `json:"summary"`
	Requirements []string `json:"requirements,omitempty"`
}

// IntakeSessionResponse is the durable shared Home/Work state machine value.
type IntakeSessionResponse struct {
	ID                      string    `json:"id"`
	SourceSurface           string    `json:"sourceSurface"`
	Purpose                 string    `json:"purpose"`
	ProjectID               string    `json:"projectId,omitempty"`
	SourceOpenLoopID        string    `json:"sourceOpenLoopId,omitempty"`
	Statement               string    `json:"statement"`
	Status                  string    `json:"status"`
	CurrentProposalRevision int64     `json:"currentProposalRevision"`
	ClarificationCount      int64     `json:"clarificationCount"`
	ConfirmedOutcomeID      string    `json:"confirmedOutcomeId,omitempty"`
	FailureCode             string    `json:"failureCode,omitempty"`
	CancellationReason      string    `json:"cancellationReason,omitempty"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

// IntakeConversationRefResponse returns provenance identifiers only.
type IntakeConversationRefResponse struct {
	EpisodeID string `json:"episodeId"`
	TurnID    string `json:"turnId"`
	Position  int64  `json:"position"`
}

// IntakeCriterionResponse returns one proposed criterion and evidence needs.
type IntakeCriterionResponse struct {
	ID               string   `json:"id"`
	Text             string   `json:"text"`
	EvidenceExpected []string `json:"evidenceExpected"`
}

// IntakeProposalResponse returns an immutable typed proposal revision.
type IntakeProposalResponse struct {
	ID                 string                    `json:"id"`
	Revision           int64                     `json:"revision"`
	Title              string                    `json:"title"`
	DesiredState       string                    `json:"desiredState"`
	Criteria           []IntakeCriterionResponse `json:"criteria"`
	ReviewMethod       string                    `json:"reviewMethod"`
	Constraints        []string                  `json:"constraints"`
	NonGoals           []string                  `json:"nonGoals"`
	AuthorityCeiling   IntakeAuthority           `json:"authorityCeiling"`
	StopConditions     []string                  `json:"stopConditions"`
	ClarificationNotes []string                  `json:"clarificationNotes"`
	TemporalCondition  *string                   `json:"temporalCondition,omitempty"`
	Facets             []IntakeFacet             `json:"facets"`
	CreatedAt          time.Time                 `json:"createdAt"`
}

// IntakeClarificationResponse returns the bounded material question.
type IntakeClarificationResponse struct {
	ID                  string     `json:"id"`
	Question            string     `json:"question"`
	Reason              string     `json:"reason"`
	Recommendation      string     `json:"recommendation"`
	Alternatives        []string   `json:"alternatives"`
	DeferralConsequence string     `json:"deferralConsequence"`
	Answer              string     `json:"answer,omitempty"`
	AnsweredAt          *time.Time `json:"answeredAt,omitempty"`
}

// IntakeOutcomeResponse identifies the canonical Outcome created on confirmation.
type IntakeOutcomeResponse struct {
	ID                    string    `json:"id"`
	SpaceID               string    `json:"spaceId"`
	Title                 string    `json:"title"`
	CurrentRevisionNumber int64     `json:"currentRevisionNumber"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

// IntakeSnapshotResponse is the complete shared intake read model.
type IntakeSnapshotResponse struct {
	Session           IntakeSessionResponse           `json:"session"`
	ConversationRefs  []IntakeConversationRefResponse `json:"conversationRefs"`
	Proposal          *IntakeProposalResponse         `json:"proposal,omitempty"`
	Clarification     *IntakeClarificationResponse    `json:"clarification,omitempty"`
	ConfirmedOutcome  *IntakeOutcomeResponse          `json:"confirmedOutcome,omitempty"`
	ConfirmedContract *ContractRevisionResponse       `json:"confirmedContract,omitempty"`
}

// SubmitIntakeAnalysisRequest is what a spawned agent posts back on the
// intake callback. It carries the SAME proposal shape a hand-authored revision
// uses: an agent gets no special vocabulary, so its draft passes exactly the
// same validation.
//
// Exactly one of Proposal or Clarification may be present, matching the
// at-most-one-material-question rule the offline analyzer already obeys.
type SubmitIntakeAnalysisRequest struct {
	Proposal      *IntakeProposalInput      `json:"proposal,omitempty"`
	Clarification *IntakeClarificationInput `json:"clarification,omitempty"`
}

// IntakeClarificationInput is one bounded material question an agent may ask
// instead of proposing.
type IntakeClarificationInput struct {
	Question            string   `json:"question"`
	Reason              string   `json:"reason"`
	Recommendation      string   `json:"recommendation"`
	Alternatives        []string `json:"alternatives,omitempty"`
	DeferralConsequence string   `json:"deferralConsequence"`
}

// IntakeAnalysisRequestResponse is one durable ask for an agent-authored
// Contract proposal, and what became of it.
//
// The callback token is deliberately absent: only its digest is ever stored,
// and the raw token exists solely inside the brief handed to the one session
// asked to answer.
type IntakeAnalysisRequestResponse struct {
	ID                       string    `json:"id"`
	IntakeID                 string    `json:"intakeId"`
	ExpectedProposalRevision int64     `json:"expectedProposalRevision"`
	Status                   string    `json:"status"`
	SessionID                string    `json:"sessionId,omitempty"`
	Harness                  string    `json:"harness,omitempty"`
	ExpiresAt                time.Time `json:"expiresAt"`
	// Expired is derived from the clock rather than stored, so a request that
	// timed out while the daemon was down still reads as expired.
	Expired       bool       `json:"expired"`
	RawProposal   string     `json:"rawProposal,omitempty"`
	RefusalReason string     `json:"refusalReason,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	AnsweredAt    *time.Time `json:"answeredAt,omitempty"`
}

// IntakeAnalysisRequestIDParam is the {requestId} path parameter on the intake
// callback route. The callback is addressed by REQUEST rather than by intake:
// the answering agent knows only the request it was given.
type IntakeAnalysisRequestIDParam struct {
	RequestID string `path:"requestId" description:"Intake analysis request identifier, e.g. ireq-<uuid>."`
}

// IntakeAnalysisRequestEnvelope wraps one ask.
type IntakeAnalysisRequestEnvelope struct {
	Request IntakeAnalysisRequestResponse `json:"request"`
}

// IntakeEnvelope wraps an intake API response.
type IntakeEnvelope struct {
	Intake IntakeSnapshotResponse `json:"intake"`
}

// OpenWaldoEpisodeRequest starts one bounded provider-neutral episode.
type OpenWaldoEpisodeRequest struct {
	ExpectedRevision int64                            `json:"expectedRevision"`
	ProviderRef      *WaldoProviderEpisodeRefResponse `json:"providerRef,omitempty"`
	RequestKey       string                           `json:"requestKey"`
}

// AppendWaldoTurnRequest appends one visible ordered turn.
type AppendWaldoTurnRequest struct {
	ExpectedRevision     int64                         `json:"expectedRevision"`
	EpisodeID            string                        `json:"episodeId"`
	Role                 string                        `json:"role" enum:"user,waldo"`
	Message              string                        `json:"message" maxLength:"65536"`
	ProviderRef          *WaldoProviderTurnRefResponse `json:"providerRef,omitempty"`
	ContextAttachmentIDs []string                      `json:"contextAttachmentIds,omitempty"`
	RequestKey           string                        `json:"requestKey"`
}

// AttachWaldoContextRequest explicitly attaches a canonical Project object.
type AttachWaldoContextRequest struct {
	ExpectedRevision int64                   `json:"expectedRevision"`
	Ref              WaldoContextRefResponse `json:"ref"`
	RequestKey       string                  `json:"requestKey"`
}

// DetachWaldoContextRequest consciously releases context from future turns.
type DetachWaldoContextRequest struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	Reason           string `json:"reason"`
	RequestKey       string `json:"requestKey"`
}

// ContinueWaldoConversationRequest asks daemon policy to evaluate a bounded
// provider continuation. Caller values are proposals; canonical facts decide.
type ContinueWaldoConversationRequest struct {
	FromAgentSessionRef string                            `json:"fromAgentSessionRef"`
	Reason              string                            `json:"reason"`
	ReasonDetail        string                            `json:"reasonDetail"`
	TriggerEvidence     WaldoContinuationEvidenceResponse `json:"triggerEvidence"`
	ContextDigest       string                            `json:"contextDigest"`
	ContextRefs         []WaldoContextRefResponse         `json:"contextRefs,omitempty"`
	PreviousBindings    WaldoContinuationBindingsResponse `json:"previousBindings"`
	ReplacementBindings WaldoContinuationBindingsResponse `json:"replacementBindings"`
	EffectsKnown        bool                              `json:"effectsKnown"`
	LostMaterialContext bool                              `json:"lostMaterialContext"`
	SourceRevoked       bool                              `json:"sourceRevoked"`
	FreshVerifier       bool                              `json:"freshVerifier"`
	RequestKey          string                            `json:"requestKey"`
}

// WaldoConversationResponse is the durable Project aggregate root.
type WaldoConversationResponse struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"projectId"`
	Revision           int64     `json:"revision"`
	LatestTurnSequence int64     `json:"latestTurnSequence"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// WaldoProviderEpisodeRefResponse identifies a provider-native episode without copying its transcript.
type WaldoProviderEpisodeRefResponse struct {
	Provider               string `json:"provider"`
	ProviderConversationID string `json:"providerConversationId,omitempty"`
	TranscriptRef          string `json:"transcriptRef,omitempty"`
}

// WaldoProviderTurnRefResponse identifies one provider-native turn without copying its transcript.
type WaldoProviderTurnRefResponse struct {
	Provider               string `json:"provider"`
	ProviderConversationID string `json:"providerConversationId,omitempty"`
	ProviderTurnID         string `json:"providerTurnId"`
	TranscriptRef          string `json:"transcriptRef,omitempty"`
}

// WaldoConversationEpisodeResponse describes one bounded provider-neutral conversation episode.
type WaldoConversationEpisodeResponse struct {
	ID             string                           `json:"id"`
	ConversationID string                           `json:"conversationId"`
	ProjectID      string                           `json:"projectId"`
	Ordinal        int64                            `json:"ordinal"`
	State          string                           `json:"state"`
	ProviderRef    *WaldoProviderEpisodeRefResponse `json:"providerRef,omitempty"`
	CreatedAt      time.Time                        `json:"createdAt"`
	SealedAt       *time.Time                       `json:"sealedAt,omitempty"`
	SealReason     string                           `json:"sealReason,omitempty"`
}

// WaldoContextProvenanceResponse records why a canonical object was attached.
type WaldoContextProvenanceResponse struct {
	Kind     string `json:"kind"`
	SourceID string `json:"sourceId"`
}

// WaldoContextRefResponse identifies a canonical Project-bound object at an exact revision.
type WaldoContextRefResponse struct {
	Kind       string                         `json:"kind"`
	ObjectID   string                         `json:"objectId"`
	Revision   string                         `json:"revision,omitempty"`
	Provenance WaldoContextProvenanceResponse `json:"provenance"`
}

// WaldoContextAttachmentResponse describes the explicit attach and detach lifecycle of one context ref.
type WaldoContextAttachmentResponse struct {
	ID               string                  `json:"id"`
	ConversationID   string                  `json:"conversationId"`
	ProjectID        string                  `json:"projectId"`
	Ref              WaldoContextRefResponse `json:"ref"`
	AttachedRevision int64                   `json:"attachedRevision"`
	DetachedRevision int64                   `json:"detachedRevision,omitempty"`
	Active           bool                    `json:"active"`
	CreatedAt        time.Time               `json:"createdAt"`
	DetachedAt       *time.Time              `json:"detachedAt,omitempty"`
	DetachReason     string                  `json:"detachReason,omitempty"`
}

// WaldoConversationTurnResponse is one ordered visible Project conversation turn.
type WaldoConversationTurnResponse struct {
	ID             string                        `json:"id"`
	ConversationID string                        `json:"conversationId"`
	EpisodeID      string                        `json:"episodeId"`
	ProjectID      string                        `json:"projectId"`
	Sequence       int64                         `json:"sequence"`
	Role           string                        `json:"role"`
	Message        string                        `json:"message"`
	ProviderRef    *WaldoProviderTurnRefResponse `json:"providerRef,omitempty"`
	ContextRefs    []WaldoContextRefResponse     `json:"contextRefs"`
	CreatedAt      time.Time                     `json:"createdAt"`
}

// WaldoContinuationEvidenceResponse identifies the exact continuation trigger evidence.
type WaldoContinuationEvidenceResponse struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

// WaldoContinuationBindingsResponse captures canonical facts that must remain equal for automatic continuation.
type WaldoContinuationBindingsResponse struct {
	ProjectID          string `json:"projectId"`
	OutcomeID          string `json:"outcomeId"`
	ContractRevisionID string `json:"contractRevisionId"`
	PlanRevisionID     string `json:"planRevisionId"`
	WorkUnitID         string `json:"workUnitId"`
	AttemptID          string `json:"attemptId"`
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	Profile            string `json:"profile"`
	Role               string `json:"role"`
	AuthorityDigest    string `json:"authorityDigest"`
	BudgetDigest       string `json:"budgetDigest"`
	WorkspaceOwner     string `json:"workspaceOwner"`
	EffectPolicyDigest string `json:"effectPolicyDigest"`
}

// WaldoContinuationReceiptResponse records the durable decision and replacement lineage for one continuation.
type WaldoContinuationReceiptResponse struct {
	ID                           string                            `json:"id"`
	OperationID                  string                            `json:"operationId"`
	ConversationID               string                            `json:"conversationId"`
	ProjectID                    string                            `json:"projectId"`
	FromEpisodeID                string                            `json:"fromEpisodeId"`
	ToEpisodeID                  string                            `json:"toEpisodeId,omitempty"`
	FromAgentSessionRef          string                            `json:"fromAgentSessionRef"`
	ToAgentSessionRef            string                            `json:"toAgentSessionRef,omitempty"`
	Action                       string                            `json:"action"`
	Reason                       string                            `json:"reason"`
	ReasonDetail                 string                            `json:"reasonDetail"`
	TriggerEvidence              WaldoContinuationEvidenceResponse `json:"triggerEvidence"`
	MaterialChange               bool                              `json:"materialChange"`
	ChangedFields                []string                          `json:"changedFields"`
	ContextDigest                string                            `json:"contextDigest"`
	ContextRefs                  []WaldoContextRefResponse         `json:"contextRefs"`
	PreviousBindings             WaldoContinuationBindingsResponse `json:"previousBindings"`
	ReplacementBindings          WaldoContinuationBindingsResponse `json:"replacementBindings"`
	EffectsKnown                 bool                              `json:"effectsKnown"`
	OldSessionFenced             bool                              `json:"oldSessionFenced"`
	ReplacementIdentityConfirmed bool                              `json:"replacementIdentityConfirmed"`
	FenceReceiptRef              string                            `json:"fenceReceiptRef,omitempty"`
	ReconciliationRef            string                            `json:"reconciliationRef,omitempty"`
	NeedsUserReason              string                            `json:"needsUserReason,omitempty"`
	CreatedAt                    time.Time                         `json:"createdAt"`
}

// WaldoConversationSnapshotResponse is exact restart-safe daemon truth.
type WaldoConversationSnapshotResponse struct {
	Conversation         WaldoConversationResponse          `json:"conversation"`
	Episodes             []WaldoConversationEpisodeResponse `json:"episodes"`
	Turns                []WaldoConversationTurnResponse    `json:"turns"`
	ContextAttachments   []WaldoContextAttachmentResponse   `json:"contextAttachments"`
	ContinuationReceipts []WaldoContinuationReceiptResponse `json:"continuationReceipts"`
}

// WaldoConversationEnvelope wraps the current durable conversation snapshot.
type WaldoConversationEnvelope struct {
	WaldoConversation WaldoConversationSnapshotResponse `json:"waldoConversation"`
}

// WaldoTurnEnvelope wraps an appended turn and the resulting canonical snapshot.
type WaldoTurnEnvelope struct {
	Turn              WaldoConversationTurnResponse     `json:"turn"`
	WaldoConversation WaldoConversationSnapshotResponse `json:"waldoConversation"`
}

// WaldoContinuationEnvelope wraps one durable continuation receipt.
type WaldoContinuationEnvelope struct {
	ContinuationReceipt WaldoContinuationReceiptResponse `json:"continuationReceipt"`
}

// WaldoContextAttachmentIDParam describes the context attachment route parameter.
type WaldoContextAttachmentIDParam struct {
	AttachmentID string `path:"attachmentId"`
}

// IntakeIDParam describes the intake route parameter.
type IntakeIDParam struct {
	IntakeID string `path:"intakeId"`
}

// CreateResponsibilityLinkRequest explicitly records Home-to-Work lineage.
type CreateResponsibilityLinkRequest struct {
	ProjectID            string `json:"projectId"`
	SourceOpenLoopID     string `json:"sourceOpenLoopId"`
	DestinationOutcomeID string `json:"destinationOutcomeId"`
	Reason               string `json:"reason"`
	RequestKey           string `json:"requestKey"`
}

// EndResponsibilityLinkRequest ends lineage without changing responsibilities.
type EndResponsibilityLinkRequest struct {
	Reason string `json:"reason"`
}

// ResponsibilityLinkResponse returns explicit independent lineage.
type ResponsibilityLinkResponse struct {
	ID                   string     `json:"id"`
	SourceOpenLoopID     string     `json:"sourceOpenLoopId"`
	DestinationOutcomeID string     `json:"destinationOutcomeId"`
	Creator              string     `json:"creator"`
	Reason               string     `json:"reason"`
	CreatedAt            time.Time  `json:"createdAt"`
	EndedAt              *time.Time `json:"endedAt,omitempty"`
	EndedBy              string     `json:"endedBy,omitempty"`
	EndedReason          string     `json:"endedReason,omitempty"`
}

// ResponsibilityLinkEnvelope wraps a lineage API response.
type ResponsibilityLinkEnvelope struct {
	ResponsibilityLink ResponsibilityLinkResponse `json:"responsibilityLink"`
}

// ResponsibilityLinkIDParam describes the lineage route parameter.
type ResponsibilityLinkIDParam struct {
	ResponsibilityLinkID string `path:"responsibilityLinkId"`
}

// IntakeEvidenceExpectationResponse maps evidence needs to stable criteria.
type IntakeEvidenceExpectationResponse struct {
	CriterionID  string   `json:"criterionId"`
	Descriptions []string `json:"descriptions"`
}

func intakeAuthority(value domain.ProposedAuthority) IntakeAuthority {
	return IntakeAuthority{ReadWorkspace: value.ReadWorkspace, WriteWorkspace: value.WriteWorkspace, ExecuteLocal: value.ExecuteLocal, UseNetwork: value.UseNetwork, CommitLocal: value.CommitLocal, CreatePR: value.CreatePR, Deploy: value.Deploy, ExternalEffect: value.ExternalEffect}
}

// proposedAuthority is intakeAuthority's inverse, for request bodies that
// state a ceiling rather than report one.
func proposedAuthority(value IntakeAuthority) domain.ProposedAuthority {
	return domain.ProposedAuthority{ReadWorkspace: value.ReadWorkspace, WriteWorkspace: value.WriteWorkspace, ExecuteLocal: value.ExecuteLocal, UseNetwork: value.UseNetwork, CommitLocal: value.CommitLocal, CreatePR: value.CreatePR, Deploy: value.Deploy, ExternalEffect: value.ExternalEffect}
}

func intakeFacets(values []domain.ContractFacet) []IntakeFacet {
	out := make([]IntakeFacet, 0, len(values))
	for _, value := range values {
		out = append(out, IntakeFacet{Kind: string(value.Kind), Summary: value.Summary, Requirements: value.Requirements})
	}
	return out
}

func contractCriterionResponse(criterion domain.ContractCriterion) ContractCriterionResponse {
	return ContractCriterionResponse{
		CriterionID: string(criterion.ID), ContractRevisionID: string(criterion.ContractRevisionID),
		Position: criterion.Position, Text: criterion.Text,
	}
}

// RecordEvidenceRequest appends one provenance-bearing fact to an exact
// current criterion and subject revision.
type RecordEvidenceRequest struct {
	ExpectedContractRevision int64  `json:"expectedContractRevision"`
	ContractRevisionID       string `json:"contractRevisionId"`
	CriterionID              string `json:"criterionId"`
	SubjectType              string `json:"subjectType"`
	SubjectID                string `json:"subjectId"`
	// SubjectRevision identifies which version of the subject was examined:
	// the Contract revision for an Outcome, the plan id for a Plan or WorkUnit,
	// and for an Attempt the retained artifact version its result was checked
	// against. An Attempt with nothing retained yet cannot carry proof.
	SubjectRevision string `json:"subjectRevision"`
	Kind            string `json:"kind"`
	SourceType      string `json:"sourceType"`
	SourceRef       string `json:"sourceRef"`
	ProducerType    string `json:"producerType"`
	ProducerRef     string `json:"producerRef"`
	Summary         string `json:"summary"`
	ContentDigest   string `json:"contentDigest"`
	RequestKey      string `json:"requestKey"`
}

// RecordVerificationRequest declares what was checked and the verifier's
// actual independence from the producer. It cannot accept an Outcome.
type RecordVerificationRequest struct {
	ExpectedContractRevision int64  `json:"expectedContractRevision"`
	ContractRevisionID       string `json:"contractRevisionId"`
	CriterionID              string `json:"criterionId"`
	SubjectType              string `json:"subjectType"`
	SubjectID                string `json:"subjectId"`
	// SubjectRevision identifies which version of the subject was examined:
	// the Contract revision for an Outcome, the plan id for a Plan or WorkUnit,
	// and for an Attempt the retained artifact version its result was checked
	// against. An Attempt with nothing retained yet cannot carry proof.
	SubjectRevision   string   `json:"subjectRevision"`
	EvidenceItemIDs   []string `json:"evidenceItemIds"`
	Method            string   `json:"method"`
	IndependenceClass string   `json:"independenceClass"`
	Result            string   `json:"result"`
	ProducerRef       string   `json:"producerRef,omitempty"`
	VerifierRef       string   `json:"verifierRef"`
	ProducerProvider  string   `json:"producerProvider,omitempty"`
	VerifierProvider  string   `json:"verifierProvider,omitempty"`
	Detail            string   `json:"detail,omitempty"`
	RequestKey        string   `json:"requestKey"`
}

// DecideAcceptanceRequest is the sole API authority that may append a user
// AcceptanceDecision. Rework/reopen require explicit re-entry lineage.
type DecideAcceptanceRequest struct {
	ExpectedContractRevision int64  `json:"expectedContractRevision"`
	ContractRevisionID       string `json:"contractRevisionId"`
	Kind                     string `json:"kind"`
	Summary                  string `json:"summary"`
	ResourceDisposition      string `json:"resourceDisposition"`
	ReentryTargetType        string `json:"reentryTargetType,omitempty"`
	ReentryTargetID          string `json:"reentryTargetId,omitempty"`
	RequestKey               string `json:"requestKey"`
}

// EvidenceItemResponse exposes immutable Evidence provenance and binding.
type EvidenceItemResponse struct {
	ID                 string    `json:"id"`
	ContractRevisionID string    `json:"contractRevisionId"`
	CriterionID        string    `json:"criterionId"`
	SubjectType        string    `json:"subjectType"`
	SubjectID          string    `json:"subjectId"`
	SubjectRevision    string    `json:"subjectRevision"`
	Kind               string    `json:"kind"`
	SourceType         string    `json:"sourceType"`
	SourceRef          string    `json:"sourceRef"`
	ProducerType       string    `json:"producerType"`
	ProducerRef        string    `json:"producerRef"`
	Summary            string    `json:"summary"`
	ContentDigest      string    `json:"contentDigest"`
	CreatedAt          time.Time `json:"createdAt"`
}

// VerificationRunResponse exposes the actual method and independence class.
type VerificationRunResponse struct {
	ID                 string    `json:"id"`
	ContractRevisionID string    `json:"contractRevisionId"`
	CriterionID        string    `json:"criterionId"`
	SubjectType        string    `json:"subjectType"`
	SubjectID          string    `json:"subjectId"`
	SubjectRevision    string    `json:"subjectRevision"`
	EvidenceItemIDs    []string  `json:"evidenceItemIds"`
	Method             string    `json:"method"`
	IndependenceClass  string    `json:"independenceClass"`
	Independent        bool      `json:"independent"`
	Result             string    `json:"result"`
	ProducerRef        string    `json:"producerRef,omitempty"`
	VerifierRef        string    `json:"verifierRef"`
	ProducerProvider   string    `json:"producerProvider,omitempty"`
	VerifierProvider   string    `json:"verifierProvider,omitempty"`
	Detail             string    `json:"detail,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
}

// AcceptanceDecisionResponse exposes one explicit user decision.
type AcceptanceDecisionResponse struct {
	ID                  string    `json:"id"`
	ContractRevisionID  string    `json:"contractRevisionId"`
	Kind                string    `json:"kind"`
	ActorType           string    `json:"actorType"`
	Summary             string    `json:"summary"`
	ResourceDisposition string    `json:"resourceDisposition"`
	CreatedAt           time.Time `json:"createdAt"`
}

// OutcomeCorrectionResponse exposes the durable Work re-entry target.
type OutcomeCorrectionResponse struct {
	ID                 string    `json:"id"`
	DecisionID         string    `json:"decisionId"`
	ContractRevisionID string    `json:"contractRevisionId"`
	Feedback           string    `json:"feedback"`
	TargetType         string    `json:"targetType"`
	TargetID           string    `json:"targetId"`
	CreatedAt          time.Time `json:"createdAt"`
}

// CriterionProofResponse groups proof facts for one stable criterion.
type CriterionProofResponse struct {
	CriterionID        string                    `json:"criterionId"`
	ContractRevisionID string                    `json:"contractRevisionId"`
	Position           int64                     `json:"position"`
	Text               string                    `json:"text"`
	Ready              bool                      `json:"ready"`
	Gap                string                    `json:"gap,omitempty"`
	Evidence           []EvidenceItemResponse    `json:"evidence"`
	Verifications      []VerificationRunResponse `json:"verifications"`
}

// OutcomeResultArtifactResponse is the user-facing identity of one retained
// artifact generation referenced by proof. The digest is content identity, not
// a claim that the artifact was accepted.
type OutcomeResultArtifactResponse struct {
	Revision  string `json:"revision"`
	SourceRef string `json:"sourceRef"`
	Digest    string `json:"digest"`
}

// OutcomeResultCheckResponse summarizes one daemon-owned deterministic check.
// Uncertainty is explicit: an interrupted or unmeasurable check is not a red
// verdict and must not be presented as one.
type OutcomeResultCheckResponse struct {
	CriterionID      string `json:"criterionId"`
	Command          string `json:"command"`
	ArtifactRevision string `json:"artifactRevision"`
	Verdict          string `json:"verdict"`
	Detail           string `json:"detail,omitempty"`
	Uncertain        bool   `json:"uncertain"`
}

// OutcomeResultChangeFileResponse is one measured file change from Kennel's
// own receipt of the leased workspace.
type OutcomeResultChangeFileResponse struct {
	Path       string `json:"path"`
	ChangeKind string `json:"changeKind"`
	Digest     string `json:"digest,omitempty"`
}

// OutcomeResultChangesResponse projects one retained Attempt's measured file
// changes. The receipt, not the provider, is the source; truncated is
// explicit when the projection shortens the list.
type OutcomeResultChangesResponse struct {
	AttemptID       string                            `json:"attemptId"`
	WorkUnitID      string                            `json:"workUnitId"`
	ArtifactVersion string                            `json:"artifactVersion"`
	RetentionState  string                            `json:"retentionState"`
	Files           []OutcomeResultChangeFileResponse `json:"files"`
	Truncated       bool                              `json:"truncated"`
}

// OutcomeResultSummaryResponse projects artifact evidence and verification facts for owner review.
type OutcomeResultSummaryResponse struct {
	Artifacts      []OutcomeResultArtifactResponse `json:"artifacts"`
	Checks         []OutcomeResultCheckResponse    `json:"checks"`
	Changes        []OutcomeResultChangesResponse  `json:"changes"`
	Uncertainty    []string                        `json:"uncertainty"`
	NextSafeAction string                          `json:"nextSafeAction"`
}

// OutcomeReentryTargetResponse is one daemon-derived identity a correction
// may target, so rework and reopen never ask the owner to type a raw
// identifier.
type OutcomeReentryTargetResponse struct {
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	Label      string `json:"label"`
}

// OutcomeProofResponse is the daemon-derived Prove & Close read model.
type OutcomeProofResponse struct {
	OutcomeID   string                       `json:"outcomeId"`
	Contract    ContractRevisionResponse     `json:"contractRevision"`
	Status      string                       `json:"status"`
	NextAction  string                       `json:"nextAction"`
	Result      OutcomeResultSummaryResponse `json:"result"`
	Criteria    []CriterionProofResponse     `json:"criteria"`
	Decisions   []AcceptanceDecisionResponse `json:"decisions"`
	Corrections []OutcomeCorrectionResponse  `json:"corrections"`
	// ActiveCorrectionID names the correction that set the current proof
	// horizon — what the owner most recently asked to be changed. Corrections
	// is an append-only history; this is the one still standing. Absent when no
	// rework or reopen stands against the current Contract revision.
	ActiveCorrectionID string     `json:"activeCorrectionId,omitempty"`
	ProofHorizon       *time.Time `json:"proofHorizon,omitempty"`
	// ReentryTargets are the daemon-derived correction targets for the current
	// Contract revision.
	ReentryTargets []OutcomeReentryTargetResponse `json:"reentryTargets"`
}

// OutcomeProofEnvelope wraps the canonical proof response.
type OutcomeProofEnvelope struct {
	Proof OutcomeProofResponse `json:"proof"`
}

func outcomeProofResponse(view outcomevc.ProofView) OutcomeProofResponse {
	response := OutcomeProofResponse{
		OutcomeID: string(view.OutcomeID), Contract: contractRevisionResponse(view.Contract),
		Status: string(view.Status), NextAction: view.NextAction,
		Result:         OutcomeResultSummaryResponse{Artifacts: []OutcomeResultArtifactResponse{}, Checks: []OutcomeResultCheckResponse{}, Changes: []OutcomeResultChangesResponse{}, Uncertainty: []string{}, NextSafeAction: view.NextAction},
		Criteria:       make([]CriterionProofResponse, 0, len(view.Criteria)),
		Decisions:      make([]AcceptanceDecisionResponse, 0, len(view.Decisions)),
		Corrections:    make([]OutcomeCorrectionResponse, 0, len(view.Corrections)),
		ReentryTargets: make([]OutcomeReentryTargetResponse, 0, len(view.ReentryTargets)),
	}
	for _, target := range view.ReentryTargets {
		response.ReentryTargets = append(response.ReentryTargets, OutcomeReentryTargetResponse{
			TargetType: string(target.TargetType), TargetID: target.TargetID, Label: target.Label,
		})
	}
	for _, change := range view.Changes {
		projected := OutcomeResultChangesResponse{
			AttemptID: string(change.AttemptID), WorkUnitID: string(change.WorkUnitID),
			ArtifactVersion: change.ArtifactVersion, RetentionState: string(change.RetentionState),
			Files: make([]OutcomeResultChangeFileResponse, 0, len(change.Files)), Truncated: change.Truncated,
		}
		for _, file := range change.Files {
			projected.Files = append(projected.Files, OutcomeResultChangeFileResponse{
				Path: file.RelativePath, ChangeKind: string(file.ChangeKind), Digest: file.ContentDigest,
			})
		}
		response.Result.Changes = append(response.Result.Changes, projected)
	}
	if !view.ProofHorizon.IsZero() {
		horizon := view.ProofHorizon
		response.ProofHorizon = &horizon
	}
	if view.ActiveCorrection != nil {
		response.ActiveCorrectionID = string(view.ActiveCorrection.ID)
	}
	for _, criterion := range view.Criteria {
		item := CriterionProofResponse{
			CriterionID: string(criterion.Criterion.ID), ContractRevisionID: string(criterion.Criterion.ContractRevisionID),
			Position: criterion.Criterion.Position, Text: criterion.Criterion.Text, Ready: criterion.Ready, Gap: criterion.Gap,
			Evidence: make([]EvidenceItemResponse, 0, len(criterion.Evidence)), Verifications: make([]VerificationRunResponse, 0, len(criterion.Verifications)),
		}
		for _, evidence := range criterion.Evidence {
			item.Evidence = append(item.Evidence, evidenceItemResponse(evidence))
		}
		for _, verification := range criterion.Verifications {
			item.Verifications = append(item.Verifications, verificationRunResponse(verification))
		}
		for _, evidence := range criterion.Evidence {
			if evidence.SourceType == domain.EvidenceSourceArtifact {
				response.Result.Artifacts = append(response.Result.Artifacts, OutcomeResultArtifactResponse{
					Revision: evidence.SubjectRevision, SourceRef: evidence.SourceRef, Digest: evidence.ContentDigest,
				})
			}
			if evidence.SourceType == domain.EvidenceSourceDeterministicCheck {
				check := OutcomeResultCheckResponse{CriterionID: string(evidence.CriterionID), Command: evidence.SourceRef, ArtifactRevision: evidence.SubjectRevision, Detail: evidence.Summary, Uncertain: true}
				for _, verification := range criterion.Verifications {
					if slices.Contains(verification.EvidenceItemIDs, evidence.ID) {
						check.Verdict = string(verification.Result)
						check.Detail = verification.Detail
						check.Uncertain = verification.Result == domain.VerificationInconclusive
						break
					}
				}
				if check.Uncertain {
					response.Result.Uncertainty = append(response.Result.Uncertainty, check.Detail)
				}
				response.Result.Checks = append(response.Result.Checks, check)
			}
		}
		response.Criteria = append(response.Criteria, item)
	}
	for _, decision := range view.Decisions {
		response.Decisions = append(response.Decisions, acceptanceDecisionResponse(decision))
	}
	for _, correction := range view.Corrections {
		response.Corrections = append(response.Corrections, OutcomeCorrectionResponse{
			ID: string(correction.ID), DecisionID: string(correction.DecisionID), ContractRevisionID: string(correction.ContractRevisionID),
			Feedback: correction.Feedback, TargetType: string(correction.TargetType), TargetID: correction.TargetID, CreatedAt: correction.CreatedAt,
		})
	}
	return response
}

func evidenceItemResponse(item domain.EvidenceItem) EvidenceItemResponse {
	return EvidenceItemResponse{
		ID: string(item.ID), ContractRevisionID: string(item.ContractRevisionID), CriterionID: string(item.CriterionID),
		SubjectType: string(item.SubjectType), SubjectID: item.SubjectID, SubjectRevision: item.SubjectRevision,
		Kind: string(item.Kind), SourceType: string(item.SourceType), SourceRef: item.SourceRef,
		ProducerType: string(item.ProducerType), ProducerRef: item.ProducerRef, Summary: item.Summary,
		ContentDigest: item.ContentDigest, CreatedAt: item.CreatedAt,
	}
}

func verificationRunResponse(run domain.VerificationRun) VerificationRunResponse {
	ids := make([]string, 0, len(run.EvidenceItemIDs))
	for _, id := range run.EvidenceItemIDs {
		ids = append(ids, string(id))
	}
	return VerificationRunResponse{
		ID: string(run.ID), ContractRevisionID: string(run.ContractRevisionID), CriterionID: string(run.CriterionID),
		SubjectType: string(run.SubjectType), SubjectID: run.SubjectID, SubjectRevision: run.SubjectRevision,
		EvidenceItemIDs: ids, Method: run.Method, IndependenceClass: string(run.IndependenceClass), Independent: run.IsIndependent(), Result: string(run.Result),
		ProducerRef: run.ProducerRef, VerifierRef: run.VerifierRef, ProducerProvider: run.ProducerProvider, VerifierProvider: run.VerifierProvider,
		Detail: run.Detail, CreatedAt: run.CreatedAt,
	}
}

func outcomeResponse(view outcomevc.View) OutcomeResponse {
	history := make([]ContractRevisionResponse, 0, len(view.History))
	for _, rev := range view.History {
		history = append(history, contractRevisionResponse(rev))
	}
	resp := OutcomeResponse{
		ID:                    string(view.Outcome.ID),
		SpaceID:               string(view.Outcome.SpaceID),
		ParentID:              string(view.Outcome.ParentID),
		Title:                 view.Outcome.Title,
		CurrentRevisionNumber: view.Outcome.CurrentRevisionNumber,
		Current:               contractRevisionResponse(view.Current),
		History:               history,
		CreatedAt:             view.Outcome.CreatedAt,
		UpdatedAt:             view.Outcome.UpdatedAt,
	}
	if resp.History == nil {
		resp.History = []ContractRevisionResponse{}
	}
	if view.LatestPlan != nil {
		plan := planRevisionResponse(*view.LatestPlan)
		resp.LatestPlan = &plan
	}
	return resp
}

// PlanIDParam is the {planId} path parameter shared by the plan approval route.
type PlanIDParam struct {
	PlanID string `path:"planId" description:"Plan revision identifier, e.g. plan-<uuid>."`
}

// ProposePlanRequest is the body for POST /outcomes/{outcomeId}/plans.
// ExpectedContractRevision must name the current revision the plan executes.
type ProposePlanRequest struct {
	ExpectedContractRevision int64 `json:"expectedContractRevision"`
}

// ReplanPlanRequest is the explicit feedback boundary for a new immutable
// proposal. Reloading the ordinary plan route remains idempotent.
type ReplanPlanRequest struct {
	ExpectedContractRevision int64  `json:"expectedContractRevision"`
	Feedback                 string `json:"feedback"`
}

// ApprovePlanRequest is the body for POST
// /outcomes/{outcomeId}/plans/{planId}/approval. ExpectedContractRevision
// guards against approving while the contract moved ahead unseen.
type ApprovePlanRequest struct {
	ExpectedContractRevision int64 `json:"expectedContractRevision"`
}

// PlanWorkUnitResponse is the single planned unit inside a PlanRevision.
type PlanWorkUnitResponse struct {
	ID                      string   `json:"id"`
	Kind                    string   `json:"kind"`
	Title                   string   `json:"title"`
	ContractRevisionNumber  int64    `json:"contractRevisionNumber"`
	DependsOn               []string `json:"dependsOn"`
	CriterionIDs            []string `json:"criterionIds"`
	Provider                string   `json:"provider,omitempty"`
	ModelSelection          string   `json:"modelSelection,omitempty"`
	Model                   string   `json:"model,omitempty"`
	RequiredCapabilities    []string `json:"requiredCapabilities"`
	OutputSummary           string   `json:"outputSummary"`
	EvidenceChecks          []string `json:"evidenceChecks"`
	VerificationRequirement string   `json:"verificationRequirement"`
	StopConditions          []string `json:"stopConditions"`
	// ApprovedChecks are the deterministic commands Kennel itself will run
	// and record as independent observation. EvidenceChecks above stay prose
	// for the provider to read; these are authority frozen at approval.
	ApprovedChecks []ApprovedCheckResponse `json:"approvedChecks"`
}

// ApprovedCheckResponse is one deterministic check the owner authorized,
// bound to the criterion it proves. Argv is a discrete argument vector, never
// a command line: a shell string would make approved authority unreadable.
type ApprovedCheckResponse struct {
	ID             string   `json:"id"`
	CriterionID    string   `json:"criterionId"`
	Argv           []string `json:"argv"`
	TimeoutSeconds int64    `json:"timeoutSeconds"`
}

// RoutingPreferenceResponse describes the effective provider/model preference.
type RoutingPreferenceResponse struct {
	Provider       string `json:"provider"`
	ModelSelection string `json:"modelSelection"`
	Model          string `json:"model,omitempty"`
}

// RoutingDecisionResponse explains how the daemon resolved one WorkUnit route.
type RoutingDecisionResponse struct {
	WorkUnitID                string                     `json:"workUnitId"`
	Status                    string                     `json:"status"`
	PolicyVersion             string                     `json:"policyVersion"`
	CapabilitySnapshot        string                     `json:"capabilitySnapshot,omitempty"`
	Role                      string                     `json:"role"`
	EffectivePreference       *RoutingPreferenceResponse `json:"effectivePreference,omitempty"`
	RecommendedProvider       string                     `json:"recommendedProvider,omitempty"`
	RecommendedModelSelection string                     `json:"recommendedModelSelection,omitempty"`
	RecommendedModel          string                     `json:"recommendedModel,omitempty"`
}

// CapabilityGrantResponse is one scoped capability the plan authorizes.
type CapabilityGrantResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

// PlanRevisionResponse is the canonical Decide & Authorize read model: the
// frozen Work Unit, active-on-approval grants, and the RunBrief core digest.
type PlanRevisionResponse struct {
	ID                      string                    `json:"id"`
	OutcomeID               string                    `json:"outcomeId"`
	Number                  int64                     `json:"number"`
	ContractRevisionNumber  int64                     `json:"contractRevisionNumber"`
	Status                  string                    `json:"status"`
	Summary                 string                    `json:"summary"`
	Assumptions             []string                  `json:"assumptions"`
	Blockers                []string                  `json:"blockers"`
	WorkUnits               []PlanWorkUnitResponse    `json:"workUnits"`
	Grants                  []CapabilityGrantResponse `json:"grants"`
	RoutingDecisions        []RoutingDecisionResponse `json:"routingDecisions"`
	RunBriefCoreDigest      string                    `json:"runBriefCoreDigest"`
	RunBriefCompiledDigest  string                    `json:"runBriefCompiledDigest,omitempty"`
	PlanningSessionID       string                    `json:"planningSessionId,omitempty"`
	SourceIntelligenceRunID string                    `json:"sourceIntelligenceRunId,omitempty"`
	CreatedAt               time.Time                 `json:"createdAt"`
}

// PlanEnvelope is the { plan } response body for plan reads and writes.
type PlanEnvelope struct {
	Plan PlanRevisionResponse `json:"plan"`
}

// PlanningSessionIDParam documents the planning-session route parameter.
type PlanningSessionIDParam struct {
	PlanningSessionID string `path:"planningSessionId" description:"Planning conversation identifier."`
}

// PlanningCandidatesQuery binds candidate discovery to a Contract revision.
type PlanningCandidatesQuery struct {
	ContractRevision int64 `query:"contractRevision" minimum:"1" description:"Confirmed Contract revision to plan against."`
}

// StartPlanningRequest opens a conversation with one exact candidate.
type StartPlanningRequest struct {
	ExpectedContractRevision int64  `json:"expectedContractRevision" minimum:"1"`
	CandidateID              string `json:"candidateId"`
	ContextMode              string `json:"contextMode,omitempty" enum:"repository_read,supplied_packet" description:"Defaults to repository_read."`
	RequestKey               string `json:"requestKey"`
}

// PlanningMessageRequest appends one idempotent owner message.
type PlanningMessageRequest struct {
	ExpectedSessionRevision int64  `json:"expectedSessionRevision" minimum:"1"`
	Text                    string `json:"text"`
	RequestKey              string `json:"requestKey"`
}

// PlanningFinalizeRequest asks the planner for a structured proposal.
type PlanningFinalizeRequest struct {
	ExpectedSessionRevision int64  `json:"expectedSessionRevision" minimum:"1"`
	RequestKey              string `json:"requestKey"`
}

// PlanningCancelRequest closes the optimistic planning revision.
type PlanningCancelRequest struct {
	ExpectedSessionRevision int64 `json:"expectedSessionRevision" minimum:"1"`
}

// PlanningBindingResponse exposes the frozen provider/model selection.
type PlanningBindingResponse struct {
	Mode           string `json:"mode" enum:"direct_api,native_harness"`
	Provider       string `json:"provider"`
	ModelSelection string `json:"modelSelection" enum:"provider_default,explicit"`
	Model          string `json:"model,omitempty"`
	Effort         string `json:"effort,omitempty"`
}

// PlanningCandidateResponse describes one selectable or unavailable planner.
type PlanningCandidateResponse struct {
	ID                string                  `json:"id"`
	Binding           PlanningBindingResponse `json:"binding"`
	Ready             bool                    `json:"ready"`
	UnavailableCode   string                  `json:"unavailableCode,omitempty"`
	UnavailableDetail string                  `json:"unavailableDetail,omitempty"`
}

// PlanningCandidatesEnvelope is the candidate-list response body.
type PlanningCandidatesEnvelope struct {
	Candidates []PlanningCandidateResponse `json:"candidates"`
}

// PlanningClarificationResponse is a compact owner-decision card.
type PlanningClarificationResponse struct {
	Question       string   `json:"question"`
	Reason         string   `json:"reason"`
	Recommendation string   `json:"recommendation"`
	Alternatives   []string `json:"alternatives"`
}

// PlanningContractChangeResponse recommends but never applies Contract edits.
type PlanningContractChangeResponse struct {
	Summary       string   `json:"summary"`
	ChangedFields []string `json:"changedFields"`
}

// PlanningTurnResponse is one normalized owner-visible conversation turn.
type PlanningTurnResponse struct {
	ID                string                          `json:"id"`
	Sequence          int64                           `json:"sequence"`
	ReplyToTurnID     string                          `json:"replyToTurnId,omitempty"`
	Role              string                          `json:"role" enum:"owner,planner"`
	Kind              string                          `json:"kind" enum:"message,finalize_request,clarification,contract_change_proposal,plan_proposal"`
	Text              string                          `json:"text"`
	Clarification     *PlanningClarificationResponse  `json:"clarification,omitempty"`
	ContractChange    *PlanningContractChangeResponse `json:"contractChange,omitempty"`
	IntelligenceRunID string                          `json:"intelligenceRunId,omitempty"`
	CreatedAt         time.Time                       `json:"createdAt"`
}

// PlanningSessionResponse exposes canonical conversation state and provenance.
type PlanningSessionResponse struct {
	ID                     string                  `json:"id"`
	OutcomeID              string                  `json:"outcomeId"`
	ContractRevisionID     string                  `json:"contractRevisionId"`
	ContractRevisionNumber int64                   `json:"contractRevisionNumber"`
	Revision               int64                   `json:"revision"`
	Status                 string                  `json:"status" enum:"active,proposal_ready,superseded,cancelled"`
	WaitingOn              string                  `json:"waitingOn" enum:"owner,provider,none"`
	Binding                PlanningBindingResponse `json:"binding"`
	ContextMode            string                  `json:"contextMode" enum:"repository_read,supplied_packet"`
	ContextDigest          string                  `json:"contextDigest"`
	PlanningGrantDigest    string                  `json:"planningGrantDigest"`
	EffectiveProvider      string                  `json:"effectiveProvider,omitempty"`
	EffectiveModel         string                  `json:"effectiveModel,omitempty"`
	LastFailureCode        string                  `json:"lastFailureCode,omitempty"`
	LastFailureDetail      string                  `json:"lastFailureDetail,omitempty"`
	CreatedAt              time.Time               `json:"createdAt"`
	UpdatedAt              time.Time               `json:"updatedAt"`
}

// PlanningResponse joins one session, its turns, and optional proposed Plan.
type PlanningResponse struct {
	Session      PlanningSessionResponse `json:"session"`
	Turns        []PlanningTurnResponse  `json:"turns"`
	ProposedPlan *PlanRevisionResponse   `json:"proposedPlan,omitempty"`
}

// PlanningEnvelope is the interactive-planning response body.
type PlanningEnvelope struct {
	Planning PlanningResponse `json:"planning"`
}

// ScheduleWorkUnitResponse is the daemon-derived state for one canonical
// WorkUnit. React must render this projection rather than recreate eligibility.
type ScheduleWorkUnitResponse struct {
	WorkUnit PlanWorkUnitResponse   `json:"workUnit"`
	State    string                 `json:"state" enum:"blocked,runnable,executing,proven,retryable,paused"`
	Attempts []ScheduleAttemptBrief `json:"attempts"`
	// BlockedReason distinguishes waiting on dependency proof, waiting on the
	// serial custody fence, and a dependency that is proved but whose output
	// cannot be handed down. Empty unless the unit is blocked.
	BlockedReason string `json:"blockedReason,omitempty" enum:"awaiting_dependency_proof,custody_held,upstream_artifact_unavailable"`
	// BlockedDetail names the specific refusal behind the reason, so
	// "upstream artifact unavailable" can say whether the predecessor's result
	// is missing, incomplete, unfrozen or mislineaged.
	BlockedDetail        string          `json:"blockedDetail,omitempty"`
	BlockingDependencies []string        `json:"blockingDependencies"`
	CriterionReady       map[string]bool `json:"criterionReady"`
}

// ScheduleAttemptBrief is the bounded Attempt identity shown in a schedule.
type ScheduleAttemptBrief struct {
	ID         string    `json:"id"`
	WorkUnitID string    `json:"workUnitId"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// ScheduleResponse is the daemon-derived, read-only Plan schedule projection.
type ScheduleResponse struct {
	OutcomeID              string                     `json:"outcomeId"`
	Plan                   PlanRevisionResponse       `json:"plan"`
	WorkUnits              []ScheduleWorkUnitResponse `json:"workUnits"`
	NextRunnableWorkUnitID string                     `json:"nextRunnableWorkUnitId,omitempty"`
	ActiveAttempt          *ScheduleAttemptBrief      `json:"activeAttempt,omitempty"`
	// CustodyHeldByWorkUnitID names the unit holding the serial fence, if any.
	CustodyHeldByWorkUnitID string `json:"custodyHeldByWorkUnitId,omitempty"`
	// NoRunnableReason explains an empty runnable set, so a Mission with
	// nothing to start can say why instead of showing an endless spinner.
	NoRunnableReason string `json:"noRunnableReason,omitempty" enum:"all_units_proven,attempt_executing,attempt_paused,awaiting_proof"`
}

// ScheduleEnvelope wraps a daemon-derived Plan schedule response.
type ScheduleEnvelope struct {
	Schedule ScheduleResponse `json:"schedule"`
}

func workUnitResponse(unit domain.WorkUnit) PlanWorkUnitResponse {
	return PlanWorkUnitResponse{
		ID:                      string(unit.ID),
		Kind:                    string(unit.Kind),
		Title:                   unit.Title,
		ContractRevisionNumber:  unit.ContractRevisionNumber,
		DependsOn:               stringWorkUnitIDs(unit.DependsOn),
		CriterionIDs:            stringCriterionIDs(unit.CriterionIDs),
		Provider:                string(unit.Provider),
		ModelSelection:          string(unit.ModelSelection),
		Model:                   unit.Model,
		RequiredCapabilities:    append([]string(nil), unit.RequiredCapabilities...),
		OutputSummary:           unit.OutputSummary,
		EvidenceChecks:          unit.EvidenceChecks,
		VerificationRequirement: unit.VerificationRequirement,
		StopConditions:          unit.StopConditions,
		ApprovedChecks:          approvedCheckResponses(unit.Checks),
	}
}

func approvedCheckResponses(checks []domain.ApprovedCheck) []ApprovedCheckResponse {
	out := make([]ApprovedCheckResponse, 0, len(checks))
	for _, check := range checks {
		out = append(out, ApprovedCheckResponse{
			ID: string(check.ID), CriterionID: string(check.CriterionID),
			Argv: append([]string(nil), check.Argv...), TimeoutSeconds: check.TimeoutSeconds,
		})
	}
	return out
}

func stringWorkUnitIDs(ids []domain.WorkUnitID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func stringCriterionIDs(ids []domain.CriterionID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func routingDecisionResponse(decision domain.WorkUnitRoutingDecision) RoutingDecisionResponse {
	response := RoutingDecisionResponse{
		WorkUnitID: string(decision.WorkUnitID), Status: string(decision.Decision.Status),
		PolicyVersion: decision.Decision.PolicyVersion, CapabilitySnapshot: decision.Decision.CapabilitySnapshot,
		Role: string(decision.Decision.Role), RecommendedProvider: decision.Decision.RecommendedProvider,
		RecommendedModelSelection: string(decision.Decision.RecommendedModelSelection), RecommendedModel: decision.Decision.RecommendedModel,
	}
	if preference := decision.Decision.EffectivePreference; preference != nil {
		response.EffectivePreference = &RoutingPreferenceResponse{Provider: preference.Provider, ModelSelection: string(preference.ModelSelection), Model: preference.Model}
	}
	return response
}

func capabilityGrantResponse(grant domain.CapabilityGrant) CapabilityGrantResponse {
	return CapabilityGrantResponse{
		ID:    string(grant.ID),
		Name:  grant.Name,
		Scope: grant.Scope,
	}
}

func scheduleResponse(view outcomevc.ScheduleView) ScheduleResponse {
	units := make([]ScheduleWorkUnitResponse, 0, len(view.WorkUnits))
	for _, entry := range view.WorkUnits {
		attempts := make([]ScheduleAttemptBrief, 0, len(entry.Attempts))
		for _, attempt := range entry.Attempts {
			attempts = append(attempts, ScheduleAttemptBrief{ID: string(attempt.ID), WorkUnitID: string(attempt.WorkUnitID), Status: string(attempt.Status), CreatedAt: attempt.CreatedAt, UpdatedAt: attempt.UpdatedAt})
		}
		ready := make(map[string]bool, len(entry.CriterionReady))
		for criterion, value := range entry.CriterionReady {
			ready[string(criterion)] = value
		}
		dependencies := make([]string, 0, len(entry.BlockingDependencies))
		for _, dependency := range entry.BlockingDependencies {
			dependencies = append(dependencies, string(dependency))
		}
		units = append(units, ScheduleWorkUnitResponse{WorkUnit: workUnitResponse(entry.WorkUnit), State: string(entry.State), Attempts: attempts, BlockedReason: string(entry.BlockedReason), BlockedDetail: entry.BlockedDetail, BlockingDependencies: dependencies, CriterionReady: ready})
	}
	response := ScheduleResponse{
		OutcomeID: string(view.Plan.OutcomeID), Plan: planRevisionResponse(view.Plan), WorkUnits: units,
		NextRunnableWorkUnitID:  string(view.NextRunnableID),
		CustodyHeldByWorkUnitID: string(view.CustodyHeldBy),
		NoRunnableReason:        string(view.NoRunnableReason),
	}
	if view.ActiveAttempt != nil {
		response.ActiveAttempt = &ScheduleAttemptBrief{ID: string(view.ActiveAttempt.ID), WorkUnitID: string(view.ActiveAttempt.WorkUnitID), Status: string(view.ActiveAttempt.Status), CreatedAt: view.ActiveAttempt.CreatedAt, UpdatedAt: view.ActiveAttempt.UpdatedAt}
	}
	return response
}

func planRevisionResponse(plan domain.PlanRevision) PlanRevisionResponse {
	units := make([]PlanWorkUnitResponse, 0, len(plan.WorkUnits))
	for _, unit := range plan.WorkUnits {
		units = append(units, workUnitResponse(unit))
	}
	grants := make([]CapabilityGrantResponse, 0, len(plan.Grants))
	for _, grant := range plan.Grants {
		grants = append(grants, capabilityGrantResponse(grant))
	}
	routing := make([]RoutingDecisionResponse, 0, len(plan.RoutingDecisions))
	for _, decision := range plan.RoutingDecisions {
		routing = append(routing, routingDecisionResponse(decision))
	}
	return PlanRevisionResponse{
		ID:                      string(plan.ID),
		OutcomeID:               string(plan.OutcomeID),
		Number:                  plan.Number,
		ContractRevisionNumber:  plan.ContractRevisionNumber,
		Status:                  string(plan.Status),
		Summary:                 plan.Summary,
		Assumptions:             append([]string(nil), plan.Assumptions...),
		Blockers:                append([]string(nil), plan.Blockers...),
		WorkUnits:               units,
		Grants:                  grants,
		RoutingDecisions:        routing,
		RunBriefCoreDigest:      plan.RunBriefCoreDigest,
		RunBriefCompiledDigest:  plan.RunBriefCompiledDigest,
		PlanningSessionID:       plan.PlanningSessionID.String(),
		SourceIntelligenceRunID: plan.SourceIntelligenceRunID.String(),
		CreatedAt:               plan.CreatedAt,
	}
}

// StartOutcomeAttemptRequest is the body for POST
// /outcomes/{outcomeId}/attempts. RequestKey makes admission exactly-once:
// replaying a delivered key resolves the original attempt.
//
// WorkUnitID is an optional legacy assertion. When omitted, the daemon selects
// the next dependency-ready WorkUnit from the approved Plan.
//
// Harness is accepted only for historical clients and is ignored. Provider and
// model come from the approved WorkUnit's immutable ExecutionBinding, and no
// caller or mutable Project default may reinterpret it after approval.
type StartOutcomeAttemptRequest struct {
	PlanRevisionID string `json:"planRevisionId"`
	WorkUnitID     string `json:"workUnitId,omitempty"`
	Harness        string `json:"harness,omitempty"`
	RequestKey     string `json:"requestKey"`
}

// RecordObservationRequest is the body for POST
// /outcomes/{outcomeId}/attempts/{attemptId}/observations.
type RecordObservationRequest struct {
	Kind    string `json:"kind"`
	Payload string `json:"payload,omitempty" description:"Optional JSON object with observation detail."`
}

// AttemptRecoveryRequest is the body for POST
// /outcomes/{outcomeId}/attempts/{attemptId}/recovery.
type AttemptRecoveryRequest struct {
	Action string `json:"action" description:"One of contain, reconcile, replace, attention." enum:"contain,reconcile,replace,attention"`
	// ConfirmProviderStopped is the owner's explicit assertion that the bound
	// provider session is stopped. It unlocks custody release without machine
	// proof and is recorded as an auditable containment observation.
	ConfirmProviderStopped bool `json:"confirmProviderStopped,omitempty"`
}

// AttemptPresentationResponse is the DERIVED read-time truth about an
// attempt: phase, whether liveness is unproven, and whether it ended without
// a result classification. Never stored; always computed from durable facts.
type AttemptPresentationResponse struct {
	Phase             domain.AttemptPhase `json:"phase" enum:"awaiting_start,executing,suspended,unconfirmed,needs_input,ended_unclassified,halted_failed,halted_cancelled,suspect_lost,succeeded"`
	Unconfirmed       bool                `json:"unconfirmed"`
	EndedUnclassified bool                `json:"endedUnclassified"`
	// Attention names the durable activity behind needs_input.
	Attention  string `json:"attention,omitempty" enum:"waiting_input,blocked"`
	NextAction string `json:"nextAction"`
}

// AttemptSessionRefResponse is one immutable provider-session binding.
type AttemptSessionRefResponse struct {
	ID                     string    `json:"id"`
	Seq                    int64     `json:"seq"`
	SessionID              string    `json:"sessionId"`
	Harness                string    `json:"harness"`
	Mode                   string    `json:"mode,omitempty"`
	RunBriefCoreDigest     string    `json:"runBriefCoreDigest"`
	RunBriefCompiledDigest string    `json:"runBriefCompiledDigest,omitempty"`
	BoundAt                time.Time `json:"boundAt"`
	// ProtocolProvenance is the persisted protocol-negotiation episode that
	// answers for this binding (ADR 0016): which provider build and negotiated
	// surface the work ran on. Absent when no episode was recorded - a driver
	// that cannot report provenance, or a record lost to the fail-soft write;
	// never a statement that the session was unnegotiated.
	ProtocolProvenance *ChatProtocolProvenanceResponse `json:"protocolProvenance,omitempty"`
}

// ChatProtocolProvenanceResponse is one persisted protocol-negotiation
// episode. negotiatedAt stays explicit so a fallback to the earliest episode
// (none predates the binding) is visible rather than implied.
type ChatProtocolProvenanceResponse struct {
	Provider             string    `json:"provider"`
	InstalledVersion     string    `json:"installedVersion,omitempty"`
	GeneratedFrom        string    `json:"generatedFrom,omitempty"`
	ProtocolDigest       string    `json:"protocolDigest"`
	GeneratedDigest      string    `json:"generatedDigest,omitempty"`
	MatchesGenerated     bool      `json:"matchesGenerated"`
	DegradedCapabilities []string  `json:"degradedCapabilities,omitempty"`
	MissingFloor         []string  `json:"missingFloor,omitempty"`
	NegotiatedAt         time.Time `json:"negotiatedAt"`
}

func chatProtocolProvenanceResponse(rec domain.ChatProtocolProvenance) *ChatProtocolProvenanceResponse {
	return &ChatProtocolProvenanceResponse{
		Provider:             rec.Provider,
		InstalledVersion:     rec.InstalledVersion,
		GeneratedFrom:        rec.GeneratedFrom,
		ProtocolDigest:       rec.ProtocolDigest,
		GeneratedDigest:      rec.GeneratedDigest,
		MatchesGenerated:     rec.MatchesGenerated,
		DegradedCapabilities: rec.DegradedCapabilities,
		MissingFloor:         rec.MissingFloor,
		NegotiatedAt:         rec.NegotiatedAt,
	}
}

// AttemptObservationResponse is one ordered, append-only observation.
type AttemptObservationResponse struct {
	ID        string    `json:"id"`
	Seq       int64     `json:"seq"`
	Kind      string    `json:"kind"`
	Payload   string    `json:"payload,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// RecoveryReceiptResponse records one reconcile verdict.
type RecoveryReceiptResponse struct {
	ID                   string    `json:"id"`
	Resolution           string    `json:"resolution" description:"resumed | replacement_attempt | needs_attention." enum:"resumed,replacement_attempt,needs_attention"`
	ReplacementAttemptID string    `json:"replacementAttemptId,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
}

// AttemptFenceResponse is the custody lock over one worktree subject while it
// is open for this attempt. lastRenewedAt is the renewable lease stamp: a
// stale renewal flags custody that may outlive its provider.
type AttemptFenceResponse struct {
	ID            string     `json:"id"`
	Subject       string     `json:"subject"`
	IssuedAt      time.Time  `json:"issuedAt"`
	LastRenewedAt time.Time  `json:"lastRenewedAt"`
	ReleasedAt    *time.Time `json:"releasedAt,omitempty"`
}

// AttemptResponse is the Act & Observe read model: durable lineage plus
// derived presentation. Provider completion is never presented as success.
type AttemptResponse struct {
	ID                     string                       `json:"id"`
	OutcomeID              string                       `json:"outcomeId"`
	PlanRevisionID         string                       `json:"planRevisionId"`
	WorkUnitID             string                       `json:"workUnitId"`
	Number                 int64                        `json:"number"`
	Status                 string                       `json:"status" enum:"queued,running,paused,succeeded,failed,cancelled,lost,reconciled"`
	ContractRevisionNumber int64                        `json:"contractRevisionNumber"`
	Sessions               []AttemptSessionRefResponse  `json:"sessions"`
	Observations           []AttemptObservationResponse `json:"observations"`
	Receipts               []RecoveryReceiptResponse    `json:"receipts"`
	Fence                  *AttemptFenceResponse        `json:"fence,omitempty"`
	Presentation           AttemptPresentationResponse  `json:"presentation"`
	CreatedAt              time.Time                    `json:"createdAt"`
	UpdatedAt              time.Time                    `json:"updatedAt"`
}

// AttemptEnvelope is the { attempt } response body.
type AttemptEnvelope struct {
	Attempt AttemptResponse `json:"attempt"`
}

// AttemptListEnvelope is the { attempts } response body.
type AttemptListEnvelope struct {
	Attempts []AttemptResponse `json:"attempts"`
}

// ObservationEnvelope is the { observation } response body.
type ObservationEnvelope struct {
	Observation AttemptObservationResponse `json:"observation"`
}

// AttemptRecoveryEnvelope pairs the post-recovery attempt with its verdict
// receipt when one was recorded.
type AttemptRecoveryEnvelope struct {
	Attempt AttemptResponse          `json:"attempt"`
	Receipt *RecoveryReceiptResponse `json:"receipt,omitempty"`
}

func attemptSessionRefResponse(ref domain.AttemptSessionRef, provenance *domain.ChatProtocolProvenance) AttemptSessionRefResponse {
	resp := AttemptSessionRefResponse{
		ID:                     string(ref.ID),
		Seq:                    ref.Seq,
		SessionID:              ref.SessionID,
		Harness:                string(ref.Harness),
		Mode:                   string(ref.Mode),
		RunBriefCoreDigest:     ref.RunBriefCoreDigest,
		RunBriefCompiledDigest: ref.RunBriefCompiledDigest,
		BoundAt:                ref.BoundAt,
	}
	if provenance != nil {
		resp.ProtocolProvenance = chatProtocolProvenanceResponse(*provenance)
	}
	return resp
}

func attemptObservationResponse(obs domain.AttemptObservation) AttemptObservationResponse {
	return AttemptObservationResponse{
		ID:        obs.ID,
		Seq:       obs.Seq,
		Kind:      obs.Kind,
		Payload:   obs.Payload,
		CreatedAt: obs.CreatedAt,
	}
}

func recoveryReceiptResponse(receipt domain.AttemptRecoveryReceipt) RecoveryReceiptResponse {
	return RecoveryReceiptResponse{
		ID:                   receipt.ID,
		Resolution:           string(receipt.Resolution),
		ReplacementAttemptID: string(receipt.ReplacementAttemptID),
		CreatedAt:            receipt.CreatedAt,
	}
}

func attemptResponse(view outcomevc.AttemptView) AttemptResponse {
	sessions := make([]AttemptSessionRefResponse, 0, len(view.Sessions))
	for _, ref := range view.Sessions {
		var provenance *domain.ChatProtocolProvenance
		if rec, ok := view.ProtocolProvenance[ref.ID]; ok {
			provenance = &rec
		}
		sessions = append(sessions, attemptSessionRefResponse(ref, provenance))
	}
	observations := make([]AttemptObservationResponse, 0, len(view.Observations))
	for _, obs := range view.Observations {
		observations = append(observations, attemptObservationResponse(obs))
	}
	receipts := make([]RecoveryReceiptResponse, 0, len(view.Receipts))
	for _, receipt := range view.Receipts {
		receipts = append(receipts, recoveryReceiptResponse(receipt))
	}
	resp := AttemptResponse{
		ID:                     string(view.Attempt.ID),
		OutcomeID:              string(view.Attempt.OutcomeID),
		PlanRevisionID:         string(view.Attempt.PlanRevisionID),
		WorkUnitID:             string(view.Attempt.WorkUnitID),
		Number:                 view.Attempt.Number,
		Status:                 string(view.Attempt.Status),
		ContractRevisionNumber: view.Attempt.ContractRevisionNumber,
		Sessions:               sessions,
		Observations:           observations,
		Receipts:               receipts,
		Presentation: AttemptPresentationResponse{
			Phase:             view.Presentation.Phase,
			Unconfirmed:       view.Presentation.Unconfirmed,
			EndedUnclassified: view.Presentation.EndedUnclassified,
			Attention:         view.Presentation.Attention,
			NextAction:        view.Presentation.NextAction,
		},
		CreatedAt: view.Attempt.CreatedAt,
		UpdatedAt: view.Attempt.UpdatedAt,
	}
	if view.Fence != nil {
		var releasedAt *time.Time
		if !view.Fence.ReleasedAt.IsZero() {
			released := view.Fence.ReleasedAt
			releasedAt = &released
		}
		resp.Fence = &AttemptFenceResponse{
			ID:            view.Fence.ID,
			Subject:       view.Fence.Subject,
			IssuedAt:      view.Fence.IssuedAt,
			LastRenewedAt: view.Fence.LastRenewedAt,
			ReleasedAt:    releasedAt,
		}
	}
	return resp
}

// --- Composed Outcomes (ADR 0007) ---

// ContributionLinkResponse is one immutable criterion binding.
type ContributionLinkResponse struct {
	ID                       string    `json:"id"`
	ParentOutcomeID          string    `json:"parentOutcomeId"`
	ChildOutcomeID           string    `json:"childOutcomeId"`
	ParentContractRevisionID string    `json:"parentContractRevisionId"`
	ParentCriterionID        string    `json:"parentCriterionId"`
	CreatedAt                time.Time `json:"createdAt"`
}

// ContributorResponse is one contributing Outcome and its bindings.
type ContributorResponse struct {
	Outcome OutcomeResponse            `json:"outcome"`
	Links   []ContributionLinkResponse `json:"links"`
	// Stale reports a binding to a superseded parent revision. It blocks new
	// authorization; it does not mean running work is dead.
	Stale bool `json:"stale"`
	// BlockedBy names declared upstream contributions that are not yet
	// accepted; Waived names those the owner explicitly overrode. A waived
	// dependency stays visible: it is overridden, not forgotten.
	BlockedBy []UpstreamBlockResponse      `json:"blockedBy"`
	Waived    []UpstreamBlockResponse      `json:"waived"`
	Attention ContributorAttentionResponse `json:"attention"`
}

// UpstreamBlockResponse is one unmet or waived dependency.
type UpstreamBlockResponse struct {
	Ref       string `json:"ref"`
	OutcomeID string `json:"outcomeId,omitempty"`
	Title     string `json:"title,omitempty"`
	Reason    string `json:"reason"`
}

// ContributorAttentionResponse is one roll-up item.
type ContributorAttentionResponse struct {
	OutcomeID  string `json:"outcomeId"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	NextAction string `json:"nextAction,omitempty"`
}

// ParentAttentionResponse summarises a decomposed Outcome for its owner.
type ParentAttentionResponse struct {
	Headline     string                         `json:"headline,omitempty"`
	Items        []ContributorAttentionResponse `json:"items"`
	Counts       map[string]int                 `json:"counts"`
	AcceptedOf   int                            `json:"acceptedOf"`
	Contributors int                            `json:"contributors"`
}

// WaiveContributionDependencyRequest is the body for POST
// /outcomes/{outcomeId}/decomposition/waivers. The reason is required and
// durable: a waiver nobody can explain later is indistinguishable from a
// mistake.
type WaiveContributionDependencyRequest struct {
	FromRef string `json:"fromRef"`
	ToRef   string `json:"toRef"`
	Reason  string `json:"reason"`
}

// CriterionClaimResponse is one parent criterion and who claims it. An empty
// claimedBy is a truthful report of an incomplete decomposition, not an error.
type CriterionClaimResponse struct {
	CriterionID string   `json:"criterionId"`
	Position    int64    `json:"position"`
	Text        string   `json:"text"`
	ClaimedBy   []string `json:"claimedBy"`
}

// OutcomeCompositionResponse is the derived composition read model. Shape and
// attention are computed from durable facts and are never stored.
type OutcomeCompositionResponse struct {
	Shape        string                   `json:"shape"`
	ParentID     string                   `json:"parentId,omitempty"`
	Contributors []ContributorResponse    `json:"contributors"`
	Coverage     []CriterionClaimResponse `json:"coverage"`
	// Attention rolls each contributor's situation up to the parent, most
	// demanding first, every item naming the contributor it came from.
	Attention ParentAttentionResponse `json:"attention"`
	// UnclaimedCriteria repeats the criteria nothing claims, so a caller does
	// not have to re-derive the one fact that decides whether a decomposition
	// is complete.
	UnclaimedCriteria []CriterionClaimResponse `json:"unclaimedCriteria"`
}

// OutcomeCompositionEnvelope is the { composition } response body.
type OutcomeCompositionEnvelope struct {
	Composition OutcomeCompositionResponse `json:"composition"`
}

func contributionLinkResponse(link domain.ContributionLink) ContributionLinkResponse {
	return ContributionLinkResponse{
		ID:                       string(link.ID),
		ParentOutcomeID:          string(link.ParentOutcomeID),
		ChildOutcomeID:           string(link.ChildOutcomeID),
		ParentContractRevisionID: string(link.ParentContractRevisionID),
		ParentCriterionID:        string(link.ParentCriterionID),
		CreatedAt:                link.CreatedAt,
	}
}

func criterionClaimResponse(claim domain.CriterionClaim) CriterionClaimResponse {
	claimedBy := make([]string, 0, len(claim.ClaimedBy))
	for _, id := range claim.ClaimedBy {
		claimedBy = append(claimedBy, string(id))
	}
	return CriterionClaimResponse{
		CriterionID: string(claim.CriterionID),
		Position:    claim.Position,
		Text:        claim.Text,
		ClaimedBy:   claimedBy,
	}
}

func outcomeCompositionResponse(view outcomevc.CompositionView, contributors []outcomevc.View) OutcomeCompositionResponse {
	resp := OutcomeCompositionResponse{
		Shape:             string(view.Shape),
		Contributors:      make([]ContributorResponse, 0, len(view.Contributors)),
		Coverage:          make([]CriterionClaimResponse, 0, len(view.Coverage)),
		UnclaimedCriteria: make([]CriterionClaimResponse, 0),
	}
	if view.Parent != nil {
		resp.ParentID = string(view.Parent.ID)
	}
	resp.Attention = parentAttentionResponse(view.Attention)
	for i, contributor := range view.Contributors {
		links := make([]ContributionLinkResponse, 0, len(contributor.Links))
		for _, link := range contributor.Links {
			links = append(links, contributionLinkResponse(link))
		}
		entry := ContributorResponse{
			Links: links, Stale: contributor.Stale,
			BlockedBy: upstreamBlocks(contributor.Gate.Blocked),
			Waived:    upstreamBlocks(contributor.Gate.Waived),
			Attention: contributorAttentionResponse(contributor.Attention),
		}
		if i < len(contributors) {
			entry.Outcome = outcomeResponse(contributors[i])
		}
		resp.Contributors = append(resp.Contributors, entry)
	}
	for _, claim := range view.Coverage {
		resp.Coverage = append(resp.Coverage, criterionClaimResponse(claim))
	}
	for _, claim := range view.Unclaimed() {
		resp.UnclaimedCriteria = append(resp.UnclaimedCriteria, criterionClaimResponse(claim))
	}
	return resp
}

// --- Decomposition authority (ADR 0007 phase 2) ---

// ProposeDecompositionRequest is the body for POST
// /outcomes/{outcomeId}/decompositions. Omitting contributors asks the daemon
// for its deterministic starting point: one contributing Outcome per parent
// criterion, which the owner then corrects.
type ProposeDecompositionRequest struct {
	// ExpectedContractRevision must name the parent's current revision.
	ExpectedContractRevision int64 `json:"expectedContractRevision"`
	// Rationale explains the topology in plain language. A decomposition the
	// owner cannot evaluate is not reviewable.
	Rationale    string                        `json:"rationale,omitempty"`
	Contributors []ProposedContributionRequest `json:"contributors,omitempty"`
	// RetainedCriteria are parent criteria the owner will prove directly
	// rather than delegate.
	RetainedCriteria []string                        `json:"retainedCriteria,omitempty"`
	Dependencies     []ContributionDependencyRequest `json:"dependencies,omitempty"`
}

// ProposedContributionRequest is one contributing Outcome as offered. It
// carries a whole contract because a contributing Outcome is a full
// responsibility, not a task.
type ProposedContributionRequest struct {
	Ref             string          `json:"ref,omitempty"`
	Title           string          `json:"title"`
	Goal            string          `json:"goal"`
	SuccessCriteria []string        `json:"successCriteria"`
	Review          string          `json:"review"`
	Constraints     []string        `json:"constraints,omitempty"`
	NonGoals        []string        `json:"nonGoals,omitempty"`
	Authority       IntakeAuthority `json:"authority,omitempty"`
	ClaimedCriteria []string        `json:"claimedCriteria"`
}

// ContributionDependencyRequest declares that fromRef must finish before toRef
// starts.
type ContributionDependencyRequest struct {
	FromRef string `json:"fromRef"`
	ToRef   string `json:"toRef"`
}

// ProposedContributionResponse is one contributing Outcome as recorded.
// ChildOutcomeID is absent until authorization creates the Outcome.
type ProposedContributionResponse struct {
	Ref             string          `json:"ref"`
	Position        int64           `json:"position"`
	Title           string          `json:"title"`
	Goal            string          `json:"goal"`
	SuccessCriteria []string        `json:"successCriteria"`
	Review          string          `json:"review"`
	Constraints     []string        `json:"constraints"`
	NonGoals        []string        `json:"nonGoals"`
	Authority       IntakeAuthority `json:"authority"`
	ClaimedCriteria []string        `json:"claimedCriteria"`
	ChildOutcomeID  string          `json:"childOutcomeId,omitempty"`
}

// ContributionDependencyResponse is one recorded ordering.
type ContributionDependencyResponse struct {
	ID      string `json:"id"`
	FromRef string `json:"fromRef"`
	ToRef   string `json:"toRef"`
}

// DecompositionResponse is one decomposition revision: a decomposed Outcome's
// plan. Proposed means nothing exists yet; authorized means the contributing
// Outcomes were created.
type DecompositionResponse struct {
	ID                 string                           `json:"id"`
	OutcomeID          string                           `json:"outcomeId"`
	Number             int64                            `json:"number"`
	ContractRevisionID string                           `json:"contractRevisionId"`
	Status             string                           `json:"status"`
	Rationale          string                           `json:"rationale"`
	Contributors       []ProposedContributionResponse   `json:"contributors"`
	RetainedCriteria   []string                         `json:"retainedCriteria"`
	Dependencies       []ContributionDependencyResponse `json:"dependencies"`
	// Stale reports that the parent contract moved on after this
	// decomposition was proposed. A stale proposal cannot be authorized.
	Stale        bool       `json:"stale"`
	CreatedAt    time.Time  `json:"createdAt"`
	AuthorizedAt *time.Time `json:"authorizedAt,omitempty"`
}

// DecompositionEnvelope is the { decomposition } response body.
type DecompositionEnvelope struct {
	Decomposition DecompositionResponse `json:"decomposition"`
}

func proposeDecompositionInput(req ProposeDecompositionRequest) outcomevc.ProposeDecompositionInput {
	contributors := make([]outcomevc.ProposedContributionInput, 0, len(req.Contributors))
	for _, contributor := range req.Contributors {
		claimed := make([]domain.CriterionID, 0, len(contributor.ClaimedCriteria))
		for _, id := range contributor.ClaimedCriteria {
			claimed = append(claimed, domain.CriterionID(id))
		}
		contributors = append(contributors, outcomevc.ProposedContributionInput{
			Ref: contributor.Ref, Title: contributor.Title, Goal: contributor.Goal,
			SuccessCriteria: contributor.SuccessCriteria, Review: contributor.Review,
			Constraints: contributor.Constraints, NonGoals: contributor.NonGoals,
			Authority: proposedAuthority(contributor.Authority), ClaimedCriteria: claimed,
		})
	}
	retained := make([]domain.CriterionID, 0, len(req.RetainedCriteria))
	for _, id := range req.RetainedCriteria {
		retained = append(retained, domain.CriterionID(id))
	}
	dependencies := make([]outcomevc.ContributionDependencyInput, 0, len(req.Dependencies))
	for _, dependency := range req.Dependencies {
		dependencies = append(dependencies, outcomevc.ContributionDependencyInput{FromRef: dependency.FromRef, ToRef: dependency.ToRef})
	}
	return outcomevc.ProposeDecompositionInput{
		ExpectedContractRevision: req.ExpectedContractRevision,
		Rationale:                req.Rationale,
		Contributors:             contributors,
		RetainedCriteria:         retained,
		Dependencies:             dependencies,
	}
}

func decompositionResponse(view outcomevc.DecompositionView) DecompositionResponse {
	revision := view.Decomposition
	resp := DecompositionResponse{
		ID: string(revision.ID), OutcomeID: string(revision.OutcomeID), Number: revision.Number,
		ContractRevisionID: string(revision.ContractRevisionID), Status: string(revision.Status),
		Rationale:        revision.Rationale,
		Contributors:     make([]ProposedContributionResponse, 0, len(revision.Contributors)),
		RetainedCriteria: make([]string, 0, len(revision.RetainedCriteria)),
		Dependencies:     make([]ContributionDependencyResponse, 0, len(revision.Dependencies)),
		Stale:            view.Stale,
		CreatedAt:        revision.CreatedAt,
		AuthorizedAt:     revision.AuthorizedAt,
	}
	for _, contributor := range revision.Contributors {
		claimed := make([]string, 0, len(contributor.ClaimedCriteria))
		for _, id := range contributor.ClaimedCriteria {
			claimed = append(claimed, string(id))
		}
		resp.Contributors = append(resp.Contributors, ProposedContributionResponse{
			Ref: contributor.Ref, Position: contributor.Position, Title: contributor.Title,
			Goal: contributor.Goal, SuccessCriteria: nonNilList(contributor.SuccessCriteria),
			Review: contributor.Review, Constraints: nonNilList(contributor.Constraints),
			NonGoals: nonNilList(contributor.NonGoals), Authority: intakeAuthority(contributor.Authority),
			ClaimedCriteria: claimed, ChildOutcomeID: string(contributor.ChildOutcomeID),
		})
	}
	for _, id := range revision.RetainedCriteria {
		resp.RetainedCriteria = append(resp.RetainedCriteria, string(id))
	}
	for _, dependency := range revision.Dependencies {
		resp.Dependencies = append(resp.Dependencies, ContributionDependencyResponse{
			ID: dependency.ID, FromRef: dependency.FromRef, ToRef: dependency.ToRef,
		})
	}
	return resp
}

func nonNilList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func upstreamBlocks(blocks []domain.UpstreamBlock) []UpstreamBlockResponse {
	out := make([]UpstreamBlockResponse, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, UpstreamBlockResponse{
			Ref: block.Ref, OutcomeID: string(block.OutcomeID),
			Title: block.Title, Reason: block.Reason,
		})
	}
	return out
}

func contributorAttentionResponse(item domain.ContributorAttention) ContributorAttentionResponse {
	return ContributorAttentionResponse{
		OutcomeID: string(item.OutcomeID), Title: item.Title,
		Kind: string(item.Kind), Reason: item.Reason, NextAction: item.NextAction,
	}
}

func parentAttentionResponse(summary domain.ParentAttention) ParentAttentionResponse {
	resp := ParentAttentionResponse{
		Headline:     string(summary.Headline),
		Items:        make([]ContributorAttentionResponse, 0, len(summary.Items)),
		Counts:       make(map[string]int, len(summary.Counts)),
		AcceptedOf:   summary.AcceptedOf,
		Contributors: summary.Contributors,
	}
	for _, item := range summary.Items {
		resp.Items = append(resp.Items, contributorAttentionResponse(item))
	}
	for kind, count := range summary.Counts {
		resp.Counts[string(kind)] = count
	}
	return resp
}

// --- Batched parent acceptance (ADR 0007 phase 4) ---

// AcceptContributorBatchRequest is one owner sitting over a decomposed
// Outcome's ready contributors. It produces one immutable decision per
// Outcome accepted, never one merged decision.
type AcceptContributorBatchRequest struct {
	ExpectedContractRevision int64 `json:"expectedContractRevision"`
	// OutcomeIds narrows the sitting; omit for every eligible contributor.
	OutcomeIds []string `json:"outcomeIds,omitempty"`
	Summary    string   `json:"summary"`
	// AcceptParent also accepts the Project-level Outcome, permitted only
	// when this batch leaves every parent criterion proved.
	AcceptParent        bool   `json:"acceptParent,omitempty"`
	ResourceDisposition string `json:"resourceDisposition"`
	RequestKey          string `json:"requestKey"`
}

// BatchEntryVerdictResponse is the daemon's answer on one contributor. The
// daemon may only withhold; it never accepts.
type BatchEntryVerdictResponse struct {
	OutcomeID string `json:"outcomeId"`
	Title     string `json:"title"`
	Eligible  bool   `json:"eligible"`
	Reason    string `json:"reason"`
	Remedy    string `json:"remedy,omitempty"`
}

// BatchEligibilityEnvelope reports who could be accepted right now.
type BatchEligibilityEnvelope struct {
	Contributors []BatchEntryVerdictResponse `json:"contributors"`
}

// AcceptBatchResponse reports what one sitting decided and what it withheld.
type AcceptBatchResponse struct {
	Accepted       []AcceptanceDecisionResponse `json:"accepted"`
	Excluded       []BatchEntryVerdictResponse  `json:"excluded"`
	ParentAccepted *AcceptanceDecisionResponse  `json:"parentAccepted,omitempty"`
	Parent         OutcomeProofResponse         `json:"parent"`
}

// AcceptBatchEnvelope is the { batch } response body.
type AcceptBatchEnvelope struct {
	Batch AcceptBatchResponse `json:"batch"`
}

func batchEntryVerdictResponse(verdict domain.BatchEntryVerdict) BatchEntryVerdictResponse {
	return BatchEntryVerdictResponse{
		OutcomeID: string(verdict.OutcomeID), Title: verdict.Title,
		Eligible: verdict.Eligible, Reason: verdict.Reason, Remedy: verdict.Remedy,
	}
}

func batchEntryVerdicts(verdicts []domain.BatchEntryVerdict) []BatchEntryVerdictResponse {
	out := make([]BatchEntryVerdictResponse, 0, len(verdicts))
	for _, verdict := range verdicts {
		out = append(out, batchEntryVerdictResponse(verdict))
	}
	return out
}

func acceptBatchResponse(view outcomevc.AcceptBatchView) AcceptBatchResponse {
	resp := AcceptBatchResponse{
		Accepted: make([]AcceptanceDecisionResponse, 0, len(view.Accepted)),
		Excluded: batchEntryVerdicts(view.Excluded),
		Parent:   outcomeProofResponse(view.Parent),
	}
	for _, decision := range view.Accepted {
		resp.Accepted = append(resp.Accepted, acceptanceDecisionResponse(decision))
	}
	if view.ParentAccepted != nil {
		parent := acceptanceDecisionResponse(*view.ParentAccepted)
		resp.ParentAccepted = &parent
	}
	return resp
}

func acceptanceDecisionResponse(decision domain.AcceptanceDecision) AcceptanceDecisionResponse {
	return AcceptanceDecisionResponse{
		ID: string(decision.ID), ContractRevisionID: string(decision.ContractRevisionID),
		Kind: string(decision.Kind), ActorType: string(decision.ActorType),
		Summary: decision.Summary, ResourceDisposition: string(decision.ResourceDisposition),
		CreatedAt: decision.CreatedAt,
	}
}

// --- Agent-authored decomposition asks (ADR 0007 phase 2b) ---

// DecompositionRequestIDParam is the {requestId} path parameter on the
// callback route. The callback is addressed by REQUEST rather than by Outcome:
// the answering agent knows only the request it was given.
type DecompositionRequestIDParam struct {
	RequestID string `path:"requestId" description:"Decomposition request identifier, e.g. dreq-<uuid>."`
}

// AskForDecompositionRequest opens a durable ask. The proposal arrives later
// over the callback route; this returns as soon as an agent has been started.
type AskForDecompositionRequest struct {
	ExpectedContractRevision int64 `json:"expectedContractRevision"`
}

// SubmitAgentProposalRequest is the body a spawned agent POSTs back. It is the
// same shape a person would author, because an agent proposal passes exactly
// the same gates — there is no trusted-proposer path.
type SubmitAgentProposalRequest struct {
	Rationale        string                          `json:"rationale"`
	Contributors     []ProposedContributionRequest   `json:"contributors"`
	RetainedCriteria []string                        `json:"retainedCriteria,omitempty"`
	Dependencies     []ContributionDependencyRequest `json:"dependencies,omitempty"`
}

// DecompositionRequestResponse reports one ask and what became of it.
type DecompositionRequestResponse struct {
	ID                 string `json:"id"`
	OutcomeID          string `json:"outcomeId"`
	ContractRevisionID string `json:"contractRevisionId"`
	Status             string `json:"status"`
	// Expired is derived from the clock, so an ask that timed out while the
	// daemon was down still reads as expired.
	Expired bool `json:"expired"`
	// RefusalReason carries the daemon's own words when it turned a proposal
	// down, and RawProposal the draft it turned down — kept so the owner can
	// correct one field instead of regenerating.
	RefusalReason   string     `json:"refusalReason,omitempty"`
	RawProposal     string     `json:"rawProposal,omitempty"`
	DecompositionID string     `json:"decompositionId,omitempty"`
	SessionID       string     `json:"sessionId,omitempty"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	AnsweredAt      *time.Time `json:"answeredAt,omitempty"`
}

// DecompositionRequestEnvelope is the { request } response body.
type DecompositionRequestEnvelope struct {
	Request DecompositionRequestResponse `json:"request"`
}

func decompositionRequestResponse(view outcomevc.DecompositionRequestView) DecompositionRequestResponse {
	request := view.Request
	resp := DecompositionRequestResponse{
		ID: string(request.ID), OutcomeID: string(request.OutcomeID),
		ContractRevisionID: string(request.ContractRevisionID),
		Status:             string(request.Status),
		Expired:            view.Expired,
		RefusalReason:      request.RefusalReason,
		RawProposal:        request.RawProposal,
		DecompositionID:    string(request.DecompositionID),
		SessionID:          request.SessionID,
		ExpiresAt:          request.ExpiresAt,
		CreatedAt:          request.CreatedAt,
		AnsweredAt:         request.AnsweredAt,
	}
	return resp
}

// agentProposalInput maps a callback body onto the same input a person's
// proposal uses. ExpectedContractRevision is deliberately absent: the request
// already froze which revision was asked about, and letting the agent name one
// would let it answer a different question than it was given.
func agentProposalInput(req SubmitAgentProposalRequest) outcomevc.ProposeDecompositionInput {
	return proposeDecompositionInput(ProposeDecompositionRequest{
		Rationale:        req.Rationale,
		Contributors:     req.Contributors,
		RetainedCriteria: req.RetainedCriteria,
		Dependencies:     req.Dependencies,
	})
}

// OutcomeDeletionEnvelope is the daemon's current erasure scope and blockers.
type OutcomeDeletionEnvelope struct {
	Deletion ports.OutcomeDeletionPreview `json:"deletion"`
}

// OutcomeTrashEnvelope lists recoverable Outcomes and cleanup jobs.
type OutcomeTrashEnvelope struct {
	Outcomes []ports.OutcomeTrashEntry `json:"outcomes"`
}

// ChangeOutcomeDeletionRequest names the current revision and explicit owner action.
type ChangeOutcomeDeletionRequest struct {
	Revision     int64  `json:"revision"`
	Action       string `json:"action"`
	Confirmation string `json:"confirmation"`
}

// OutcomeDeletionResult acknowledges a completed lifecycle action.
type OutcomeDeletionResult struct {
	Action string `json:"action"`
}

// MissionProjectionResponse is the authoritative, read-only WorkUnit graph.
type MissionProjectionResponse struct {
	Version                 int                   `json:"version"`
	OutcomeID               string                `json:"outcomeId"`
	MissionID               string                `json:"missionId"`
	ContractRevisionNumber  int64                 `json:"contractRevisionNumber"`
	PlanRevisionID          string                `json:"planRevisionId"`
	PlanRevisionNumber      int64                 `json:"planRevisionNumber"`
	TopologyFingerprint     string                `json:"topologyFingerprint"`
	TopologyGeneration      int64                 `json:"topologyGeneration"`
	Generation              int64                 `json:"generation"`
	UpdatedAt               time.Time             `json:"updatedAt"`
	Nodes                   []MissionNodeResponse `json:"nodes"`
	Edges                   []MissionEdgeResponse `json:"edges"`
	NextRunnableWorkUnitID  string                `json:"nextRunnableWorkUnitId,omitempty"`
	CustodyHeldByWorkUnitID string                `json:"custodyHeldByWorkUnitId,omitempty"`
	NoRunnableReason        string                `json:"noRunnableReason,omitempty"`
}
type MissionNodeResponse struct {
	WorkUnitID           string                    `json:"workUnitId"`
	PlanRevisionID       string                    `json:"planRevisionId"`
	Title                string                    `json:"title"`
	DependsOn            []string                  `json:"dependsOn"`
	ScheduleState        string                    `json:"scheduleState"`
	BlockingDependencies []string                  `json:"blockingDependencies"`
	BlockedReason        string                    `json:"blockedReason,omitempty"`
	BlockedDetail        string                    `json:"blockedDetail,omitempty"`
	CriterionIDs         []string                  `json:"criterionIds"`
	CriterionReady       map[string]bool           `json:"criterionReady"`
	CurrentAttempt       *MissionAttemptResponse   `json:"currentAttempt,omitempty"`
	Attention            *MissionAttentionResponse `json:"attention,omitempty"`
	NextAction           string                    `json:"nextAction,omitempty"`
	Responsibility       string                    `json:"responsibility" enum:"agent,owner,unconfirmed"`
	UpdatedAt            time.Time                 `json:"updatedAt"`
	Generation           int64                     `json:"generation"`
}
type MissionAttemptResponse struct {
	ID        string                  `json:"attemptId"`
	Number    int64                   `json:"number"`
	Status    string                  `json:"status"`
	CreatedAt time.Time               `json:"createdAt"`
	UpdatedAt time.Time               `json:"updatedAt"`
	Session   *MissionSessionResponse `json:"session,omitempty"`
}
type MissionSessionResponse struct {
	RefID     string    `json:"attemptSessionRefId"`
	Seq       int64     `json:"generation"`
	SessionID string    `json:"sessionId"`
	Harness   string    `json:"harness"`
	Mode      string    `json:"mode"`
	Status    string    `json:"status" enum:"unknown"`
	BoundAt   time.Time `json:"boundAt"`
}
type MissionAttentionResponse struct {
	Kind       string `json:"kind" enum:"needs_choice,needs_input"`
	ReasonCode string `json:"reasonCode"`
	QuestionID string `json:"questionId,omitempty"`
	Generation string `json:"generation"`
}
type MissionEdgeResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type MissionEnvelope struct {
	Mission MissionProjectionResponse `json:"mission"`
}

func missionResponse(view outcomevc.MissionProjection) MissionProjectionResponse {
	out := MissionProjectionResponse{Version: view.Version, OutcomeID: string(view.OutcomeID), MissionID: string(view.MissionID), ContractRevisionNumber: view.ContractRevisionNumber, PlanRevisionID: string(view.PlanRevisionID), PlanRevisionNumber: view.PlanRevisionNumber, TopologyFingerprint: view.TopologyFingerprint, TopologyGeneration: view.TopologyGeneration, Generation: view.Generation, UpdatedAt: view.UpdatedAt, NextRunnableWorkUnitID: string(view.NextRunnableID), CustodyHeldByWorkUnitID: string(view.CustodyHeldBy), NoRunnableReason: view.NoRunnableReason}
	out.Nodes = make([]MissionNodeResponse, 0, len(view.Nodes))
	out.Edges = make([]MissionEdgeResponse, 0, len(view.Edges))
	for _, n := range view.Nodes {
		r := MissionNodeResponse{WorkUnitID: n.WorkUnitID, PlanRevisionID: n.PlanRevisionID, Title: n.Title, DependsOn: stringWorkUnitIDs(n.DependsOn), ScheduleState: n.ScheduleState, BlockingDependencies: stringWorkUnitIDs(n.BlockingDependencies), BlockedReason: n.BlockedReason, BlockedDetail: n.BlockedDetail, CriterionIDs: stringCriterionIDs(n.CriterionIDs), CriterionReady: map[string]bool{}, NextAction: n.NextAction, Responsibility: n.Responsibility, UpdatedAt: n.UpdatedAt, Generation: n.Generation}
		for k, v := range n.CriterionReady {
			r.CriterionReady[string(k)] = v
		}
		if n.CurrentAttempt != nil {
			a := n.CurrentAttempt
			r.CurrentAttempt = &MissionAttemptResponse{ID: a.ID, Number: a.Number, Status: a.Status, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
			if a.Session != nil {
				x := a.Session
				r.CurrentAttempt.Session = &MissionSessionResponse{RefID: x.RefID, Seq: x.Seq, SessionID: x.SessionID, Harness: x.Harness, Mode: x.Mode, Status: x.Status, BoundAt: x.BoundAt}
			}
		}
		if n.Attention != nil {
			q := n.Attention
			r.Attention = &MissionAttentionResponse{Kind: q.Kind, ReasonCode: q.ReasonCode, QuestionID: q.QuestionID, Generation: q.Generation}
		}
		out.Nodes = append(out.Nodes, r)
	}
	for _, e := range view.Edges {
		out.Edges = append(out.Edges, MissionEdgeResponse{From: string(e.From), To: string(e.To)})
	}
	return out
}

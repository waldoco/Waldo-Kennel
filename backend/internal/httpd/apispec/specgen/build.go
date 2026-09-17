// Package specgen builds the code-first OpenAPI document from the Go contract
// types. It lives outside apispec because it imports the controllers (to
// reflect their request/response shapes), and controllers import apispec (for
// the 501 stub) — keeping Build here breaks that cycle. apispec only embeds and
// serves the committed openapi.yaml; specgen produces it.
package specgen

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	jsonschema "github.com/swaggest/jsonschema-go"
	openapi "github.com/swaggest/openapi-go"
	"github.com/swaggest/openapi-go/openapi31"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/controllers"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/envelope"
	projectsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/project"
)

// Build reflects the Go contract types and the operation registry below into
// the OpenAPI document. It is the single source of truth for the /api/v1
// contract: `cmd/genspec` writes its output to apispec/openapi.yaml (the
// committed, embedded artifact) and TestBuild_MatchesEmbedded asserts the embed
// equals fresh Build() output so the two can never drift. Schema facets live as
// struct tags on the service.*/controllers.* types; operation metadata (path,
// status codes, summaries) lives here.
//
// Every wire shape is reflected straight from where it is used at runtime — the
// request bodies, path params, and response envelopes from controllers, the
// error envelope from httpd/envelope — so the served responses and the
// generated schema share one definition each.
func Build() ([]byte, error) {
	r := openapi31.NewReflector()
	// Derive `required` from the idiomatic Go convention: a JSON field without
	// `omitempty` is required. swaggest does not infer this on its own, so the
	// structs stay clean (only description/enum tags) and this hook adds the
	// required array. nonNullableSlices drops the spurious "null" type swaggest
	// stamps on every Go slice.
	r.DefaultOptions = append(r.DefaultOptions,
		jsonschema.InterceptProp(requiredFromJSONTag),
		jsonschema.InterceptNullability(nonNullableSlices),
		// Clean component schema names (which become the generated TS type names):
		// swaggest defaults to PackageType, e.g. "ProjectProject", "EnvelopeAPIError".
		jsonschema.InterceptDefName(schemaName),
	)

	r.Spec.SetTitle("Kennel HTTP daemon")
	r.Spec.SetVersion("0.1.0-route-shell")
	r.Spec.SetDescription("Loopback-only HTTP surface served by the Go daemon. " +
		"Generated from Go (code-first) — do not edit by hand; run `go generate ./...`.")
	r.Spec.Servers = []openapi31.Server{
		*(&openapi31.Server{URL: "http://127.0.0.1:3031"}).WithDescription("Local daemon (loopback only)"),
	}
	r.Spec.Tags = []openapi31.Tag{
		*(&openapi31.Tag{Name: "agents"}).WithDescription(
			"Supported and locally runnable agent adapters"),
		*(&openapi31.Tag{Name: "projects"}).WithDescription(
			"Project registry, configuration, and lifecycle administration"),
		*(&openapi31.Tag{Name: "sessions"}).WithDescription(
			"Agent session lifecycle and messaging"),
		*(&openapi31.Tag{Name: "prs"}).WithDescription(
			"Pull-request actions (SCM lane)"),
		*(&openapi31.Tag{Name: "reviews"}).WithDescription(
			"Code-review runs and findings"),
		*(&openapi31.Tag{Name: "notifications"}).WithDescription(
			"Durable dashboard notifications"),
		*(&openapi31.Tag{Name: "outcomes"}).WithDescription(
			"Canonical Work Outcome contracts and immutable revisions"),
		*(&openapi31.Tag{Name: "intakes"}).WithDescription(
			"Shared Home and Work adaptive intake plus explicit responsibility lineage"),
		*(&openapi31.Tag{Name: "usage"}).WithDescription(
			"Token usage telemetry for Kennel sessions"),
		*(&openapi31.Tag{Name: "push"}).WithDescription(
			"Mobile push-device registration for OS push notifications"),
		*(&openapi31.Tag{Name: "events"}).WithDescription(
			"Server-sent CDC event stream with durable replay"),
		*(&openapi31.Tag{Name: "harness-authority"}).WithDescription("Renderer-safe harness pairing and connection projections"),
		*(&openapi31.Tag{Name: "import"}).WithDescription(
			"Legacy Kennel project import (availability probe and run)"),
		*(&openapi31.Tag{Name: "dev"}).WithDescription(
			"Developer-only maintenance operations"),
		*(&openapi31.Tag{Name: "mobile"}).WithDescription(
			"Connect Mobile LAN bridge control (loopback/desktop only)"),
		*(&openapi31.Tag{Name: "browser"}).WithDescription(
			"Target-isolated desktop browser runtime (loopback only)"),
	}

	for _, op := range operations() {
		oc, err := r.NewOperationContext(op.method, op.path)
		if err != nil {
			return nil, fmt.Errorf("new operation %s %s: %w", op.method, op.path, err)
		}
		oc.SetID(op.id)
		oc.SetSummary(op.summary)
		oc.SetTags(op.tag)
		for _, param := range op.pathParams {
			oc.AddReqStructure(param)
		}
		if op.reqBody != nil {
			// AddReqStructure leaves requestBody.required absent, which
			// OpenAPI reads as optional. Most of these bodies are mandatory, so
			// force it — otherwise validators/generators treat the body as
			// skippable. Ops that genuinely accept an empty body opt out.
			if op.optionalReqBody {
				oc.AddReqStructure(op.reqBody)
			} else {
				oc.AddReqStructure(op.reqBody, openapi.WithCustomize(markRequestBodyRequired))
			}
		}
		for _, resp := range op.resps {
			opts := []openapi.ContentOption{openapi.WithHTTPStatus(resp.status)}
			if op.contentTypes != nil && op.contentTypes[resp.status] != "" {
				opts = append(opts, openapi.WithContentType(op.contentTypes[resp.status]))
			}
			oc.AddRespStructure(resp.body, opts...)
		}
		if err := r.AddOperation(oc); err != nil {
			return nil, fmt.Errorf("add operation %s %s: %w", op.method, op.path, err)
		}
	}

	return r.Spec.MarshalYAML()
}

// schemaName maps swaggest's default PackageType component names (e.g.
// "ProjectProject", "EnvelopeAPIError") to the clean, stable schema names that
// become the generated TypeScript type names. Every reflected type is listed
// explicitly: an unrecognised default name is returned verbatim, so a new type
// surfaces as a visibly-wrong "PackageType" name in the diff (and the drift
// test) rather than silently colliding with an existing schema via a
// TrimPrefix catch-all.
func schemaName(_ reflect.Type, defaultName string) string {
	if clean, ok := schemaNames[defaultName]; ok {
		return clean
	}
	return defaultName
}

// schemaNames is the exhaustive default→clean mapping for every type reflected
// by projectOperations(). Add an entry when a new contract type is introduced;
// the drift test fails until the spec is regenerated, which flags the gap.
var schemaNames = map[string]string{
	"ControllersSettingsResponse":                     "SettingsResponse",
	"ControllersUpdateSessionInterfaceRequest":        "UpdateSessionInterfaceRequest",
	"ControllersReasoningResponse":                    "ReasoningResponse",
	"ControllersUpdateReasoningRequest":               "UpdateReasoningRequest",
	"ControllersConversationSnapshotResponse":         "ConversationSnapshotResponse",
	"ControllersConversationTurnResponse":             "ConversationTurnResponse",
	"ControllersConversationTurnDiffResponse":         "ConversationTurnDiffResponse",
	"ControllersConversationDiffFileResponse":         "ConversationDiffFileResponse",
	"ControllersConversationMessageResponse":          "ConversationMessageResponse",
	"ControllersConversationActivityResponse":         "ConversationActivityResponse",
	"ControllersSendConversationMessageRequest":       "SendConversationMessageRequest",
	"ControllersConversationImageContentRequest":      "ConversationImageContentRequest",
	"ControllersConversationResourceContentRequest":   "ConversationResourceContentRequest",
	"ControllersSendConversationMessageResponse":      "SendConversationMessageResponse",
	"ControllersEditConversationMessageRequest":       "EditConversationMessageRequest",
	"ControllersConversationContentSummaryResponse":   "ConversationContentSummaryResponse",
	"ControllersEditConversationMessageResponse":      "EditConversationMessageResponse",
	"ControllersActivateConversationBranchResponse":   "ActivateConversationBranchResponse",
	"ControllersConversationBranchPointResponse":      "ConversationBranchPointResponse",
	"ControllersResolveConversationApprovalRequest":   "ResolveConversationApprovalRequest",
	"ControllersResolveConversationInputRequest":      "ResolveConversationInputRequest",
	"ControllersConversationModelsResponse":           "ConversationModelsResponse",
	"ControllersConversationModelResponse":            "ConversationModelResponse",
	"ControllersConversationConfigOptionsResponse":    "ConversationConfigOptionsResponse",
	"ControllersConversationConfigOptionResponse":     "ConversationConfigOptionResponse",
	"ControllersConversationConfigChoiceResponse":     "ConversationConfigChoiceResponse",
	"ControllersSetConversationConfigOptionRequest":   "SetConversationConfigOptionRequest",
	"ControllersConversationSkillsResponse":           "ConversationSkillsResponse",
	"ControllersConversationSkillResponse":            "ConversationSkillResponse",
	"ControllersConversationTurnSettingsPayload":      "ConversationTurnSettingsPayload",
	"ControllersConversationUsagePayload":             "ConversationUsagePayload",
	"ControllersConversationRateLimitsPayload":        "ConversationRateLimitsPayload",
	"ControllersConversationPlanResponse":             "ConversationPlanResponse",
	"ControllersConversationPlanStepResponse":         "ConversationPlanStepResponse",
	"ControllersConversationModelReroutePayload":      "ConversationModelReroutePayload",
	"ControllersConversationAccountPayload":           "ConversationAccountPayload",
	"ControllersConversationThreadStatePayload":       "ConversationThreadStatePayload",
	"ControllersConversationMCPServerPayload":         "ConversationMCPServerPayload",
	"ControllersReloadConversationMCPServersResponse": "ReloadConversationMCPServersResponse",
	"ControllersCompactConversationResponse":          "CompactConversationResponse",
	"ControllersRollbackConversationResponse":         "RollbackConversationResponse",
	"ControllersSetConversationTitleRequest":          "SetConversationTitleRequest",
	"ControllersSetConversationTitleResponse":         "SetConversationTitleResponse",
	"ControllersSteerConversationRequest":             "SteerConversationRequest",
	"ControllersSteerConversationResponse":            "SteerConversationResponse",
	"ControllersPromoteQueuedTurnResponse":            "PromoteQueuedTurnResponse",
	// httpd/envelope
	"EnvelopeAPIError": "APIError",
	// domain
	"DomainProjectID":                 "ProjectID",
	"DomainSessionID":                 "SessionID",
	"DomainIssueID":                   "IssueID",
	"DomainSession":                   "Session",
	"DomainProjectConfig":             "ProjectConfig",
	"DomainTrackerIntakeConfig":       "TrackerIntakeConfig",
	"ControllersTriggerReviewRequest": "TriggerReviewRequest",
	"DomainContainerReapConfig":       "ContainerReapConfig",
	"DomainAgentConfig":               "AgentConfig",
	"DomainRoleOverride":              "RoleOverride",
	"DomainProjectAgentPreferences":   "ProjectAgentPreferences",
	"DomainResolvedMissionRoles":      "ResolvedMissionRoles",
	"DomainResolvedAgentRole":         "ResolvedAgentRole",
	"DomainRoleSource":                "RoleSource",
	// httpd/controllers (wire envelopes)
	"ControllersListProjectsResponse":                     "ListProjectsResponse",
	"ControllersProjectResponse":                          "ProjectResponse",
	"ControllersResolvedMissionRolesResponse":             "ResolvedMissionRolesResponse",
	"ControllersAgentIDParam":                             "AgentIDParam",
	"ControllersGetProjectResponse":                       "ProjectGetResponse",
	"ControllersProjectOrDegraded":                        "ProjectOrDegraded",
	"ControllersListSessionsQuery":                        "ListSessionsQuery",
	"ControllersCleanupSessionsQuery":                     "CleanupSessionsQuery",
	"ControllersListSessionsResponse":                     "ListSessionsResponse",
	"ControllersSpawnSessionRequest":                      "SpawnSessionRequest",
	"ControllersSpawnSessionResponse":                     "SpawnSessionResponse",
	"ControllersSessionResponse":                          "SessionResponse",
	"ControllersSessionPreviewResponse":                   "SessionPreviewResponse",
	"ControllersSetSessionPreviewRequest":                 "SetSessionPreviewRequest",
	"ControllersStartPreviewServerRequest":                "StartPreviewServerRequest",
	"ControllersPreviewServerStatusResponse":              "PreviewServerStatusResponse",
	"ControllersBrowserStatusQuery":                       "BrowserStatusQuery",
	"ControllersBrowserStatusResponse":                    "BrowserStatusResponse",
	"ControllersBrowserCommandRequest":                    "BrowserCommandRequest",
	"ControllersBrowserCommandResponse":                   "BrowserCommandResponse",
	"ControllersSetSessionMergePolicyRequest":             "SetSessionMergePolicyRequest",
	"ControllersSetSessionMergePolicyResponse":            "SetSessionMergePolicyResponse",
	"ControllersSetSessionAutoInjectReviewRequest":        "SetSessionAutoInjectReviewRequest",
	"ControllersSetSessionAutoInjectReviewResponse":       "SetSessionAutoInjectReviewResponse",
	"ControllersSetSessionAutoInjectCIRequest":            "SetSessionAutoInjectCIRequest",
	"ControllersSetSessionAutoInjectCIResponse":           "SetSessionAutoInjectCIResponse",
	"ControllersRenameSessionRequest":                     "RenameSessionRequest",
	"ControllersSetSessionReviewerRequest":                "SetSessionReviewerRequest",
	"ControllersRenameSessionResponse":                    "RenameSessionResponse",
	"ControllersRestoreSessionResponse":                   "RestoreSessionResponse",
	"ControllersResumeAgentResponse":                      "ResumeAgentResponse",
	"ControllersSwitchAgentRequest":                       "SwitchAgentRequest",
	"ControllersAgentSwitchView":                          "AgentSwitch",
	"ControllersAgentSwitchResponse":                      "AgentSwitchResponse",
	"ControllersListAgentSwitchesResponse":                "ListAgentSwitchesResponse",
	"ControllersSubmitAgentHandoffRequest":                "SubmitAgentHandoffRequest",
	"ControllersStartSessionInterfaceTransitionRequest":   "StartSessionInterfaceTransitionRequest",
	"ControllersSessionInterfaceTransitionView":           "SessionInterfaceTransition",
	"ControllersSessionInterfaceTransitionStatusResponse": "SessionInterfaceTransitionStatusResponse",
	"ControllersStartSessionInterfaceTransitionResponse":  "StartSessionInterfaceTransitionResponse",
	"ControllersCancelSessionInterfaceTransitionResponse": "CancelSessionInterfaceTransitionResponse",
	"ControllersCleanupSessionsResponse":                  "CleanupSessionsResponse",
	"ControllersCleanupSkippedSession":                    "CleanupSkippedSession",
	"ControllersWorkspaceFileQuery":                       "WorkspaceFileQuery",
	"ControllersStageSessionAttachmentsRequest":           "StageSessionAttachmentsRequest",
	"ControllersStageSessionAttachmentsResponse":          "StageSessionAttachmentsResponse",
	"ControllersAttachmentInput":                          "AttachmentInput",
	"ControllersListWorkspaceFilesResponse":               "ListWorkspaceFilesResponse",
	"ControllersWorkspaceFileSummary":                     "WorkspaceFileSummary",
	"ControllersWorkspaceFileResponse":                    "WorkspaceFileResponse",
	"ControllersKillSessionResponse":                      "KillSessionResponse",
	"ControllersRollbackSessionResponse":                  "RollbackSessionResponse",
	"ControllersSendSessionMessageRequest":                "SendSessionMessageRequest",
	"ControllersSendSessionMessageResponse":               "SendSessionMessageResponse",
	"ControllersDelegateTaskRequest":                      "DelegateTaskRequest",
	"ControllersDelegateTaskResponse":                     "DelegateTaskResponse",
	"ControllersClaimPRResponse":                          "ClaimPRResponse",
	"ControllersClaimPRRequest":                           "ClaimPRRequest",
	"ControllersSessionPRFacts":                           "SessionPRFacts",
	"ControllersSessionPRSummary":                         "SessionPRSummary",
	"ControllersSessionPRCISummary":                       "SessionPRCISummary",
	"ControllersSessionPRFailingCheck":                    "SessionPRFailingCheck",
	"ControllersSessionPRReviewSummary":                   "SessionPRReviewSummary",
	"ControllersSessionPRReviewEntry":                     "SessionPRReviewEntry",
	"ControllersSessionPRUnresolvedReviewer":              "SessionPRUnresolvedReviewer",
	"ControllersSessionPRReviewCommentLink":               "SessionPRReviewCommentLink",
	"ControllersSessionPRMergeabilitySummary":             "SessionPRMergeabilitySummary",
	"ControllersSessionPRConflictFile":                    "SessionPRConflictFile",
	"ControllersListSessionPRsResponse":                   "ListSessionPRsResponse",
	"ControllersSetActivityRequest":                       "SetActivityRequest",
	"ControllersSetActivityResponse":                      "SetActivityResponse",
	"ControllersSetReviewActivityRequest":                 "SetReviewActivityRequest",
	"ControllersSetReviewActivityResponse":                "SetReviewActivityResponse",
	"ControllersSpawnOrchestratorRequest":                 "SpawnOrchestratorRequest",
	"ControllersSpawnOrchestratorResponse":                "SpawnOrchestratorResponse",
	"ControllersOrchestratorResponse":                     "OrchestratorResponse",
	"AgentInventory":                                      "ListAgentsResponse",
	"AgentInfo":                                           "AgentInfo",
	"AgentProbeResult":                                    "ProbeAgentResponse",
	"PortsAgentModelCatalog":                              "AgentModelsResponse",
	"PortsAgentModelInfo":                                 "AgentModelInfo",
	"ControllersListNotificationsQuery":                   "ListNotificationsQuery",
	"ControllersNotificationStreamQuery":                  "NotificationStreamQuery",
	"ControllersNotificationIDParam":                      "NotificationIDParam",
	"ControllersNotificationTarget":                       "NotificationTarget",
	"ControllersNotificationResponse":                     "NotificationResponse",
	"ControllersListNotificationsResponse":                "ListNotificationsResponse",
	"ControllersCreateOutcomeRequest":                     "CreateOutcomeRequest",
	"ControllersCreateIntakeRequest":                      "CreateIntakeRequest",
	"ControllersAnalyzeIntakeRequest":                     "AnalyzeIntakeRequest",
	"ControllersAnswerIntakeClarificationRequest":         "AnswerIntakeClarificationRequest",
	"ControllersReviseIntakeProposalRequest":              "ReviseIntakeProposalRequest",
	"ControllersConfirmIntakeRequest":                     "ConfirmIntakeRequest",
	"ControllersCancelIntakeRequest":                      "CancelIntakeRequest",
	"ControllersIntakeEnvelope":                           "IntakeEnvelope",
	"ControllersIntakeSnapshotResponse":                   "IntakeSnapshotResponse",
	"ControllersIntakeSessionResponse":                    "IntakeSessionResponse",
	"ControllersIntakeProposalInput":                      "IntakeProposalInput",
	"ControllersIntakeProposalResponse":                   "IntakeProposalResponse",
	"ControllersIntakeClarificationResponse":              "IntakeClarificationResponse",
	"ControllersIntakeIDParam":                            "IntakeIDParam",
	"ControllersCreateResponsibilityLinkRequest":          "CreateResponsibilityLinkRequest",
	"ControllersEndResponsibilityLinkRequest":             "EndResponsibilityLinkRequest",
	"ControllersResponsibilityLinkEnvelope":               "ResponsibilityLinkEnvelope",
	"ControllersResponsibilityLinkResponse":               "ResponsibilityLinkResponse",
	"ControllersResponsibilityLinkIDParam":                "ResponsibilityLinkIDParam",
	"ControllersOpenWaldoEpisodeRequest":                  "OpenWaldoEpisodeRequest",
	"ControllersAppendWaldoTurnRequest":                   "AppendWaldoTurnRequest",
	"ControllersAttachWaldoContextRequest":                "AttachWaldoContextRequest",
	"ControllersDetachWaldoContextRequest":                "DetachWaldoContextRequest",
	"ControllersContinueWaldoConversationRequest":         "ContinueWaldoConversationRequest",
	"ControllersWaldoConversationResponse":                "WaldoConversationResponse",
	"ControllersWaldoProviderEpisodeRefResponse":          "WaldoProviderEpisodeRefResponse",
	"ControllersWaldoProviderTurnRefResponse":             "WaldoProviderTurnRefResponse",
	"ControllersWaldoConversationEpisodeResponse":         "WaldoConversationEpisodeResponse",
	"ControllersWaldoContextProvenanceResponse":           "WaldoContextProvenanceResponse",
	"ControllersWaldoContextRefResponse":                  "WaldoContextRefResponse",
	"ControllersWaldoContextAttachmentResponse":           "WaldoContextAttachmentResponse",
	"ControllersWaldoConversationTurnResponse":            "WaldoConversationTurnResponse",
	"ControllersWaldoContinuationEvidenceResponse":        "WaldoContinuationEvidenceResponse",
	"ControllersWaldoContinuationBindingsResponse":        "WaldoContinuationBindingsResponse",
	"ControllersWaldoContinuationReceiptResponse":         "WaldoContinuationReceiptResponse",
	"ControllersWaldoConversationSnapshotResponse":        "WaldoConversationSnapshotResponse",
	"ControllersWaldoConversationEnvelope":                "WaldoConversationEnvelope",
	"ControllersWaldoTurnEnvelope":                        "WaldoTurnEnvelope",
	"ControllersWaldoContinuationEnvelope":                "WaldoContinuationEnvelope",
	"ControllersWaldoContextAttachmentIDParam":            "WaldoContextAttachmentIDParam",
	"ControllersReviseOutcomeContractRequest":             "ReviseOutcomeContractRequest",
	"ControllersOutcomeCompositionEnvelope":               "OutcomeCompositionEnvelope",
	"ControllersOutcomeCompositionResponse":               "OutcomeCompositionResponse",
	"ControllersContributorResponse":                      "ContributorResponse",
	"ControllersContributionLinkResponse":                 "ContributionLinkResponse",
	"ControllersCriterionClaimResponse":                   "CriterionClaimResponse",
	"ControllersProposeDecompositionRequest":              "ProposeDecompositionRequest",
	"ControllersProposedContributionRequest":              "ProposedContributionRequest",
	"ControllersContributionDependencyRequest":            "ContributionDependencyRequest",
	"ControllersProposedContributionResponse":             "ProposedContributionResponse",
	"ControllersContributionDependencyResponse":           "ContributionDependencyResponse",
	"ControllersDecompositionResponse":                    "DecompositionResponse",
	"ControllersDecompositionEnvelope":                    "DecompositionEnvelope",
	"ControllersWaiveContributionDependencyRequest":       "WaiveContributionDependencyRequest",
	"ControllersUpstreamBlockResponse":                    "UpstreamBlockResponse",
	"ControllersContributorAttentionResponse":             "ContributorAttentionResponse",
	"ControllersParentAttentionResponse":                  "ParentAttentionResponse",
	"ControllersAcceptContributorBatchRequest":            "AcceptContributorBatchRequest",
	"ControllersBatchEntryVerdictResponse":                "BatchEntryVerdictResponse",
	"ControllersBatchEligibilityEnvelope":                 "BatchEligibilityEnvelope",
	"ControllersAcceptBatchResponse":                      "AcceptBatchResponse",
	"ControllersAcceptBatchEnvelope":                      "AcceptBatchEnvelope",
	"ControllersAskForDecompositionRequest":               "AskForDecompositionRequest",
	"ControllersSubmitAgentProposalRequest":               "SubmitAgentProposalRequest",
	"ControllersDecompositionRequestResponse":             "DecompositionRequestResponse",
	"ControllersDecompositionRequestEnvelope":             "DecompositionRequestEnvelope",
	"ControllersOutcomeEnvelope":                          "OutcomeEnvelope",
	"ControllersOutcomesEnvelope":                         "OutcomesEnvelope",
	"ControllersOutcomeResponse":                          "OutcomeResponse",
	"ControllersPlanEnvelope":                             "PlanEnvelope",
	"ControllersPlanRevisionResponse":                     "PlanRevisionResponse",
	"ControllersPlanWorkUnitResponse":                     "PlanWorkUnitResponse",
	"ControllersCapabilityGrantResponse":                  "CapabilityGrantResponse",
	"ControllersScheduleWorkUnitResponse":                 "ScheduleWorkUnitResponse",
	"ControllersScheduleAttemptBrief":                     "ScheduleAttemptBrief",
	"ControllersScheduleResponse":                         "ScheduleResponse",
	"ControllersScheduleEnvelope":                         "ScheduleEnvelope",
	"ControllersProposePlanRequest":                       "ProposePlanRequest",
	"ControllersReplanPlanRequest":                        "ReplanPlanRequest",
	"ControllersApprovePlanRequest":                       "ApprovePlanRequest",
	"ControllersPlanningSessionIDParam":                   "PlanningSessionIDParam",
	"ControllersPlanningCandidatesQuery":                  "PlanningCandidatesQuery",
	"ControllersStartPlanningRequest":                     "StartPlanningRequest",
	"ControllersPlanningMessageRequest":                   "PlanningMessageRequest",
	"ControllersPlanningFinalizeRequest":                  "PlanningFinalizeRequest",
	"ControllersPlanningCancelRequest":                    "PlanningCancelRequest",
	"ControllersPlanningBindingResponse":                  "PlanningBindingResponse",
	"ControllersPlanningCandidateResponse":                "PlanningCandidateResponse",
	"ControllersPlanningCandidatesEnvelope":               "PlanningCandidatesEnvelope",
	"ControllersPlanningClarificationResponse":            "PlanningClarificationResponse",
	"ControllersPlanningContractChangeResponse":           "PlanningContractChangeResponse",
	"ControllersPlanningTurnResponse":                     "PlanningTurnResponse",
	"ControllersPlanningSessionResponse":                  "PlanningSessionResponse",
	"ControllersPlanningResponse":                         "PlanningResponse",
	"ControllersPlanningEnvelope":                         "PlanningEnvelope",
	"ControllersContractRevisionResponse":                 "ContractRevisionResponse",
	"ControllersContractCriterionResponse":                "ContractCriterionResponse",
	"ControllersOutcomeIDParam":                           "OutcomeIDParam",
	"ControllersRecordEvidenceRequest":                    "RecordEvidenceRequest",
	"ControllersRecordVerificationRequest":                "RecordVerificationRequest",
	"ControllersDecideAcceptanceRequest":                  "DecideAcceptanceRequest",
	"ControllersOutcomeProofEnvelope":                     "OutcomeProofEnvelope",
	"ControllersOutcomeProofResponse":                     "OutcomeProofResponse",
	"ControllersCriterionProofResponse":                   "CriterionProofResponse",
	"ControllersEvidenceItemResponse":                     "EvidenceItemResponse",
	"ControllersVerificationRunResponse":                  "VerificationRunResponse",
	"ControllersAcceptanceDecisionResponse":               "AcceptanceDecisionResponse",
	"ControllersOutcomeCorrectionResponse":                "OutcomeCorrectionResponse",
	"ControllersStartOutcomeAttemptRequest":               "StartOutcomeAttemptRequest",
	"ControllersRecordObservationRequest":                 "RecordObservationRequest",
	"ControllersAttemptRecoveryRequest":                   "AttemptRecoveryRequest",
	"ControllersAttemptEnvelope":                          "AttemptEnvelope",
	"ControllersAttemptListEnvelope":                      "AttemptListEnvelope",
	"ControllersObservationEnvelope":                      "ObservationEnvelope",
	"ControllersAttemptRecoveryEnvelope":                  "AttemptRecoveryEnvelope",
	"ControllersAttemptResponse":                          "AttemptResponse",
	"ControllersAttemptPresentationResponse":              "AttemptPresentationResponse",
	"ControllersAttemptSessionRefResponse":                "AttemptSessionRefResponse",
	"ControllersAttemptObservationResponse":               "AttemptObservationResponse",
	"ControllersRecoveryReceiptResponse":                  "RecoveryReceiptResponse",
	"ControllersAttemptFenceResponse":                     "AttemptFenceResponse",
	"ControllersAttemptIDParam":                           "AttemptIDParam",
	"ControllersNeedsYouQuestionIDParam":                  "NeedsYouQuestionIDParam",
	"ControllersNeedsYouQuestionsEnvelope":                "NeedsYouQuestionsEnvelope",
	"ControllersNeedsYouAnswerRequest":                    "NeedsYouAnswerRequest",
	"ControllersNeedsYouQuestionEnvelope":                 "NeedsYouQuestionEnvelope",
	"ControllersNeedsYouReconcileRequest":                 "NeedsYouReconcileRequest",
	"DomainNeedsYouQuestion":                              "NeedsYouQuestion",
	"DomainNeedsYouOption":                                "NeedsYouOption",
	"DomainChatDecisionAnswer":                            "ChatDecisionAnswer",
	"DomainChatInputAnswer":                               "ChatInputAnswer",
	"ControllersMarkNotificationReadRequest":              "MarkNotificationReadRequest",
	"ControllersNotificationEnvelope":                     "NotificationEnvelope",
	"ControllersMarkAllNotificationsReadRequest":          "MarkAllNotificationsReadRequest",
	"ControllersMarkAllNotificationsReadResponse":         "MarkAllNotificationsReadResponse",
	"ControllersUsageHookMetadata":                        "UsageHookMetadata",
	"ControllersListUsageSessionsQuery":                   "ListUsageSessionsQuery",
	"ControllersCompactSessionUsageResponse":              "CompactSessionUsageResponse",
	"ControllersListCompactSessionUsageResponse":          "ListCompactSessionUsageResponse",
	"ControllersUsageTotalsResponse":                      "UsageTotalsResponse",
	"ControllersUsageModelResponse":                       "UsageModelResponse",
	"ControllersUsageHarnessResponse":                     "UsageHarnessResponse",
	"ControllersSessionUsageResponse":                     "SessionUsageResponse",
	// httpd/controllers — standalone shell terminal wire envelopes
	"ControllersShellTerminalHandleIDParam": "ShellTerminalHandleIDParam",
	"ControllersOpenShellTerminalRequest":   "OpenShellTerminalRequest",
	"ControllersUpdateShellTerminalRequest": "UpdateShellTerminalRequest",
	"ControllersShellTerminalResponse":      "ShellTerminalResponse",
	"ControllersListShellTerminalsResponse": "ListShellTerminalsResponse",
	"ControllersShellTerminalEnvelope":      "ShellTerminalEnvelope",
	// httpd/controllers — PR wire envelopes
	"ControllersMergePRRequest":          "MergePRRequest",
	"ControllersMergePRResponse":         "MergePRResponse",
	"ControllersResolveCommentsRequest":  "ResolveCommentsRequest",
	"ControllersResolveCommentsResponse": "ResolveCommentsResponse",
	// httpd/controllers — review wire envelopes
	"ControllersListReviewsResponse":   "ListReviewsResponse",
	"ControllersReviewRunResponse":     "ReviewRunResponse",
	"ControllersTriggerReviewResponse": "TriggerReviewResponse",
	"ControllersCancelReviewResponse":  "CancelReviewResponse",
	"ControllersKillReviewResponse":    "KillReviewResponse",
	"ControllersRestoreReviewResponse": "RestoreReviewResponse",
	"ControllersSubmitReviewItem":      "SubmitReviewItem",
	"ControllersSubmitReviewInput":     "SubmitReviewInput",
	// domain review entities
	"DomainReviewRun":     "ReviewRun",
	"ReviewPRReviewState": "PRReviewState",
	// httpd/controllers: dev wire envelopes
	"ControllersDevImportProjectsRequest":  "DevImportProjectsRequest",
	"ControllersDevImportProjectsResponse": "DevImportProjectsResponse",
	// httpd/controllers: mobile wire envelopes
	"ControllersMobileStatusResponse":  "MobileStatusResponse",
	"ControllersMobileDeviceResponse":  "MobileDeviceResponse",
	"ControllersMobileDevicesResponse": "MobileDevicesResponse",
	"ControllersMuteDeviceRequest":     "MuteDeviceRequest",
	"ControllersInstallIDParam":        "InstallIDParam",
	"ControllersPushPairingIDParam":    "PushPairingIDParam",
	// devimport report
	"DevimportReport":   "DevImportProjectsReport",
	"DevimportConflict": "DevImportProjectsConflict",
	// httpd/controllers: push-device wire envelopes
	"ControllersRegisterPushDeviceRequest":    "RegisterPushDeviceRequest",
	"ControllersPushDeviceEnvelope":           "PushDeviceEnvelope",
	"ControllersPushDeviceResponse":           "PushDeviceResponse",
	"ControllersUnregisterPushDeviceResponse": "UnregisterPushDeviceResponse",
	// service/project entities + DTOs
	"ProjectProject":                    "Project",
	"ProjectSummary":                    "ProjectSummary",
	"ProjectDegraded":                   "DegradedProject",
	"ProjectAddInput":                   "AddProjectInput",
	"ProjectInitializeRepositoryInput":  "InitializeRepositoryInput",
	"ProjectInitializeRepositoryResult": "InitializeRepositoryResult",
	"ProjectRemoveResult":               "RemoveProjectResult",
	"ProjectSetConfigInput":             "SetProjectConfigInput",
	"ProjectUpdateSettingsInput":        "UpdateProjectSettingsInput",
	"ProjectWorkspaceRepo":              "WorkspaceRepo",
	"SessionWorkspaceFileStatus":        "WorkspaceFileStatus",
}

// markRequestBodyRequired sets requestBody.required: true on the operation's
// JSON body. swaggest leaves it absent (== optional) for AddReqStructure bodies.
func markRequestBodyRequired(cor openapi.ContentOrReference) {
	if rb, ok := cor.(*openapi31.RequestBodyOrReference); ok && rb.RequestBody != nil {
		rb.RequestBody.WithRequired(true)
	}
}

// nonNullableSlices drops the "null" that swaggest unions into every Go slice
// type (a nil slice marshals as JSON null). A required array field should be
// `T[]`, not `T[] | null`; the handlers normalise nil to an empty slice, so
// null never reaches the wire. Byte slices (base64 strings) are left alone.
func nonNullableSlices(p jsonschema.InterceptNullabilityParams) {
	if !p.NullAdded || p.Type == nil || p.Type.Kind() != reflect.Slice {
		return
	}
	if p.Type.Elem().Kind() == reflect.Uint8 {
		return
	}
	p.Schema.TypeEns().WithSimpleTypes(jsonschema.Array)
	p.Schema.Type.SliceOfSimpleTypeValues = nil
}

// requiredFromJSONTag marks a property required when its json tag lacks
// `omitempty` (the Go convention for "always present"). Runs after default
// processing so ParentSchema exists; skips fields without a json tag (e.g. path
// params, which swaggest marks required on their own).
func requiredFromJSONTag(p jsonschema.InterceptPropParams) error {
	if !p.Processed || p.ParentSchema == nil {
		return nil
	}
	jsonTag := p.Field.Tag.Get("json")
	if jsonTag == "" || jsonTag == "-" {
		return nil
	}
	parts := strings.Split(jsonTag, ",")
	name := parts[0]
	if name == "" {
		name = p.Name
	}
	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			return nil
		}
	}
	for _, existing := range p.ParentSchema.Required {
		if existing == name {
			return nil
		}
	}
	p.ParentSchema.Required = append(p.ParentSchema.Required, name)
	return nil
}

// --- operation registry -----------------------------------------------------

type respUnit struct {
	status int
	body   any
}

type operation struct {
	method, path, id, summary string
	tag                       string
	pathParams                []any // path/query param containers (e.g. ProjectIDParam)
	reqBody                   any   // JSON request body struct, nil when the op takes none
	// optionalReqBody declares the body without marking it required, for the
	// handlers that accept an empty body as a meaningful default.
	optionalReqBody bool
	resps           []respUnit
	contentTypes    map[int]string // optional non-JSON response content types by status
}

func operations() []operation {
	ops := append([]operation{}, eventOperations()...)
	ops = append(ops, harnessAuthorityOperations()...)
	ops = append(ops, agentOperations()...)
	ops = append(ops, projectOperations()...)
	ops = append(ops, sessionOperations()...)
	ops = append(ops, prOperations()...)
	ops = append(ops, reviewOperations()...)
	ops = append(ops, notificationOperations()...)
	ops = append(ops, intakeOperations()...)
	ops = append(ops, waldoConversationOperations()...)
	ops = append(ops, usageOperations()...)
	ops = append(ops, pushOperations()...)
	ops = append(ops, devOperations()...)
	ops = append(ops, mobileOperations()...)
	ops = append(ops, mobileDeviceOperations()...)
	ops = append(ops, browserOperations()...)
	ops = append(ops, shellTerminalOperations()...)
	ops = append(ops, outcomeRunOperations()...)
	return ops
}

// outcomeRunOperations declares the Mission supervision, delivery and
// attributed-usage operations. Must stay 1:1 with the routes
// OutcomesController.registerRunRoutes mounts (enforced by the parity test).
//
// The write operations are declared before their services exist so the
// generated contract carries their exact shape while they still answer 501.
func outcomeRunOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/projects/{id}/outcome-run-states", id: "listProjectOutcomeRunStates", tag: "outcomes",
			summary:    "Board projection: Mission state and eligible actions for a Project's Outcomes",
			pathParams: []any{controllers.ProjectIDParam{}, controllers.OutcomeRunScopeParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeRunStatesEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/run", id: "getOutcomeRunState", tag: "outcomes",
			summary:    "Read one Outcome's Mission state, eligible actions and blocker",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeRunStateEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/run", id: "commandOutcomeRun", tag: "outcomes",
			summary:    "Record durable run intent (start, pause, resume, cancel)",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.OutcomeRunCommandRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeRunStateEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/projects/{id}/outcome-trash", id: "listTrashedOutcomes", tag: "outcomes",
			summary: "List recoverable Outcomes and pending permanent cleanup", pathParams: []any{controllers.ProjectIDParam{}},
			resps: []respUnit{{http.StatusOK, controllers.OutcomeTrashEnvelope{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/deletion", id: "previewOutcomeDeletion", tag: "outcomes",
			summary: "Preview Outcome deletion scope and active execution blockers", pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{{http.StatusOK, controllers.OutcomeDeletionEnvelope{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/deletion", id: "changeOutcomeDeletion", tag: "outcomes",
			summary: "Move to Trash, restore, or permanently delete an inactive Outcome", pathParams: []any{controllers.OutcomeIDParam{}}, reqBody: controllers.ChangeOutcomeDeletionRequest{},
			resps: []respUnit{{http.StatusOK, controllers.OutcomeDeletionResult{}}, {http.StatusBadRequest, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/deliveries", id: "listOutcomeDeliveries", tag: "outcomes",
			summary:    "List durable deliveries of this Outcome's retained results",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeDeliveriesEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/deliveries", id: "requestOutcomeDelivery", tag: "outcomes",
			summary:    "Deliver one exact reviewed artifact to a destination",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.RequestOutcomeDeliveryRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.OutcomeDeliveryEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/deliveries/{deliveryId}", id: "getOutcomeDelivery", tag: "outcomes",
			summary:    "Read one delivery record",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.DeliveryIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeDeliveryEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/documents", id: "getOutcomeDocumentContext", tag: "outcomes",
			summary:    "Read the selected supplied documents and whether their sources changed",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeDocumentContextEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/documents", id: "selectOutcomeDocuments", tag: "outcomes",
			summary:    "Select local documents as this Outcome's material and snapshot their bytes",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.SelectOutcomeDocumentsRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.OutcomeDocumentContextEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/documents/approval", id: "approveOutcomeDocuments", tag: "outcomes",
			summary:    "Approve the reviewed document scope so work may be staged from it",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.ApproveOutcomeDocumentsRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeDocumentContextEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/usage", id: "getOutcomeUsage", tag: "outcomes",
			summary:    "Read reasoning and execution usage attributed to this Outcome",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeUsageEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

func waldoConversationOperations() []operation {
	standard := func(successes []respUnit) []respUnit {
		return append(successes,
			respUnit{http.StatusBadRequest, envelope.APIError{}},
			respUnit{http.StatusConflict, envelope.APIError{}},
			respUnit{http.StatusNotFound, envelope.APIError{}},
			respUnit{http.StatusInternalServerError, envelope.APIError{}},
			respUnit{http.StatusNotImplemented, envelope.APIError{}},
		)
	}
	project := []any{controllers.ProjectIDParam{}}
	return []operation{
		{method: http.MethodPost, path: "/api/v1/projects/{id}/waldo-conversation", id: "openProjectWaldoConversation", tag: "waldo-conversations", summary: "Open the one durable Waldo conversation for a Project", pathParams: project, resps: standard([]respUnit{{http.StatusCreated, controllers.WaldoConversationEnvelope{}}})},
		{method: http.MethodGet, path: "/api/v1/projects/{id}/waldo-conversation", id: "getProjectWaldoConversation", tag: "waldo-conversations", summary: "Read exact restart-safe Project Waldo conversation truth", pathParams: project, resps: standard([]respUnit{{http.StatusOK, controllers.WaldoConversationEnvelope{}}})},
		{method: http.MethodPost, path: "/api/v1/projects/{id}/waldo-conversation/episodes", id: "openProjectWaldoEpisode", tag: "waldo-conversations", summary: "Open one bounded provider-neutral conversation episode", pathParams: project, reqBody: controllers.OpenWaldoEpisodeRequest{}, resps: standard([]respUnit{{http.StatusCreated, controllers.WaldoConversationEnvelope{}}})},
		{method: http.MethodPost, path: "/api/v1/projects/{id}/waldo-conversation/turns", id: "appendProjectWaldoTurn", tag: "waldo-conversations", summary: "Append one ordered idempotent Project-bound turn", pathParams: project, reqBody: controllers.AppendWaldoTurnRequest{}, resps: standard([]respUnit{{http.StatusCreated, controllers.WaldoTurnEnvelope{}}, {http.StatusOK, controllers.WaldoTurnEnvelope{}}})},
		{method: http.MethodPost, path: "/api/v1/projects/{id}/waldo-conversation/context", id: "attachProjectWaldoContext", tag: "waldo-conversations", summary: "Explicitly attach provenance-bearing canonical context", pathParams: project, reqBody: controllers.AttachWaldoContextRequest{}, resps: standard([]respUnit{{http.StatusCreated, controllers.WaldoConversationEnvelope{}}})},
		{method: http.MethodPost, path: "/api/v1/projects/{id}/waldo-conversation/context/{attachmentId}/detach", id: "detachProjectWaldoContext", tag: "waldo-conversations", summary: "Explicitly detach context from future turns", pathParams: []any{controllers.ProjectIDParam{}, controllers.WaldoContextAttachmentIDParam{}}, reqBody: controllers.DetachWaldoContextRequest{}, resps: standard([]respUnit{{http.StatusOK, controllers.WaldoConversationEnvelope{}}})},
		{method: http.MethodPost, path: "/api/v1/projects/{id}/waldo-conversation/continuations", id: "continueProjectWaldoConversation", tag: "waldo-conversations", summary: "Evaluate and durably record bounded provider continuation policy", pathParams: project, reqBody: controllers.ContinueWaldoConversationRequest{}, resps: standard([]respUnit{{http.StatusCreated, controllers.WaldoContinuationEnvelope{}}})},
	}
}

func intakeOperations() []operation {
	standard := func(success int, body any) []respUnit {
		return []respUnit{{success, body}, {http.StatusBadRequest, envelope.APIError{}}, {http.StatusConflict, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}}
	}
	return []operation{
		{method: http.MethodPost, path: "/api/v1/projects/{id}/intakes", id: "createOutcomeIntake", tag: "intakes", summary: "Capture one simple natural-language Outcome statement", pathParams: []any{controllers.ProjectIDParam{}}, reqBody: controllers.CreateIntakeRequest{}, resps: standard(http.StatusCreated, controllers.IntakeEnvelope{})},
		{method: http.MethodGet, path: "/api/v1/intakes/{intakeId}", id: "getIntake", tag: "intakes", summary: "Read durable shared intake state", pathParams: []any{controllers.IntakeIDParam{}}, resps: standard(http.StatusOK, controllers.IntakeEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/intakes/{intakeId}/analysis", id: "analyzeIntake", tag: "intakes", summary: "Analyze intent into one material question or a Contract proposal", pathParams: []any{controllers.IntakeIDParam{}}, reqBody: controllers.AnalyzeIntakeRequest{}, resps: standard(http.StatusOK, controllers.IntakeEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/intakes/{intakeId}/clarification", id: "answerIntakeClarification", tag: "intakes", summary: "Answer the intake's single material clarification", pathParams: []any{controllers.IntakeIDParam{}}, reqBody: controllers.AnswerIntakeClarificationRequest{}, resps: standard(http.StatusOK, controllers.IntakeEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/intakes/{intakeId}/proposals", id: "reviseIntakeProposal", tag: "intakes", summary: "Append an edited immutable Contract proposal revision", pathParams: []any{controllers.IntakeIDParam{}}, reqBody: controllers.ReviseIntakeProposalRequest{}, resps: standard(http.StatusOK, controllers.IntakeEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/intakes/{intakeId}/confirmation", id: "confirmIntakeOutcome", tag: "intakes", summary: "Atomically confirm exactly one Outcome and ContractRevision", pathParams: []any{controllers.IntakeIDParam{}}, reqBody: controllers.ConfirmIntakeRequest{}, resps: standard(http.StatusCreated, controllers.IntakeEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/intakes/{intakeId}/cancellation", id: "cancelIntake", tag: "intakes", summary: "Consciously release an unconfirmed intake", pathParams: []any{controllers.IntakeIDParam{}}, reqBody: controllers.CancelIntakeRequest{}, resps: standard(http.StatusOK, controllers.IntakeEnvelope{})},
		{method: http.MethodGet, path: "/api/v1/intakes/{intakeId}/analysis-request", id: "getLatestIntakeAnalysisRequest", tag: "intakes", summary: "Read the newest agent analysis ask and what became of it", pathParams: []any{controllers.IntakeIDParam{}}, resps: standard(http.StatusOK, controllers.IntakeAnalysisRequestEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/intakes/{intakeId}/analysis-request/cancellation", id: "cancelIntakeAnalysisRequest", tag: "intakes", summary: "Stop waiting for an agent and return the intake to a retryable state", pathParams: []any{controllers.IntakeIDParam{}}, resps: standard(http.StatusOK, controllers.IntakeEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/intake-analysis-requests/{requestId}/proposal", id: "submitIntakeAnalysisProposal", tag: "intakes", summary: "Callback: a spawned agent submits its proposed Contract", pathParams: []any{controllers.IntakeAnalysisRequestIDParam{}}, reqBody: controllers.SubmitIntakeAnalysisRequest{}, resps: standard(http.StatusOK, controllers.IntakeEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/responsibility-links", id: "createResponsibilityLink", tag: "intakes", summary: "Preserve explicit Home Open Loop to Work Outcome lineage", reqBody: controllers.CreateResponsibilityLinkRequest{}, resps: standard(http.StatusCreated, controllers.ResponsibilityLinkEnvelope{})},
		{method: http.MethodGet, path: "/api/v1/responsibility-links/{responsibilityLinkId}", id: "getResponsibilityLink", tag: "intakes", summary: "Read explicit responsibility lineage", pathParams: []any{controllers.ResponsibilityLinkIDParam{}}, resps: standard(http.StatusOK, controllers.ResponsibilityLinkEnvelope{})},
		{method: http.MethodPost, path: "/api/v1/responsibility-links/{responsibilityLinkId}/end", id: "endResponsibilityLink", tag: "intakes", summary: "End lineage without changing either responsibility lifecycle", pathParams: []any{controllers.ResponsibilityLinkIDParam{}}, reqBody: controllers.EndResponsibilityLinkRequest{}, resps: standard(http.StatusOK, controllers.ResponsibilityLinkEnvelope{})},
	}
}

func browserOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/browser/status", id: "getBrowserStatus", tag: "browser",
			summary:    "Check whether the desktop browser runtime is connected for a session",
			pathParams: []any{controllers.BrowserStatusQuery{}, controllers.BrowserCapabilityHeader{}},
			resps: []respUnit{
				{http.StatusOK, controllers.BrowserStatusResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/browser/commands", id: "executeBrowserCommand", tag: "browser",
			summary:    "Execute a target-scoped command in a session's desktop browser",
			pathParams: []any{controllers.BrowserCapabilityHeader{}},
			reqBody:    controllers.BrowserCommandRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.BrowserCommandResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusServiceUnavailable, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

type conversationSnapshotQuery struct {
	BeforeSequence *int64 `query:"beforeSequence,omitempty" minimum:"1" description:"Read items older than this conversation sequence. Omit for the newest page."`
	Limit          *int64 `query:"limit,omitempty" minimum:"1" maximum:"500" description:"Maximum combined messages and activities to return. Defaults to 200."`
}

func usageOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/usage/sessions", id: "listCompactSessionUsage", tag: "usage",
			summary:    "List compact token usage for session cards",
			pathParams: []any{controllers.ListUsageSessionsQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ListCompactSessionUsageResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/usage/sessions/{sessionId}", id: "getSessionUsage", tag: "usage",
			summary:    "Get detailed token usage for one session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionUsageResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

// shellTerminalOperations describes the standalone shell terminal surface:
// shells the user opens by hand, with no agent session behind them.
func shellTerminalOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/settings", id: "getSettings", tag: "settings",
			summary: "Read the daemon-owned user preferences",
			resps: []respUnit{
				{http.StatusOK, controllers.SettingsResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/settings/session-interface", id: "updateSessionInterface", tag: "settings",
			summary: "Choose the default interface for new sessions",
			reqBody: controllers.UpdateSessionInterfaceRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SettingsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/settings/reasoning", id: "updateReasoning", tag: "settings",
			summary: "Configure Waldo reasoning without returning the stored credential",
			reqBody: controllers.UpdateReasoningRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.ReasoningResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/settings/reasoning/verification", id: "verifyReasoning", tag: "settings",
			summary: "Probe the configured reasoning provider once and record whether it works",
			resps: []respUnit{
				{http.StatusOK, controllers.ReasoningResponse{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusServiceUnavailable, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/settings/repository-context", id: "updateRepositoryContextLimits", tag: "settings",
			summary: "Configure the owner's bounds for Waldo's bounded repository-context packet",
			reqBody: controllers.UpdateRepositoryContextLimitsRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.RepositoryContextLimitsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/conversation", id: "getSessionConversation", tag: "conversations",
			summary:    "Read a chat session's durable conversation",
			pathParams: []any{controllers.SessionIDParam{}, conversationSnapshotQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ConversationSnapshotResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/messages", id: "sendSessionConversationMessage", tag: "conversations",
			summary:    "Send a message to a chat session's agent",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SendConversationMessageRequest{},
			resps: []respUnit{
				{http.StatusAccepted, controllers.SendConversationMessageResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/turns/{turnId}/edit", id: "editSessionConversationMessage", tag: "conversations",
			summary:    "Branch before and replace an earlier human prompt",
			pathParams: []any{controllers.SessionIDParam{}, controllers.ConversationTurnIDParam{}},
			reqBody:    controllers.EditConversationMessageRequest{},
			resps: []respUnit{
				{http.StatusAccepted, controllers.EditConversationMessageResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/branches/{branchId}/activate", id: "activateSessionConversationBranch", tag: "conversations",
			summary:    "Resume a durable conversation branch without sending",
			pathParams: []any{controllers.SessionIDParam{}, controllers.ConversationBranchIDParam{}},
			resps: []respUnit{
				{http.StatusAccepted, controllers.ActivateConversationBranchResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/approvals/{requestId}/resolve", id: "resolveSessionConversationApproval", tag: "conversations",
			summary:    "Answer a pending approval in a chat session",
			pathParams: []any{controllers.SessionIDParam{}, controllers.ConversationRequestIDParam{}},
			reqBody:    controllers.ResolveConversationApprovalRequest{},
			resps: []respUnit{
				{http.StatusNoContent, nil},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/inputs/{requestId}/resolve", id: "resolveSessionConversationInput", tag: "conversations",
			summary:    "Answer a structured input request in a chat session",
			pathParams: []any{controllers.SessionIDParam{}, controllers.ConversationRequestIDParam{}},
			reqBody:    controllers.ResolveConversationInputRequest{},
			resps: []respUnit{
				{http.StatusNoContent, nil},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/compact", id: "compactSessionConversation", tag: "conversations",
			summary:    "Summarize earlier history to reclaim context in a chat session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusAccepted, controllers.CompactConversationResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/mcp/reload", id: "reloadSessionConversationMcpServers", tag: "conversations",
			summary:    "Restart the tool servers a chat session can reach",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ReloadConversationMCPServersResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/conversation/models", id: "listSessionConversationModels", tag: "conversations",
			summary:    "List the models the provider offers for a chat session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ConversationModelsResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/conversation/config-options", id: "listSessionConversationConfigOptions", tag: "conversations",
			summary:    "List the live session controls the provider advertises",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ConversationConfigOptionsResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/sessions/{sessionId}/conversation/config-options/{configId}", id: "setSessionConversationConfigOption", tag: "conversations",
			summary:    "Choose one provider-advertised session configuration value",
			pathParams: []any{controllers.SessionIDParam{}, controllers.ConversationConfigIDParam{}},
			reqBody:    controllers.SetConversationConfigOptionRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.ConversationConfigOptionsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/conversation/skills", id: "listSessionConversationSkills", tag: "conversations",
			summary:    "List the named skills the provider offers for a chat session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ConversationSkillsResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/sessions/{sessionId}/conversation/settings", id: "setSessionConversationTurnSettings", tag: "conversations",
			summary:    "Choose the model, reasoning effort and approval mode for the next turn",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.ConversationTurnSettingsPayload{},
			resps: []respUnit{
				{http.StatusOK, controllers.ConversationTurnSettingsPayload{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/interrupt", id: "interruptSessionConversationTurn", tag: "conversations",
			summary:    "Cancel the in-flight turn in a chat session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusNoContent, nil},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/steer", id: "steerSessionConversationTurn", tag: "conversations",
			summary:    "Send guidance into the in-flight turn of a chat session",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SteerConversationRequest{},
			resps: []respUnit{
				{http.StatusAccepted, controllers.SteerConversationResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/turns/{turnId}/steer", id: "promoteQueuedSessionConversationTurn", tag: "conversations",
			summary:    "Promote a queued message into the in-flight turn",
			pathParams: []any{controllers.SessionIDParam{}, controllers.ConversationTurnIDParam{}},
			resps: []respUnit{
				{http.StatusAccepted, controllers.PromoteQueuedTurnResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/conversation/turns/{turnId}/rollback", id: "rollbackSessionConversation", tag: "conversations",
			summary:    "Discard a turn and everything after it from the agent's memory",
			pathParams: []any{controllers.SessionIDParam{}, controllers.ConversationTurnIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.RollbackConversationResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPut, path: "/api/v1/sessions/{sessionId}/conversation/title", id: "setSessionConversationTitle", tag: "conversations",
			summary:    "Name the provider's conversation thread",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetConversationTitleRequest{},
			resps: []respUnit{
				{http.StatusAccepted, controllers.SetConversationTitleResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/shell-terminals", id: "listShellTerminals", tag: "shellTerminals",
			summary: "List the standalone shell terminals owned by the current app run",
			resps: []respUnit{
				{http.StatusOK, controllers.ListShellTerminalsResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/shell-terminals", id: "openShellTerminal", tag: "shellTerminals",
			summary: "Open a standalone shell terminal",
			reqBody: controllers.OpenShellTerminalRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.ShellTerminalEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/shell-terminals/{handleId}", id: "renameShellTerminal", tag: "shellTerminals",
			summary:    "Rename a standalone shell terminal tab",
			pathParams: []any{controllers.ShellTerminalHandleIDParam{}},
			reqBody:    controllers.UpdateShellTerminalRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.ShellTerminalEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/shell-terminals/{handleId}", id: "closeShellTerminal", tag: "shellTerminals",
			summary:    "Close a standalone shell terminal and destroy its PTY",
			pathParams: []any{controllers.ShellTerminalHandleIDParam{}},
			resps: []respUnit{
				{http.StatusNoContent, nil},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

func agentOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/agents", id: "listAgents", tag: "agents",
			summary: "Return cached supported and locally installed agent adapters",
			resps: []respUnit{
				{http.StatusOK, controllers.ListAgentsResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/agents/refresh", id: "refreshAgents", tag: "agents",
			summary: "Refresh the cached local agent adapter catalog",
			resps: []respUnit{
				{http.StatusOK, controllers.RefreshAgentsResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/agents/{agent}/probe", id: "probeAgent", tag: "agents",
			summary:    "Run a fresh local readiness probe for one agent adapter",
			pathParams: []any{controllers.AgentIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ProbeAgentResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/agents/{agent}/models", id: "getAgentModels", tag: "agents",
			summary:    "Return the cached model picker for one agent, discovering it on first use",
			pathParams: []any{controllers.AgentIDParam{}, controllers.AgentModelsQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.AgentModelsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/agents/{agent}/models/refresh", id: "refreshAgentModels", tag: "agents",
			summary:    "Refresh and cache the model picker for one agent",
			pathParams: []any{controllers.AgentIDParam{}, controllers.AgentModelsRefreshQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.AgentModelsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

// mobileOperations declares the 5 /mobile control operations. These are
// mounted on the loopback router (mountMobile in router.go), not the REST
// /api/v1 group — only the desktop/CLI may enable, disable, or regenerate the
// phone's LAN access; the phone never toggles its own connection. Must stay
// 1:1 with the routes mountMobile registers (enforced by the parity test).
func mobileOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/mobile/status", id: "getMobileStatus", tag: "mobile",
			summary: "Check whether Connect Mobile's LAN bridge is enabled",
			resps: []respUnit{
				{http.StatusOK, controllers.MobileStatusResponse{}},
				{http.StatusForbidden, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/mobile/enable", id: "enableMobile", tag: "mobile",
			summary: "Enable the Connect Mobile LAN bridge and issue a fresh password",
			resps: []respUnit{
				{http.StatusOK, controllers.MobileStatusResponse{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/mobile/disable", id: "disableMobile", tag: "mobile",
			summary: "Disable the Connect Mobile LAN bridge",
			resps: []respUnit{
				{http.StatusOK, controllers.MobileStatusResponse{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/mobile/regenerate", id: "regenerateMobile", tag: "mobile",
			summary: "Rotate the Connect Mobile password, dropping any connected phone",
			resps: []respUnit{
				{http.StatusOK, controllers.MobileStatusResponse{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/mobile/secure-pairing", id: "setMobileSecurePairing", tag: "mobile",
			summary: "Turn TLS-over-Tailscale secure pairing on or off",
			reqBody: controllers.SetSecurePairingRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.MobileStatusResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
	}
}

// mobileDeviceOperations declares the desktop-only mobile device roster
// routes. These sit under /api/v1/mobile — like mobileOperations above — so
// they inherit the LAN listener's transport-level block; a paired phone can
// neither list nor manage the household's other devices. Must stay 1:1 with
// the routes mountMobileDevices registers (enforced by the parity test).
func mobileDeviceOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/mobile/devices", id: "listMobileDevices", tag: "mobile",
			summary: "List paired mobile devices with their live/muted status",
			resps: []respUnit{
				{http.StatusOK, controllers.MobileDevicesResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusServiceUnavailable, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/mobile/devices/{installId}", id: "muteMobileDevice", tag: "mobile",
			summary:    "Mute or unmute push notifications for a paired device",
			pathParams: []any{controllers.InstallIDParam{}},
			reqBody:    controllers.MuteDeviceRequest{},
			resps: []respUnit{
				{http.StatusOK, map[string]bool{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusServiceUnavailable, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/mobile/devices/{installId}", id: "removeMobileDevice", tag: "mobile",
			summary:    "Remove a paired device from the roster",
			pathParams: []any{controllers.InstallIDParam{}},
			resps: []respUnit{
				{http.StatusNoContent, nil},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusServiceUnavailable, envelope.APIError{}},
			},
		},
	}
}

// devOperations declares developer-only API operations. Must stay 1:1 with
// the routes DevController.Register mounts (enforced by the parity test).
func devOperations() []operation {
	return []operation{
		{
			method: http.MethodPost, path: "/api/v1/dev/import-projects", id: "runDevImportProjects", tag: "dev",
			summary: "Run the developer project-registry import through the daemon store",
			reqBody: controllers.DevImportProjectsRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.DevImportProjectsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

func notificationOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/notifications", id: "listNotifications", tag: "notifications",
			summary:    "List notification history",
			pathParams: []any{controllers.ListNotificationsQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ListNotificationsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/notifications/{id}", id: "markNotificationRead", tag: "notifications",
			summary:    "Mark a notification read",
			pathParams: []any{controllers.NotificationIDParam{}},
			reqBody:    controllers.MarkNotificationReadRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.NotificationEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/notifications/read-all", id: "markAllNotificationsRead", tag: "notifications",
			summary: "Mark notifications read",
			reqBody: controllers.MarkAllNotificationsReadRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.MarkAllNotificationsReadResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/projects/{id}/outcomes", id: "listProjectOutcomes", tag: "outcomes",
			summary:    "List canonical Outcomes and current contracts for one Project",
			pathParams: []any{controllers.ProjectIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomesEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/projects/{id}/outcomes", id: "createOutcome", tag: "outcomes",
			summary:    "Create an Outcome with its first immutable contract revision",
			pathParams: []any{controllers.ProjectIDParam{}},
			reqBody:    controllers.CreateOutcomeRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.OutcomeEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}", id: "getOutcome", tag: "outcomes",
			summary:    "Read one Outcome with its full revision history",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/revisions", id: "reviseOutcomeContract", tag: "outcomes",
			summary:    "Append an immutable contract revision (optimistic concurrency)",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.ReviseOutcomeContractRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/composition", id: "getOutcomeComposition", tag: "outcomes",
			summary:    "Read derived shape, contributing Outcomes, and criterion coverage",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeCompositionEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/decompositions", id: "proposeOutcomeDecomposition", tag: "outcomes",
			summary:    "Propose a decomposition into contributing Outcomes (creates nothing)",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.ProposeDecompositionRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.DecompositionEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/decompositions/{decompositionId}/authorization", id: "authorizeOutcomeDecomposition", tag: "outcomes",
			summary:    "Authorize a decomposition, creating its contributing Outcomes",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.DecompositionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.DecompositionEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/decomposition/waivers", id: "waiveOutcomeContributionDependency", tag: "outcomes",
			summary:    "Waive one declared contribution ordering, with a durable reason",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.WaiveContributionDependencyRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.DecompositionEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/decomposition-requests", id: "askForOutcomeDecomposition", tag: "outcomes",
			summary:    "Ask an agent to propose a decomposition (answers later on the callback route)",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.AskForDecompositionRequest{},
			resps: []respUnit{
				{http.StatusAccepted, controllers.DecompositionRequestEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/decomposition-request", id: "getLatestOutcomeDecompositionRequest", tag: "outcomes",
			summary:    "Read the newest decomposition ask and what became of it",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.DecompositionRequestEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/decomposition-requests/{requestId}/proposal", id: "submitOutcomeDecompositionProposal", tag: "outcomes",
			summary:    "Callback: a spawned agent submits its proposed decomposition",
			pathParams: []any{controllers.DecompositionRequestIDParam{}},
			reqBody:    controllers.SubmitAgentProposalRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.DecompositionRequestEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/decomposition", id: "getLatestOutcomeDecomposition", tag: "outcomes",
			summary:    "Read the newest decomposition and whether the parent contract moved past it",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.DecompositionEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/planning-candidates", id: "listOutcomePlanningCandidates", tag: "outcomes",
			summary:    "List exact available planning-agent choices for a confirmed Contract",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.PlanningCandidatesQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.PlanningCandidatesEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusServiceUnavailable, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/planning-sessions", id: "startOutcomePlanning", tag: "outcomes",
			summary:    "Start one Contract-bound planning conversation with frozen read-only context",
			pathParams: []any{controllers.OutcomeIDParam{}}, reqBody: controllers.StartPlanningRequest{},
			resps: []respUnit{{http.StatusCreated, controllers.PlanningEnvelope{}}, {http.StatusBadRequest, envelope.APIError{}}, {http.StatusConflict, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusServiceUnavailable, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/planning-session", id: "getCurrentOutcomePlanning", tag: "outcomes",
			summary:    "Read the newest planning conversation for an Outcome",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps:      []respUnit{{http.StatusOK, controllers.PlanningEnvelope{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}", id: "getOutcomePlanning", tag: "outcomes",
			summary:    "Read one bounded planning conversation and its proposal",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.PlanningSessionIDParam{}},
			resps:      []respUnit{{http.StatusOK, controllers.PlanningEnvelope{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/messages", id: "continueOutcomePlanning", tag: "outcomes",
			summary:    "Send one owner message and receive one structured planning reply",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.PlanningSessionIDParam{}}, reqBody: controllers.PlanningMessageRequest{},
			resps: []respUnit{{http.StatusOK, controllers.PlanningEnvelope{}}, {http.StatusBadRequest, envelope.APIError{}}, {http.StatusConflict, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusServiceUnavailable, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/proposal", id: "finalizeOutcomePlanning", tag: "outcomes",
			summary:    "Ask for a Plan proposal; clarification may still be returned when required",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.PlanningSessionIDParam{}}, reqBody: controllers.PlanningFinalizeRequest{},
			resps: []respUnit{{http.StatusOK, controllers.PlanningEnvelope{}}, {http.StatusBadRequest, envelope.APIError{}}, {http.StatusConflict, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusServiceUnavailable, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/cancel", id: "cancelOutcomePlanning", tag: "outcomes",
			summary:    "Cancel an active planning conversation without creating execution authority",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.PlanningSessionIDParam{}}, reqBody: controllers.PlanningCancelRequest{},
			resps: []respUnit{{http.StatusOK, controllers.PlanningEnvelope{}}, {http.StatusBadRequest, envelope.APIError{}}, {http.StatusConflict, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/plans", id: "proposeOutcomePlan", tag: "outcomes",
			summary:    "Propose the direct Work Unit plan bound to the current contract revision",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.ProposePlanRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.PlanEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/plans/replan", id: "replanOutcomePlan", tag: "outcomes",
			summary:    "Create a new immutable Plan proposal from explicit owner feedback",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.ReplanPlanRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.PlanEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/plans/{planId}/approval", id: "approveOutcomePlan", tag: "outcomes",
			summary:    "Authorize a proposed plan (owner authority gate; fails closed on narrowed authority)",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.PlanIDParam{}},
			reqBody:    controllers.ApprovePlanRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.PlanEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/plan", id: "getLatestOutcomePlan", tag: "outcomes",
			summary:    "Read the newest plan of any status for this Outcome",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.PlanEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule", id: "getOutcomePlanSchedule", tag: "outcomes",
			summary:    "Read daemon-derived WorkUnit schedule state without launching work",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.PlanIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ScheduleEnvelope{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/proof", id: "getOutcomeProof", tag: "outcomes",
			summary:    "Read criterion-bound Evidence, Verification, explicit decisions, and derived proof state",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.OutcomeProofEnvelope{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/evidence", id: "recordOutcomeEvidence", tag: "outcomes",
			summary:    "Append provenance-bearing Evidence to an exact current criterion and subject revision",
			pathParams: []any{controllers.OutcomeIDParam{}}, reqBody: controllers.RecordEvidenceRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.OutcomeProofEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/verifications", id: "recordOutcomeVerification", tag: "outcomes",
			summary:    "Append a Verification result with its actual independence class",
			pathParams: []any{controllers.OutcomeIDParam{}}, reqBody: controllers.RecordVerificationRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.OutcomeProofEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/acceptance-decisions", id: "decideOutcomeAcceptance", tag: "outcomes",
			summary:    "Append the user's explicit accept, request-rework, or reopen decision",
			pathParams: []any{controllers.OutcomeIDParam{}}, reqBody: controllers.DecideAcceptanceRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.OutcomeProofEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/acceptance-batch", id: "getOutcomeAcceptanceBatchEligibility", tag: "outcomes",
			summary:    "Report which contributing Outcomes could be accepted together right now",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.BatchEligibilityEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/acceptance-batch", id: "acceptOutcomeContributorBatch", tag: "outcomes",
			summary:    "Accept every eligible contributing Outcome in one sitting, as separate decisions",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.AcceptContributorBatchRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.AcceptBatchEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/needs-you", id: "getOutcomeNeedsYou", tag: "outcomes",
			summary:    "Read current typed owner questions for the Outcome's active Attempt and WorkUnits",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps:      []respUnit{{http.StatusOK, controllers.NeedsYouQuestionsEnvelope{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/needs-you/{questionId}/answers", id: "answerOutcomeNeedsYou", tag: "outcomes",
			summary:    "Answer one exact Needs-You generation through the governed command lifecycle",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.NeedsYouQuestionIDParam{}}, reqBody: controllers.NeedsYouAnswerRequest{},
			resps: []respUnit{{http.StatusOK, controllers.NeedsYouQuestionEnvelope{}}, {http.StatusBadRequest, envelope.APIError{}}, {http.StatusConflict, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/needs-you/{questionId}/reconcile", id: "reconcileOutcomeNeedsYou", tag: "outcomes",
			summary:    "Settle delivery_unknown only when later provider evidence closed the question",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.NeedsYouQuestionIDParam{}}, reqBody: controllers.NeedsYouReconcileRequest{},
			resps: []respUnit{{http.StatusOK, controllers.NeedsYouQuestionEnvelope{}}, {http.StatusBadRequest, envelope.APIError{}}, {http.StatusConflict, envelope.APIError{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/attempts", id: "startOutcomeAttempt", tag: "outcomes",
			summary:    "Admit an approved plan onto a real provider session (fail-closed; idempotent by requestKey)",
			pathParams: []any{controllers.OutcomeIDParam{}},
			reqBody:    controllers.StartOutcomeAttemptRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.AttemptEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/attempts", id: "listOutcomeAttempts", tag: "outcomes",
			summary:    "Read the Outcome's attempt lineage with derived presentation",
			pathParams: []any{controllers.OutcomeIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.AttemptListEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/outcomes/{outcomeId}/attempts/{attemptId}", id: "getOutcomeAttempt", tag: "outcomes",
			summary:    "Read one attempt: lineage, observations, receipts, custody, derived state",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.AttemptIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.AttemptEnvelope{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/observations", id: "recordOutcomeAttemptObservation", tag: "outcomes",
			summary:    "Append one ordered observation (insertable on any attempt state; never mutates current truth)",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.AttemptIDParam{}},
			reqBody:    controllers.RecordObservationRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.ObservationEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/cancel", id: "cancelOutcomeAttempt", tag: "outcomes",
			summary:    "Cancel an active attempt by owner decision",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.AttemptIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.AttemptEnvelope{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/recovery", id: "recoverOutcomeAttempt", tag: "outcomes",
			summary:    "Contain, reconcile, resume, replace, or escalate an attempt (custody-safe recovery)",
			pathParams: []any{controllers.OutcomeIDParam{}, controllers.AttemptIDParam{}},
			reqBody:    controllers.AttemptRecoveryRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.AttemptRecoveryEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/notifications/stream", id: "streamNotifications", tag: "notifications",
			summary:    "Stream created notifications",
			pathParams: []any{controllers.NotificationStreamQuery{}},
			resps: []respUnit{
				{http.StatusOK, ""},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
			contentTypes: map[int]string{http.StatusOK: "text/event-stream"},
		},
	}
}

// reviewOperations declares the session-scoped /reviews operations. Must stay
// 1:1 with the routes ReviewsController.Register mounts (enforced by the parity
// test).
// pushOperations declares the /push/devices operations. Must stay 1:1 with the
// routes PushController.Register mounts (enforced by the parity test).
func pushOperations() []operation {
	return []operation{
		{
			method: http.MethodPost, path: "/api/v1/push/devices", id: "registerPushDevice", tag: "push",
			summary: "Register (upsert) a phone's Expo push token",
			reqBody: controllers.RegisterPushDeviceRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.PushDeviceEnvelope{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/push/devices/{token}", id: "unregisterPushDevice", tag: "push",
			summary:    "Unregister a phone's Expo push token, leaving it paired",
			pathParams: []any{controllers.PushDeviceTokenParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.UnregisterPushDeviceResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/push/pairings/{id}", id: "unpairPushDevice", tag: "push",
			summary:    "Unpair this phone from the daemon, removing it from the roster",
			pathParams: []any{controllers.PushPairingIDParam{}},
			resps: []respUnit{
				{http.StatusNoContent, nil},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

func reviewOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/reviews", id: "listReviews", tag: "reviews",
			summary:    "List a worker's code-review runs",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ListReviewsResponse{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/reviews/trigger", id: "triggerReview", tag: "reviews",
			summary:    "Trigger a code review of a worker's PR",
			pathParams: []any{controllers.SessionIDParam{}},
			// Optional: an empty body runs under the project's configured reviewer.
			reqBody:         controllers.TriggerReviewRequest{},
			optionalReqBody: true,
			resps: []respUnit{
				{http.StatusOK, controllers.TriggerReviewResponse{}},
				{http.StatusCreated, controllers.TriggerReviewResponse{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/reviews/comments/resolve", id: "resolveReviewComment", tag: "reviews",
			summary:    "Resolve an external review comment thread",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.ResolveReviewCommentRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.ResolveReviewCommentResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/reviews/rerequest", id: "requestRereview", tag: "reviews",
			summary:    "Ask an external reviewer to re-review a worker's PR",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.RequestRereviewRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.RequestRereviewResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/reviews/cancel", id: "cancelReview", tag: "reviews",
			summary:    "Cancel a running code review",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.CancelReviewResponse{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/reviews/kill", id: "killReviewSession", tag: "reviews",
			summary:    "Kill a worker's reviewer terminal session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.KillReviewResponse{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/reviews/restore", id: "restoreReviewSession", tag: "reviews",
			summary:    "Restore a worker's reviewer terminal session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.RestoreReviewResponse{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/reviews/switch", id: "switchReviewSession", tag: "reviews",
			summary:    "Switch a worker's reviewer harness",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetSessionReviewerRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.ListReviewsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/reviews/submit", id: "submitReview", tag: "reviews",
			summary:    "Record a reviewer's result for a worker's PR",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SubmitReviewInput{},
			resps: []respUnit{
				{http.StatusOK, controllers.ReviewRunResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

type eventsQuery struct {
	After *int64 `query:"after,omitempty" minimum:"0" description:"Replay events with seq greater than this cursor. When omitted, clients may send Last-Event-ID instead."`
}

type harnessIntentParam struct {
	IntentID string `path:"intentId"`
}
type harnessConnectionParam struct {
	ConnectionID string `path:"connectionId"`
}
type harnessIntentQuery struct {
	ProjectID string `query:"projectId,omitempty"`
	Limit     int    `query:"limit,omitempty" minimum:"1" maximum:"200"`
}
type harnessConnectionQuery struct {
	MissionID string `query:"missionId,omitempty"`
	Limit     int    `query:"limit,omitempty" minimum:"1" maximum:"200"`
}
type harnessListResponse struct {
	Data map[string]any `json:"data"`
}

func harnessAuthorityOperations() []operation {
	ok := []respUnit{{http.StatusOK, harnessListResponse{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}}
	detail := []respUnit{{http.StatusOK, harnessListResponse{}}, {http.StatusNotFound, envelope.APIError{}}, {http.StatusInternalServerError, envelope.APIError{}}, {http.StatusNotImplemented, envelope.APIError{}}}
	return []operation{
		{method: http.MethodGet, path: "/api/v1/harness-pairing-intents", id: "listHarnessPairingIntents", tag: "harness-authority", summary: "List pairing intents", pathParams: []any{harnessIntentQuery{}}, resps: ok},
		{method: http.MethodGet, path: "/api/v1/harness-pairing-intents/{intentId}", id: "getHarnessPairingIntent", tag: "harness-authority", summary: "Get pairing intent", pathParams: []any{harnessIntentParam{}}, resps: detail},
		{method: http.MethodGet, path: "/api/v1/harness-connections", id: "listHarnessConnections", tag: "harness-authority", summary: "List harness connections", pathParams: []any{harnessConnectionQuery{}}, resps: ok},
		{method: http.MethodGet, path: "/api/v1/harness-connections/{connectionId}", id: "getHarnessConnection", tag: "harness-authority", summary: "Get harness connection", pathParams: []any{harnessConnectionParam{}}, resps: detail}}
}

func eventOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/events", id: "streamEvents", tag: "events",
			summary:    "Stream CDC events with durable replay",
			pathParams: []any{eventsQuery{}},
			resps: []respUnit{
				{http.StatusOK, ""},
				{status: http.StatusBadRequest, body: envelope.APIError{}},
				{status: http.StatusInternalServerError, body: envelope.APIError{}},
				{status: http.StatusNotImplemented, body: envelope.APIError{}},
			},
			contentTypes: map[int]string{http.StatusOK: "text/event-stream"},
		},
	}
}

// projectOperations declares the canonical /projects operations. The set must
// stay 1:1 with the routes ProjectsController.Register mounts —
// TestRouteSpecParity fails the build otherwise.
func projectOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/projects", id: "listProjects", tag: "projects",
			summary: "List all registered projects (active + degraded)",
			resps: []respUnit{
				{http.StatusOK, controllers.ListProjectsResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/projects", id: "addProject", tag: "projects",
			summary: "Register a new project from a git repository path",
			reqBody: projectsvc.AddInput{},
			resps: []respUnit{
				{http.StatusCreated, controllers.ProjectResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/projects/initialize", id: "initializeProjectRepository", tag: "projects",
			summary: "Initialize a selected folder as a Git repository with an initial commit",
			reqBody: projectsvc.InitializeRepositoryInput{},
			resps: []respUnit{
				{http.StatusOK, projectsvc.InitializeRepositoryResult{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		}, {
			method: http.MethodGet, path: "/api/v1/projects/{id}", id: "getProject", tag: "projects",
			summary:    "Fetch one project; discriminates ok vs degraded",
			pathParams: []any{controllers.ProjectIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.GetProjectResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/projects/{id}/resolved-mission-roles", id: "getResolvedMissionRoles", tag: "projects",
			summary:    "Read the daemon-resolved Mission-role proposal for one Project",
			pathParams: []any{controllers.ProjectIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ResolvedMissionRolesResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPut, path: "/api/v1/projects/{id}", id: "updateProjectSettings", tag: "projects",
			summary:    "Atomically replace a project's display name and config",
			pathParams: []any{controllers.ProjectIDParam{}},
			reqBody:    projectsvc.UpdateSettingsInput{},
			resps: []respUnit{
				{http.StatusOK, controllers.ProjectResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPut, path: "/api/v1/projects/{id}/config", id: "setProjectConfig", tag: "projects",
			summary:    "Replace a project's per-project config",
			pathParams: []any{controllers.ProjectIDParam{}},
			reqBody:    projectsvc.SetConfigInput{},
			resps: []respUnit{
				{http.StatusOK, controllers.ProjectResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/projects/{id}", id: "removeProject", tag: "projects",
			summary:    "Remove a project; stops sessions, cleans workspaces, unregisters",
			pathParams: []any{controllers.ProjectIDParam{}},
			resps: []respUnit{
				{http.StatusOK, projectsvc.RemoveResult{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
	}
}

func sessionOperations() []operation {
	return []operation{
		{
			method: http.MethodGet, path: "/api/v1/sessions", id: "listSessions", tag: "sessions",
			summary:    "List sessions",
			pathParams: []any{controllers.ListSessionsQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ListSessionsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions", id: "spawnSession", tag: "sessions",
			summary: "Spawn a new agent session",
			reqBody: controllers.SpawnSessionRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.SpawnSessionResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}", id: "getSession", tag: "sessions",
			summary:    "Fetch one session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/pin", id: "pinSession", tag: "sessions",
			summary:    "Pin a session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/sessions/{sessionId}/pin", id: "unpinSession", tag: "sessions",
			summary:    "Unpin a session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/preview", id: "getSessionPreview", tag: "sessions",
			summary:    "Discover a browser preview URL for a session workspace",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionPreviewResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/preview", id: "setSessionPreview", tag: "sessions",
			summary:    "Set (or autodetect) the browser preview URL for a session",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetSessionPreviewRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/sessions/{sessionId}/preview", id: "clearSessionPreview", tag: "sessions",
			summary:    "Clear the browser preview URL for a session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/preview/server", id: "getSessionPreviewServer", tag: "sessions",
			summary:    "Get the managed preview server status for a session",
			pathParams: []any{controllers.SessionIDParam{}, controllers.BrowserCapabilityHeader{}},
			resps: []respUnit{
				{http.StatusOK, controllers.PreviewServerStatusResponse{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/preview/server", id: "startSessionPreviewServer", tag: "sessions",
			summary:    "Start a session-owned server from .kennel/launch.json and open its application preview",
			pathParams: []any{controllers.SessionIDParam{}, controllers.BrowserCapabilityHeader{}},
			reqBody:    controllers.StartPreviewServerRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.PreviewServerStatusResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusRequestTimeout, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusGatewayTimeout, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/sessions/{sessionId}/preview/server", id: "stopSessionPreviewServer", tag: "sessions",
			summary:    "Stop the managed preview server for a session",
			pathParams: []any{controllers.SessionIDParam{}, controllers.BrowserCapabilityHeader{}},
			resps: []respUnit{
				{http.StatusOK, controllers.PreviewServerStatusResponse{}},
				{http.StatusForbidden, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/preview/files/*", id: "getSessionPreviewFile", tag: "sessions",
			summary:    "Serve a static browser preview file from a session workspace",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, ""},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
			contentTypes: map[int]string{http.StatusOK: "text/html"},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/attachments", id: "stageSessionAttachments", tag: "sessions",
			summary:    "Write images into a running session's worktree and return their paths",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.StageSessionAttachmentsRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.StageSessionAttachmentsResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/workspace/files", id: "listSessionWorkspaceFiles", tag: "sessions",
			summary:    "List files in a session workspace with git change status",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ListWorkspaceFilesResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/workspace/events", id: "streamSessionWorkspaceChanges", tag: "sessions",
			summary:    "Stream session workspace file changes",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, ""},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
			contentTypes: map[int]string{http.StatusOK: "text/event-stream"},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/workspace/file", id: "getSessionWorkspaceFile", tag: "sessions",
			summary:    "Read one session workspace file and its git diff",
			pathParams: []any{controllers.SessionIDParam{}, controllers.WorkspaceFileQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.WorkspaceFileResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/pr", id: "listSessionPRs", tag: "sessions",
			summary:    "List pull requests owned by a session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ListSessionPRsResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/pr/claim", id: "claimSessionPR", tag: "sessions",
			summary:    "Claim an existing pull request for a session",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.ClaimPRRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.ClaimPRResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusServiceUnavailable, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/sessions/{sessionId}", id: "renameSession", tag: "sessions",
			summary:    "Rename a session display name",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.RenameSessionRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.RenameSessionResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/sessions/{sessionId}/merge-policy", id: "setSessionMergePolicy", tag: "sessions",
			summary:    "Configure whether PR completion terminates the session",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetSessionMergePolicyRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SetSessionMergePolicyResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/sessions/{sessionId}/auto-inject-review", id: "setSessionAutoInjectReview", tag: "sessions",
			summary:    "Set the auto-inject review setting for a session",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetSessionAutoInjectReviewRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SetSessionAutoInjectReviewResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPatch, path: "/api/v1/sessions/{sessionId}/auto-inject-ci", id: "setSessionAutoInjectCI", tag: "sessions",
			summary:    "Set the automatic CI-failure injection default for new session PRs",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetSessionAutoInjectCIRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SetSessionAutoInjectCIResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPut, path: "/api/v1/sessions/{sessionId}/reviewer", id: "setSessionReviewer", tag: "sessions",
			summary:    "Set the reviewer harness for a session",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetSessionReviewerRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPut, path: "/api/v1/sessions/{sessionId}/auto-review", id: "setSessionAutoReview", tag: "sessions",
			summary:    "Enable or disable automatic review for a session",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetSessionAutoReviewRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/cleanup", id: "cleanupSessions", tag: "sessions",
			summary:    "Clean up terminated session workspaces",
			pathParams: []any{controllers.CleanupSessionsQuery{}},
			resps: []respUnit{
				{http.StatusOK, controllers.CleanupSessionsResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/restore", id: "restoreSession", tag: "sessions",
			summary:    "Restore a terminated session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.RestoreSessionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/resume-agent", id: "resumeAgent", tag: "sessions",
			summary:    "Resume an exited agent in its existing session",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ResumeAgentResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/switch-agent", id: "switchSessionAgent", tag: "sessions",
			summary:    "Switch a logical Kennel session to another agent harness",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SwitchAgentRequest{},
			resps: []respUnit{
				{http.StatusAccepted, controllers.AgentSwitchResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/agent-switches", id: "listSessionAgentSwitches", tag: "sessions",
			summary:    "List a session's durable agent-switch history",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.ListAgentSwitchesResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/agent-switches/{switchId}/recover", id: "recoverSessionAgentSwitch", tag: "sessions",
			summary:    "Retry safe source restoration for an agent switch",
			pathParams: []any{controllers.SessionIDParam{}, controllers.AgentSwitchIDParam{}},
			resps: []respUnit{
				{http.StatusAccepted, controllers.AgentSwitchResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/agent-switches/{switchId}/handoff", id: "submitSessionAgentHandoff", tag: "sessions",
			summary:    "Submit a generation-fenced source-agent handoff",
			pathParams: []any{controllers.SessionIDParam{}, controllers.AgentSwitchIDParam{}},
			reqBody:    controllers.SubmitAgentHandoffRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.AgentSwitchResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/sessions/{sessionId}/interface-transition", id: "getSessionInterfaceTransition", tag: "sessions",
			summary:    "Inspect TUI and Chat interface handoff support and progress",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionInterfaceTransitionStatusResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/interface-transition", id: "startSessionInterfaceTransition", tag: "sessions",
			summary:    "Switch a live session between its TUI and Chat controllers",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.StartSessionInterfaceTransitionRequest{},
			resps: []respUnit{
				{http.StatusAccepted, controllers.StartSessionInterfaceTransitionResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodDelete, path: "/api/v1/sessions/{sessionId}/interface-transition", id: "cancelSessionInterfaceTransition", tag: "sessions",
			summary:    "Cancel an interface handoff before its source controller stops",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusAccepted, controllers.CancelSessionInterfaceTransitionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/kill", id: "killSession", tag: "sessions",
			summary:    "Mark a session terminated and tear down runtime/workspace resources",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.KillSessionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/rollback", id: "rollbackSession", tag: "sessions",
			summary:    "Undo a partially-completed spawn (delete seed row, or kill if spawn output exists)",
			pathParams: []any{controllers.SessionIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.RollbackSessionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/send", id: "sendSessionMessage", tag: "sessions",
			summary:    "Send a message to a running session's agent",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SendSessionMessageRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SendSessionMessageResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				// Conflict: the session is terminated, or paused on a permission
				// decision (SESSION_AWAITING_DECISION) — the guarded send refuses
				// to paste into a pending dialog.
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/sessions/{sessionId}/activity", id: "setSessionActivity", tag: "sessions",
			summary:    "Report an agent activity-state signal for a session",
			pathParams: []any{controllers.SessionIDParam{}},
			reqBody:    controllers.SetActivityRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SetActivityResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/reviews/{reviewSessionID}/activity", id: "setReviewActivity", tag: "reviews",
			summary:    "Report a reviewer-owned hook signal",
			pathParams: []any{controllers.ReviewSessionIDParam{}},
			reqBody:    controllers.SetReviewActivityRequest{},
			resps: []respUnit{
				{http.StatusOK, controllers.SetReviewActivityResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/orchestrators", id: "listOrchestrators", tag: "sessions",
			summary: "List orchestrator sessions across projects",
			resps: []respUnit{
				{http.StatusOK, controllers.ListSessionsResponse{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/orchestrators", id: "spawnOrchestrator", tag: "sessions",
			summary: "Spawn an orchestrator session",
			reqBody: controllers.SpawnOrchestratorRequest{},
			resps: []respUnit{
				{http.StatusCreated, controllers.SpawnOrchestratorResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/orchestrators/delegate", id: "delegateTask", tag: "sessions",
			summary: "Start a worker task and ask the orchestrator to title it",
			reqBody: controllers.DelegateTaskRequest{},
			resps: []respUnit{
				{http.StatusAccepted, controllers.DelegateTaskResponse{}},
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodGet, path: "/api/v1/orchestrators/{id}", id: "getOrchestrator", tag: "sessions",
			summary:    "Fetch one orchestrator session",
			pathParams: []any{controllers.OrchestratorIDParam{}},
			resps: []respUnit{
				{http.StatusOK, controllers.SessionResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusInternalServerError, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

// prOperations declares the PR action operations. These live in the SCM lane:
// the handler delegates to a PRService backed by the SCM provider. A nil
// PRService (SCM not configured) returns 501 for both routes.
func prOperations() []operation {
	return []operation{
		{
			method: http.MethodPost, path: "/api/v1/prs/{id}/merge", id: "mergePR", tag: "prs",
			summary:    "Squash-merge a pull request",
			pathParams: []any{controllers.PRIDParam{}},
			reqBody:    controllers.MergePRRequest{},
			resps: []respUnit{
				{http.StatusBadRequest, envelope.APIError{}},
				{http.StatusOK, controllers.MergePRResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusConflict, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
		{
			method: http.MethodPost, path: "/api/v1/prs/{id}/resolve-comments", id: "resolveComments", tag: "prs",
			summary:    "Resolve review threads on a pull request",
			pathParams: []any{controllers.PRIDParam{}},
			reqBody:    nil, // body is optional: omitting it resolves all unresolved threads
			resps: []respUnit{
				{http.StatusOK, controllers.ResolveCommentsResponse{}},
				{http.StatusNotFound, envelope.APIError{}},
				{http.StatusUnprocessableEntity, envelope.APIError{}},
				{http.StatusNotImplemented, envelope.APIError{}},
			},
		},
	}
}

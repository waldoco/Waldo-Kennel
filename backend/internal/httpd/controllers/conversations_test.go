package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/config"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	chatsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/chat"
)

func conversationTestServer(t *testing.T, service *fakeConversationService) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{
		Sessions: newFakeSessionService(), Conversations: service,
	}, httpd.ControlDeps{}))
	t.Cleanup(server.Close)
	return server
}

// The wire shape for streamed command output and turn diffs.
//
// Asserted through the real route rather than against the mapping helpers, so the
// JSON a client actually parses is what is checked.

type fakeConversationService struct {
	snapshot       chatsvc.Snapshot
	skills         []ports.ChatSkill
	skillErr       error
	configOptions  []ports.ChatConfigOption
	configErr      error
	setConfigID    string
	setConfigValue ports.ChatConfigOptionValue
	mcpServers     []domain.ConversationMCPServer
	reloadErr      error
	sent           ports.ChatUserMessage
	inputRequestID string
	inputResponse  ports.ChatInputResponse
	sendTurn       domain.ConversationTurn
	sendErr        error
}

func (f *fakeConversationService) EditMessage(context.Context, domain.SessionID, string, ports.ChatUserMessage) (chatsvc.EditMessageResult, error) {
	return chatsvc.EditMessageResult{}, nil
}

func (f *fakeConversationService) ActivateBranch(context.Context, domain.SessionID, string) (string, error) {
	return "", nil
}

func (f *fakeConversationService) Snapshot(context.Context, domain.SessionID) (chatsvc.Snapshot, error) {
	return f.snapshot, nil
}

func (f *fakeConversationService) Send(_ context.Context, _ domain.SessionID, message ports.ChatUserMessage) (domain.ConversationTurn, error) {
	f.sent = message
	if f.sendErr != nil {
		return f.sendTurn, f.sendErr
	}
	return domain.ConversationTurn{ID: "turn-1", State: domain.TurnStateRunning}, nil
}

func (f *fakeConversationService) Resolve(context.Context, domain.SessionID, string, ports.ChatDecision) error {
	return nil
}

func (f *fakeConversationService) ResolveInput(_ context.Context, _ domain.SessionID, requestID string, response ports.ChatInputResponse) error {
	f.inputRequestID = requestID
	f.inputResponse = response
	return nil
}

func (f *fakeConversationService) Interrupt(context.Context, domain.SessionID) error { return nil }

func (f *fakeConversationService) Models(context.Context, domain.SessionID) ([]ports.ChatModel, domain.ConversationSettings, error) {
	return nil, domain.ConversationSettings{}, nil
}

func (f *fakeConversationService) ConfigOptions(context.Context, domain.SessionID) ([]ports.ChatConfigOption, error) {
	return f.configOptions, f.configErr
}

func (f *fakeConversationService) SetConfigOption(_ context.Context, _ domain.SessionID, id string, value ports.ChatConfigOptionValue) ([]ports.ChatConfigOption, error) {
	f.setConfigID = id
	f.setConfigValue = value
	return f.configOptions, f.configErr
}

func (f *fakeConversationService) SetTurnSettings(context.Context, domain.SessionID, domain.ConversationSettings) (domain.ConversationSettings, error) {
	return domain.ConversationSettings{}, nil
}

func (f *fakeConversationService) Compact(context.Context, domain.SessionID) (ports.ChatCompactionResult, error) {
	return ports.ChatCompactionResult{}, nil
}

// History operations belong to a sibling slice; stubbed so this fake satisfies the
// controller's interface without pretending to implement them.
func (f *fakeConversationService) Rollback(context.Context, domain.SessionID, string) (int, error) {
	return 0, nil
}

func (f *fakeConversationService) SetTitle(context.Context, domain.SessionID, string) (string, error) {
	return "", nil
}

func (f *fakeConversationService) ReloadMCPServers(
	context.Context,
	domain.SessionID,
) ([]domain.ConversationMCPServer, error) {
	return f.mcpServers, f.reloadErr
}

func (f *fakeConversationService) Skills(context.Context, domain.SessionID) ([]ports.ChatSkill, error) {
	return f.skills, f.skillErr
}

// conversationSnapshotBody fetches the snapshot route and decodes it loosely, the
// way a client sees it.
func conversationSnapshotBody(t *testing.T, snapshot chatsvc.Snapshot) map[string]any {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{
		Sessions:      newFakeSessionService(),
		Conversations: &fakeConversationService{snapshot: snapshot},
	}, httpd.ControlDeps{}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/sessions/p1-1/conversation")
	if err != nil {
		t.Fatalf("GET conversation: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
}

func TestConversationSnapshotExposesSafeEditContentAndBranchMetadata(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	body := conversationSnapshotBody(t, chatsvc.Snapshot{
		Conversation: domain.ConversationRecord{ID: "conversation-1", ActiveBranchID: "branch-child"},
		SessionID:    domain.SessionID("p1-1"),
		Messages: []domain.ConversationMessage{
			{
				ID: "valid", Role: domain.MessageRoleUser, Origin: domain.MessageOriginHuman,
				Text: "inspect", CreatedAt: now,
				DeliveryContentJSON: `[{"type":"text","text":"inspect"},{"type":"image","data":"secret-bytes","mimeType":"image/png","name":"diagram.png"},{"type":"resource","uri":"file:///notes.md","name":"notes.md","text":"secret resource text"},{"type":"skill_binding"}]`,
			},
			{ID: "legacy", Role: domain.MessageRoleUser, Origin: domain.MessageOriginHuman, Text: "legacy", CreatedAt: now},
			{ID: "malformed", Role: domain.MessageRoleUser, Origin: domain.MessageOriginHuman, Text: "broken", DeliveryContentJSON: `{broken`, CreatedAt: now},
		},
		BranchPoints: []domain.ConversationBranchPoint{{
			TurnID: "turn-edited", Position: 2, Total: 2, PreviousBranchID: "branch-root",
		}},
		BranchedFromEarlierMessage: true,
	})
	if body["activeBranchId"] != "branch-child" {
		t.Fatalf("activeBranchId = %#v", body["activeBranchId"])
	}
	if body["branchedFromEarlierMessage"] != true {
		t.Fatalf("branchedFromEarlierMessage = %#v", body["branchedFromEarlierMessage"])
	}
	points := body["branchPoints"].([]any)
	if len(points) != 1 || points[0].(map[string]any)["previousBranchId"] != "branch-root" {
		t.Fatalf("branchPoints = %#v", points)
	}
	messages := body["messages"].([]any)
	valid := messages[0].(map[string]any)
	if valid["editAvailable"] != true {
		t.Fatalf("valid editAvailable = %#v", valid["editAvailable"])
	}
	content := valid["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("content summaries = %#v", content)
	}
	for _, raw := range content {
		summary := raw.(map[string]any)
		if _, exists := summary["data"]; exists {
			t.Fatalf("content summary leaked data: %#v", summary)
		}
		if _, exists := summary["text"]; exists {
			t.Fatalf("content summary leaked text: %#v", summary)
		}
	}
	if content[2].(map[string]any)["name"] != "skill_binding" {
		t.Fatalf("unknown content summary = %#v", content[2])
	}
	if messages[1].(map[string]any)["editAvailable"] != true {
		t.Fatalf("legacy message is not editable: %#v", messages[1])
	}
	if messages[2].(map[string]any)["editAvailable"] != false {
		t.Fatalf("malformed message is editable: %#v", messages[2])
	}
}

func TestSendConversationPreservesNativeImageAndResourceContent(t *testing.T) {
	service := &fakeConversationService{}
	server := conversationTestServer(t, service)
	body := []byte(`{
		"text":"inspect this",
		"clientMessageId":"message-1",
		"attachments":[{"mimeType":"image/png","data":"aW1hZ2U="}],
		"resources":[
			{"uri":"file:///repo/README.md","name":"README.md"},
			{"uri":"file:///repo/notes.txt","name":"notes.txt","mimeType":"text/plain","text":"notes"}
		]
	}`)
	request, err := http.NewRequest(http.MethodPost,
		server.URL+"/api/v1/sessions/p1-1/conversation/messages", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST message: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		got, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, got)
	}
	if service.sent.ClientMessageID != "message-1" || len(service.sent.Content) != 3 {
		t.Fatalf("sent = %#v", service.sent)
	}
	if service.sent.Content[0].Type != "image" || service.sent.Content[1].Type != "resource_link" || service.sent.Content[2].Type != "resource" {
		t.Fatalf("content = %#v", service.sent.Content)
	}
}

func TestResolveConversationInputCarriesActionAndStructuredContent(t *testing.T) {
	service := &fakeConversationService{}
	server := conversationTestServer(t, service)
	body := []byte(`{"action":"accept","content":{"choice":"native","fast":true}}`)
	request, err := http.NewRequest(http.MethodPost,
		server.URL+"/api/v1/sessions/p1-1/conversation/inputs/request-7/resolve", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST input: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNoContent {
		got, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, got)
	}
	if service.inputRequestID != "request-7" || service.inputResponse.Action != "accept" || service.inputResponse.Content["choice"] != "native" {
		t.Fatalf("input = %q %#v", service.inputRequestID, service.inputResponse)
	}
}

func TestSnapshotExposesTurnDiff(t *testing.T) {
	body := conversationSnapshotBody(t, chatsvc.Snapshot{
		SessionID: domain.SessionID("p1-1"),
		Mode:      domain.SessionModeChat,
		Turns: []domain.ConversationTurn{{
			ID:          "turn-1",
			State:       domain.TurnStateCompleted,
			RequestedAt: time.Now().UTC(),
			Diff: &domain.ConversationTurnDiff{
				Files: []domain.ConversationDiffFile{
					{Path: "new.txt", OldPath: "old.txt", Additions: 3, Deletions: 1, Status: "renamed"},
				},
			},
		}},
	})

	turns, ok := body["turns"].([]any)
	if !ok || len(turns) != 1 {
		t.Fatalf("turns = %#v", body["turns"])
	}
	diff, ok := turns[0].(map[string]any)["diff"].(map[string]any)
	if !ok {
		t.Fatalf("turn has no diff: %#v", turns[0])
	}
	files, ok := diff["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("diff files = %#v", diff["files"])
	}

	file := files[0].(map[string]any)
	if file["path"] != "new.txt" || file["status"] != "renamed" {
		t.Errorf("file = %#v", file)
	}
	// A rename shown only as its new path reads as an addition, so both ends ship.
	if file["oldPath"] != "old.txt" {
		t.Errorf("oldPath = %#v", file["oldPath"])
	}
	if file["additions"] != float64(3) || file["deletions"] != float64(1) {
		t.Errorf("counts = %#v / %#v", file["additions"], file["deletions"])
	}
	// No patch text: this body is polled once a second while a turn runs.
	if _, present := file["patch"]; present {
		t.Error("diff file carried patch text onto the polled snapshot")
	}
}

// A turn the provider never reported a diff for must not claim an empty diff.
// "Changed nothing" and "this agent does not report diffs" are different answers.
func TestSnapshotOmitsAbsentTurnDiff(t *testing.T) {
	body := conversationSnapshotBody(t, chatsvc.Snapshot{
		SessionID: domain.SessionID("p1-1"),
		Mode:      domain.SessionModeChat,
		Turns: []domain.ConversationTurn{{
			ID: "turn-1", State: domain.TurnStateCompleted, RequestedAt: time.Now().UTC(),
		}},
	})

	turn := body["turns"].([]any)[0].(map[string]any)
	if _, present := turn["diff"]; present {
		t.Errorf("turn with no reported diff still shipped one: %#v", turn["diff"])
	}
}

// The accumulated stream replaces the provider's aggregate and says which source
// the client is looking at. Both are partial for different reasons, so the hedge
// stays either way.
func TestSnapshotPrefersStreamedOutputOverAggregate(t *testing.T) {
	aggregate, err := json.Marshal(map[string]any{
		"command":            "go test ./...",
		"output":             "ok  pkg/b\n",
		"outputMayBePartial": true,
		"outputSource":       "aggregate",
		"exitCode":           0,
	})
	if err != nil {
		t.Fatalf("encode detail: %v", err)
	}

	body := conversationSnapshotBody(t, chatsvc.Snapshot{
		SessionID: domain.SessionID("p1-1"),
		Mode:      domain.SessionModeChat,
		Activities: []domain.ConversationActivity{{
			ID:                     "act-1",
			Kind:                   domain.ActivityKindCommand,
			Status:                 domain.ActivityStatusCompleted,
			Summary:                "go test ./...",
			Detail:                 aggregate,
			CommandOutput:          "ok  pkg/a\nok  pkg/b\n",
			CommandOutputTruncated: true,
			CreatedAt:              time.Now().UTC(),
		}},
	})

	activities, ok := body["activities"].([]any)
	if !ok || len(activities) != 1 {
		t.Fatalf("activities = %#v", body["activities"])
	}
	detail, ok := activities[0].(map[string]any)["detail"].(map[string]any)
	if !ok {
		t.Fatalf("activity has no detail: %#v", activities[0])
	}

	if detail["output"] != "ok  pkg/a\nok  pkg/b\n" {
		t.Errorf("output = %#v, want the accumulated stream", detail["output"])
	}
	if detail["outputSource"] != "stream" {
		t.Errorf("outputSource = %#v, want stream", detail["outputSource"])
	}
	if detail["outputTruncated"] != true {
		t.Errorf("outputTruncated = %#v", detail["outputTruncated"])
	}
	// Still partial: the provider was measured dropping the first chunk from the
	// delta stream too, so the stream is not a complete record either.
	if detail["outputMayBePartial"] != true {
		t.Errorf("outputMayBePartial = %#v, want true", detail["outputMayBePartial"])
	}
	// Fields the aggregate carried are untouched.
	if detail["command"] != "go test ./..." {
		t.Errorf("command = %#v", detail["command"])
	}
}

// With no streamed output at all the aggregate stands, labelled as such.
func TestSnapshotKeepsAggregateWhenNoStreamArrived(t *testing.T) {
	aggregate, err := json.Marshal(map[string]any{
		"output":             "done\n",
		"outputMayBePartial": true,
		"outputSource":       "aggregate",
	})
	if err != nil {
		t.Fatalf("encode detail: %v", err)
	}

	body := conversationSnapshotBody(t, chatsvc.Snapshot{
		SessionID: domain.SessionID("p1-1"),
		Mode:      domain.SessionModeChat,
		Activities: []domain.ConversationActivity{{
			ID: "act-1", Kind: domain.ActivityKindCommand,
			Status: domain.ActivityStatusCompleted, Summary: "echo done",
			Detail: aggregate, CreatedAt: time.Now().UTC(),
		}},
	})

	detail := body["activities"].([]any)[0].(map[string]any)["detail"].(map[string]any)
	if detail["output"] != "done\n" {
		t.Errorf("output = %#v", detail["output"])
	}
	if detail["outputSource"] != "aggregate" {
		t.Errorf("outputSource = %#v, want aggregate", detail["outputSource"])
	}
	if _, present := detail["outputTruncated"]; present {
		t.Error("untruncated output still carried the truncation flag")
	}
}

// TestSendConversationReturns202OnDeliveryUnknown proves the send route maps
// chatsvc.ErrGovernedDeliveryUnknown to the same 202 shape as an ordinary
// accepted send, echoing the real turn the service returned alongside that
// error rather than failing the request or fabricating a body.
func TestSendConversationReturns202OnDeliveryUnknown(t *testing.T) {
	service := &fakeConversationService{
		sendErr:  chatsvc.ErrGovernedDeliveryUnknown,
		sendTurn: domain.ConversationTurn{ID: "turn-unknown-1", State: domain.TurnStateQueued},
	}
	server := conversationTestServer(t, service)
	body := []byte(`{"text":"do it","clientMessageId":"message-unknown"}`)
	request, err := http.NewRequest(http.MethodPost,
		server.URL+"/api/v1/sessions/p1-1/conversation/messages", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST message: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted {
		got, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, got)
	}
	var decoded map[string]any
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded["turnId"] != "turn-unknown-1" || decoded["state"] != "queued" || decoded["duplicate"] != false {
		t.Fatalf("body = %#v", decoded)
	}
}

// TestSendConversationOtherErrorsStillFail confirms the new delivery-unknown
// branch changed nothing else: an ordinary chat-service error still maps
// through writeConversationError, not through the 202 path.
func TestSendConversationOtherErrorsStillFail(t *testing.T) {
	service := &fakeConversationService{sendErr: chatsvc.ErrNoController}
	server := conversationTestServer(t, service)
	body := []byte(`{"text":"do it","clientMessageId":"message-1"}`)
	request, err := http.NewRequest(http.MethodPost,
		server.URL+"/api/v1/sessions/p1-1/conversation/messages", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST message: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusConflict {
		got, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, got)
	}
}

// TestSnapshotExposesGovernedTurnAndControlBlocks checks the wire shape for
// both new session-wide lists, and that a turn's own dispatchBlockedSince /
// dispatchBlockedState only appear when that turn has a matching entry in
// governedTurnBlocks -- with the block's real state echoed, not forced to
// delivery_unknown.
func TestSnapshotExposesGovernedTurnAndControlBlocks(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	body := conversationSnapshotBody(t, chatsvc.Snapshot{
		SessionID: domain.SessionID("p1-1"),
		Mode:      domain.SessionModeChat,
		Turns: []domain.ConversationTurn{
			{ID: "turn-blocked", State: domain.TurnStateQueued, RequestedAt: now},
			{ID: "turn-free", State: domain.TurnStateCompleted, RequestedAt: now},
		},
		GovernedTurnBlocks: []chatsvc.GovernedTurnBlock{{
			TurnID: "turn-blocked", State: domain.GovernedCommandDispatching,
			Quiescence: domain.GovernedCommandQuiescenceNotApplicable, UpdatedAt: now,
		}},
		GovernedControlBlocks: []chatsvc.GovernedControlBlock{
			{
				ID: "control-interrupt", Class: domain.GovernedControlInterrupt,
				State: domain.GovernedCommandDeliveryUnknown, Quiescence: domain.GovernedCommandQuiescencePending,
				ProviderTurnID: "provider-turn-1", UpdatedAt: now,
			},
			{
				ID: "control-answer", Class: domain.GovernedControlAnswer,
				State: domain.GovernedCommandClaimed, Quiescence: domain.GovernedCommandQuiescenceNotApplicable,
				RequestInstanceID: "request-instance-1", UpdatedAt: now,
			},
		},
	})

	turns := body["turns"].([]any)
	blocked := turns[0].(map[string]any)
	if blocked["dispatchBlockedSince"] != now.Format(time.RFC3339) || blocked["dispatchBlockedState"] != "dispatching" {
		t.Fatalf("blocked turn = %#v", blocked)
	}
	free := turns[1].(map[string]any)
	if _, present := free["dispatchBlockedSince"]; present {
		t.Fatalf("unblocked turn wrongly carries dispatchBlockedSince: %#v", free)
	}
	if _, present := free["dispatchBlockedState"]; present {
		t.Fatalf("unblocked turn wrongly carries dispatchBlockedState: %#v", free)
	}

	turnBlocks := body["governedTurnBlocks"].([]any)
	if len(turnBlocks) != 1 {
		t.Fatalf("governedTurnBlocks = %#v", turnBlocks)
	}
	turnBlock := turnBlocks[0].(map[string]any)
	if turnBlock["kind"] != "turn" || turnBlock["turnId"] != "turn-blocked" ||
		turnBlock["state"] != "dispatching" || turnBlock["since"] != now.Format(time.RFC3339) {
		t.Fatalf("turn block = %#v", turnBlock)
	}

	controlBlocks := body["governedControlBlocks"].([]any)
	if len(controlBlocks) != 2 {
		t.Fatalf("governedControlBlocks = %#v", controlBlocks)
	}
	interrupt := controlBlocks[0].(map[string]any)
	if interrupt["kind"] != "interrupt" || interrupt["state"] != "delivery_unknown" ||
		interrupt["quiescence"] != "pending" || interrupt["providerTurnId"] != "provider-turn-1" {
		t.Fatalf("containment-failed-shaped interrupt block = %#v", interrupt)
	}
	if _, present := interrupt["requestInstanceId"]; present {
		t.Fatalf("interrupt block wrongly carries requestInstanceId: %#v", interrupt)
	}
	answer := controlBlocks[1].(map[string]any)
	if answer["kind"] != "answer" || answer["state"] != "claimed" || answer["requestInstanceId"] != "request-instance-1" {
		t.Fatalf("answer block = %#v", answer)
	}
	if _, present := answer["providerTurnId"]; present {
		t.Fatalf("answer block wrongly carries providerTurnId: %#v", answer)
	}
}

package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/agent/codex"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/chatdriver/codexappserver"
	llmanthropic "github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/llm/anthropic"
	llmopenai "github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/llm/openai"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	intelligencesvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence"
	settingssvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/settings"
)

// Waldo reasons with the owner's own model (ADR 0012). Which provider that is
// belongs to the owner too: the reasoning port is provider-neutral, so
// supporting a second one is a wiring decision, not an architectural one.
const (
	providerAnthropic = "anthropic"
	providerOpenAI    = "openai"
	providerCodex     = "codex"
)

// planningProviderID maps a reasoning settings provider identifier onto the
// exact provenance ID its LLM client reports as EffectiveProvider. The
// anthropic and openai clients' IDs already equal their settings provider
// string, but the Codex harness client's ID is
// codexappserver.IntelligenceProviderID ("codex-app-server"), not "codex". A
// PlanningBinding built from the raw settings string never matches Codex's
// actual response provenance, so every native Codex planning turn is refused
// as PLANNING_PROVIDER_MISMATCH regardless of the model's real answer.
func planningProviderID(settingsProvider string) domain.IntelligenceProviderID {
	if settingsProvider == providerCodex {
		return domain.IntelligenceProviderID(codexappserver.IntelligenceProviderID)
	}
	return domain.IntelligenceProviderID(settingsProvider)
}

// reasoningConfig is the resolved answer to "whose model, and with what key".
type reasoningConfig struct {
	Provider string
	APIKey   string
	Model    string
	Effort   string
	// KeySource names the environment variable the key came from, for a log
	// line that tells the owner which of several keys Waldo actually picked up.
	KeySource string
	// BaseURL redirects reasoning at a local stand-in. It is a development and
	// test seam only: it comes from KENNEL_WALDO_BASE_URL and is never
	// persisted in settings, so a packaged install always talks to the real
	// provider. It exists so the reasoning path — credential handling,
	// classification, readiness — can be exercised against a controlled local
	// endpoint instead of a live billed provider.
	BaseURL string
}

// resolveReasoningConfig decides which provider Waldo thinks with.
//
// An explicit KENNEL_WALDO_PROVIDER always wins. Otherwise the choice follows
// whichever credential is present, and Anthropic keeps precedence so an
// install that worked before this adapter existed keeps behaving identically.
func resolveReasoningConfig(lookup func(string) string) (reasoningConfig, error) {
	cfg := reasoningConfig{
		Model:   strings.TrimSpace(lookup("KENNEL_WALDO_MODEL")),
		Effort:  strings.TrimSpace(lookup("KENNEL_WALDO_EFFORT")),
		BaseURL: strings.TrimSpace(lookup("KENNEL_WALDO_BASE_URL")),
	}

	sharedKey := strings.TrimSpace(lookup("KENNEL_WALDO_API_KEY"))
	anthropicKey := strings.TrimSpace(lookup("ANTHROPIC_API_KEY"))
	openaiKey := strings.TrimSpace(lookup("OPENAI_API_KEY"))

	switch requested := strings.ToLower(strings.TrimSpace(lookup("KENNEL_WALDO_PROVIDER"))); requested {
	case providerAnthropic:
		cfg.Provider = providerAnthropic
	case providerOpenAI:
		cfg.Provider = providerOpenAI
	case providerCodex:
		cfg.Provider = providerCodex
	case "":
		// No stated preference: follow the key the owner actually has.
		switch {
		case sharedKey != "", anthropicKey != "":
			cfg.Provider = providerAnthropic
		case openaiKey != "":
			cfg.Provider = providerOpenAI
		default:
			// Nothing is configured. Name the historical default so the
			// unconfigured-key error below reads as one clear problem.
			cfg.Provider = providerAnthropic
		}
	default:
		return reasoningConfig{}, fmt.Errorf(
			"KENNEL_WALDO_PROVIDER is %q; Waldo reasons with %q, %q, or %q", requested, providerAnthropic, providerOpenAI, providerCodex)
	}
	if cfg.Provider == providerCodex {
		cfg.KeySource = "codex-app-server-sign-in"
		return cfg, nil
	}

	// KENNEL_WALDO_API_KEY is preferred for both providers so an owner can keep
	// Waldo's reasoning credential separate from the key their coding agents
	// already use. The provider's own variable is the familiar fallback.
	fallback := "ANTHROPIC_API_KEY"
	fallbackKey := anthropicKey
	if cfg.Provider == providerOpenAI {
		fallback, fallbackKey = "OPENAI_API_KEY", openaiKey
	}
	switch {
	case sharedKey != "":
		cfg.APIKey, cfg.KeySource = sharedKey, "KENNEL_WALDO_API_KEY"
	case fallbackKey != "":
		cfg.APIKey, cfg.KeySource = fallbackKey, fallback
	default:
		return cfg, fmt.Errorf("no reasoning key for %s: set KENNEL_WALDO_API_KEY or %s", cfg.Provider, fallback)
	}
	return cfg, nil
}

// newReasoner builds Waldo's reasoning client for the resolved provider.
//
// The key belongs to the user and stays on this machine: ADR 0003 rules out a
// Waldo-operated LLM API and Waldo-funded inference, not Waldo reasoning with
// the owner's own authenticated provider.
// Each branch returns an explicit nil interface on failure. Handing back the
// adapter's typed nil pointer instead would produce a non-nil interface value
// holding nil, and every `reasoner != nil` check downstream would be wrong.
func newReasoner(cfg reasoningConfig) (ports.LLMClient, error) {
	switch cfg.Provider {
	case providerOpenAI:
		client, err := llmopenai.New(llmopenai.Config{APIKey: cfg.APIKey, Model: cfg.Model, Effort: cfg.Effort, BaseURL: cfg.BaseURL, MaxRetries: 0})
		if err != nil {
			return nil, err
		}
		return client, nil
	case providerCodex:
		return newCodexReasoner(cfg, slog.Default(), "")
	default:
		client, err := llmanthropic.New(llmanthropic.Config{APIKey: cfg.APIKey, Model: cfg.Model, Effort: cfg.Effort, BaseURL: cfg.BaseURL, MaxRetries: 0})
		if err != nil {
			return nil, err
		}
		return client, nil
	}
}

// configuredIntelligenceProvider resolves settings at call time. A fresh
// profile can therefore configure a credential and retry without restarting
// the daemon; the renderer never receives the credential.
type configuredIntelligenceProvider struct {
	settings *settingssvc.Service
	log      *slog.Logger
}

var _ ports.IntelligenceProvider = (*configuredIntelligenceProvider)(nil)
var _ ports.PlanningIntelligenceProvider = (*configuredIntelligenceProvider)(nil)

func newConfiguredIntelligenceProvider(settings *settingssvc.Service, log *slog.Logger) *configuredIntelligenceProvider {
	if log == nil {
		log = slog.Default()
	}
	return &configuredIntelligenceProvider{settings: settings, log: log}
}

func (*configuredIntelligenceProvider) ID() domain.IntelligenceProviderID {
	return intelligencesvc.LLMProviderID
}

func (p *configuredIntelligenceProvider) client(ctx context.Context) (ports.LLMClient, error) {
	if p == nil || p.settings == nil {
		return nil, fmt.Errorf("reasoning settings are unavailable")
	}
	cfg, err := p.settings.ResolveReasoning(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Provider == providerCodex {
		return newCodexReasoner(reasoningConfig{Provider: cfg.Provider, Model: cfg.Model, Effort: cfg.Effort}, p.log, "")
	}
	return newReasoner(reasoningConfig{
		Provider: cfg.Provider, APIKey: cfg.APIKey, Model: cfg.Model, Effort: cfg.Effort,
		BaseURL: p.settings.ReasoningBaseURL(),
	})
}

// newCodexReasoner builds the Codex-backed reasoning client. codexHome binds
// the launch to a scoped, verified CODEX_HOME (native-harness planning turns
// pass the home the mission-plugin provisioner just verified); empty runs
// against the ambient home (probes and settings verification).
func newCodexReasoner(cfg reasoningConfig, log *slog.Logger, codexHome string) (ports.LLMClient, error) {
	if log == nil {
		log = slog.Default()
	}
	// The scoped home and the skill invocation travel together: a verified
	// home without the mission skill's activation would plan with the skill's
	// rules switched off, and a skill path without the verified home would
	// invoke an unverified artifact.
	skillPath := ""
	if codexHome != "" {
		skillPath = codex.MissionSkillMDPath(codexHome)
	}
	driver := codexappserver.New(codex.New(), log)
	return codexappserver.NewIntelligenceClient(driver, codexappserver.IntelligenceConfig{
		Model: cfg.Model, Effort: cfg.Effort, Timeout: 2 * time.Minute, CodexHome: codexHome, MissionSkillPath: skillPath,
	}), nil
}

func (p *configuredIntelligenceProvider) AnalyzeContract(ctx context.Context, request ports.ContractIntelligenceRequest) (ports.ContractIntelligenceResponse, error) {
	client, err := p.client(ctx)
	if err != nil {
		return ports.ContractIntelligenceResponse{}, err
	}
	return intelligencesvc.NewLLMProvider(client).AnalyzeContract(ctx, request)
}

func (p *configuredIntelligenceProvider) DraftPlan(ctx context.Context, request ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	client, err := p.client(ctx)
	if err != nil {
		return ports.PlanIntelligenceResponse{}, err
	}
	return intelligencesvc.NewLLMProvider(client).DraftPlan(ctx, request)
}

func (p *configuredIntelligenceProvider) PlanningCandidates(ctx context.Context) ([]ports.PlanningCandidate, error) {
	if p == nil || p.settings == nil {
		return nil, fmt.Errorf("reasoning settings are unavailable")
	}
	status, err := p.settings.GetReasoning(ctx)
	if err != nil {
		return nil, err
	}
	selection := domain.PlanningModelProviderDefault
	model := ""
	if strings.TrimSpace(status.Model) != "" {
		selection, model = domain.PlanningModelExplicit, strings.TrimSpace(status.Model)
	}
	mode := domain.PlanningModeDirectAPI
	ready := status.Ready
	code, detail := status.ErrorCode, status.Error
	if status.Provider == providerCodex {
		mode = domain.PlanningModeNativeHarness
		// Local availability only proves that Kennel can attempt the signed-in
		// harness. Native planning becomes selectable after the owner's real
		// packet probe succeeds for this exact provider/model selection.
		if ready && !status.Verified {
			ready = false
			code = "REASONING_NOT_VERIFIED"
			detail = "Verify the signed-in Codex planning path in Settings before starting"
		}
	}
	binding := domain.PlanningBinding{
		Mode: mode, Provider: planningProviderID(status.Provider), ModelSelection: selection,
		Model: model, Effort: strings.TrimSpace(status.Effort),
	}
	return []ports.PlanningCandidate{{
		ID: planningCandidateID(binding), Binding: binding, Ready: ready,
		UnavailableCode: code, UnavailableDetail: detail,
	}}, nil
}

func (p *configuredIntelligenceProvider) DiscussPlan(ctx context.Context, request ports.PlanningDiscussionRequest) (ports.PlanningDiscussionResponse, error) {
	if p == nil || p.settings == nil {
		return ports.PlanningDiscussionResponse{}, ports.NewReasoningFailure(
			ports.ReasoningNotConfigured, "Reasoning settings are unavailable", nil)
	}
	cfg, err := p.settings.ResolveReasoning(ctx)
	if err != nil {
		return ports.PlanningDiscussionResponse{}, err
	}
	selection := domain.PlanningModelProviderDefault
	model := ""
	if strings.TrimSpace(cfg.Model) != "" {
		selection, model = domain.PlanningModelExplicit, strings.TrimSpace(cfg.Model)
	}
	mode := domain.PlanningModeDirectAPI
	if cfg.Provider == providerCodex {
		mode = domain.PlanningModeNativeHarness
	}
	current := domain.PlanningBinding{
		Mode: mode, Provider: planningProviderID(cfg.Provider),
		ModelSelection: selection, Model: model, Effort: strings.TrimSpace(cfg.Effort),
	}
	if current != request.Binding {
		return ports.PlanningDiscussionResponse{}, ports.NewReasoningFailure(
			ports.ReasoningUnavailable,
			"The selected planning provider or model changed; start a new planning session instead of silently substituting it", nil)
	}
	var client ports.LLMClient
	if cfg.Provider == providerCodex {
		// The planning turn provably runs inside the environment the outcome
		// service just verified: request.HarnessHome is the mission-plugin
		// provisioner's own return for this session's space. A native-harness
		// turn without it is refused rather than silently run in the ambient
		// home, because there the /mission command is unverified.
		if request.Binding.Mode == domain.PlanningModeNativeHarness && strings.TrimSpace(request.HarnessHome) == "" {
			return ports.PlanningDiscussionResponse{}, ports.NewReasoningFailure(
				ports.ReasoningUnavailable,
				"Native-harness planning requires the verified mission-plugin home from the planning service", nil)
		}
		client, err = newCodexReasoner(reasoningConfig{Provider: cfg.Provider, Model: cfg.Model, Effort: cfg.Effort}, p.log, request.HarnessHome)
	} else {
		client, err = newReasoner(reasoningConfig{
			Provider: cfg.Provider, APIKey: cfg.APIKey, Model: cfg.Model, Effort: cfg.Effort, BaseURL: cfg.BaseURL,
		})
	}
	if err != nil {
		return ports.PlanningDiscussionResponse{}, err
	}
	return intelligencesvc.NewLLMProvider(client).DiscussPlan(ctx, request)
}

func planningCandidateID(binding domain.PlanningBinding) string {
	return strings.Join([]string{
		string(binding.Mode), string(binding.Provider), string(binding.ModelSelection), binding.Model, binding.Effort,
	}, ":")
}

// probeReasoningBudget bounds one readiness probe. Named operational policy: a
// probe that hangs must not hold the settings request open.
const probeReasoningBudget = 30 * time.Second

// probeReasoning performs the smallest real reasoning call that still proves
// the whole path works: credential accepted, model reachable, and a structured
// reply that parses.
//
// It exists because "a credential is stored" is not evidence that reasoning
// works — a key can be revoked, mistyped, or lack access to the selected model,
// and reporting that as ready sends the owner into a Plan proposal that then
// fails at the provider. This is the only thing that may set verified state,
// and it runs only when the owner asks, because the call may be billed.
func probeReasoning(ctx context.Context, cfg settingssvc.ReasoningConfig) error {
	var client ports.LLMClient
	var err error
	if cfg.Provider == providerCodex {
		client, err = newCodexReasoner(reasoningConfig{Provider: cfg.Provider, Model: cfg.Model, Effort: cfg.Effort}, slog.Default(), "")
	} else {
		client, err = newReasoner(reasoningConfig{
			Provider: cfg.Provider, APIKey: cfg.APIKey, Model: cfg.Model, Effort: cfg.Effort,
			BaseURL: cfg.BaseURL,
		})
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, probeReasoningBudget)
	defer cancel()
	// A trivial fixed schema, so the probe measures the provider path and not
	// the model's ability to handle a hard request.
	_, err = client.Complete(ctx, ports.LLMRequest{
		System:     "Reply with the requested JSON object and nothing else.",
		User:       "Return {\"ok\": true}.",
		SchemaName: "readiness_probe",
		MaxTokens:  256,
		Schema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"ok": map[string]any{"type": "boolean"}},
			"required":             []any{"ok"},
			"additionalProperties": false,
		},
	})
	return err
}

// probeReasoningAvailability is local/protocol evidence only. It never marks
// the selected model verified and never sends a model turn.
func probeReasoningAvailability(ctx context.Context, cfg settingssvc.ReasoningConfig) error {
	if cfg.Provider != providerCodex {
		return nil
	}
	driver := codexappserver.New(codex.New(), slog.Default())
	return driver.ProbeIntelligence(ctx)
}

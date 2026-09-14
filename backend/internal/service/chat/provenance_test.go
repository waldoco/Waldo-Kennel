package chat_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	chatsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/chat"
)

// provenanceDriver is a chat driver that reports negotiated protocol
// provenance the way the Codex adapter does after PR #180.
type provenanceDriver struct {
	fakeDriver
	provenance ports.ChatProtocolProvenance
}

func (d provenanceDriver) ProtocolProvenance(context.Context) (ports.ChatProtocolProvenance, error) {
	return d.provenance, nil
}

func TestStartPersistsNegotiatedProtocolProvenance(t *testing.T) {
	st := openStore(t)
	reported := ports.ChatProtocolProvenance{
		Provider:             "codex app-server",
		InstalledVersion:     "0.154.0",
		GeneratedFrom:        "0.153.4",
		ProtocolDigest:       "821a34c2aebae893",
		GeneratedDigest:      "821a34c2aebae893",
		MatchesGenerated:     true,
		DegradedCapabilities: []ports.ChatCapability{ports.ChatCapabilitySteer},
	}
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Drivers: fakeRegistry{driver: provenanceDriver{fakeDriver: fakeDriver{conv: newFakeConversation()}, provenance: reported}},
		Log:     slog.New(slog.DiscardHandler),
		NewID:   func() string { return "conversation-provenance" },
	})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })

	if _, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		DataDir: t.TempDir(), WorkspacePath: t.TempDir(),
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	rec, found, err := st.ChatProtocolProvenanceForBinding(context.Background(), string(testSession), time.Now().UTC())
	if err != nil || !found {
		t.Fatalf("provenance after start: found=%v err=%v", found, err)
	}
	if rec.SessionID != string(testSession) || rec.Seq != 1 || rec.Harness != domain.HarnessCodex {
		t.Fatalf("record identity = %+v", rec)
	}
	if rec.Provider != reported.Provider || rec.InstalledVersion != reported.InstalledVersion ||
		rec.ProtocolDigest != reported.ProtocolDigest || !rec.MatchesGenerated {
		t.Fatalf("record = %+v, want the negotiated report", rec)
	}
	if len(rec.DegradedCapabilities) != 1 || rec.DegradedCapabilities[0] != string(ports.ChatCapabilitySteer) {
		t.Fatalf("degraded = %v, want [steer]", rec.DegradedCapabilities)
	}

	// A restart of the session negotiates again and APPENDS a second episode;
	// it never rewrites the first. (A Start against the live controller
	// returns it without renegotiating, by design, so the second episode
	// needs a real stop first.)
	if err := svc.Stop(context.Background(), testSession); err != nil {
		t.Fatalf("Stop before resume: %v", err)
	}
	if _, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		DataDir: t.TempDir(), WorkspacePath: t.TempDir(), ProviderConversationID: "thread-9",
	}); err != nil {
		t.Fatalf("resume Start: %v", err)
	}
	latest, found, err := st.ChatProtocolProvenanceForBinding(context.Background(), string(testSession), time.Now().UTC())
	if err != nil || !found {
		t.Fatalf("provenance after resume: found=%v err=%v", found, err)
	}
	if latest.Seq != 2 {
		t.Fatalf("seq after resume = %d, want 2 (append-only episodes)", latest.Seq)
	}
}

func TestStartWithoutProvenanceReporterPersistsNothing(t *testing.T) {
	st := openStore(t)
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Drivers: fakeRegistry{driver: fakeDriver{conv: newFakeConversation()}},
		Log:     slog.New(slog.DiscardHandler),
		NewID:   func() string { return "conversation-no-provenance" },
	})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })

	if _, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		DataDir: t.TempDir(), WorkspacePath: t.TempDir(),
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, found, err := st.ChatProtocolProvenanceForBinding(context.Background(), string(testSession), time.Now().UTC()); err != nil || found {
		t.Fatalf("provenance for non-reporting driver: found=%v err=%v, want honest absence", found, err)
	}
}

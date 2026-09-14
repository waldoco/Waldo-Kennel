package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func provenanceEpisode(sessionID string, negotiatedAt time.Time, digest string) domain.ChatProtocolProvenance {
	return domain.ChatProtocolProvenance{
		SessionID:            sessionID,
		Harness:              domain.HarnessCodex,
		Provider:             "codex app-server",
		InstalledVersion:     "0.154.0",
		GeneratedFrom:        "0.153.4",
		ProtocolDigest:       digest,
		GeneratedDigest:      "821a34c2aebae893",
		MatchesGenerated:     digest == "821a34c2aebae893",
		DegradedCapabilities: []string{"steer", "rollback"},
		MissingFloor:         []string{},
		NegotiatedAt:         negotiatedAt,
	}
}

func TestChatProtocolProvenanceAppendsEpisodesWithSequentialSeq(t *testing.T) {
	st := sqlitetest.MustOpenAt(t, t.TempDir())
	ctx := context.Background()
	base := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	first := provenanceEpisode("session-1", base, "821a34c2aebae893")
	second := provenanceEpisode("session-1", base.Add(time.Hour), "9f00d1gest0000000")
	other := provenanceEpisode("session-2", base, "aaaabbbbccccdddd")
	for _, episode := range []domain.ChatProtocolProvenance{first, second, other} {
		if err := st.RecordChatProtocolProvenance(ctx, episode); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	rec, found, err := st.ChatProtocolProvenanceForBinding(ctx, "session-1", base.Add(2*time.Hour))
	if err != nil || !found {
		t.Fatalf("read latest: found=%v err=%v", found, err)
	}
	if rec.Seq != 2 || rec.ProtocolDigest != second.ProtocolDigest {
		t.Fatalf("latest episode = seq %d digest %s, want seq 2 of the later negotiation", rec.Seq, rec.ProtocolDigest)
	}
	otherRec, found, err := st.ChatProtocolProvenanceForBinding(ctx, "session-2", base.Add(2*time.Hour))
	if err != nil || !found {
		t.Fatalf("read other session: found=%v err=%v", found, err)
	}
	if otherRec.Seq != 1 {
		t.Fatalf("session-2 seq = %d, want its own sequence starting at 1", otherRec.Seq)
	}
}

func TestChatProtocolProvenanceRoundTripsCapabilityLists(t *testing.T) {
	st := sqlitetest.MustOpenAt(t, t.TempDir())
	ctx := context.Background()
	base := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	if err := st.RecordChatProtocolProvenance(ctx, provenanceEpisode("session-1", base, "821a34c2aebae893")); err != nil {
		t.Fatalf("record: %v", err)
	}
	rec, found, err := st.ChatProtocolProvenanceForBinding(ctx, "session-1", base)
	if err != nil || !found {
		t.Fatalf("read: found=%v err=%v", found, err)
	}
	if len(rec.DegradedCapabilities) != 2 || rec.DegradedCapabilities[0] != "steer" || rec.DegradedCapabilities[1] != "rollback" {
		t.Fatalf("degraded = %v, want [steer rollback]", rec.DegradedCapabilities)
	}
	if len(rec.MissingFloor) != 0 {
		t.Fatalf("missing floor = %v, want empty", rec.MissingFloor)
	}
	if !rec.MatchesGenerated || rec.InstalledVersion != "0.154.0" || rec.Harness != domain.HarnessCodex {
		t.Fatalf("record = %+v", rec)
	}
}

func TestChatProtocolProvenanceForBindingNeverAnswersWithALaterEpisode(t *testing.T) {
	st := sqlitetest.MustOpenAt(t, t.TempDir())
	ctx := context.Background()
	base := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	old := provenanceEpisode("session-1", base, "olddigest0000001")
	upgraded := provenanceEpisode("session-1", base.Add(24*time.Hour), "newdigest0000002")
	if err := st.RecordChatProtocolProvenance(ctx, old); err != nil {
		t.Fatalf("record old: %v", err)
	}
	if err := st.RecordChatProtocolProvenance(ctx, upgraded); err != nil {
		t.Fatalf("record upgraded: %v", err)
	}

	// A binding made before the upgrade must still resolve to the OLD episode:
	// the provider build the work actually ran on, not the one installed now.
	rec, found, err := st.ChatProtocolProvenanceForBinding(ctx, "session-1", base.Add(time.Hour))
	if err != nil || !found {
		t.Fatalf("read at old binding: found=%v err=%v", found, err)
	}
	if rec.ProtocolDigest != old.ProtocolDigest {
		t.Fatalf("binding before upgrade resolved digest %s, want the episode contemporaneous with the work", rec.ProtocolDigest)
	}

	// A binding made before ANY recorded episode falls back to the earliest:
	// the negotiation that created the session predates its first binding.
	rec, found, err = st.ChatProtocolProvenanceForBinding(ctx, "session-1", base.Add(-time.Hour))
	if err != nil || !found {
		t.Fatalf("read before all episodes: found=%v err=%v", found, err)
	}
	if rec.ProtocolDigest != old.ProtocolDigest || !rec.NegotiatedAt.Equal(base) {
		t.Fatalf("earliest fallback = %+v, want the first episode with its own NegotiatedAt", rec)
	}

	// No recorded provenance is honest absence, not an error.
	if _, found, err := st.ChatProtocolProvenanceForBinding(ctx, "session-unknown", base); err != nil || found {
		t.Fatalf("unknown session: found=%v err=%v, want absence", found, err)
	}
}

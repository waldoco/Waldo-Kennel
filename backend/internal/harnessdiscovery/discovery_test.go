package harnessdiscovery

import (
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"testing"
)

func TestClassifyFailsClosedOnMissingIdentity(t *testing.T) {
	m := domain.HarnessAdapterManifest{}
	if got := Classify(m, ports.HarnessInstallation{}, nil, nil); got != domain.ManifestDigestTamper {
		t.Fatalf("got %s", got)
	}
}
func TestNormalizeCapabilities(t *testing.T) {
	got, err := NormalizeCapabilities([]domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn, domain.HarnessCapabilityAnswer})
	if err != nil || got[0] != domain.HarnessCapabilityAnswer {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := NormalizeCapabilities([]domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn, domain.HarnessCapabilityTurn}); err == nil {
		t.Fatal("duplicate accepted")
	}
}

package bridgeactivation

import (
	"context"
	"crypto/ed25519"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"strings"
	"testing"
	"time"
)

func localEnv(flag, origin, owner string) func(string) string {
	return func(k string) string {
		return map[string]string{"KENNEL_WALDO_BRIDGE_ENABLED": flag, "KENNEL_WALDO_ORIGIN": origin, "KENNEL_WALDO_OWNER_ID": owner}[k]
	}
}
func TestLocalConfigDefaultsOff(t *testing.T) {
	for _, flag := range []string{"", "0", "true", " 1", "2"} {
		cfg, e := LoadLocalConfig(localEnv(flag, "garbage", "garbage"))
		if e != nil || cfg.Enabled {
			t.Fatal("off validated config")
		}
	}
}
func TestLocalConfigOriginRules(t *testing.T) {
	for _, origin := range []string{"http://host", "ws://host", "wss://host", "https://", "https://host/path", "https://host/", "https://host?x=1", "https://host?", "https://host#x", "https://host#", "https://user@host", "HTTPS://host", " https://host", "https://host ", "http://localhost", "http://127.0.0.1"} {
		if _, e := LoadLocalConfig(localEnv("1", origin, "owner")); e == nil {
			t.Fatalf("accepted origin %q", origin)
		}
	}
	for _, origin := range []string{"https://host", "https://host:8443"} {
		cfg, e := LoadLocalConfig(localEnv("1", origin, "owner"))
		if e != nil || !cfg.Enabled || cfg.Origin != origin {
			t.Fatal(e)
		}
	}
}

type identifierProbe struct{}

var identifierValid = errors.New("owner passed devicebridge identifier gate")

func (identifierProbe) GetPairedDeviceBridgeDevice(context.Context, string) (domain.DeviceBridgeDevice, bool, error) {
	return domain.DeviceBridgeDevice{}, false, identifierValid
}
func (identifierProbe) CreateDeviceBridgeDevice(context.Context, domain.DeviceBridgeDevice) error {
	return identifierValid
}
func (identifierProbe) TransitionDeviceBridgeDevice(context.Context, domain.DeviceBridgeDeviceID, domain.DeviceBridgeState, time.Time) (bool, error) {
	return false, identifierValid
}

type identifierKeys struct{}

func (identifierKeys) Put(context.Context, ed25519.PrivateKey) (string, error) {
	return "", identifierValid
}
func (identifierKeys) Clear(context.Context, string) error { return nil }
func TestLocalConfigOwnerIDMatchesDeviceBridgeRule(t *testing.T) {
	p, e := devicebridge.NewPairingCoordinator(identifierProbe{}, identifierKeys{}, nil, nil, "https://example.test"+devicebridge.RedeemPath)
	if e != nil {
		t.Fatal(e)
	}
	// "" is not an identifier; it selects owner binding (TestLocalConfigWithoutOwnerBindsAtPairing).
	for _, owner := range []string{"owner", "A_9-x", "a.b", "a/b", " a", "é", strings.Repeat("a", devicebridge.IdentifierMaxBytes), strings.Repeat("a", devicebridge.IdentifierMaxBytes+1)} {
		_, cfgErr := LoadLocalConfig(localEnv("1", "https://example.test", owner))
		_, pairErr := p.Pair(context.Background(), devicebridge.PairRequest{OwnerID: owner, Code: code(), Label: "Mac", Capabilities: []string{"notify_local"}})
		if (cfgErr == nil) != errors.Is(pairErr, identifierValid) {
			t.Fatalf("identifier disagreement %q", owner)
		}
	}
}

func TestLocalConfigWithoutOwnerBindsAtPairing(t *testing.T) {
	cfg, e := LoadLocalConfig(localEnv("1", "https://example.test", ""))
	if e != nil || !cfg.Enabled || cfg.Origin != "https://example.test" || cfg.OwnerID != "" {
		t.Fatalf("owner-less config = %+v, %v", cfg, e)
	}
	if _, e := LoadLocalConfig(localEnv("1", "", "")); e == nil {
		t.Fatal("accepted a missing origin")
	}
}

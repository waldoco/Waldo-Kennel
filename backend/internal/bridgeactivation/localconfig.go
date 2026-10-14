package bridgeactivation

import (
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"net/url"
	"strings"
)

type LocalConfig struct {
	Enabled         bool
	Origin, OwnerID string
}

// LoadLocalConfig reads trusted boot inputs only. OFF ignores every other value.
// The owner is optional: without it the bridge learns its owner from the first
// owner-scoped pairing code (see Controller.NewBound).
func LoadLocalConfig(getenv func(string) string) (LocalConfig, error) {
	if getenv("KENNEL_WALDO_BRIDGE_ENABLED") != "1" {
		return LocalConfig{}, nil
	}
	origin, owner := getenv("KENNEL_WALDO_ORIGIN"), getenv("KENNEL_WALDO_OWNER_ID")
	u, e := url.Parse(origin)
	if e != nil || !strings.HasPrefix(origin, "https://") || strings.ContainsAny(origin, "?#") || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.Path != "" || u.RawPath != "" || u.User != nil || u.Opaque != "" || (owner != "" && !validOwnerID(owner)) {
		return LocalConfig{}, ErrInvalid
	}
	return LocalConfig{Enabled: true, Origin: origin, OwnerID: owner}, nil
}
func validOwnerID(s string) bool {
	if len(s) == 0 || len(s) > devicebridge.IdentifierMaxBytes {
		return false
	}
	for i := range s {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

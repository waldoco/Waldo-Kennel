package ownercommand

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const AuthorizationScheme = "KennelOwner"

type Authority struct{ token, appRunID string }

// Authentication is the typed identity bound to a valid owner-command token.
type Authentication struct {
	Principal string
	AppRunID  string
}

func NewAuthority(token, appRunID string) *Authority {
	if token == "" || appRunID == "" {
		return nil
	}
	return &Authority{token: token, appRunID: appRunID}
}
func (a *Authority) Authenticate(header string) (Authentication, bool) {
	if a == nil {
		return Authentication{}, false
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || scheme != AuthorizationScheme || !TokenMatches(token, a.token) {
		return Authentication{}, false
	}
	return Authentication{Principal: "local-owner:" + a.appRunID, AppRunID: a.appRunID}, true
}
func Fingerprint(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

var ErrUnavailable = errors.New("trusted local-owner command authority is unavailable")

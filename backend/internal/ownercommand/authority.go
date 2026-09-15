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

func NewAuthority(token, appRunID string) *Authority {
	if token == "" || appRunID == "" {
		return nil
	}
	return &Authority{token: token, appRunID: appRunID}
}
func (a *Authority) Authenticate(header string) (string, bool) {
	if a == nil {
		return "", false
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || scheme != AuthorizationScheme || !TokenMatches(token, a.token) {
		return "", false
	}
	return "local-owner:" + a.appRunID, true
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

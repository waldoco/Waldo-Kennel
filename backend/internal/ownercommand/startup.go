// Package ownercommand owns the app-run capability used only by Electron main
// to create durable owner decisions. The bearer never enters renderer JS.
package ownercommand

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const StartupSecretsStdinEnv = "KENNEL_STARTUP_SECRETS_STDIN" //nolint:gosec // marker, not a credential

type StartupSecrets struct {
	BrowserRuntimeToken string `json:"browserRuntimeToken"`
	OwnerCommandToken   string `json:"ownerCommandToken"`
	AppRunID            string `json:"appRunId"`
}

func ReadStartupSecrets(r io.Reader) (StartupSecrets, error) {
	b, err := io.ReadAll(io.LimitReader(r, 2049))
	if err != nil {
		return StartupSecrets{}, fmt.Errorf("read startup secrets: %w", err)
	}
	if len(b) > 2048 {
		return StartupSecrets{}, errors.New("startup secret handoff was too long")
	}
	line := strings.TrimSpace(string(b))
	if line == "" {
		return StartupSecrets{}, errors.New("startup secret handoff was empty")
	}
	raw, err := base64.RawURLEncoding.DecodeString(line)
	if err != nil {
		return StartupSecrets{}, errors.New("startup secret handoff was invalid")
	}
	var s StartupSecrets
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		return StartupSecrets{}, errors.New("startup secret envelope was invalid")
	}
	if !validToken(s.BrowserRuntimeToken) || !validToken(s.OwnerCommandToken) || !validAppRunID(s.AppRunID) {
		return StartupSecrets{}, errors.New("startup secret envelope was incomplete")
	}
	return s, nil
}
func validToken(token string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(raw) == 32
}

func validAppRunID(id string) bool {
	if !strings.HasPrefix(id, "apprun-") || len(id) < len("apprun-")+1 || len(id) > 128 {
		return false
	}
	for _, r := range id[len("apprun-"):] {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}

func TokenMatches(got, want string) bool {
	return got != "" && len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

package ownercommand

import (
	"encoding/base64"
	"strings"
	"testing"
)

const (
	browserToken = "YmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmI"
	ownerToken   = "b29vb29vb29vb29vb29vb29vb29vb29vb29vb29vb28"
)

func envelope(raw string) string { return base64.RawURLEncoding.EncodeToString([]byte(raw)) + "\n" }

func TestStartupSecretsAndAuthority(t *testing.T) {
	s, err := ReadStartupSecrets(strings.NewReader(envelope(`{"browserRuntimeToken":"` + browserToken + `","ownerCommandToken":"` + ownerToken + `","appRunId":"apprun-1"}`)))
	if err != nil {
		t.Fatal(err)
	}
	a := NewAuthority(s.OwnerCommandToken, s.AppRunID)
	if authentication, ok := a.Authenticate("KennelOwner " + s.OwnerCommandToken); !ok || authentication.Principal != "local-owner:apprun-1" || authentication.AppRunID != "apprun-1" {
		t.Fatalf("auth=(%+v,%v)", authentication, ok)
	}
	if _, ok := a.Authenticate("KennelOwner wrong"); ok {
		t.Fatal("accepted wrong token")
	}
}

func TestStartupSecretsRejectMalformedOrExpandedEnvelope(t *testing.T) {
	valid := `{"browserRuntimeToken":"` + browserToken + `","ownerCommandToken":"` + ownerToken + `","appRunId":"apprun-1"}`
	for _, tc := range []struct{ name, input string }{
		{"empty", ""},
		{"too-long", strings.Repeat("x", 2049)},
		{"unknown-field", envelope(strings.TrimSuffix(valid, "}") + `,"extra":true}`)},
		{"trailing-json", envelope(valid + `{}`)},
		{"short-token", envelope(strings.Replace(valid, ownerToken, "short", 1))},
		{"noncanonical-token", envelope(strings.Replace(valid, ownerToken, strings.Repeat("*", 43), 1))},
		{"invalid-app-run", envelope(strings.Replace(valid, "apprun-1", "apprun-../../x", 1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ReadStartupSecrets(strings.NewReader(tc.input)); err == nil {
				t.Fatal("accepted invalid envelope")
			}
		})
	}
}

func TestAppRunCapabilityCannotCrossRuns(t *testing.T) {
	first := NewAuthority(ownerToken, "apprun-first")
	second := NewAuthority(browserToken, "apprun-second")
	if _, ok := second.Authenticate("KennelOwner " + ownerToken); ok {
		t.Fatal("old app-run capability authenticated to new app run")
	}
	if _, ok := first.Authenticate("KennelOwner " + browserToken); ok {
		t.Fatal("new app-run capability authenticated to old app run")
	}
}

func TestStartupSecretsUnavailableStdinFailsClosed(t *testing.T) {
	if _, err := ReadStartupSecrets(strings.NewReader("")); err == nil {
		t.Fatal("unavailable stdin was accepted")
	}
}

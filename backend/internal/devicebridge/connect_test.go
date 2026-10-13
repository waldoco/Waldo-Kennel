package devicebridge

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"
)

func TestSignedConnectBindsExactQueryAndDevice(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	var nonce [NonceBytes]byte
	u, headers, err := SignedConnect("https://waldo.example:8443", []string{ClassMachineStateQuery, ClassNotifyLocal}, key, time.Unix(1790200800, 0), nonce, "dev_1")
	if err != nil {
		t.Fatal(err)
	}
	want := "wss://waldo.example:8443/devices/connect?contract_version=0.3.0&declared_capabilities=machine_state_query,notify_local"
	if u != want || headers.Get("X-Waldo-Device-Id") != "dev_1" {
		t.Fatalf("connect = %s, %v", u, headers)
	}
	base, err := HTTPBaseString(time.Unix(1790200800, 0), nonce, "GET", "/devices/connect?contract_version=0.3.0&declared_capabilities=machine_state_query,notify_local", nil)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(headers.Get("X-Waldo-Signature"))
	if err != nil || !ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(base), sig) {
		t.Fatal("connect signature did not bind query")
	}
	for _, origin := range []string{"http://waldo.example", "https://waldo.example/path", "https://waldo.example?x=1", "https://user@waldo.example"} {
		if _, _, err := SignedConnect(origin, []string{ClassMachineStateQuery}, key, time.Unix(1790200800, 0), nonce, "dev_1"); err != ErrInvalidSigningInput {
			t.Fatalf("accepted origin %q: %v", origin, err)
		}
	}
}

package devicebridge

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"
)

func TestRedeemSixFieldBodyAndStrictParse(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	key := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	body, err := RedeemBody("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", key, "Test Mac", []string{ClassMachineStateQuery, ClassNotifyLocal})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"code":"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8","contract_version":"0.3.0","declared_capabilities":["machine_state_query","notify_local"],"device_pubkey":"A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg","label":"Test Mac","session_consent":false}`
	if string(body) != want {
		t.Fatalf("body = %s", body)
	}
	got, err := ParseRedeemBody(body)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatal("raw body key parsing failed")
	}
	for _, raw := range []string{
		string(bytes.ReplaceAll(body, []byte(`"label":"Test Mac"`), []byte(`"label":"Test Mac","device_id":"dev_1"`))),
		string(bytes.ReplaceAll(body, []byte(`"contract_version":"0.3.0"`), []byte(`"contract_version":"0.3.0","contract_version":"0.3.0"`))),
		string(bytes.ReplaceAll(body, []byte(`"machine_state_query","notify_local"`), []byte(`"notify_local","machine_state_query"`))),
	} {
		if _, err := ParseRedeemBody([]byte(raw)); !errors.Is(err, ErrPairingInvalid) {
			t.Fatalf("accepted %s: %v", raw, err)
		}
	}
}

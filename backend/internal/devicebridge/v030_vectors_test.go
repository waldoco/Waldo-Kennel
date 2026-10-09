package devicebridge

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestV030GoldenVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/contracts/waldo-device-bridge-v0.3.0.vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		PublicKey string `json:"public_key"`
		Vectors   []struct{ Name, Surface, Canonical, Digest, Base, Signature, Wire string }
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	public, err := base64.RawURLEncoding.DecodeString(fixture.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range fixture.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			digest := sha256.Sum256([]byte(v.Canonical))
			if hex.EncodeToString(digest[:]) != v.Digest {
				t.Fatal("digest mismatch")
			}
			if v.Canonical != "" {
				object, err := parseJSONObject([]byte(v.Canonical))
				if err != nil {
					t.Fatal(err)
				}
				canon, err := marshalCanonical(object)
				if err != nil || string(canon) != v.Canonical {
					t.Fatal("canonical bytes mismatch")
				}
			}
			if v.Signature != "" {
				signature, err := base64.RawURLEncoding.DecodeString(v.Signature)
				if err != nil || !ed25519.Verify(public, []byte(v.Base), signature) {
					t.Fatal("signature mismatch")
				}
				if ed25519.Verify(public, []byte(v.Base+"x"), signature) {
					t.Fatal("tampered base accepted")
				}
			}
			if len([]byte(v.Wire)) > 8192 {
				t.Fatal("signed frame exceeds ceiling")
			}
		})
	}
}

package devicebridge

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"
)

// Fixtures are literal normative Appendix A bytes from the hash-bound v0.2.3 contract.
func TestS3ContractAppendixA(t *testing.T) {
	seed, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	key := ed25519.NewKeyFromSeed(seed)
	var nonce [NonceBytes]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}
	now := time.Unix(1790200800, 0)
	for _, tc := range []struct{ name, body, digest, base, signature string }{
		{"0. Redeem HTTP signing", "{\"code\":\"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8\",\"contract_version\":\"0.2.3\",\"declared_capabilities\":[\"machine_state_query\",\"notify_local\"],\"device_pubkey\":\"A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg\",\"label\":\"Test Mac\"}", "90d6daa8d89edfae3a13f3584eb36cf04155539bf5c2c2ccf2f73de122ba6175", "1790200800\nAAECAwQFBgcICQoLDA0ODw\nPOST\n/devices/redeem\n90d6daa8d89edfae3a13f3584eb36cf04155539bf5c2c2ccf2f73de122ba6175", "VlVfMNC0TTbHKXsDsjGjWyBHnaVAMSud0b_DlEukL78UzWBdJhs_knA09uH0NSzsuFbWpLSU-xYhqKEsFGPbDw"},
		{"1. Accepted ack", "{\"command_id\":\"cmd_1\",\"contract_version\":\"0.2.3\",\"device_id\":\"dev_1\",\"idempotency_key\":\"idem_1\",\"message_id\":\"01ARZ3NDEKTSV4RRFFQ69G5FAW\",\"nonce\":\"AAECAwQFBgcICQoLDA0ODw\",\"owner_id\":\"owner_1\",\"payload\":{\"state\":\"accepted\"},\"revision\":1,\"timestamp\":1790200800,\"type\":\"ack\"}", "7650d9debd01414a3cc82e11e117a8faad12d59de977d37010c49bf91d66eb4f", "1790200800\nack\n01ARZ3NDEKTSV4RRFFQ69G5FAW\nAAECAwQFBgcICQoLDA0ODw\n7650d9debd01414a3cc82e11e117a8faad12d59de977d37010c49bf91d66eb4f", "c2GwY1y5gWNCSPdf9Ufb8IIRgMGx_t9wpOi02pbKlHcC-QdOb7i7R5wp-cwmk-u2Lb-mO6ah2hugIFYflTQNBw"},
		{"2. Answered result", "{\"command_id\":\"cmd_1\",\"contract_version\":\"0.2.3\",\"device_id\":\"dev_1\",\"idempotency_key\":\"idem_1\",\"message_id\":\"01ARZ3NDEKTSV4RRFFQ69G5FAX\",\"nonce\":\"AAECAwQFBgcICQoLDA0ODw\",\"owner_id\":\"owner_1\",\"payload\":{\"answer\":{\"query_id\":\"query_1\",\"query_kind\":\"session_status\",\"state\":\"idle\"},\"status\":\"answered\"},\"revision\":1,\"timestamp\":1790200800,\"type\":\"result\"}", "1a0b0591d76e18bcf22772e2b9bdb68d63da0e55dc671ced8397941ec657210c", "1790200800\nresult\n01ARZ3NDEKTSV4RRFFQ69G5FAX\nAAECAwQFBgcICQoLDA0ODw\n1a0b0591d76e18bcf22772e2b9bdb68d63da0e55dc671ced8397941ec657210c", "Ur-XO9PNQy3Kszxe7A_SJp7GssLVSKp6wkZUqEZAYHkKGon0IqbLhA-vhD1ya_o-HQQkb2wOB9Sgyyd8PWpDAA"},
		{"3. Heartbeat", "{\"contract_version\":\"0.2.3\",\"device_id\":\"dev_1\",\"message_id\":\"01ARZ3NDEKTSV4RRFFQ69G5FAZ\",\"nonce\":\"AAECAwQFBgcICQoLDA0ODw\",\"owner_id\":\"owner_1\",\"payload\":{\"declared_capabilities\":[\"machine_state_query\",\"notify_local\"],\"outbox_depth\":0},\"timestamp\":1790200800,\"type\":\"heartbeat\"}", "431e3fa8304bcdca049b84a0545cd82a6b7ed1f9596f18e227adc8b9b42c931f", "1790200800\nheartbeat\n01ARZ3NDEKTSV4RRFFQ69G5FAZ\nAAECAwQFBgcICQoLDA0ODw\n431e3fa8304bcdca049b84a0545cd82a6b7ed1f9596f18e227adc8b9b42c931f", "1DzrSOApCr5iU_sTuJFozlndCY-A1EhY0XVtYGf07nP_OoW_Q8bI1a9OakAvlfJBBfBQb1NDevyyH4Q3tbWzDA"},
		{"4. Redeem HTTP Unicode/HTML-escape fixture", "{\"code\":\"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8\",\"contract_version\":\"0.2.3\",\"declared_capabilities\":[\"machine_state_query\"],\"device_pubkey\":\"A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg\",\"label\":\"Café <Go> 𐀀\"}", "cc34011527d0ae10bad46dcc1977f79898a14b2b4b71ce25f3d7b1280bb6e00a", "1790200800\nAAECAwQFBgcICQoLDA0ODw\nPOST\n/devices/redeem\ncc34011527d0ae10bad46dcc1977f79898a14b2b4b71ce25f3d7b1280bb6e00a", "LN1dFNztIWTdRKIPPqWtUolXKHhmGygGapStEuvsxIftaLrwwGfo9V_cED3D5T1fQufFL4cAYsKjoBTBe4PhAA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := parseJSONObject([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := marshalCanonical(parsed)
			if err != nil || !bytes.Equal(canonical, []byte(tc.body)) {
				t.Fatal("canonical bytes differ from contract")
			}
			hash := sha256.Sum256(canonical)
			if hex.EncodeToString(hash[:]) != tc.digest {
				t.Fatal("digest differs from contract")
			}
			var base string
			if parsed["type"] == nil {
				// Historical canonical bytes and signatures remain immutable. The
				// current builder intentionally emits the new six-field body.
				pub, parseErr := ParseRedeemBody(canonical)
				if parseErr != nil || !bytes.Equal(pub, key.Public().(ed25519.PublicKey)) {
					t.Fatal("legacy redeem parser differs", parseErr)
				}
				base, err = HTTPBaseString(now, nonce, "POST", RedeemPath, canonical)
			} else {
				base, err = frameBaseString(parsed)
				delete(parsed, "timestamp")
				delete(parsed, "nonce")
				unsigned, e := marshalCanonical(parsed)
				if e != nil {
					t.Fatal(e)
				}
				wire, e := SignFrame(unsigned, key, now, nonce)
				if e != nil {
					t.Fatal(e)
				}
				signed, e := parseJSONObject(wire)
				if e != nil {
					t.Fatal(e)
				}
				if signed["signature"] != tc.signature {
					t.Error("frame signature differs from contract")
				}
				if _, e = VerifyFrame(wire, key.Public().(ed25519.PublicKey), now); e != nil {
					t.Fatal(e)
				}
			}
			if err != nil || base != tc.base {
				t.Fatal("base string differs from contract")
			}
			sig, e := base64.RawURLEncoding.DecodeString(tc.signature)
			if e != nil || !ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(base), sig) {
				t.Fatal("normative signature does not verify")
			}
			if !bytes.Equal(ed25519.Sign(key, []byte(base)), sig) {
				t.Fatal("signature bytes differ from contract")
			}
		})
	}
}

func TestS3VersionAndStrictPairingCode(t *testing.T) {
	if ContractVersion != "0.3.0" {
		t.Errorf("version = %s, want 0.3.0", ContractVersion)
	}
	code := "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	if !validPairingCode(code) {
		t.Error("valid normative fixture rejected")
	}
	for _, bad := range []string{"TEST-CODE", code + "=", code[:42], "!" + code[1:], code[:42] + "9"} {
		if validPairingCode(bad) {
			t.Error("accepted invalid/noncanonical pairing code")
		}
	}
}

func TestS3AppendixASerializerFixtureIsNotAFrame(t *testing.T) {
	raw, err := hex.DecodeString("7b2265cc81223a22636f6d62696e696e67222c2274657874223a223c3e262f5c6e5c745c225c5c222c22c3a9223a22636166c3a9222c22ee8080223a2270726976617465222c22f0908080223a22706c616e65227d")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := marshalCanonical(parsed)
	if err != nil || !bytes.Equal(raw, canonical) {
		t.Fatal("serializer fixture 5 bytes differ")
	}
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	if _, err := SignFrame(raw, key, time.Unix(1790200800, 0), [NonceBytes]byte{}); err == nil {
		t.Fatal("serializer-only fixture was accepted as protocol frame")
	}
}

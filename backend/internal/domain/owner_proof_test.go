package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOwnerCommandClassesMaterialityAndTransportMapping(t *testing.T) {
	classes := []struct {
		c        OwnerCommandClass
		material bool
	}{{OwnerCommandTurn, false}, {OwnerCommandSteer, false}, {OwnerCommandAnswer, false}, {OwnerCommandInterrupt, false}, {OwnerCommandCancel, false}, {OwnerCommandReplace, true}, {OwnerCommandApproval, true}, {OwnerCommandAccept, true}}
	for _, tc := range classes {
		if !tc.c.Valid() || tc.c.Material() != tc.material {
			t.Fatalf("class %s", tc.c)
		}
		mapped, ok := OwnerCommandClassForTransport(HarnessCapabilityClass(tc.c))
		if !ok || mapped != tc.c {
			t.Fatalf("map %s", tc.c)
		}
	}
	if _, ok := OwnerCommandClassForTransport("unknown"); ok {
		t.Fatal("unknown mapped")
	}
}
func TestOwnerProofNoVerifierInStringOrJSON(t *testing.T) {
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	p := OwnerProof{ID: "p", Verifier: DigestSHA256([]byte("secret")), AppRunID: "run", MissionID: "mission", ContentDigest: DigestSHA256([]byte("content")), TargetDigest: DigestSHA256([]byte("target")), Class: OwnerCommandTurn, ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	if strings.Contains(p.String(), p.Verifier.String()) {
		t.Fatal("String leak")
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), p.Verifier.String()) || strings.Contains(string(raw), "Verifier") {
		t.Fatalf("JSON leak %s", raw)
	}
}

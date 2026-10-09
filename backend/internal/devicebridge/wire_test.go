package devicebridge

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"
)

const commandFixture = `{"class":"machine_state_query","command_id":"cmd_1","contract_version":"0.3.0","device_id":"dev_1","expires_at":1790700000,"idempotency_key":"idem_1","message_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","owner_id":"owner_1","payload":{"query_id":"query_1","query_kind":"session_status"},"revision":1,"type":"command"}`
const receiptFixture = `{"command_id":"cmd_1","contract_version":"0.3.0","device_id":"dev_1","idempotency_key":"idem_1","message_id":"01ARZ3NDEKTSV4RRFFQ69G5FAY","owner_id":"owner_1","payload":{"received_at":1790200802,"result_message_id":"01ARZ3NDEKTSV4RRFFQ69G5FAX"},"revision":1,"type":"receipt"}`
const resultFixture = `{"command_id":"cmd_1","contract_version":"0.3.0","device_id":"dev_1","idempotency_key":"idem_1","message_id":"01ARZ3NDEKTSV4RRFFQ69G5FAX","owner_id":"owner_1","payload":{"answer":{"query_id":"query_1","query_kind":"session_status","state":"idle"},"status":"answered"},"revision":1,"type":"result"}`

func TestClosedBackendFrameShapes(t *testing.T) {
	for _, raw := range []string{commandFixture, receiptFixture} {
		if _, err := ParseBackendFrame([]byte(raw)); err != nil {
			t.Fatalf("rejected fixture: %v", err)
		}
	}
	wrongVersion := bytes.ReplaceAll([]byte(commandFixture), []byte(`"contract_version":"0.3.0"`), []byte(`"contract_version":"0.2.1"`))
	if _, err := ParseBackendFrame(wrongVersion); !errors.Is(err, ErrContractVersionMismatch) {
		t.Fatalf("wrong version error = %v", err)
	}
	for _, raw := range []string{
		string(bytes.ReplaceAll([]byte(commandFixture), []byte(`"revision":1`), []byte(`"revision":2`))),
		string(bytes.ReplaceAll([]byte(commandFixture), []byte(`"type":"command"`), []byte(`"type":"command","extra":1`))),
		string(bytes.ReplaceAll([]byte(commandFixture), []byte(`"query_kind":"session_status"`), []byte(`"query_kind":"unknown_kind"`))),
		string(bytes.ReplaceAll([]byte(receiptFixture), []byte(`"result_message_id":"01ARZ3NDEKTSV4RRFFQ69G5FAX"`), []byte(`"result_message_id":"bad"`))),
	} {
		if _, err := ParseBackendFrame([]byte(raw)); !errors.Is(err, ErrInvalidFrameShape) {
			t.Fatalf("accepted invalid frame %s: %v", raw, err)
		}
	}
}

func TestResultResendRetainsLogicalFingerprint(t *testing.T) {
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{4}, ed25519.SeedSize))
	var a, b [NonceBytes]byte
	b[0] = 1
	first, err := SignFrame([]byte(resultFixture), key, time.Unix(1790200800, 0), a)
	if err != nil {
		t.Fatal(err)
	}
	resent, err := ResignFrame(first, key, time.Unix(1790200801, 0), b)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, resent) {
		t.Fatal("resend did not refresh signing fields")
	}
	firstFrame, _ := parseJSONObject(first)
	resentFrame, _ := parseJSONObject(resent)
	f1, err := LogicalFingerprint(firstFrame)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := LogicalFingerprint(resentFrame)
	if err != nil || f1 != f2 {
		t.Fatal("logical frame changed on resend")
	}
	if _, err := VerifyFrame(resent, key.Public().(ed25519.PublicKey), time.Unix(1790200801, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := ResignFrame(first, key, time.Unix(1790200801, 0), a); !errors.Is(err, ErrInvalidSigningInput) {
		t.Fatal("reused nonce accepted")
	}
	command, err := ParseBackendFrame([]byte(commandFixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateResultForCommand(resentFrame, command); err != nil {
		t.Fatal(err)
	}
	receipt, err := ParseBackendFrame([]byte(receiptFixture))
	if err != nil || ValidateReceiptForResult(receipt, resentFrame) != nil {
		t.Fatal("matching receipt rejected")
	}
	receipt["owner_id"] = "other_owner"
	if err := ValidateReceiptForResult(receipt, resentFrame); !errors.Is(err, ErrInvalidFrameShape) {
		t.Fatal("cross-owner receipt accepted")
	}
}

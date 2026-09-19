package domain

import (
	"strings"
	"testing"
	"time"
)

func TestAttemptCustodyFenceValidate(t *testing.T) {
	valid := AttemptCustodyFence{
		AttemptID: "att-11111111-1111-1111-1111-111111111111",
		SessionID: "ses-22222222-2222-2222-2222-222222222222",
		FencedAt:  time.Date(2026, 9, 20, 2, 0, 0, 0, time.UTC),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid fence refused: %v", err)
	}
	missing := valid
	missing.AttemptID = ""
	if err := missing.Validate(); err == nil {
		t.Fatal("fence without attempt id accepted")
	}
	missing = valid
	missing.SessionID = strings.Repeat(" ", 4)
	if err := missing.Validate(); err == nil {
		t.Fatal("fence without session id accepted")
	}
	missing = valid
	missing.FencedAt = time.Time{}
	if err := missing.Validate(); err == nil {
		t.Fatal("fence without fenced-at accepted")
	}
}

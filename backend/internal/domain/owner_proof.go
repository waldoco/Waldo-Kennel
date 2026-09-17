package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type OwnerProofID string
type OwnerCommandClass string

const (
	OwnerCommandTurn      OwnerCommandClass = "turn"
	OwnerCommandSteer     OwnerCommandClass = "steer"
	OwnerCommandAnswer    OwnerCommandClass = "answer"
	OwnerCommandInterrupt OwnerCommandClass = "interrupt"
	OwnerCommandCancel    OwnerCommandClass = "cancel"
	OwnerCommandReplace   OwnerCommandClass = "replace"
	OwnerCommandApproval  OwnerCommandClass = "approval"
	OwnerCommandAccept    OwnerCommandClass = "accept"
)

func (c OwnerCommandClass) Valid() bool {
	switch c {
	case OwnerCommandTurn, OwnerCommandSteer, OwnerCommandAnswer, OwnerCommandInterrupt, OwnerCommandCancel, OwnerCommandReplace, OwnerCommandApproval, OwnerCommandAccept:
		return true
	}
	return false
}
func (c OwnerCommandClass) Material() bool {
	return c == OwnerCommandReplace || c == OwnerCommandApproval || c == OwnerCommandAccept
}

type OwnerProof struct {
	ID                   OwnerProofID
	Verifier             SHA256Digest `json:"-"`
	AppRunID, MissionID  string
	ContentDigest        SHA256Digest
	TargetDigest         SHA256Digest
	Class                OwnerCommandClass
	ConfirmationRef      string
	ExpiresAt, CreatedAt time.Time
	ConsumedAt           *time.Time
}

func (p OwnerProof) String() string { return fmt.Sprintf("OwnerProof(%s,class=%s)", p.ID, p.Class) }
func (p OwnerProof) Validate() error {
	if strings.TrimSpace(string(p.ID)) == "" || !p.Verifier.Valid() || strings.TrimSpace(p.AppRunID) == "" || strings.TrimSpace(p.MissionID) == "" || !p.ContentDigest.Valid() || !p.TargetDigest.Valid() || !p.Class.Valid() || p.CreatedAt.IsZero() || !p.ExpiresAt.After(p.CreatedAt) || (p.ConsumedAt != nil && p.ConsumedAt.Before(p.CreatedAt)) || (p.Class.Material() && strings.TrimSpace(p.ConfirmationRef) == "") || (!p.Class.Material() && p.ConfirmationRef != "") {
		return ErrOwnerProofInvalid
	}
	return nil
}

var (
	ErrOwnerProofInvalid        = errors.New("invalid owner proof")
	ErrOwnerProofConflict       = errors.New("owner proof conflict")
	ErrOwnerProofAuthentication = errors.New("owner proof authentication failed")
)

// OwnerCommandClassForTransport is the only mapping between the closed S3.1
// transport vocabulary and owner-proof classes. It does not grant either kind
// of authority; callers must authenticate transport and owner proof separately.
func OwnerCommandClassForTransport(class HarnessCapabilityClass) (OwnerCommandClass, bool) {
	mapped := OwnerCommandClass(class)
	return mapped, mapped.Valid()
}

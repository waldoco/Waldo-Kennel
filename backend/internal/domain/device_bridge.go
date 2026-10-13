package domain

import (
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgecontract"
	"strings"
	"time"
)

// Device bridge (ADR 0018, contract v0.1): durable state for the paired-device
// principal and the outbound Waldo bridge. This is infrastructure state, not
// work-state: these records never enter change_log and carry no owner
// authority. Pairing authenticates transport only.

type DeviceBridgeDeviceID string

type DeviceBridgeState string

const (
	DeviceBridgeStatePairing DeviceBridgeState = "pairing"
	DeviceBridgeStatePaired  DeviceBridgeState = "paired"
	DeviceBridgeStateRevoked DeviceBridgeState = "revoked"
)

func (s DeviceBridgeState) Valid() bool {
	switch s {
	case DeviceBridgeStatePairing, DeviceBridgeStatePaired, DeviceBridgeStateRevoked:
		return true
	}
	return false
}

// DeviceBridgeMessageClass is the frozen contract v0.1 vocabulary. No other
// class exists; unknown classes reject at the domain boundary.
type DeviceBridgeMessageClass string

const (
	DeviceBridgeClassMachineStateQuery DeviceBridgeMessageClass = "machine_state_query"
	DeviceBridgeClassNotifyLocal       DeviceBridgeMessageClass = "notify_local"
	DeviceBridgeClassShellExec         DeviceBridgeMessageClass = "shell_exec"
	DeviceBridgeClassSessionQuery      DeviceBridgeMessageClass = "session_query"
	DeviceBridgeClassSessionControl    DeviceBridgeMessageClass = "session_control"
	DeviceBridgeClassSessionEvent      DeviceBridgeMessageClass = "session_event"
	DeviceBridgeClassAck               DeviceBridgeMessageClass = "ack"
	DeviceBridgeClassResult            DeviceBridgeMessageClass = "result"
	DeviceBridgeClassReceipt           DeviceBridgeMessageClass = "receipt"
)

func (c DeviceBridgeMessageClass) Valid() bool {
	switch c {
	case DeviceBridgeClassShellExec, DeviceBridgeClassSessionQuery, DeviceBridgeClassSessionControl, DeviceBridgeClassSessionEvent, DeviceBridgeClassMachineStateQuery, DeviceBridgeClassNotifyLocal, DeviceBridgeClassAck, DeviceBridgeClassResult, DeviceBridgeClassReceipt:
		return true
	}
	return false
}

// InboundCommand reports whether the class may arrive from the backend as a
// journaled command in v0.1. Receipts are protocol traffic handled against the
// outbox directly; they are not journaled commands.
func (c DeviceBridgeMessageClass) InboundCommand() bool {
	return c == DeviceBridgeClassMachineStateQuery || c == DeviceBridgeClassNotifyLocal || c == DeviceBridgeClassShellExec || c == DeviceBridgeClassSessionQuery || c == DeviceBridgeClassSessionControl
}

// OutboxMessage reports whether the class may be spooled outbound in v0.1.
func (c DeviceBridgeMessageClass) OutboxMessage() bool {
	return c == DeviceBridgeClassAck || c == DeviceBridgeClassResult || c == DeviceBridgeClassSessionEvent
}

// DeviceBridgeDevice is the durable record of one paired device. PublicKey is
// the device's ed25519 public key. KeyCustodyRef is an opaque reference to the
// local secret custody of the private key (secretstore); private key material
// never crosses this boundary, never enters SQLite, and never appears in logs.
type DeviceBridgeDevice struct {
	DeviceID          DeviceBridgeDeviceID
	OwnerID           string
	Label             string
	PublicKey         string
	KeyCustodyRef     string
	ContractVersion   string
	CapabilityClasses []string
	State             DeviceBridgeState
	PairedAt          *time.Time
	RevokedAt         *time.Time
	LastSeenAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (d DeviceBridgeDevice) Validate() error {
	if strings.TrimSpace(string(d.DeviceID)) == "" || strings.TrimSpace(d.OwnerID) == "" || strings.TrimSpace(d.Label) == "" || strings.TrimSpace(d.PublicKey) == "" || strings.TrimSpace(d.KeyCustodyRef) == "" || strings.TrimSpace(d.ContractVersion) == "" || len(d.CapabilityClasses) == 0 || !d.State.Valid() || d.CreatedAt.IsZero() || d.UpdatedAt.Before(d.CreatedAt.UTC()) {
		return ErrDeviceBridgeInvalid
	}
	if !bridgecontract.ValidCapabilities(d.CapabilityClasses) {
		return ErrDeviceBridgeInvalid
	}
	for _, c := range d.CapabilityClasses {
		if !DeviceBridgeMessageClass(c).Valid() {
			return ErrDeviceBridgeInvalid
		}
	}
	switch d.State {
	case DeviceBridgeStatePairing:
		if d.PairedAt != nil || d.RevokedAt != nil {
			return ErrDeviceBridgeInvalid
		}
	case DeviceBridgeStatePaired:
		if d.PairedAt == nil || d.RevokedAt != nil {
			return ErrDeviceBridgeInvalid
		}
	case DeviceBridgeStateRevoked:
		if d.RevokedAt == nil {
			return ErrDeviceBridgeInvalid
		}
	}
	return nil
}

type DeviceBridgeOutboxState string

const (
	DeviceBridgeOutboxPending   DeviceBridgeOutboxState = "pending"
	DeviceBridgeOutboxSent      DeviceBridgeOutboxState = "sent"
	DeviceBridgeOutboxReceipted DeviceBridgeOutboxState = "receipted"
)

func (s DeviceBridgeOutboxState) Valid() bool {
	switch s {
	case DeviceBridgeOutboxPending, DeviceBridgeOutboxSent, DeviceBridgeOutboxReceipted:
		return true
	}
	return false
}

// DeviceBridgeOutboxMessage is one spooled outbound message. Seq is the
// store-assigned durable cursor: drain order is seq order, and an entry clears
// only on backend receipt. (command_id, revision) is unique: a redelivered
// logical message never spools twice.
type DeviceBridgeOutboxMessage struct {
	Seq         int64
	MessageID   string
	CommandID   string
	Revision    int64
	Class       DeviceBridgeMessageClass
	Payload     string
	State       DeviceBridgeOutboxState
	CreatedAt   time.Time
	SentAt      *time.Time
	ReceiptedAt *time.Time
}

func (m DeviceBridgeOutboxMessage) Validate() error {
	if strings.TrimSpace(m.MessageID) == "" || strings.TrimSpace(m.CommandID) == "" || m.Revision < 1 || !m.Class.Valid() || !m.Class.OutboxMessage() || strings.TrimSpace(m.Payload) == "" || !m.State.Valid() || m.CreatedAt.IsZero() {
		return ErrDeviceBridgeInvalid
	}
	if m.State == DeviceBridgeOutboxPending && (m.SentAt != nil || m.ReceiptedAt != nil) {
		return ErrDeviceBridgeInvalid
	}
	if m.State == DeviceBridgeOutboxSent && m.ReceiptedAt != nil {
		return ErrDeviceBridgeInvalid
	}
	if m.State == DeviceBridgeOutboxReceipted && m.ReceiptedAt == nil {
		return ErrDeviceBridgeInvalid
	}
	return nil
}

type DeviceBridgeInboxState string

const (
	DeviceBridgeInboxReceived DeviceBridgeInboxState = "received"
	DeviceBridgeInboxAcked    DeviceBridgeInboxState = "acked"
	DeviceBridgeInboxRejected DeviceBridgeInboxState = "rejected"
	DeviceBridgeInboxExpired  DeviceBridgeInboxState = "expired"
	DeviceBridgeInboxResulted DeviceBridgeInboxState = "resulted"
)

func (s DeviceBridgeInboxState) Valid() bool {
	switch s {
	case DeviceBridgeInboxReceived, DeviceBridgeInboxAcked, DeviceBridgeInboxRejected, DeviceBridgeInboxExpired, DeviceBridgeInboxResulted:
		return true
	}
	return false
}

// DeviceBridgeInboxCommand is the replay-safe journal of one inbound command.
// IdempotencyKey is unique: redelivery of the same logical command maps to the
// existing row (one effect), and a redelivery whose payload fingerprint
// differs is a conflict, never a silent overwrite.
type DeviceBridgeInboxCommand struct {
	CommandID          string
	Revision           int64
	IdempotencyKey     string
	PayloadFingerprint SHA256Digest
	Class              DeviceBridgeMessageClass
	Payload            string
	State              DeviceBridgeInboxState
	RejectReason       string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (c DeviceBridgeInboxCommand) Validate() error {
	if strings.TrimSpace(c.CommandID) == "" || c.Revision < 1 || strings.TrimSpace(c.IdempotencyKey) == "" || !c.PayloadFingerprint.Valid() || !c.Class.Valid() || !c.Class.InboundCommand() || strings.TrimSpace(c.Payload) == "" || !c.State.Valid() || c.CreatedAt.IsZero() || c.UpdatedAt.Before(c.CreatedAt.UTC()) {
		return ErrDeviceBridgeInvalid
	}
	if (c.State == DeviceBridgeInboxRejected && strings.TrimSpace(c.RejectReason) == "") ||
		(c.State != DeviceBridgeInboxRejected && c.RejectReason != "") {
		return ErrDeviceBridgeInvalid
	}
	return nil
}

var (
	ErrDeviceBridgeInvalid  = errors.New("invalid device bridge record")
	ErrDeviceBridgeConflict = errors.New("device bridge conflict")
)

package devicebridge

import (
	"encoding/base64"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgecontract"
	"strings"
)

// Contract constants are collected here so jointly reviewed proposed limits
// and labels can be changed without hunting through transport code.
const (
	ContractVersion           = bridgecontract.Version
	ApprovalTTLSeconds        = bridgecontract.ApprovalTTLSeconds
	ExecutionDefaultSeconds   = bridgecontract.ExecutionDefaultSeconds
	ExecutionMaxSeconds       = bridgecontract.ExecutionMaxSeconds
	OutputTailBytes           = bridgecontract.OutputTailBytes
	RedeemPath                = bridgecontract.RedeemPath
	ConnectPath               = bridgecontract.ConnectPath
	ConnectVersionQueryPrefix = bridgecontract.ConnectVersionQueryPrefix
	ConnectCapabilitiesQuery  = bridgecontract.ConnectCapabilitiesQuery
	TypeCommand               = bridgecontract.TypeCommand
	TypeAck                   = bridgecontract.TypeAck
	TypeResult                = bridgecontract.TypeResult
	TypeReceipt               = bridgecontract.TypeReceipt
	TypeHeartbeat             = bridgecontract.TypeHeartbeat
	TypeSessionEvent          = bridgecontract.TypeSessionEvent
	ClassShellExec            = bridgecontract.ClassShellExec
	ClassSessionQuery         = bridgecontract.ClassSessionQuery
	ClassSessionControl       = bridgecontract.ClassSessionControl
	ResultCompleted           = bridgecontract.ResultCompleted
	ResultDenied              = bridgecontract.ResultDenied
	ResultExpired             = bridgecontract.ResultExpired
	ResultOutcomeUnknown      = bridgecontract.ResultOutcomeUnknown
	ClassMachineStateQuery    = bridgecontract.ClassMachineStateQuery
	ClassNotifyLocal          = bridgecontract.ClassNotifyLocal
	AckAccepted               = bridgecontract.AckAccepted
	AckRejected               = bridgecontract.AckRejected
	AckExpired                = bridgecontract.AckExpired
	ResultAnswered            = bridgecontract.ResultAnswered
	ResultDelivered           = bridgecontract.ResultDelivered
	ResultFailed              = bridgecontract.ResultFailed
	ReasonInvalidShape        = bridgecontract.ReasonInvalidShape
	ReasonVersionMismatch     = bridgecontract.ReasonVersionMismatch
	ReasonIdempotencyConflict = bridgecontract.ReasonIdempotencyConflict
	ReasonUnknownCommand      = bridgecontract.ReasonUnknownCommand
	ReasonUnknownMessage      = bridgecontract.ReasonUnknownMessage
	ReasonExpired             = bridgecontract.ReasonExpired
	ReasonDeliveryUnknown     = bridgecontract.ReasonDeliveryUnknown
	ReasonProcessingFailed    = bridgecontract.ReasonProcessingFailed
	QuerySessionStatus        = bridgecontract.QuerySessionStatus
	QueryAttemptStatus        = bridgecontract.QueryAttemptStatus
	QueryWorktreeWatch        = bridgecontract.QueryWorktreeWatch
	StateIdle                 = bridgecontract.StateIdle
	StateRunning              = bridgecontract.StateRunning
	StateDone                 = bridgecontract.StateDone
	StateFailed               = bridgecontract.StateFailed
	StateUnknown              = bridgecontract.StateUnknown
	StateWatching             = bridgecontract.StateWatching
	StateStopped              = bridgecontract.StateStopped
	SeverityInfo              = bridgecontract.SeverityInfo
	SeverityWarning           = bridgecontract.SeverityWarning
	SeverityError             = bridgecontract.SeverityError
	NonceBytes                = bridgecontract.NonceBytes
	NonceEncodedLength        = bridgecontract.NonceEncodedLength
	SignatureEncodedLength    = bridgecontract.SignatureEncodedLength
	ULIDLength                = bridgecontract.ULIDLength
	ReplayWindowSeconds       = bridgecontract.ReplayWindowSeconds
	HeartbeatSeconds          = bridgecontract.HeartbeatSeconds
	RedeemBodyMaxBytes        = bridgecontract.RedeemBodyMaxBytes
	RedeemResponseMaxBytes    = bridgecontract.RedeemResponseMaxBytes
	FrameMaxBytes             = bridgecontract.FrameMaxBytes
	IdentifierMaxBytes        = bridgecontract.IdentifierMaxBytes
	LabelMaxBytes             = bridgecontract.LabelMaxBytes
	TitleMaxBytes             = bridgecontract.TitleMaxBytes
	NotificationBodyMaxBytes  = bridgecontract.NotificationBodyMaxBytes
	OutboxDepthMax            = bridgecontract.OutboxDepthMax
	TombstonesPerDeviceMax    = bridgecontract.TombstonesPerDeviceMax
	CommandTTLMaxSeconds      = bridgecontract.CommandTTLMaxSeconds
	PairingCodeMaxBytes       = bridgecontract.PairingCodeMaxBytes
	MaxTimestampDigits        = bridgecontract.MaxTimestampDigits
	CommandRevision           = bridgecontract.CommandRevision
)

// ConnectRequestTarget preserves the required query order and ASCII spelling.
func ConnectRequestTarget(capabilities []string) (string, error) {
	if !validCapabilities(capabilities) {
		return "", ErrInvalidSigningInput
	}
	target := ConnectPath + ConnectVersionQueryPrefix + ContractVersion + ConnectCapabilitiesQuery + strings.Join(capabilities, ",")
	return target, nil
}

func validCapabilities(c []string) bool {
	return bridgecontract.ValidCapabilities(c)
}

func validPairingCode(code string) bool {
	if len(code) != PairingCodeMaxBytes {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(code)
	return err == nil && len(decoded) == bridgecontract.PairingCodeBytes && base64.RawURLEncoding.EncodeToString(decoded) == code
}

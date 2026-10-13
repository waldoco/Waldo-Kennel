// Package bridgecontract owns the v0.2.3 wire values and limits.
// Changes to these normative values require a reviewed contract version.
package bridgecontract

const (
	Version                   = "0.3.0"
	RedeemPath                = "/devices/redeem"
	ConnectPath               = "/devices/connect"
	ConnectVersionQueryPrefix = "?contract_version="
	ConnectCapabilitiesQuery  = "&declared_capabilities="
	TypeCommand               = "command"
	TypeAck                   = "ack"
	TypeResult                = "result"
	TypeReceipt               = "receipt"
	TypeHeartbeat             = "heartbeat"
	TypeSessionEvent          = "session_event"
	ClassShellExec            = "shell_exec"
	ClassSessionQuery         = "session_query"
	ClassSessionControl       = "session_control"
	ResultCompleted           = "completed"
	ResultDenied              = "denied"
	ResultExpired             = "expired"
	ResultOutcomeUnknown      = "outcome_unknown"
	ApprovalTTLSeconds        = 900
	ExecutionDefaultSeconds   = 120
	ExecutionMaxSeconds       = 600
	OutputTailBytes           = 6144
	ClassMachineStateQuery    = "machine_state_query"
	ClassNotifyLocal          = "notify_local"
	AckAccepted               = "accepted"
	AckRejected               = "rejected"
	AckExpired                = "expired"
	ResultAnswered            = "answered"
	ResultDelivered           = "delivered"
	ResultFailed              = "failed"
	ReasonInvalidShape        = "invalid_shape"
	ReasonVersionMismatch     = "version_mismatch"
	ReasonIdempotencyConflict = "idempotency_conflict"
	ReasonUnknownCommand      = "unknown_command"
	ReasonUnknownMessage      = "unknown_message"
	ReasonExpired             = "expired"
	ReasonDeliveryUnknown     = "delivery_unknown"
	ReasonProcessingFailed    = "processing_failed"
	PairingCodeBytes          = 32
	PairingCodeExpirySeconds  = 600
	QuerySessionStatus        = "session_status"
	QueryAttemptStatus        = "attempt_status"
	QueryWorktreeWatch        = "worktree_watch"
	StateIdle                 = "idle"
	StateRunning              = "running"
	StateDone                 = "done"
	StateFailed               = "failed"
	StateUnknown              = "unknown"
	StateWatching             = "watching"
	StateStopped              = "stopped"
	SeverityInfo              = "info"
	SeverityWarning           = "warning"
	SeverityError             = "error"
	NonceBytes                = 16
	NonceEncodedLength        = 22
	SignatureEncodedLength    = 86
	ULIDLength                = 26
	ReplayWindowSeconds       = 300
	HeartbeatSeconds          = 30
	RedeemBodyMaxBytes        = 2048
	// Proposed local response resource bound; not a normative wire limit.
	RedeemResponseMaxBytes   = 4096
	FrameMaxBytes            = 8192
	IdentifierMaxBytes       = 128
	LabelMaxBytes            = 120
	TitleMaxBytes            = 120
	NotificationBodyMaxBytes = 1024
	OutboxDepthMax           = 1000000
	TombstonesPerDeviceMax   = 100000
	CommandTTLMaxSeconds     = 86400
	PairingCodeMaxBytes      = 43
	MaxTimestampDigits       = 10
	CommandRevision          = 1
)

// ValidCapabilities accepts a nonempty ordered subset; reconnects cannot widen pairing.
func ValidCapabilities(c []string) bool {
	if len(c) == 0 || len(c) > 5 {
		return false
	}
	all := []string{ClassMachineStateQuery, ClassNotifyLocal, ClassShellExec, ClassSessionQuery, ClassSessionControl}
	next := 0
	for _, value := range c {
		for next < len(all) && all[next] != value {
			next++
		}
		if next == len(all) {
			return false
		}
		next++
	}
	return true
}

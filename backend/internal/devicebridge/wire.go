package devicebridge

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

var ErrInvalidFrameShape = errors.New("invalid device bridge frame shape")
var ErrContractVersionMismatch = errors.New("device bridge contract version mismatch")

// ParseBackendFrame accepts only an unsigned, canonical command or receipt.
// Backend authenticity is supplied by the verified TLS connection.
func ParseBackendFrame(raw []byte) (map[string]any, error) {
	if len(raw) > FrameMaxBytes {
		return nil, ErrInvalidFrameShape
	}
	frame, err := parseJSONObject(raw)
	if err != nil {
		return nil, ErrInvalidFrameShape
	}
	canonical, err := marshalCanonical(frame)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, ErrInvalidFrameShape
	}
	if frame["contract_version"] != ContractVersion && frame["contract_version"] != "0.2.3" {
		return nil, ErrContractVersionMismatch
	}
	if validateFrameShape(frame, false) != nil {
		return nil, ErrInvalidFrameShape
	}
	return frame, nil
}

// LogicalFingerprint excludes only signing fields. It remains stable across
// fresh signatures of the same message and changes if any payload/identity
// field changes.
func LogicalFingerprint(frame map[string]any) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	if frame == nil {
		return zero, ErrInvalidFrameShape
	}
	logical := make(map[string]any, len(frame))
	for k, v := range frame {
		if k != "timestamp" && k != "nonce" && k != "signature" {
			logical[k] = v
		}
	}
	if err := validateFrameShape(logical, false); err != nil {
		// An unsigned device frame is valid after its signing fields are removed.
		if err := validateUnsignedDeviceShape(logical); err != nil {
			return zero, err
		}
	}
	canonical, err := marshalCanonical(logical)
	if err != nil {
		return zero, ErrInvalidFrameShape
	}
	return sha256.Sum256(canonical), nil
}

func validateUnsignedDeviceShape(frame map[string]any) error {
	copy := make(map[string]any, len(frame)+3)
	for k, v := range frame {
		copy[k] = v
	}
	copy["timestamp"] = json.Number("1")
	copy["nonce"] = base64.RawURLEncoding.EncodeToString(make([]byte, NonceBytes))
	copy["signature"] = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	return validateFrameShape(copy, true)
}

func validateFrameShape(frame map[string]any, deviceOutbound bool) error {
	if frame["contract_version"] == "0.2.3" {
		p, _ := frame["payload"].(map[string]any)
		if frame["type"] == TypeSessionEvent || newCommandClass(frame["class"]) || p["event_message_id"] != nil || (frame["type"] == TypeResult && oneOf(p["status"], ResultCompleted, ResultDenied, ResultExpired, ResultOutcomeUnknown)) {
			return ErrInvalidFrameShape
		}
		if frame["type"] == TypeHeartbeat {
			caps, _ := p["declared_capabilities"].([]any)
			for _, cap := range caps {
				if cap != ClassMachineStateQuery && cap != ClassNotifyLocal {
					return ErrInvalidFrameShape
				}
			}
		}
	}
	if hasNull(frame) {
		return ErrInvalidFrameShape
	}
	typeName, ok := frame["type"].(string)
	if !ok || !validFrameType(typeName) {
		return ErrInvalidFrameShape
	}
	if deviceOutbound {
		if typeName != TypeAck && typeName != TypeResult && typeName != TypeHeartbeat && typeName != TypeSessionEvent {
			return ErrInvalidFrameShape
		}
	} else if typeName != TypeCommand && typeName != TypeReceipt {
		return ErrInvalidFrameShape
	}
	allowed := []string{"contract_version", "type", "message_id", "device_id", "owner_id", "payload"}
	if frameHasReference(frame) {
		allowed = append(allowed, "command_id", "revision", "idempotency_key")
	}
	if typeName == TypeCommand {
		allowed = append(allowed, "class", "expires_at")
	}
	if deviceOutbound {
		allowed = append(allowed, "timestamp", "nonce", "signature")
	}
	if !exactFields(frame, allowed...) || (frame["contract_version"] != ContractVersion && frame["contract_version"] != "0.2.3") {
		return ErrInvalidFrameShape
	}
	messageID, ok := frame["message_id"].(string)
	if !ok || !validULID(messageID) {
		return ErrInvalidFrameShape
	}
	for _, field := range []string{"device_id", "owner_id"} {
		value, ok := frame[field].(string)
		if !ok || !validIdentifier(value) {
			return ErrInvalidFrameShape
		}
	}
	if frameHasReference(frame) {
		for _, field := range []string{"command_id", "idempotency_key"} {
			value, ok := frame[field].(string)
			if !ok || !validIdentifier(value) {
				return ErrInvalidFrameShape
			}
		}
		if !integerEqual(frame["revision"], CommandRevision) {
			return ErrInvalidFrameShape
		}
	}
	if typeName == TypeCommand {
		class, ok := frame["class"].(string)
		if !ok || !validCommandClass(class) || !positiveInteger(frame["expires_at"]) {
			return ErrInvalidFrameShape
		}
	}
	if deviceOutbound {
		if !positiveTimestamp(frame["timestamp"]) {
			return ErrInvalidFrameShape
		}
		for _, field := range []struct {
			name             string
			decoded, encoded int
		}{{"nonce", NonceBytes, NonceEncodedLength}, {"signature", 64, SignatureEncodedLength}} {
			value, ok := frame[field.name].(string)
			if !ok || len(value) != field.encoded {
				return ErrInvalidFrameShape
			}
			decoded, err := base64.RawURLEncoding.DecodeString(value)
			if err != nil || len(decoded) != field.decoded || base64.RawURLEncoding.EncodeToString(decoded) != value {
				return ErrInvalidFrameShape
			}
		}
	}
	payload, ok := frame["payload"].(map[string]any)
	if !ok {
		return ErrInvalidFrameShape
	}
	return validatePayload(typeName, frame, payload)
}

func validatePayload(typeName string, frame, payload map[string]any) error {
	switch typeName {
	case TypeCommand:
		if newCommandClass(frame["class"]) {
			return validateMacCommand(frame["class"], payload)
		}
		if frame["class"] == ClassMachineStateQuery {
			return validateQueryPayload(payload)
		}
		if !exactFields(payload, "notification_id", "title", "body", "severity") || !stringID(payload["notification_id"]) || !boundedString(payload["title"], TitleMaxBytes) || !boundedString(payload["body"], NotificationBodyMaxBytes) || !oneOf(payload["severity"], SeverityInfo, SeverityWarning, SeverityError) {
			return ErrInvalidFrameShape
		}
	case TypeAck:
		if exactFields(payload, "state") && payload["state"] == AckAccepted {
			return nil
		}
		if !exactFields(payload, "state", "reason") || !oneOf(payload["state"], AckRejected, AckExpired) || !oneOf(payload["reason"], ReasonInvalidShape, ReasonVersionMismatch, ReasonIdempotencyConflict, ReasonUnknownCommand, ReasonExpired) {
			return ErrInvalidFrameShape
		}
		if (payload["state"] == AckExpired) != (payload["reason"] == ReasonExpired) {
			return ErrInvalidFrameShape
		}
	case TypeResult:
		if validMacResult(payload) {
			return nil
		}
		if exactFields(payload, "status", "answer") && payload["status"] == ResultAnswered {
			answer, ok := payload["answer"].(map[string]any)
			if !ok || !exactFields(answer, "query_id", "query_kind", "state") || !stringID(answer["query_id"]) || !validQueryKind(answer["query_kind"]) || !validQueryState(answer["query_kind"], answer["state"]) {
				return ErrInvalidFrameShape
			}
			return nil
		}
		if exactFields(payload, "status") && payload["status"] == ResultDelivered {
			return nil
		}
		if !exactFields(payload, "status", "reason") || payload["status"] != ResultFailed || !oneOf(payload["reason"], ReasonProcessingFailed, ReasonDeliveryUnknown) {
			return ErrInvalidFrameShape
		}
	case TypeReceipt:
		if exactFields(payload, "event_message_id", "received_at") {
			id, ok := payload["event_message_id"].(string)
			if ok && validULID(id) && positiveInteger(payload["received_at"]) {
				return nil
			}
			return ErrInvalidFrameShape
		}
		if !exactFields(payload, "result_message_id", "received_at") {
			return ErrInvalidFrameShape
		}
		id, ok := payload["result_message_id"].(string)
		if !ok || !validULID(id) || !positiveInteger(payload["received_at"]) {
			return ErrInvalidFrameShape
		}
	case TypeSessionEvent:
		if !exactFields(payload, "session_id", "project", "event") || !stringID(payload["session_id"]) || !boundedString(payload["project"], 128) || !oneOf(payload["event"], "needs_input", "blocked_on_tool", "turn_finished", "exited") {
			return ErrInvalidFrameShape
		}
	case TypeHeartbeat:
		if !exactFields(payload, "declared_capabilities", "outbox_depth") {
			return ErrInvalidFrameShape
		}
		values, ok := payload["declared_capabilities"].([]any)
		if !ok {
			return ErrInvalidFrameShape
		}
		capabilities := make([]string, len(values))
		for i, value := range values {
			capabilities[i], ok = value.(string)
			if !ok {
				return ErrInvalidFrameShape
			}
		}
		if !validCapabilities(capabilities) {
			return ErrInvalidFrameShape
		}
		depth, ok := parseInteger(payload["outbox_depth"])
		if !ok || depth < 0 || depth > OutboxDepthMax {
			return ErrInvalidFrameShape
		}
	}
	return nil
}

func validateQueryPayload(payload map[string]any) error {
	if !exactFields(payload, "query_id", "query_kind") || !stringID(payload["query_id"]) || !validQueryKind(payload["query_kind"]) {
		return ErrInvalidFrameShape
	}
	return nil
}
func validQueryKind(v any) bool {
	return oneOf(v, QuerySessionStatus, QueryAttemptStatus, QueryWorktreeWatch)
}
func validQueryState(kind, state any) bool {
	if kind == QueryWorktreeWatch {
		return oneOf(state, StateWatching, StateStopped, StateUnknown)
	}
	return oneOf(state, StateIdle, StateRunning, StateDone, StateFailed, StateUnknown)
}
func oneOf(v any, options ...string) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	for _, option := range options {
		if s == option {
			return true
		}
	}
	return false
}
func boundedString(v any, max int) bool {
	s, ok := v.(string)
	return ok && len(s) > 0 && len(s) <= max
}
func stringID(v any) bool { s, ok := v.(string); return ok && validIdentifier(s) }
func validIdentifier(s string) bool {
	if len(s) == 0 || len(s) > IdentifierMaxBytes {
		return false
	}
	for i := range s {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func positiveTimestamp(v any) bool        { n, ok := parseInteger(v); return ok && validTimestamp(n) }
func positiveInteger(v any) bool          { n, ok := parseInteger(v); return ok && n > 0 }
func integerEqual(v any, want int64) bool { n, ok := parseInteger(v); return ok && n == want }
func parseInteger(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok || !validIntegerSyntax(string(n)) {
		return 0, false
	}
	i, err := strconv.ParseInt(string(n), 10, 64)
	return i, err == nil
}
func exactFields(object map[string]any, fields ...string) bool {
	if len(object) != len(fields) {
		return false
	}
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return false
		}
	}
	return true
}
func hasNull(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case map[string]any:
		for _, item := range v {
			if hasNull(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if hasNull(item) {
				return true
			}
		}
	}
	return false
}

// ValidateResultForCommand checks the lineage and class-specific outcome that
// cannot be decided from a result frame alone (results do not carry class).
func ValidateResultForCommand(result, command map[string]any) error {
	if validateFrameShape(result, true) != nil || result["type"] != TypeResult || validateFrameShape(command, false) != nil || command["type"] != TypeCommand {
		return ErrInvalidFrameShape
	}
	for _, field := range []string{"device_id", "owner_id", "command_id", "revision", "idempotency_key"} {
		if result[field] != command[field] {
			return ErrInvalidFrameShape
		}
	}
	payload := result["payload"].(map[string]any)
	if newCommandClass(command["class"]) {
		if !validMacResult(payload) && !(payload["status"] == ResultFailed && payload["reason"] == ReasonProcessingFailed) {
			return ErrInvalidFrameShape
		}
		if command["class"] == ClassSessionQuery && payload["status"] == ResultCompleted {
			return ErrInvalidFrameShape
		}
		if command["class"] != ClassSessionQuery && payload["status"] == ResultAnswered {
			return ErrInvalidFrameShape
		}

		request := command["payload"].(map[string]any)
		if payload["status"] == ResultCompleted {
			if command["class"] == ClassShellExec && payload["session_id"] != nil {
				return ErrInvalidFrameShape
			}
			if command["class"] == ClassSessionControl {
				if !stringID(payload["session_id"]) || (request["action"] != "start_session" && payload["session_id"] != request["session_id"]) {
					return ErrInvalidFrameShape
				}
			}
		}
		if payload["status"] == ResultAnswered {
			a := payload["answer"].(map[string]any)
			if request["action"] == "read_session" {
				if a["session_id"] != request["session_id"] {
					return ErrInvalidFrameShape
				}
			} else if a["sessions"] == nil {
				return ErrInvalidFrameShape
			}
		}
		return nil
	}
	if command["class"] == ClassMachineStateQuery {
		if payload["status"] == ResultAnswered {
			answer := payload["answer"].(map[string]any)
			query := command["payload"].(map[string]any)
			if answer["query_id"] != query["query_id"] || answer["query_kind"] != query["query_kind"] {
				return ErrInvalidFrameShape
			}
		} else if payload["status"] != ResultFailed {
			return ErrInvalidFrameShape
		} else if payload["reason"] != ReasonProcessingFailed {
			return ErrInvalidFrameShape
		}
	} else if payload["status"] == ResultAnswered || (payload["status"] == ResultFailed && payload["reason"] != ReasonDeliveryUnknown) {
		return ErrInvalidFrameShape
	}
	return nil
}

// ValidateReceiptForResult binds the receipt's own message identity to the
// durable result it names, including owner and device scope.
func ValidateReceiptForResult(receipt, result map[string]any) error {
	if validateFrameShape(receipt, false) != nil || receipt["type"] != TypeReceipt || validateFrameShape(result, true) != nil || result["type"] != TypeResult {
		return ErrInvalidFrameShape
	}
	for _, field := range []string{"device_id", "owner_id", "command_id", "revision", "idempotency_key"} {
		if receipt[field] != result[field] {
			return ErrInvalidFrameShape
		}
	}
	if receipt["payload"].(map[string]any)["result_message_id"] != result["message_id"] {
		return ErrInvalidFrameShape
	}
	return nil
}

// ResignFrame changes only the three signing fields of a verified logical
// device message. The caller must provide a fresh nonce for each transmission.
func ResignFrame(raw []byte, key ed25519.PrivateKey, now time.Time, nonce [NonceBytes]byte) ([]byte, error) {
	frame, err := parseJSONObject(raw)
	canonical, canonicalErr := marshalCanonical(frame)
	if err != nil || canonicalErr != nil || !bytes.Equal(raw, canonical) || !validTimestamp(now.Unix()) || len(key) != ed25519.PrivateKeySize {
		return nil, ErrInvalidSigningInput
	}
	if err := validateFrameShape(frame, true); err != nil {
		return nil, ErrInvalidSigningInput
	}
	if frame["nonce"] == base64.RawURLEncoding.EncodeToString(nonce[:]) {
		return nil, ErrInvalidSigningInput
	}
	base, err := frameBaseString(frame)
	if err != nil {
		return nil, ErrInvalidSigningInput
	}
	priorSignature, err := base64.RawURLEncoding.DecodeString(frame["signature"].(string))
	if err != nil || !ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(base), priorSignature) {
		return nil, ErrInvalidSigningInput
	}
	delete(frame, "timestamp")
	delete(frame, "nonce")
	delete(frame, "signature")
	unsigned, err := marshalCanonical(frame)
	if err != nil {
		return nil, ErrInvalidSigningInput
	}
	return SignFrame(unsigned, key, now, nonce)
}

package devicebridge

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidFrameSignature = errors.New("invalid device bridge frame signature")

// SignFrame signs one v0.2.3 socket envelope. It sets fresh timestamp and nonce
// fields and returns compact JSON; the caller supplies only public wire fields.
func SignFrame(raw []byte, key ed25519.PrivateKey, now time.Time, nonce [NonceBytes]byte) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize || now.IsZero() {
		return nil, ErrInvalidSigningInput
	}
	frame, err := parseJSONObject(raw)
	if err != nil {
		return nil, ErrInvalidSigningInput
	}
	if _, alreadySigned := frame["signature"]; alreadySigned {
		return nil, ErrInvalidSigningInput
	}
	if !validTimestamp(now.Unix()) {
		return nil, ErrInvalidSigningInput
	}
	frame["timestamp"] = json.Number(strconv.FormatInt(now.Unix(), 10))
	frame["nonce"] = base64.RawURLEncoding.EncodeToString(nonce[:])
	base, err := frameBaseString(frame)
	if err != nil {
		return nil, ErrInvalidSigningInput
	}
	frame["signature"] = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(base)))
	if err := validateFrameShape(frame, true); err != nil {
		return nil, ErrInvalidSigningInput
	}
	wire, err := marshalCanonical(frame)
	if err != nil || len(wire) > FrameMaxBytes {
		return nil, ErrInvalidSigningInput
	}
	return wire, nil
}

// VerifyFrame checks v0.2.3 canonicalization, closed payload, signature,
// timestamp window, and nonce syntax. Replay nonce storage and authenticated
// owner/device binding remain transport-session responsibilities.
func VerifyFrame(raw []byte, sender ed25519.PublicKey, now time.Time) (map[string]any, error) {
	if len(sender) != ed25519.PublicKeySize || now.IsZero() {
		return nil, ErrInvalidFrameSignature
	}
	if len(raw) > FrameMaxBytes {
		return nil, ErrInvalidFrameSignature
	}
	frame, err := parseJSONObject(raw)
	if err != nil {
		return nil, ErrInvalidFrameSignature
	}
	canonical, err := marshalCanonical(frame)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, ErrInvalidFrameSignature
	}
	encodedSignature, ok := frame["signature"].(string)
	if !ok {
		return nil, ErrInvalidFrameSignature
	}
	signature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, ErrInvalidFrameSignature
	}
	base, err := frameBaseString(frame)
	if err != nil || !ed25519.Verify(sender, []byte(base), signature) {
		return nil, ErrInvalidFrameSignature
	}
	stamp, _ := frameTimestamp(frame)
	delta := now.Unix() - stamp
	if delta < -ReplayWindowSeconds || delta > ReplayWindowSeconds {
		return nil, ErrInvalidFrameSignature
	}
	if frame["contract_version"] != ContractVersion && frame["contract_version"] != "0.2.3" {
		return nil, ErrContractVersionMismatch
	}
	if err := validateFrameShape(frame, true); err != nil {
		return nil, ErrInvalidFrameSignature
	}
	return frame, nil
}

func frameBaseString(frame map[string]any) (string, error) {
	stamp, err := frameTimestamp(frame)
	if err != nil {
		return "", err
	}
	typeName, ok := frame["type"].(string)
	if !ok || !validFrameType(typeName) {
		return "", ErrInvalidFrameSignature
	}
	messageID, ok := frame["message_id"].(string)
	if !ok || !validULID(messageID) {
		return "", ErrInvalidFrameSignature
	}
	nonce, ok := frame["nonce"].(string)
	if !ok || len(nonce) != NonceEncodedLength {
		return "", ErrInvalidFrameSignature
	}
	decodedNonce, err := base64.RawURLEncoding.DecodeString(nonce)
	if err != nil || len(decodedNonce) != NonceBytes || base64.RawURLEncoding.EncodeToString(decodedNonce) != nonce {
		return "", ErrInvalidFrameSignature
	}
	unsigned := make(map[string]any, len(frame))
	for key, value := range frame {
		if key != "signature" {
			unsigned[key] = value
		}
	}
	body, err := marshalCanonical(unsigned)
	if err != nil {
		return "", ErrInvalidFrameSignature
	}
	hash := sha256.Sum256(body)
	return strings.Join([]string{
		strconv.FormatInt(stamp, 10), typeName, messageID, nonce, hex.EncodeToString(hash[:]),
	}, "\n"), nil
}

func frameTimestamp(frame map[string]any) (int64, error) {
	number, ok := frame["timestamp"].(json.Number)
	if !ok {
		return 0, ErrInvalidFrameSignature
	}
	stamp, err := strconv.ParseInt(string(number), 10, 64)
	if err != nil || !validTimestamp(stamp) || strconv.FormatInt(stamp, 10) != string(number) {
		return 0, ErrInvalidFrameSignature
	}
	return stamp, nil
}

func validFrameType(kind string) bool {
	switch kind {
	case TypeCommand, TypeAck, TypeResult, TypeReceipt, TypeHeartbeat, TypeSessionEvent:
		return true
	default:
		return false
	}
}

// parseJSONObject rejects duplicate keys at every depth. Different JSON
// parsers must not be able to see different payloads under the same signature.
func parseJSONObject(raw []byte) (map[string]any, error) {
	if !utf8.Valid(raw) || !validJSONSurrogates(raw) {
		return nil, ErrInvalidFrameSignature
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := parseJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidFrameSignature
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, ErrInvalidFrameSignature
	}
	return object, nil
}

func parseJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, ErrInvalidFrameSignature
				}
				if _, exists := object[key]; exists {
					return nil, ErrInvalidFrameSignature
				}
				value, err := parseJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				object[key] = value
			}
			if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
				return nil, ErrInvalidFrameSignature
			}
			return object, nil
		case '[':
			var array []any
			for decoder.More() {
				value, err := parseJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
				return nil, ErrInvalidFrameSignature
			}
			return array, nil
		default:
			return nil, ErrInvalidFrameSignature
		}
	}
	return token, nil
}

func marshalCanonical(value any) ([]byte, error) {
	var out bytes.Buffer
	if err := writeCanonical(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeCanonical(out *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			if !utf8.ValidString(key) {
				return ErrInvalidFrameSignature
			}
			keys = append(keys, key)
		}
		// Valid UTF-8 byte order is Unicode code-point order.
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i != 0 {
				out.WriteByte(',')
			}
			writeCanonicalString(out, key)
			out.WriteByte(':')
			if err := writeCanonical(out, v[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i != 0 {
				out.WriteByte(',')
			}
			if err := writeCanonical(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case string:
		if !utf8.ValidString(v) {
			return ErrInvalidFrameSignature
		}
		writeCanonicalString(out, v)
	case json.Number:
		s := string(v)
		if !validIntegerSyntax(s) {
			return ErrInvalidFrameSignature
		}
		out.WriteString(s)
	case bool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case nil:
		out.WriteString("null")
	default:
		return ErrInvalidFrameSignature
	}
	return nil
}

func validIntegerSyntax(s string) bool {
	if s == "0" {
		return true
	}
	if s == "" {
		return false
	}
	start := 0
	if s[0] == '-' {
		start = 1
	}
	if start == len(s) || s[start] == '0' {
		return false
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func writeCanonicalString(out *bytes.Buffer, s string) {
	const hex = "0123456789abcdef"
	out.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\b':
			out.WriteString(`\b`)
		case '\t':
			out.WriteString(`\t`)
		case '\n':
			out.WriteString(`\n`)
		case '\f':
			out.WriteString(`\f`)
		case '\r':
			out.WriteString(`\r`)
		default:
			if r < 0x20 {
				out.WriteString(`\u00`)
				out.WriteByte(hex[byte(r)>>4])
				out.WriteByte(hex[byte(r)&15])
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
}

// encoding/json replaces unpaired UTF-16 surrogate escapes with U+FFFD.
// Reject those escapes before decoding so the signed bytes retain one meaning.
func validJSONSurrogates(raw []byte) bool {
	inString, escaped := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !inString {
			if c == '"' {
				inString = true
			}
			continue
		}
		if escaped {
			escaped = false
			if c != 'u' {
				continue
			}
			if i+4 >= len(raw) {
				return false
			}
			n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			if err != nil {
				return false
			}
			i += 4
			if n >= 0xdc00 && n <= 0xdfff {
				return false
			}
			if n >= 0xd800 && n <= 0xdbff {
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				m, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
				if err != nil || m < 0xdc00 || m > 0xdfff {
					return false
				}
				i += 6
			}
			continue
		}
		if c == '\\' {
			escaped = true
		} else if c == '"' {
			inString = false
		}
	}
	return !inString
}

func validULID(id string) bool {
	if len(id) != ULIDLength {
		return false
	}
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for i := range id {
		if !strings.ContainsRune(alphabet, rune(id[i])) {
			return false
		}
	}
	return id[0] <= '7'
}

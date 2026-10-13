package devicebridge

import (
	"crypto/ed25519"
	"encoding/base64"
)

// RedeemBody constructs the closed v0.3.0 six-field request. Pairing code
// and private key never enter logs or durable bridge payloads.
func RedeemBody(code string, publicKey ed25519.PublicKey, label string, capabilities []string, consent ...bool) ([]byte, error) {
	if !validPairingCode(code) || len(publicKey) != ed25519.PublicKeySize || !boundedString(label, LabelMaxBytes) || !validCapabilities(capabilities) {
		return nil, ErrPairingInvalid
	}
	sessionConsent := len(consent) == 1 && consent[0]
	if len(consent) > 1 || (requiresSessionConsent(capabilities) && !sessionConsent) {
		return nil, ErrPairingInvalid
	}
	values := make([]any, len(capabilities))
	for i, capability := range capabilities {
		values[i] = capability
	}
	body, err := marshalCanonical(map[string]any{
		"code": code, "device_pubkey": base64.RawURLEncoding.EncodeToString(publicKey),
		"label": label, "declared_capabilities": values, "contract_version": ContractVersion, "session_consent": sessionConsent,
	})
	if err != nil || len(body) > RedeemBodyMaxBytes {
		return nil, ErrPairingInvalid
	}
	return body, nil
}

// ParseRedeemBody performs the strict raw-body parsing that the backend must
// use before verifying the key proof against these same bytes.
func ParseRedeemBody(raw []byte) (ed25519.PublicKey, error) {
	if len(raw) > RedeemBodyMaxBytes {
		return nil, ErrPairingInvalid
	}
	body, err := parseJSONObject(raw)
	if err != nil || !validRedeemShape(body) || hasNull(body) {
		return nil, ErrPairingInvalid
	}
	code, ok := body["code"].(string)
	if !ok || !validPairingCode(code) || !boundedString(body["label"], LabelMaxBytes) {
		return nil, ErrPairingInvalid
	}
	encoded, ok := body["device_pubkey"].(string)
	if !ok {
		return nil, ErrPairingInvalid
	}
	key, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(key) != ed25519.PublicKeySize || base64.RawURLEncoding.EncodeToString(key) != encoded {
		return nil, ErrPairingInvalid
	}
	values, ok := body["declared_capabilities"].([]any)
	if !ok {
		return nil, ErrPairingInvalid
	}
	capabilities := make([]string, len(values))
	for i, value := range values {
		capabilities[i], ok = value.(string)
		if !ok {
			return nil, ErrPairingInvalid
		}
	}
	if !validCapabilities(capabilities) || (body["contract_version"] == "0.2.3" && !legacyCapabilities(capabilities)) || (requiresSessionConsent(capabilities) && body["session_consent"] != true) {
		return nil, ErrPairingInvalid
	}
	return ed25519.PublicKey(key), nil
}

func requiresSessionConsent(c []string) bool {
	for _, v := range c {
		if v == ClassSessionQuery || v == ClassSessionControl {
			return true
		}
	}
	return false
}
func legacyCapabilities(c []string) bool {
	for _, v := range c {
		if v != ClassMachineStateQuery && v != ClassNotifyLocal {
			return false
		}
	}
	return validCapabilities(c)
}
func validRedeemShape(b map[string]any) bool {
	if b["contract_version"] == "0.2.3" {
		return exactFields(b, "code", "device_pubkey", "label", "declared_capabilities", "contract_version")
	}
	_, ok := b["session_consent"].(bool)
	return b["contract_version"] == ContractVersion && ok && exactFields(b, "code", "device_pubkey", "label", "declared_capabilities", "contract_version", "session_consent")
}

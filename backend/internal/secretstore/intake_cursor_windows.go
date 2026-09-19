//go:build windows

package secretstore

import "fmt"

const IntakeCursorMACKeySize = 32
const intakeCursorMACKeyFilename = "waldo-intake-cursor-hmac-v1"

// LoadOrCreateIntakeCursorMACKey fails closed on Windows. Unix 0600 ownership
// semantics cannot be truthfully established from Windows ACLs by this file,
// and accepting a weaker approximation would expose the daemon signing root.
func LoadOrCreateIntakeCursorMACKey(string) ([]byte, error) {
	return nil, fmt.Errorf("intake cursor MAC key is unavailable: private owner-only file semantics are not supported on Windows")
}

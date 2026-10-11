package secretstore

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	keychainDeviceKeyRefPrefix = "keychain-device-key:"
	keychainDeviceKeyService   = "in.heywaldo.kennel.device-bridge-key"
	// securityPath is never resolved through PATH.
	securityPath = "/usr/bin/security"
	// keychainItemNotFound is errSecItemNotFound as security(1) reports it.
	keychainItemNotFound = 44
	securityTimeout      = 15 * time.Second
	securityOutputBytes  = 4096
)

type securityRunner func(ctx context.Context, args []string, stdin []byte) (stdout []byte, exitCode int, err error)

// KeychainDeviceKeyStore keeps bridge device keys as generic-password items in the
// user's default (login) Keychain through /usr/bin/security, without cgo.
//
// Protection, stated plainly: the key leaves the Kennel data directory, so copies,
// backups and directory scans of ~/.kennel never contain it, and it is encrypted at
// rest under the login Keychain. It does NOT protect against processes running as
// the same user: items created by security(1) trust security(1), so any such process
// can read the item back through it without a prompt.
//
// The key reaches security(1) only over stdin in interactive mode (-i), never argv.
// Reads and deletes pass only the service and the random account locator in argv.
type KeychainDeviceKeyStore struct {
	run securityRunner
}

func NewKeychainDeviceKeyStore() *KeychainDeviceKeyStore {
	return &KeychainDeviceKeyStore{run: execSecurity}
}

func execSecurity(ctx context.Context, args []string, stdin []byte) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, securityTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, securityPath, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.Bytes(), exit.ExitCode(), nil
	}
	if err != nil {
		return nil, -1, fmt.Errorf("run security: %w", err)
	}
	return out.Bytes(), 0, nil
}

// limitedBuffer bounds captured output; anything beyond the limit is dropped.
type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := securityOutputBytes - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

// Put stores the key and returns its opaque reference only after reading the same
// key back. Neither the key nor security(1) output is included in errors.
func (s *KeychainDeviceKeyStore) Put(ctx context.Context, key ed25519.PrivateKey) (string, error) {
	if s == nil || s.run == nil || !validDevicePrivateKey(key) {
		return "", ErrDeviceKeyInvalid
	}
	encoded := base64.RawURLEncoding.EncodeToString(key)
	for attempt := 0; attempt < 3; attempt++ {
		var random [16]byte
		if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
			return "", fmt.Errorf("generate device key reference: %w", err)
		}
		account := hex.EncodeToString(random[:])
		ref := keychainDeviceKeyRefPrefix + account
		if _, err := s.find(ctx, account); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		// Plain tokens only (hex account, constant service, base64url key), one line.
		line := fmt.Sprintf("add-generic-password -a %s -s %s -w %s\n", account, keychainDeviceKeyService, encoded)
		if _, _, err := s.run(ctx, []string{"-i"}, []byte(line)); err != nil {
			return "", fmt.Errorf("store device key in keychain: %w", err)
		}
		// Interactive mode does not report per-command status; trust only a read-back.
		stored, err := s.find(ctx, account)
		if err != nil {
			return "", fmt.Errorf("verify device key in keychain: %w", err)
		}
		if subtle.ConstantTimeCompare([]byte(stored), []byte(encoded)) != 1 {
			_ = s.Clear(ctx, ref)
			return "", errors.New("verify device key in keychain: stored item does not match")
		}
		return ref, nil
	}
	return "", errors.New("allocate device key reference")
}

func (s *KeychainDeviceKeyStore) Get(ctx context.Context, ref string) (ed25519.PrivateKey, error) {
	account, err := keychainAccount(ref)
	if err != nil || s == nil || s.run == nil {
		return nil, ErrDeviceKeyInvalid
	}
	encoded, err := s.find(ctx, account)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || !validDevicePrivateKey(decoded) {
		return nil, ErrDeviceKeyInvalid
	}
	return ed25519.PrivateKey(decoded), nil
}

// Clear deletes the item; an item that is already absent counts as cleared.
func (s *KeychainDeviceKeyStore) Clear(ctx context.Context, ref string) error {
	account, err := keychainAccount(ref)
	if err != nil || s == nil || s.run == nil {
		return ErrDeviceKeyInvalid
	}
	_, code, err := s.run(ctx, []string{"delete-generic-password", "-a", account, "-s", keychainDeviceKeyService}, nil)
	if err != nil {
		return fmt.Errorf("clear device key in keychain: %w", err)
	}
	if code != 0 && code != keychainItemNotFound {
		return fmt.Errorf("clear device key in keychain: security exit %d", code)
	}
	return nil
}

func (s *KeychainDeviceKeyStore) find(ctx context.Context, account string) (string, error) {
	out, code, err := s.run(ctx, []string{"find-generic-password", "-a", account, "-s", keychainDeviceKeyService, "-w"}, nil)
	if err != nil {
		return "", fmt.Errorf("read device key from keychain: %w", err)
	}
	if code == keychainItemNotFound {
		return "", os.ErrNotExist
	}
	if code != 0 {
		return "", fmt.Errorf("read device key from keychain: security exit %d", code)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func keychainAccount(ref string) (string, error) {
	account, ok := strings.CutPrefix(ref, keychainDeviceKeyRefPrefix)
	if !ok || len(account) != 32 || strings.ToLower(account) != account {
		return "", ErrDeviceKeyInvalid
	}
	if _, err := hex.DecodeString(account); err != nil {
		return "", ErrDeviceKeyInvalid
	}
	return account, nil
}

// DeviceKeyCustody routes every reference to the backend that issued it. On macOS
// new keys go to the login Keychain; elsewhere they stay in restrictive files.
// Existing file references ("device-key:") keep working everywhere, and a Keychain
// failure never silently downgrades a new key to a file.
type DeviceKeyCustody struct {
	files    *DeviceKeyStore
	keychain *KeychainDeviceKeyStore
}

func NewDeviceKeyCustody(dataDir string) *DeviceKeyCustody {
	c := &DeviceKeyCustody{files: NewDeviceKeyStore(dataDir)}
	if runtime.GOOS == "darwin" {
		c.keychain = NewKeychainDeviceKeyStore()
	}
	return c
}

func (c *DeviceKeyCustody) Put(ctx context.Context, key ed25519.PrivateKey) (string, error) {
	if c == nil {
		return "", ErrDeviceKeyInvalid
	}
	if c.keychain != nil {
		return c.keychain.Put(ctx, key)
	}
	return c.files.Put(ctx, key)
}

func (c *DeviceKeyCustody) Get(ctx context.Context, ref string) (ed25519.PrivateKey, error) {
	if c == nil {
		return nil, ErrDeviceKeyInvalid
	}
	if strings.HasPrefix(ref, keychainDeviceKeyRefPrefix) {
		if c.keychain == nil {
			return nil, ErrDeviceKeyInvalid
		}
		return c.keychain.Get(ctx, ref)
	}
	return c.files.Get(ctx, ref)
}

func (c *DeviceKeyCustody) Clear(ctx context.Context, ref string) error {
	if c == nil {
		return ErrDeviceKeyInvalid
	}
	if strings.HasPrefix(ref, keychainDeviceKeyRefPrefix) {
		if c.keychain == nil {
			return ErrDeviceKeyInvalid
		}
		return c.keychain.Clear(ctx, ref)
	}
	return c.files.Clear(ctx, ref)
}

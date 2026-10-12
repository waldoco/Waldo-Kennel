package secretstore

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeKeychain models the login keychain behind /usr/bin/security for unit tests.
// Real-keychain verification is the opt-in TestKeychainDeviceKeyStoreLoginKeychain.
type fakeKeychain struct {
	items   map[string]string
	calls   []securityCall
	dropAdd bool   // add appears to succeed but stores nothing
	corrupt string // add stores this value instead
	findErr int    // non-zero exit for find (e.g. locked keychain)
}
type securityCall struct {
	args  []string
	stdin string
}

func (k *fakeKeychain) run(_ context.Context, args []string, stdin []byte) ([]byte, int, error) {
	k.calls = append(k.calls, securityCall{append([]string(nil), args...), string(stdin)})
	flag := func(parts []string, name string) string {
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == name {
				return parts[i+1]
			}
		}
		return ""
	}
	switch args[0] {
	case "-i":
		for _, line := range strings.Split(strings.TrimSpace(string(stdin)), "\n") {
			parts := strings.Fields(line)
			if len(parts) == 0 || parts[0] != "add-generic-password" || flag(parts, "-s") != keychainDeviceKeyService {
				return nil, 0, nil
			}
			if k.dropAdd {
				continue
			}
			value := flag(parts, "-w")
			if k.corrupt != "" {
				value = k.corrupt
			}
			k.items[flag(parts, "-a")] = value
		}
		return []byte("security> "), 0, nil
	case "find-generic-password":
		if k.findErr != 0 {
			return nil, k.findErr, nil
		}
		value, ok := k.items[flag(args, "-a")]
		if !ok || flag(args, "-s") != keychainDeviceKeyService {
			return nil, keychainItemNotFound, nil
		}
		return []byte(value + "\n"), 0, nil
	case "delete-generic-password":
		if _, ok := k.items[flag(args, "-a")]; !ok {
			return nil, keychainItemNotFound, nil
		}
		delete(k.items, flag(args, "-a"))
		return []byte("deleted"), 0, nil
	}
	return nil, 1, nil
}

func newFakeKeychainStore() (*KeychainDeviceKeyStore, *fakeKeychain) {
	k := &fakeKeychain{items: map[string]string{}}
	return &KeychainDeviceKeyStore{run: k.run}, k
}

func testKey(seed byte) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
}

func TestKeychainDeviceKeyStoreCustody(t *testing.T) {
	s, k := newFakeKeychainStore()
	key := testKey(7)
	ref, err := s.Put(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimPrefix(ref, keychainDeviceKeyRefPrefix)
	if !strings.HasPrefix(ref, keychainDeviceKeyRefPrefix) || len(id) != 32 || strings.ToLower(id) != id {
		t.Fatalf("unexpected reference shape %q", ref)
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("reference is not a random hex locator: %q", ref)
	}
	got, err := s.Get(context.Background(), ref)
	if err != nil || !got.Equal(key) {
		t.Fatalf("round trip failed: %v", err)
	}
	if err := s.Clear(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), ref); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleared key still resolves: %v", err)
	}
	if err := s.Clear(context.Background(), ref); err != nil {
		t.Fatalf("repeated clear must be idempotent: %v", err)
	}
	if len(k.items) != 0 {
		t.Fatal("keychain item left behind")
	}
}

func TestKeychainKeyMaterialNeverReachesArgvOrErrors(t *testing.T) {
	key := testKey(9)
	secrets := []string{base64.RawURLEncoding.EncodeToString(key), hex.EncodeToString(key), base64.StdEncoding.EncodeToString(key)}
	s, k := newFakeKeychainStore()
	ref, err := s.Put(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if err = s.Clear(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	sawStdin := false
	for _, call := range k.calls {
		for _, arg := range call.args {
			for _, secret := range secrets {
				if strings.Contains(arg, secret) {
					t.Fatalf("key material in argv of %v", call.args[0])
				}
			}
		}
		if call.args[0] == "-i" {
			if len(call.args) != 1 {
				t.Fatalf("interactive mode must take no further arguments: %v", call.args)
			}
			sawStdin = strings.Contains(call.stdin, secrets[0])
			// One command line of plain tokens: nothing the interactive parser could reinterpret.
			if strings.Count(call.stdin, "\n") != 1 || strings.ContainsAny(strings.TrimSuffix(call.stdin, "\n"), "\"'\\\t\r") {
				t.Fatalf("unexpected interactive input shape")
			}
		}
	}
	if !sawStdin {
		t.Fatal("key was not handed to security over stdin")
	}
	// Failure paths must not echo the key either.
	for _, broken := range []*fakeKeychain{{items: map[string]string{}, dropAdd: true}, {items: map[string]string{}, corrupt: base64.RawURLEncoding.EncodeToString(testKey(1))}} {
		_, err := (&KeychainDeviceKeyStore{run: broken.run}).Put(context.Background(), key)
		if err == nil {
			t.Fatal("unverified write accepted")
		}
		for _, secret := range secrets {
			if strings.Contains(err.Error(), secret) {
				t.Fatal("key material in error")
			}
		}
	}
}

func TestKeychainPutFailsClosedWithoutVerifiedReadBack(t *testing.T) {
	dropped := &fakeKeychain{items: map[string]string{}, dropAdd: true}
	if ref, err := (&KeychainDeviceKeyStore{run: dropped.run}).Put(context.Background(), testKey(3)); err == nil || ref != "" {
		t.Fatal("write that did not land was accepted")
	}
	corrupt := &fakeKeychain{items: map[string]string{}, corrupt: base64.RawURLEncoding.EncodeToString(testKey(4))}
	if ref, err := (&KeychainDeviceKeyStore{run: corrupt.run}).Put(context.Background(), testKey(3)); err == nil || ref != "" {
		t.Fatal("mismatched read-back accepted")
	}
	if len(corrupt.items) != 0 {
		t.Fatal("mismatched item was not removed")
	}
	locked := &fakeKeychain{items: map[string]string{}, findErr: 51}
	if _, err := (&KeychainDeviceKeyStore{run: locked.run}).Put(context.Background(), testKey(3)); err == nil {
		t.Fatal("unavailable keychain accepted a write")
	}
}

func TestKeychainRejectsInvalidReferencesAndMaterialWithoutRunningSecurity(t *testing.T) {
	s, k := newFakeKeychainStore()
	for _, ref := range []string{"", "device-key:" + strings.Repeat("a", 32), keychainDeviceKeyRefPrefix + strings.Repeat("A", 32), keychainDeviceKeyRefPrefix + "abc", keychainDeviceKeyRefPrefix + strings.Repeat("g", 32), keychainDeviceKeyRefPrefix + strings.Repeat("a", 31) + " "} {
		if _, err := s.Get(context.Background(), ref); !errors.Is(err, ErrDeviceKeyInvalid) {
			t.Fatalf("Get accepted %q: %v", ref, err)
		}
		if err := s.Clear(context.Background(), ref); !errors.Is(err, ErrDeviceKeyInvalid) {
			t.Fatalf("Clear accepted %q: %v", ref, err)
		}
	}
	if _, err := s.Put(context.Background(), ed25519.PrivateKey(make([]byte, 10))); !errors.Is(err, ErrDeviceKeyInvalid) {
		t.Fatal("invalid key material accepted")
	}
	if len(k.calls) != 0 {
		t.Fatal("security ran for invalid input")
	}
	if securityPath != "/usr/bin/security" {
		t.Fatal("security must run from its absolute system path, never PATH lookup")
	}
}

func TestDeviceKeyCustodyRoutesByReference(t *testing.T) {
	dir := t.TempDir()
	files := NewDeviceKeyStore(dir)
	legacy, err := files.Put(context.Background(), testKey(5))
	if err != nil {
		t.Fatal(err)
	}
	keychain, k := newFakeKeychainStore()
	mac := &DeviceKeyCustody{files: files, keychain: keychain}
	ref, err := mac.Put(context.Background(), testKey(6))
	if err != nil || !strings.HasPrefix(ref, keychainDeviceKeyRefPrefix) {
		t.Fatalf("macOS custody must issue Keychain references: %q %v", ref, err)
	}
	for r, want := range map[string]ed25519.PrivateKey{legacy: testKey(5), ref: testKey(6)} {
		got, err := mac.Get(context.Background(), r)
		if err != nil || !got.Equal(want) {
			t.Fatalf("reference %q not served by its issuing backend: %v", r, err)
		}
	}
	if err := mac.Clear(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Get(context.Background(), legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("file reference not cleared by file custody")
	}
	if err := mac.Clear(context.Background(), ref); err != nil || len(k.items) != 0 {
		t.Fatal("Keychain reference not cleared by Keychain custody")
	}

	other := &DeviceKeyCustody{files: files}
	ref, err = other.Put(context.Background(), testKey(8))
	if err != nil || !strings.HasPrefix(ref, deviceKeyRefPrefix) {
		t.Fatalf("non-macOS custody must stay file-backed: %q %v", ref, err)
	}
	if _, err := other.Get(context.Background(), keychainDeviceKeyRefPrefix+strings.Repeat("a", 32)); !errors.Is(err, ErrDeviceKeyInvalid) {
		t.Fatal("Keychain reference accepted where no Keychain backend exists")
	}
}

// TestKeychainDeviceKeyStoreLoginKeychain is the one-time, owner-approved real
// verification: it creates exactly one generic-password item in the login
// Keychain, reads it back, deletes it, and confirms it is gone. Opt-in only.
func TestKeychainDeviceKeyStoreLoginKeychain(t *testing.T) {
	if os.Getenv("KENNEL_KEYCHAIN_INTEGRATION") != "1" {
		t.Skip("set KENNEL_KEYCHAIN_INTEGRATION=1 on macOS to touch the login Keychain")
	}
	s := NewKeychainDeviceKeyStore()
	key := testKey(11)
	ref, err := s.Put(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Clear(context.Background(), ref) })
	t.Logf("created login Keychain item: service=%s account=%s", keychainDeviceKeyService, strings.TrimPrefix(ref, keychainDeviceKeyRefPrefix))
	got, err := s.Get(context.Background(), ref)
	if err != nil || !got.Equal(key) {
		t.Fatalf("read-back failed: %v", err)
	}
	if err = s.Clear(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(context.Background(), ref); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("item still present after delete: %v", err)
	}
	t.Logf("deleted login Keychain item: service=%s account=%s", keychainDeviceKeyService, strings.TrimPrefix(ref, keychainDeviceKeyRefPrefix))
}

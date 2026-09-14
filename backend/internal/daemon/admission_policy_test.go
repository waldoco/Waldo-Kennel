package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAdmissionPolicyRejectsUnsafeFiles(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "admission-policy.json")
	if err := os.WriteFile(p, []byte(`{"unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAdmissionPolicy(d); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", p); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAdmissionPolicy(d); err == nil {
		t.Fatal("symlink accepted")
	}
}

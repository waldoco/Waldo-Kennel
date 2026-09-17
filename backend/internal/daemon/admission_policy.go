package daemon

import (
	"encoding/json"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"io"
	"os"
	"path/filepath"
)

func loadAdmissionPolicy(dataDir string) (*domain.AdmissionPolicy, error) {
	path := filepath.Join(dataDir, "admission-policy.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("admission policy must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("admission policy permissions must not grant group/other access")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var p domain.AdmissionPolicy
	if err := dec.Decode(&p); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("admission policy must contain exactly one JSON value")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

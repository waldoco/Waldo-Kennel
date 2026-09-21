package daemon

import (
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/admissionpolicy"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func loadAdmissionPolicy(dataDir string) (*domain.AdmissionPolicy, error) {
	return admissionpolicy.Load(dataDir)
}

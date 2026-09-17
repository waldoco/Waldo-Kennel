package domain

import (
	"errors"
	"runtime"
	"strings"
	"time"
)

type HarnessAdapterInstallState string

const (
	AdapterInstallIdle                   HarnessAdapterInstallState = "idle"
	AdapterInstallPrepared               HarnessAdapterInstallState = "prepared"
	AdapterInstallVerified               HarnessAdapterInstallState = "verified"
	AdapterInstallActivatedPendingHealth HarnessAdapterInstallState = "activated_pending_health"
	AdapterInstallCommitted              HarnessAdapterInstallState = "committed"
	AdapterInstallRolledBack             HarnessAdapterInstallState = "rolled_back"
	AdapterInstallActionNeeded           HarnessAdapterInstallState = "action_needed"
)

type HarnessAdapterDrift string

const (
	AdapterDriftInSync                    HarnessAdapterDrift = "in_sync"
	AdapterDriftUpgradeAvailable          HarnessAdapterDrift = "upgrade_available"
	AdapterDriftOptionalDegradation       HarnessAdapterDrift = "optional_degradation"
	AdapterDriftRequiredCapabilityMissing HarnessAdapterDrift = "required_capability_missing"
	AdapterDriftProtocol                  HarnessAdapterDrift = "protocol_drift"
	AdapterDriftArtifactTamper            HarnessAdapterDrift = "artifact_tamper"
	AdapterDriftLocalModification         HarnessAdapterDrift = "local_modification"
	AdapterDriftSource                    HarnessAdapterDrift = "source_drift"
	AdapterDriftRollbackAttempt           HarnessAdapterDrift = "rollback_attempt"
	AdapterDriftMetadataExpired           HarnessAdapterDrift = "metadata_expired"
	AdapterDriftMixedGeneration           HarnessAdapterDrift = "mixed_generation"
	AdapterDriftActivationIncomplete      HarnessAdapterDrift = "activation_incomplete"
	AdapterDriftRollbackFailed            HarnessAdapterDrift = "rollback_failed"
)

type HarnessAdapterInstallRepair string

const (
	AdapterRepairStageUpgrade      HarnessAdapterInstallRepair = "stage_upgrade"
	AdapterRepairReviewDegraded    HarnessAdapterInstallRepair = "review_degraded_capability"
	AdapterRepairKeepCurrent       HarnessAdapterInstallRepair = "keep_current_and_repair_compatibility"
	AdapterRepairQuarantineRestore HarnessAdapterInstallRepair = "quarantine_and_restore_last_known_good"
	AdapterRepairReviewLocal       HarnessAdapterInstallRepair = "review_local_drift"
	AdapterRepairReviewSource      HarnessAdapterInstallRepair = "review_installation_source"
	AdapterRepairRejectCandidate   HarnessAdapterInstallRepair = "reject_candidate"
	AdapterRepairRefreshMetadata   HarnessAdapterInstallRepair = "refresh_release_metadata"
	AdapterRepairReconcileRollback HarnessAdapterInstallRepair = "reconcile_or_rollback"
	AdapterRepairManual            HarnessAdapterInstallRepair = "manual_repair"
)

// TrustedHarnessAdapterRelease is supplied only by a trusted release resolver.
// Its shape cannot itself establish trust; TrustRootID identifies the resolver's
// already-verified root and is retained as provenance.
type TrustedHarnessAdapterRelease struct {
	TrustRootID    string
	AdapterID      string
	Version        string
	Sequence       uint64
	ArtifactDigest SHA256Digest
	ManifestDigest SHA256Digest
	OS             string
	Architecture   string
	Size           int64
	ExpiresAt      time.Time
}

func (r TrustedHarnessAdapterRelease) Validate(now time.Time) error {
	if strings.TrimSpace(r.TrustRootID) == "" || strings.TrimSpace(r.AdapterID) == "" ||
		strings.TrimSpace(r.Version) == "" || r.Sequence == 0 || !r.ArtifactDigest.Valid() ||
		!r.ManifestDigest.Valid() || strings.TrimSpace(r.OS) == "" || strings.TrimSpace(r.Architecture) == "" ||
		r.Size <= 0 || r.ExpiresAt.IsZero() || !now.Before(r.ExpiresAt) {
		return ErrHarnessAdapterReleaseInvalid
	}
	return nil
}
func (r TrustedHarnessAdapterRelease) MatchesRuntime() bool {
	return r.OS == runtime.GOOS && r.Architecture == runtime.GOARCH
}

var (
	ErrHarnessAdapterReleaseInvalid     = errors.New("invalid or expired trusted adapter release")
	ErrHarnessAdapterInstallConflict    = errors.New("adapter install operation conflict")
	ErrHarnessAdapterInstallBusy        = errors.New("adapter install operation already active")
	ErrHarnessAdapterPairingUnavailable = errors.New("S3.3 adapter pairing activation is not implemented")
)

type HarnessAdapterInstallOperation struct {
	ID              string                      `json:"id"`
	RequestDigest   SHA256Digest                `json:"request_digest"`
	State           HarnessAdapterInstallState  `json:"state"`
	Drift           HarnessAdapterDrift         `json:"drift,omitempty"`
	Repair          HarnessAdapterInstallRepair `json:"repair,omitempty"`
	CandidateDigest SHA256Digest                `json:"candidate_digest"`
	PreviousDigest  SHA256Digest                `json:"previous_digest,omitempty"`
	ReleaseSequence uint64                      `json:"release_sequence"`
	CandidatePath   string                      `json:"candidate_path,omitempty"`
	PreviousPath    string                      `json:"previous_path,omitempty"`
	Failure         string                      `json:"failure,omitempty"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
}

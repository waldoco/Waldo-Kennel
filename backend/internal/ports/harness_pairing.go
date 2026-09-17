package ports

import (
	"context"
	"net"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// HarnessPairingChallengeStore durably tracks the one-time pairing/rotation
// challenge lifecycle. It never receives or persists a raw challenge secret;
// only domain.HarnessPairingChallenge.ProofVerifier (a one-way hash) crosses
// this boundary.
type HarnessPairingChallengeStore interface {
	// SupersedePendingHarnessPairingChallenges marks every still-pending
	// challenge for connectionID as superseded, so issuing a fresh challenge
	// invalidates any prior one for the same connection.
	// ReplacePendingHarnessPairingChallenge atomically supersedes prior pending
	// intents for the connection and inserts this owner-created intent.
	ReplacePendingHarnessPairingChallenge(ctx context.Context, challenge domain.HarnessPairingChallenge) (domain.HarnessPairingChallenge, error)
	GetHarnessPairingChallenge(ctx context.Context, id domain.PairingChallengeID) (domain.HarnessPairingChallenge, bool, error)
	// ConsumeHarnessPairingChallenge is the single atomic point of no return:
	// it transitions pending -> consumed only when unexpired, and reports
	// whether this exact call won that transition. Losing this call must
	// never call the S3.1 kernel and must never mint a bearer.
	ConsumeHarnessPairingChallenge(ctx context.Context, id domain.PairingChallengeID, now time.Time) (bool, error)
	// RecordHarnessPairingResult attaches the terminal, non-secret audit
	// result code. It only ever sets a previously-unset result_code; a crash
	// before this call leaves the challenge consumed with no recorded
	// result, which is retained evidence, not a fabricated outcome.
	RecordHarnessPairingResult(ctx context.Context, id domain.PairingChallengeID, code domain.HarnessPairingResultCode, now time.Time) (bool, error)
}

// LocalPeerVerifier checks that a connected local transport peer satisfies
// the platform identity/permission check before any pairing RPC is served.
// It answers only "is this peer allowed at all" — never who the peer claims
// to be at the protocol layer, which remains the coordinator's job.
type LocalPeerVerifier interface {
	VerifyLocalPeer(conn net.Conn) error
}

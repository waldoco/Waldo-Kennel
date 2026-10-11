// Package bridgeruntime connects an already paired identity. It never creates
// identities, redeems codes, or authorizes owner commands.
package bridgeruntime

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
)

// UnknownQueries intentionally ships no machine evidence, so queries answer
// unknown. Notifications use the injected Display (DisplayRelay); without one,
// or without the shell's acknowledgement, they stay delivery_unknown.
type UnknownQueries struct{}

var _ devicebridge.QueryEvidenceSource = UnknownQueries{}

func (UnknownQueries) QueryState(context.Context, devicebridge.Scope, string, string) (string, bool, error) {
	return "", false, nil
}

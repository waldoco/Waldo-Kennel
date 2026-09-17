#!/usr/bin/env bash
# Bounded native evidence runner for the S3.3 pairing-challenge transport.
#
# Stands up the real protected local Unix-socket transport (Listen/Server/
# Client, not a mock), drives a full pair -> prove -> bearer -> authenticate
# -> restart flow plus the required adversarial cases, and greps every
# reachable surface for a known canary secret/bearer. See
# backend/internal/adapters/harnesspairing/native_proof_test.go for exactly
# what is genuinely native versus an explicit, honest gap on this platform.
#
# This is a proof runner, not a product installer: it starts no daemon and
# changes no persistent product state outside its own temp directory.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root/backend"

KENNEL_STAGE3_S33_NATIVE_PROOF=1 go test -run TestBoundedNativeMacOSTransportProof -v ./internal/adapters/harnesspairing/...

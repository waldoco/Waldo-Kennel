#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
out=${1:-"$root/native-codex-substrate-evidence"}
mkdir -p "$out"

bin=${KENNEL_CODEX_BIN:-codex}
resolved=$(command -v "$bin") || { echo "Codex binary not found: $bin" >&2; exit 1; }
version=$($resolved --version 2>&1)
if command -v shasum >/dev/null; then
  sha=$(shasum -a 256 "$resolved" | awk '{print $1}')
  hash_log() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  sha=$(sha256sum "$resolved" | awk '{print $1}')
  hash_log() { sha256sum "$1" | awk '{print $1}'; }
fi
source_sha=$(git -C "$root" rev-parse HEAD)
if [[ -n $(git -C "$root" status --porcelain) ]]; then
  source_dirty=true
else
  source_dirty=false
fi
started=$(date -u +%Y-%m-%dT%H:%M:%SZ)
log="$out/live-test.log"

# The Go test receives only the explicit variables below. liveCodexEnv applies a
# second allowlist before starting app-server. Secrets are never printed.
set +e
(
  cd "$root/backend"
  KENNEL_CODEX_LIVE=1 KENNEL_CODEX_BIN="$resolved" \
    go test ./internal/adapters/chatdriver/codexappserver \
      -run 'TestLive(PersistentCodexSubstrate|SteerKeepsTheTurnAndItsWork)$' \
      -count=1 -v
) >"$log" 2>&1
status=$?
set -e

if [[ $status -ne 0 ]]; then
  echo "Live proof failed ($status)" >&2
  exit "$status"
fi
if grep -Eq -- '--- SKIP:|^[[:space:]]*SKIP[[:space:]]*$' "$log"; then
  echo "Live proof skipped; refusing evidence" >&2
  exit 1
fi
for test in TestLivePersistentCodexSubstrate TestLiveSteerKeepsTheTurnAndItsWork; do
  grep -Fq -- "--- PASS: $test" "$log" || { echo "Missing PASS: $test" >&2; exit 1; }
done
for marker in \
  'profile=codex_native_worktree_v1' \
  'failed_test_observed=true' \
  'repaired_test_passed=true' \
  'filesystem_escape_denied=true' \
  'network_denied=true' \
  'continuation_turn=' \
  'git_status_short='; do
  grep -Fq -- "$marker" "$log" || { echo "Missing evidence marker: $marker" >&2; exit 1; }
done
finished=$(date -u +%Y-%m-%dT%H:%M:%SZ)
log_sha=$(hash_log "$log")
cat >"$out/manifest.txt" <<MANIFEST
source_commit=$source_sha
source_dirty=$source_dirty
profile=codex_native_worktree_v1
profile_permissions=accept-edits
profile_network=denied-unless-separately-authorized
started_utc=$started
finished_utc=$finished
platform=$(uname -s)
architecture=$(uname -m)
go_version=$(go version)
codex_binary=$resolved
codex_version=$version
codex_binary_sha256=$sha
log_sha256=$log_sha
result=PASS
MANIFEST
cat "$out/manifest.txt"

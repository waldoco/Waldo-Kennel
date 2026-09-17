#!/usr/bin/env bash
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
out=${1:-"$root/native-codex-substrate-evidence"}
if [[ -d "$out" && -n $(find "$out" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null) ]]; then
  echo "Evidence directory is not empty: $out" >&2
  exit 2
fi
mkdir -p "$out"
log="$out/live-test.log"
stdout_log="$out/live-test.stdout.log"
stderr_log="$out/live-test.stderr.log"
manifest="$out/manifest.txt"
: >"$log"
: >"$stdout_log"
: >"$stderr_log"

source_sha=$(git -C "$root" rev-parse HEAD 2>/dev/null || printf unknown)
if [[ -n $(git -C "$root" status --porcelain 2>/dev/null) ]]; then source_dirty=true; else source_dirty=false; fi
started=$(date -u +%Y-%m-%dT%H:%M:%SZ)
selected=unresolved
canonical=unresolved
version=unresolved
binary_sha=unresolved
status=1
result=FAIL
reason=setup_failed

hash_file() {
  if command -v shasum >/dev/null; then shasum -a 256 "$1" | awk '{print $1}'; else sha256sum "$1" | awk '{print $1}'; fi
}
finalize() {
  local shell_status=$?
  if [[ $status -eq 0 && $shell_status -ne 0 ]]; then status=$shell_status; fi
  local finished log_sha stdout_sha stderr_sha
  finished=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  {
    printf '%s
' '--- STDOUT ---'
    cat "$stdout_log"
    printf '%s
' '--- STDERR ---'
    cat "$stderr_log"
  } >"$log"
  sed -n 's/^.*policy_record=//p' "$stdout_log" >"$out/policy-records.raw.jsonl"
  sed -n 's/^.*interrupt_quiescence=//p' "$stdout_log" >"$out/interrupt-quiescence.jsonl"
  log_sha=$(hash_file "$log" 2>/dev/null || printf unavailable)
  stdout_sha=$(hash_file "$stdout_log" 2>/dev/null || printf unavailable)
  stderr_sha=$(hash_file "$stderr_log" 2>/dev/null || printf unavailable)
  cat >"$manifest" <<MANIFEST
source_commit=$source_sha
source_dirty=$source_dirty
profile=codex_native_worktree_v1
profile_permissions=accept-edits
profile_network=denied-unless-separately-authorized
started_utc=$started
finished_utc=$finished
platform=$(uname -s)
architecture=$(uname -m)
go_version=$(go version 2>&1)
codex_selected_path=$selected
codex_canonical_path=$canonical
codex_version=$version
codex_binary_sha256=$binary_sha
test_exit_status=$status
log_sha256=$log_sha
stdout_sha256=$stdout_sha
stderr_sha256=$stderr_sha
result=$result
reason=$reason
MANIFEST
  cat "$manifest"
}
trap finalize EXIT

identity=$(cd "$root/backend" && go run ./cmd/kennel-codex-runtime-identity 2>>"$stderr_log") || {
  status=$?; reason=runtime_identity_resolution_failed; exit "$status";
}
selected=$(printf '%s' "$identity" | sed -n 's/.*"selected_path":"\([^"]*\)".*/\1/p')
canonical=$(printf '%s' "$identity" | sed -n 's/.*"canonical_path":"\([^"]*\)".*/\1/p')
if [[ -z "$selected" || -z "$canonical" || ! -x "$canonical" ]]; then
  status=1; reason=invalid_runtime_identity; exit "$status"
fi
version=$($canonical --version 2>>"$stderr_log") || { status=$?; reason=version_probe_failed; exit "$status"; }
binary_sha=$(hash_file "$canonical") || { status=$?; reason=binary_hash_failed; exit "$status"; }
printf 'runtime_identity selected=%q canonical=%q\n' "$selected" "$canonical" >>"$stdout_log"

set +e
(
  cd "$root/backend"
  KENNEL_CODEX_LIVE=1 KENNEL_CODEX_BIN="$canonical" KENNEL_CODEX_SELECTED_BIN="$selected" \
    go test ./internal/adapters/chatdriver/codexappserver -run '^TestLiveCodexRuntimeCanary$' -count=1 -v &&
  KENNEL_CODEX_LIVE=1 KENNEL_CODEX_BIN="$canonical" KENNEL_CODEX_SELECTED_BIN="$selected" \
    go test ./internal/adapters/chatdriver/codexappserver \
      -run 'TestLive(PersistentCodexSubstrate|SteerKeepsTheTurnAndItsWork|NativePrerequisiteDiagnostics)$' -count=1 -v
) >>"$stdout_log" 2>>"$stderr_log"
status=$?
set -e
{
  printf '%s
' '--- STDOUT ---'
  cat "$stdout_log"
  printf '%s
' '--- STDERR ---'
  cat "$stderr_log"
} >"$log"
if [[ $status -ne 0 ]]; then reason=test_failed; exit "$status"; fi
if grep -Eq -- '--- SKIP:|^[[:space:]]*SKIP[[:space:]]*$' "$log"; then
  status=3; result=SKIP; reason=test_skipped; exit "$status"
fi
for test in TestLiveCodexRuntimeCanary TestLivePersistentCodexSubstrate TestLiveSteerKeepsTheTurnAndItsWork TestLiveNativePrerequisiteDiagnostics; do
  grep -Fq -- "--- PASS: $test" "$log" || { status=1; reason="missing_pass_$test"; exit "$status"; }
done
for marker in \
  'runtime_canary=true' \
  'profile=codex_native_worktree_v1' \
  'expected_assertion_observed=true' \
  'repaired_test_passed=true' \
  'filesystem_escape_denied=true' \
  'network_denied=true' \
  'post_resume_filesystem_denied=true' \
  'post_resume_network_denied=true' \
  'interrupt_quiescence=' \
  'continuation_turn=' \
  'git_status_short='; do
  grep -Fq -- "$marker" "$log" || { status=1; reason="missing_evidence_marker"; exit "$status"; }
done
result=PASS
reason=all_gates_passed
status=0
exit 0

#!/usr/bin/env bash
# Fixed headless Codex delegation entrypoint. The prompt is data from the
# environment and is passed as one argv element, never evaluated as shell code.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

: "${CODEX_DELEGATE_PROMPT:?CODEX_DELEGATE_PROMPT is required}"
: "${CODEX_DELEGATE_TIMEOUT_MINUTES:?CODEX_DELEGATE_TIMEOUT_MINUTES is required}"
: "${CODEX_DELEGATE_ROOT:?CODEX_DELEGATE_ROOT is required}"
CODEX_DELEGATE_RESUME_FROM="${CODEX_DELEGATE_RESUME_FROM:-}"

case "$CODEX_DELEGATE_TIMEOUT_MINUTES" in
	'' | *[!0-9]*) echo "timeout_minutes must be an integer" >&2; exit 2 ;;
esac
if ((CODEX_DELEGATE_TIMEOUT_MINUTES < 1 || CODEX_DELEGATE_TIMEOUT_MINUTES > 120)); then
	echo "timeout_minutes must be between 1 and 120" >&2
	exit 2
fi
if ! command -v codex >/dev/null 2>&1; then
	echo "codex is not available to the runner service" >&2
	exit 127
fi

mkdir -p "$CODEX_DELEGATE_ROOT/work-product"
output="$CODEX_DELEGATE_ROOT/output.log"
diagnostic="$CODEX_DELEGATE_ROOT/diagnostic.log"
status_file="$CODEX_DELEGATE_ROOT/work-product/git-status.txt"
diff_file="$CODEX_DELEGATE_ROOT/work-product/git-diff.patch"
: > "$output"

finish() {
	result=$?
	# Capture repository work even when Codex fails or times out. Intent-to-add
	# makes untracked work visible in the binary patch without committing it.
	git status --short --untracked-files=all > "$status_file" 2>&1 || true
	git add -N -- . >/dev/null 2>&1 || true
	git diff --binary --no-ext-diff HEAD -- > "$diff_file" 2>&1 || true
	{
		printf 'Kennel Codex delegation\n'
		printf 'status=%s\n' "$result"
		printf 'revision=%s\n' "$(git rev-parse HEAD 2>/dev/null || printf unknown)"
		if [[ "$result" -eq 0 ]]; then
			printf 'proof=passed: Codex exec completed and work-product capture ran\n'
		else
			printf 'proof=failed: Codex exec did not complete successfully\n'
		fi
		printf 'note=No commit or push was attempted; work product is git status plus binary diff\n'
		printf '\nLast 250 output lines:\n'
		tail -n 250 "$output"
	} > "$diagnostic"
	cat "$diagnostic"
	exit "$result"
}
trap finish EXIT

# This matches Kennel's grounded one-shot shapes: `codex exec -- <prompt>`
# and `codex exec resume <session UUID> -- <prompt>`. Remove GitHub command-file
# variables and prompt/control variables from the child. Codex retains HOME/PATH
# for the runner-service identity and login.
python3 - "$CODEX_DELEGATE_TIMEOUT_MINUTES" "$output" "$(command -v codex)" "$CODEX_DELEGATE_PROMPT" "$CODEX_DELEGATE_RESUME_FROM" <<'PY'
import os
import signal
import subprocess
import sys

timeout_minutes = int(sys.argv[1])
output_path, codex, prompt, resume_from = sys.argv[2:6]
if resume_from:
    import re
    if not re.fullmatch(r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}", resume_from):
        print("resume_from must be a Codex session UUID", file=sys.stderr)
        sys.exit(2)
env = os.environ.copy()
for key in (
    "GITHUB_ENV", "GITHUB_OUTPUT", "GITHUB_PATH", "GITHUB_STEP_SUMMARY",
    "CODEX_DELEGATE_PROMPT", "CODEX_DELEGATE_RESUME_FROM",
    "CODEX_DELEGATE_TIMEOUT_MINUTES", "CODEX_DELEGATE_ROOT",
):
    env.pop(key, None)

argv = [codex, "exec"]
if resume_from:
    argv += ["resume", resume_from]
argv += ["--", prompt]
with open(output_path, "ab", buffering=0) as stream:
    proc = subprocess.Popen(
        argv,
        cwd=os.environ["GITHUB_WORKSPACE"] + "/candidate",
        env=env,
        stdout=stream,
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    try:
        code = proc.wait(timeout=timeout_minutes * 60)
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGTERM)
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGKILL)
            proc.wait()
        print(f"Codex delegation timed out after {timeout_minutes} minute(s)", file=sys.stderr)
        sys.exit(124)
sys.exit(code)
PY

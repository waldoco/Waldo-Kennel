#!/usr/bin/env bash
# Trusted headless Codex wrapper. This file is executed only from the exact
# protected commit that contains the workflow, never from the candidate tree.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: codex-delegate.sh <candidate-workspace>" >&2
	exit 2
fi
: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is required}"
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
: "${CODEX_DELEGATE_PROMPT:?CODEX_DELEGATE_PROMPT is required}"
: "${CODEX_DELEGATE_TIMEOUT_MINUTES:?CODEX_DELEGATE_TIMEOUT_MINUTES is required}"
: "${CODEX_DELEGATE_ROOT:?CODEX_DELEGATE_ROOT is required}"
CODEX_DELEGATE_RESUME_FROM="${CODEX_DELEGATE_RESUME_FROM:-}"

workspace="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$1")"
expected_workspace="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$GITHUB_WORKSPACE/candidate")"
if [[ "$workspace" != "$expected_workspace" || ! -d "$workspace" || -L "$1" ]]; then
	echo "candidate workspace is not the canonical candidate checkout" >&2
	exit 2
fi
root="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$CODEX_DELEGATE_ROOT")"
case "$root" in
	"$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$RUNNER_TEMP")"/waldo-kennel-codex-delegate/*) ;;
	*) echo "delegation artifact root is outside RUNNER_TEMP" >&2; exit 2 ;;
esac
if [[ ! -d "$root" || -L "$CODEX_DELEGATE_ROOT" || ! -d "$root/work-product" || -L "$root/work-product" ]]; then
	echo "delegation artifact directories must be real directories" >&2
	exit 2
fi

case "$CODEX_DELEGATE_TIMEOUT_MINUTES" in
	'' | *[!0-9]*) echo "timeout_minutes must be an integer" >&2; exit 2 ;;
esac
if ((CODEX_DELEGATE_TIMEOUT_MINUTES < 1 || CODEX_DELEGATE_TIMEOUT_MINUTES > 120)); then
	echo "timeout_minutes must be between 1 and 120" >&2
	exit 2
fi
codex="$(command -v codex)"
git_bin="$(command -v git)"
trusted_path="$PATH"
initial_head="$(env PATH="$trusted_path" "$git_bin" -C "$workspace" rev-parse --verify HEAD)"

output="$root/output.log"
diagnostic="$root/diagnostic.log"
status_file="$root/work-product/git-status.txt"
diff_file="$root/work-product/git-diff.patch"
for file in "$output" "$diagnostic" "$status_file" "$diff_file"; do
	if [[ -e "$file" && (! -f "$file" || -L "$file") ]]; then
		echo "artifact path must be a regular non-symlink file: $file" >&2
		exit 2
	fi
done
: > "$output"

capture_work_product() {
	local capture=0 current_head temp_index temp_status temp_diff
	temp_index="$(mktemp "$root/work-product/.index.XXXXXX")" || return 1
	temp_status="$(mktemp "$root/work-product/.status.XXXXXX")" || capture=1
	temp_diff="$(mktemp "$root/work-product/.diff.XXXXXX")" || capture=1
	if [[ "$capture" -eq 0 ]]; then
		current_head="$(env PATH="$trusted_path" "$git_bin" -C "$workspace" rev-parse --verify HEAD)" || capture=1
		if [[ "$current_head" != "$initial_head" ]]; then
			printf 'HEAD changed: initial=%s current=%s\n' "$initial_head" "$current_head" > "$temp_status"
			capture=1
		fi
	fi
	if [[ "$capture" -eq 0 ]]; then
		rm -f "$temp_index"
		GIT_INDEX_FILE="$temp_index" env PATH="$trusted_path" "$git_bin" -C "$workspace" read-tree "$initial_head" || capture=1
		GIT_INDEX_FILE="$temp_index" env PATH="$trusted_path" "$git_bin" -C "$workspace" add -N -- . || capture=1
		env PATH="$trusted_path" "$git_bin" -C "$workspace" status --short --untracked-files=all > "$temp_status" || capture=1
		GIT_INDEX_FILE="$temp_index" env PATH="$trusted_path" "$git_bin" -C "$workspace" diff --binary --no-ext-diff "$initial_head" -- > "$temp_diff" || capture=1
	fi
	if [[ -f "$temp_status" && ! -L "$temp_status" ]]; then mv -f "$temp_status" "$status_file"; else capture=1; fi
	if [[ -f "$temp_diff" && ! -L "$temp_diff" ]]; then mv -f "$temp_diff" "$diff_file"; else capture=1; fi
	rm -f "$temp_index" "$temp_status" "$temp_diff"
	return "$capture"
}

finish() {
	codex_status=$?
	trap - EXIT
	capture_status=0
	capture_work_product || capture_status=$?
	if [[ "$codex_status" -eq 0 && "$capture_status" -eq 0 ]]; then final_status=0; else final_status=1; fi
	{
		printf 'Kennel Codex delegation\n'
		printf 'codex_status=%s\n' "$codex_status"
		printf 'capture_status=%s\n' "$capture_status"
		printf 'initial_revision=%s\n' "$initial_head"
		if [[ "$final_status" -eq 0 ]]; then
			printf 'proof=passed: Codex exec completed and best-effort working-tree patch capture succeeded\n'
		else
			printf 'proof=failed: Codex execution or work-product capture failed\n'
		fi
		printf 'capture_limit=Patch omits ignored files, nested repositories, empty directories, commits, and index-only changes\n'
		printf 'authority=Wrapper does not request commit or push; Codex retains service-account ambient git, network, HOME, and credential authority\n'
		printf '\nLast 250 output lines:\n'
		tail -n 250 "$output"
	} > "$diagnostic"
	cat "$diagnostic"
	exit "$final_status"
}
trap finish EXIT

# Grounded one-shot forms: `codex exec -- <prompt>` and
# `codex exec resume <UUID> -- <prompt>`. resume_from validation is syntax-only;
# resuming an unrelated owner session can import its context into uploaded output.
# Command-file stripping prevents GitHub command-file poisoning. It is not secret
# containment: HOME, user files, auth, git credentials, and network remain reachable.
python3 - "$CODEX_DELEGATE_TIMEOUT_MINUTES" "$output" "$codex" "$CODEX_DELEGATE_PROMPT" "$CODEX_DELEGATE_RESUME_FROM" "$workspace" <<'PY'
import os, re, signal, subprocess, sys

timeout_minutes = int(sys.argv[1])
output_path, codex, prompt, resume_from, workspace = sys.argv[2:7]
if resume_from and not re.fullmatch(r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}", resume_from):
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
    proc = subprocess.Popen(argv, cwd=workspace, env=env, stdout=stream, stderr=subprocess.STDOUT, start_new_session=True)
    try:
        code = proc.wait(timeout=timeout_minutes * 60)
    except subprocess.TimeoutExpired:
        # Best-effort cleanup of this process group; descendants can escape it.
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

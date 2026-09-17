#!/usr/bin/env bash
# Trusted headless Codex wrapper. Executed only from the exact protected workflow
# commit, never from the candidate tree.
set -euo pipefail
umask 077

if [[ $# -ne 1 ]]; then echo "usage: codex-delegate.sh <candidate-workspace>" >&2; exit 2; fi
: "${GITHUB_WORKSPACE:?}" "${RUNNER_TEMP:?}" "${GITHUB_ENV:?}"
: "${CODEX_DELEGATE_PROMPT:?}" "${CODEX_DELEGATE_TIMEOUT_MINUTES:?}"
CODEX_DELEGATE_RESUME_FROM="${CODEX_DELEGATE_RESUME_FROM:-}"

real() { python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$1"; }
temp_real="$(real "$RUNNER_TEMP")"
workspace="$(real "$1")"
expected_workspace="$(real "$GITHUB_WORKSPACE/candidate")"
if [[ "$workspace" != "$expected_workspace" || ! -d "$workspace" || -L "$1" ]]; then
	echo "candidate workspace is not the canonical candidate checkout" >&2; exit 2
fi
case "$CODEX_DELEGATE_TIMEOUT_MINUTES" in '' | *[!0-9]*) echo "timeout_minutes must be an integer" >&2; exit 2;; esac
if ((CODEX_DELEGATE_TIMEOUT_MINUTES < 1 || CODEX_DELEGATE_TIMEOUT_MINUTES > 120)); then
	echo "timeout_minutes must be between 1 and 120" >&2; exit 2
fi
codex="$(command -v codex)"
git_bin="$(command -v git)"
trusted_path="$PATH"
initial_head="$(env PATH="$trusted_path" "$git_bin" -C "$workspace" rev-parse --verify HEAD)"

# Private staging name is random and is not passed to Codex. Codex still has the
# service account's host authority; this reduces accidental collision, not access.
staging="$(mktemp -d "$RUNNER_TEMP/.codex-delegate-stage.XXXXXX")"
case "$(real "$staging")" in "$temp_real"/.codex-delegate-stage.*) ;; *) exit 2;; esac
raw_output="$staging/output.raw"
: > "$raw_output"

safe_new_dir() {
	local dir="$1" parent
	parent="$(dirname "$dir")"
	[[ -d "$parent" && ! -L "$parent" && ! -e "$dir" && ! -L "$dir" ]] || return 1
	mkdir -m 700 "$dir"
	[[ -d "$dir" && ! -L "$dir" ]]
}
safe_install() {
	local source="$1" destination="$2" parent
	parent="$(dirname "$destination")"
	[[ -f "$source" && ! -L "$source" && -d "$parent" && ! -L "$parent" ]] || return 1
	[[ ! -e "$destination" && ! -L "$destination" ]] || return 1
	mv "$source" "$destination"
	[[ -f "$destination" && ! -L "$destination" ]]
}

git_clean_env() {
	env -i \
		PATH="$trusted_path" HOME="$capture_home" \
		GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_OPTIONAL_LOCKS=0 \
		GIT_INDEX_FILE="$capture_index" \
		"$git_bin" -c core.hooksPath=/dev/null -c core.fsmonitor=false "$@"
}

finish() {
	codex_status=$?
	trap - EXIT
	capture_status=0

	# Re-resolve all roots after Codex exits. Final artifacts are created in a new,
	# unpredictable directory that was not exposed to the child.
	temp_real="$(real "$RUNNER_TEMP")" || capture_status=1
	final_root="$(mktemp -d "$RUNNER_TEMP/waldo-kennel-codex-delegate.capture.XXXXXX")" || capture_status=1
	if [[ "$capture_status" -eq 0 ]]; then
		case "$(real "$final_root")" in "$temp_real"/waldo-kennel-codex-delegate.capture.*) ;; *) capture_status=1;; esac
		[[ -d "$final_root" && ! -L "$final_root" ]] || capture_status=1
	fi
	work_product="$final_root/work-product"
	if [[ "$capture_status" -eq 0 ]]; then safe_new_dir "$work_product" || capture_status=1; fi

	capture_home="$(mktemp -d "$RUNNER_TEMP/.codex-capture-home.XXXXXX")" || capture_status=1
	capture_index="$(mktemp "$RUNNER_TEMP/.codex-capture-index.XXXXXX")" || capture_status=1
	status_tmp="$(mktemp "$RUNNER_TEMP/.codex-status.XXXXXX")" || capture_status=1
	diff_tmp="$(mktemp "$RUNNER_TEMP/.codex-diff.XXXXXX")" || capture_status=1
	output_tmp="$(mktemp "$RUNNER_TEMP/.codex-output.XXXXXX")" || capture_status=1
	diagnostic_tmp="$(mktemp "$RUNNER_TEMP/.codex-diagnostic.XXXXXX")" || capture_status=1

	current_head=""
	if [[ "$capture_status" -eq 0 ]]; then
		current_head="$(git_clean_env -C "$workspace" rev-parse --verify HEAD)" || capture_status=1
		if [[ "$current_head" != "$initial_head" ]]; then
			printf 'HEAD changed: initial=%s current=%s\n' "$initial_head" "$current_head" > "$status_tmp"
			capture_status=1
		fi
	fi
	if [[ "$capture_status" -eq 0 ]]; then
		rm -f "$capture_index"
		git_clean_env -C "$workspace" read-tree "$initial_head" || capture_status=1
		git_clean_env -C "$workspace" add -N -- . || capture_status=1
		git_clean_env -C "$workspace" status --short --untracked-files=all > "$status_tmp" || capture_status=1
		git_clean_env -C "$workspace" --no-pager diff --binary --no-ext-diff --no-textconv "$initial_head" -- > "$diff_tmp" || capture_status=1
	fi
	cat "$raw_output" > "$output_tmp" || capture_status=1

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
		printf '\nLast 250 output lines:\n'; tail -n 250 "$raw_output"
	} > "$diagnostic_tmp" || capture_status=1

	# Revalidate parents immediately before each non-overwriting destination move.
	if [[ -d "$final_root" && ! -L "$final_root" && -d "$work_product" && ! -L "$work_product" ]]; then
		safe_install "$output_tmp" "$final_root/output.log" || capture_status=1
		safe_install "$diagnostic_tmp" "$final_root/diagnostic.log" || capture_status=1
		safe_install "$status_tmp" "$work_product/git-status.txt" || capture_status=1
		safe_install "$diff_tmp" "$work_product/git-diff.patch" || capture_status=1
	else capture_status=1
	fi

	rm -rf "$staging" "$capture_home"; rm -f "$capture_index" "$status_tmp" "$diff_tmp" "$output_tmp" "$diagnostic_tmp"
	if [[ "$capture_status" -eq 0 ]]; then
		printf 'CODEX_DELEGATE_FINAL_ROOT=%s\n' "$final_root" >> "$GITHUB_ENV"
	else
		# Preserve any honest diagnostic that was safely installed; the trusted
		# finalize step will validate it or create a separate failure record.
		[[ -f "$final_root/diagnostic.log" && ! -L "$final_root/diagnostic.log" ]] && \
			printf 'CODEX_DELEGATE_FINAL_ROOT=%s\n' "$final_root" >> "$GITHUB_ENV"
		final_status=1
	fi
	[[ -f "$final_root/diagnostic.log" && ! -L "$final_root/diagnostic.log" ]] && cat "$final_root/diagnostic.log"
	exit "$final_status"
}
trap finish EXIT

# Syntax-only resume validation; an unrelated session can import private context
# into output. Command-file stripping prevents poisoning, not secret access.
python3 - "$CODEX_DELEGATE_TIMEOUT_MINUTES" "$raw_output" "$codex" "$CODEX_DELEGATE_PROMPT" "$CODEX_DELEGATE_RESUME_FROM" "$workspace" <<'PY'
import os, re, signal, subprocess, sys
mins=int(sys.argv[1]); output,codex,prompt,resume,cwd=sys.argv[2:7]
if resume and not re.fullmatch(r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}",resume): sys.exit(2)
env=os.environ.copy()
for key in ("GITHUB_ENV","GITHUB_OUTPUT","GITHUB_PATH","GITHUB_STEP_SUMMARY","CODEX_DELEGATE_PROMPT","CODEX_DELEGATE_RESUME_FROM","CODEX_DELEGATE_TIMEOUT_MINUTES","CODEX_DELEGATE_FINAL_ROOT"): env.pop(key,None)
argv=[codex,"exec"]+(["resume",resume] if resume else [])+["--",prompt]
with open(output,"ab",buffering=0) as stream:
 p=subprocess.Popen(argv,cwd=cwd,env=env,stdout=stream,stderr=subprocess.STDOUT,start_new_session=True)
 try: code=p.wait(timeout=mins*60)
 except subprocess.TimeoutExpired:
  os.killpg(p.pid,signal.SIGTERM)
  try:p.wait(timeout=10)
  except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
  sys.exit(124)
sys.exit(code)
PY

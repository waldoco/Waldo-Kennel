#!/usr/bin/env bash
# Trusted post-step: verify completion marker, digest and status consistency
# before upload, or mint an honest failure-only artifact.
set -euo pipefail
umask 077
: "${RUNNER_TEMP:?}" "${GITHUB_ENV:?}"
real() { python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$1"; }
temp_real="$(real "$RUNNER_TEMP")"
root="${CODEX_DELEGATE_FINAL_ROOT:-}"
valid=1
if [[ -n "$root" ]]; then
	root="$(real "$root")"
	case "$root" in "$temp_real"/waldo-kennel-codex-delegate.capture.*) ;; *) valid=0;; esac
	[[ -d "$root" && ! -L "$root" && -d "$root/work-product" && ! -L "$root/work-product" ]] || valid=0
	for file in "$root/output.log" "$root/diagnostic.log" "$root/work-product/git-status.txt" "$root/work-product/git-diff.patch" "$root/completion.marker"; do
		[[ -f "$file" && ! -L "$file" ]] || valid=0
		case "$(real "$file" 2>/dev/null || true)" in "$root"/*) ;; *) valid=0;; esac
	done
else valid=0
fi
if [[ "$valid" -eq 1 ]]; then
	version="$(sed -n 's/^version=//p' "$root/completion.marker")"
	codex_status="$(sed -n 's/^codex_status=//p' "$root/completion.marker")"
	capture_status="$(sed -n 's/^capture_status=//p' "$root/completion.marker")"
	final_status="$(sed -n 's/^final_status=//p' "$root/completion.marker")"
	output_digest="$(sed -n 's/^output_sha256=//p' "$root/completion.marker")"
	diagnostic_digest="$(sed -n 's/^diagnostic_sha256=//p' "$root/completion.marker")"
	status_digest="$(sed -n 's/^git_status_sha256=//p' "$root/completion.marker")"
	diff_digest="$(sed -n 's/^git_diff_sha256=//p' "$root/completion.marker")"
	[[ "$version" == 1 && "$capture_status" == 0 ]] || valid=0
	[[ "$output_digest" =~ ^[0-9a-f]{64}$ && "$output_digest" == "$(shasum -a 256 "$root/output.log" | awk '{print $1}')" ]] || valid=0
	[[ "$diagnostic_digest" =~ ^[0-9a-f]{64}$ && "$diagnostic_digest" == "$(shasum -a 256 "$root/diagnostic.log" | awk '{print $1}')" ]] || valid=0
	[[ "$status_digest" =~ ^[0-9a-f]{64}$ && "$status_digest" == "$(shasum -a 256 "$root/work-product/git-status.txt" | awk '{print $1}')" ]] || valid=0
	[[ "$diff_digest" =~ ^[0-9a-f]{64}$ && "$diff_digest" == "$(shasum -a 256 "$root/work-product/git-diff.patch" | awk '{print $1}')" ]] || valid=0
	grep -Fx "codex_status=$codex_status" "$root/diagnostic.log" >/dev/null || valid=0
	grep -Fx "capture_status=$capture_status" "$root/diagnostic.log" >/dev/null || valid=0
	if [[ "$codex_status" == 0 && "$capture_status" == 0 && "$final_status" == 0 ]]; then
		grep -Fqx 'proof=passed: Codex exec completed and best-effort working-tree patch capture succeeded' "$root/diagnostic.log" || valid=0
	else
		# A failed Codex run may still have a valid capture, but its diagnostic must
		# be failure-shaped and its marker final status must be nonzero.
		[[ "$final_status" != 0 ]] || valid=0
		grep -Fqx 'proof=failed: Codex execution or work-product capture failed' "$root/diagnostic.log" || valid=0
	fi
fi
if [[ "$valid" -ne 1 ]]; then
	root="$(mktemp -d "$RUNNER_TEMP/waldo-kennel-codex-delegate.capture.XXXXXX")"
	mkdir -m 700 "$root/work-product"
	: > "$root/output.log"; : > "$root/work-product/git-status.txt"; : > "$root/work-product/git-diff.patch"
	printf 'Kennel Codex delegation\ncodex_status=not-established\ncapture_status=failed\nproof=failed: no validated capture was produced\n' > "$root/diagnostic.log"
fi
printf 'CODEX_DELEGATE_UPLOAD_ROOT=%s\n' "$root" >> "$GITHUB_ENV"

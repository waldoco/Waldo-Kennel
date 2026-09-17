#!/usr/bin/env bash
# Trusted post-step: validate the completed capture root before upload, or mint
# an honest failure-only artifact if delegation never reached capture.
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
	for file in "$root/output.log" "$root/diagnostic.log" "$root/work-product/git-status.txt" "$root/work-product/git-diff.patch"; do
		[[ -f "$file" && ! -L "$file" ]] || valid=0
		case "$(real "$file" 2>/dev/null || true)" in "$root"/*) ;; *) valid=0;; esac
	done
else valid=0
fi
if [[ "$valid" -ne 1 ]]; then
	root="$(mktemp -d "$RUNNER_TEMP/waldo-kennel-codex-delegate.capture.XXXXXX")"
	mkdir -m 700 "$root/work-product"
	: > "$root/output.log"; : > "$root/work-product/git-status.txt"; : > "$root/work-product/git-diff.patch"
	printf 'Kennel Codex delegation\ncodex_status=not-established\ncapture_status=failed\nproof=failed: no validated capture was produced\n' > "$root/diagnostic.log"
fi
printf 'CODEX_DELEGATE_UPLOAD_ROOT=%s\n' "$root" >> "$GITHUB_ENV"

#!/usr/bin/env bash
# Fixed macOS packaging proof for the maintainer-dispatched CI workflow.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

artifact_dir="$repo_root/artifacts/package-proof"
diagnostic="$artifact_dir/diagnostic.log"
raw_log="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/kennel-package-proof-raw.log"
mkdir -p "$artifact_dir"
rm -f "$artifact_dir"/*.dmg "$artifact_dir"/*.sha256 "$diagnostic" "$raw_log"
touch "$raw_log"

finish() {
	status=$?
	{
		printf 'Kennel macOS package proof\n'
		printf 'status=%s\n' "$status"
		printf 'revision=%s\n' "$(git rev-parse HEAD 2>/dev/null || printf unknown)"
		printf 'host=%s architecture=%s\n' "$(uname -s)" "$(uname -m)"
		printf 'node=%s\n' "$(node --version 2>/dev/null || printf unavailable)"
		printf 'go=%s\n' "$(go version 2>/dev/null || printf unavailable)"
		printf '\nLast 350 build lines:\n'
		tail -n 350 "$raw_log"
	} > "$diagnostic"
	rm -f "$raw_log"
	exit "$status"
}
trap finish EXIT

if [[ "$(uname -s)" != Darwin || "$(uname -m)" != arm64 ]]; then
	echo "package proof requires a Darwin arm64 runner" >> "$raw_log"
	exit 2
fi

run_logged() {
	printf '\n==> %s\n' "$*" >> "$raw_log"
	"$@" >> "$raw_log" 2>&1
}

rm -rf frontend/out
run_logged npm run bootstrap
run_logged npm --prefix frontend run make
run_logged npm --prefix frontend run package:identity

dmg_count="$(find frontend/out/make -type f -name '*.dmg' -print | wc -l | tr -d ' ')"
app_count="$(find frontend/out -maxdepth 3 -type d -name 'Kennel.app' -print | wc -l | tr -d ' ')"
if [[ "$dmg_count" -ne 1 ]]; then
	echo "expected exactly one DMG, found $dmg_count" >> "$raw_log"
	exit 1
fi
if [[ "$app_count" -ne 1 ]]; then
	echo "expected exactly one Kennel.app, found $app_count" >> "$raw_log"
	exit 1
fi
dmg_source="$(find frontend/out/make -type f -name '*.dmg' -print)"
app="$(find frontend/out -maxdepth 3 -type d -name 'Kennel.app' -print)"

# Headless smoke: execute the all-in-one Kennel CLI shipped inside the package.
# This proves the bundled native binary starts on this Mac and answers its
# version command. It does not launch Electron or prove GUI rendering.
bundled_cli="$app/Contents/Resources/daemon/kennel"
if [[ ! -x "$bundled_cli" ]]; then
	echo "packaged CLI is missing or not executable: $bundled_cli" >> "$raw_log"
	exit 1
fi
run_logged "$bundled_cli" --version

cp "$dmg_source" "$artifact_dir/"
dmg="$artifact_dir/$(basename "$dmg_source")"
shasum -a 256 "$dmg" > "$dmg.sha256"
printf '\nDMG: %s\nSHA-256: %s\n' "$dmg" "$(cut -d' ' -f1 "$dmg.sha256")" >> "$raw_log"

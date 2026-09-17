#!/usr/bin/env bash
# Fixed macOS packaging proof for the maintainer-dispatched CI workflow.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

artifact_dir="$repo_root/artifacts/package-proof"
diagnostic="$artifact_dir/diagnostic.log"
raw_log="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/kennel-package-proof-raw.log"
mount_plist="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/kennel-package-proof-mount.plist"
mount_point=""
mkdir -p "$artifact_dir"
rm -f "$artifact_dir"/*.dmg "$artifact_dir"/*.sha256 "$diagnostic" "$raw_log" "$mount_plist"
touch "$raw_log"

finish() {
	status=$?
	if [[ -n "$mount_point" ]]; then
		hdiutil detach "$mount_point" >> "$raw_log" 2>&1 || true
	fi
	{
		printf 'Kennel macOS package proof\n'
		printf 'status=%s\n' "$status"
		printf 'revision=%s\n' "$(git rev-parse HEAD 2>/dev/null || printf unknown)"
		printf 'host=%s architecture=%s\n' "$(uname -s)" "$(uname -m)"
		printf 'node=%s\n' "$(node --version 2>/dev/null || printf unavailable)"
		printf 'go=%s\n' "$(go version 2>/dev/null || printf unavailable)"
		printf 'proof=DMG contains exactly one Kennel.app; packaged CLI executes; CLI binary carries arm64; package identity checks pass\n'
		printf 'limit=No Electron GUI/window-server launch or rendered UI is tested\n'
		printf '\nLast 350 build/proof lines:\n'
		tail -n 350 "$raw_log"
	} > "$diagnostic"
	rm -f "$raw_log" "$mount_plist"
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
if [[ "$dmg_count" -ne 1 ]]; then
	echo "expected exactly one DMG, found $dmg_count" >> "$raw_log"
	exit 1
fi
dmg_source="$(find frontend/out/make -type f -name '*.dmg' -print)"

# Prove the bytes delivered by the DMG rather than the pre-image app directory.
printf '\n==> hdiutil attach -nobrowse -readonly -plist %s\n' "$dmg_source" >> "$raw_log"
hdiutil attach -nobrowse -readonly -plist "$dmg_source" > "$mount_plist" 2>> "$raw_log"
mount_point="$(plutil -convert json -o - "$mount_plist" | python3 -c 'import json,sys; points=[e["mount-point"] for e in json.load(sys.stdin)["system-entities"] if "mount-point" in e]; print(points[0] if len(points)==1 else "")')"
if [[ -z "$mount_point" ]]; then
	echo "expected exactly one mounted DMG volume" >> "$raw_log"
	exit 1
fi
app_count="$(find "$mount_point" -maxdepth 2 -type d -name 'Kennel.app' -print | wc -l | tr -d ' ')"
if [[ "$app_count" -ne 1 ]]; then
	echo "expected exactly one Kennel.app inside DMG, found $app_count" >> "$raw_log"
	exit 1
fi
app="$(find "$mount_point" -maxdepth 2 -type d -name 'Kennel.app' -print)"
bundled_cli="$app/Contents/Resources/daemon/kennel"
if [[ ! -x "$bundled_cli" ]]; then
	echo "DMG packaged CLI is missing or not executable: $bundled_cli" >> "$raw_log"
	exit 1
fi
run_logged "$bundled_cli" --version
archs="$(lipo -archs "$bundled_cli")"
printf 'packaged CLI architectures: %s\n' "$archs" >> "$raw_log"
if [[ " $archs " != *" arm64 "* ]]; then
	echo "packaged CLI lacks an arm64 slice" >> "$raw_log"
	exit 1
fi
run_logged hdiutil detach "$mount_point"
mount_point=""

cp "$dmg_source" "$artifact_dir/"
dmg="$artifact_dir/$(basename "$dmg_source")"
shasum -a 256 "$dmg" > "$dmg.sha256"
printf '\nDMG: %s\nSHA-256: %s\n' "$dmg" "$(cut -d' ' -f1 "$dmg.sha256")" >> "$raw_log"

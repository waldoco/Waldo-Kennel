#!/usr/bin/env bash
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

failed=0
for package_dir in . packages/product-ui packages/cloud-client frontend/acp-runtime frontend frontend/src/landing; do
	echo "Auditing production dependencies: $package_dir"
	if ! npm --prefix "$package_dir" audit --omit=dev --audit-level=high; then
		failed=1
	fi
done

exit "$failed"

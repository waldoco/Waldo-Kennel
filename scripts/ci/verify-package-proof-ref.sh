#!/usr/bin/env bash
# Trust anchor for package-proof.yml. This copy runs only from the protected
# outcome-loop checkout, before any candidate files execute.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: verify-package-proof-ref.sh <40-hex-commit>" >&2
	exit 2
fi
candidate="$1"
if [[ ! "$candidate" =~ ^[0-9a-fA-F]{40}$ ]]; then
	echo "ref must be a full 40-hex commit SHA" >&2
	exit 2
fi
candidate="$(printf '%s' "$candidate" | tr '[:upper:]' '[:lower:]')"

# The trusted checkout is outcome-loop itself. Fetch that branch explicitly so
# ancestry is checked against the current protected remote head, not user input.
git fetch --no-tags origin refs/heads/outcome-loop:refs/remotes/origin/outcome-loop
if ! git cat-file -e "$candidate^{commit}" 2>/dev/null; then
	echo "commit is not available from the repository: $candidate" >&2
	exit 1
fi
if ! git merge-base --is-ancestor "$candidate" refs/remotes/origin/outcome-loop; then
	echo "commit is not an ancestor of protected outcome-loop: $candidate" >&2
	exit 1
fi
printf '%s\n' "$candidate"

#!/usr/bin/env bash
set -euo pipefail

include_package=true
case "${1:-}" in
	"") ;;
	--core) include_package=false ;;
	*)
		echo "usage: $0 [--core]" >&2
		exit 2
		;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

./scripts/check-provenance.sh

(
	cd backend
	go build ./...
	go test ./...
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --path-mode=abs
)

npm run shared:check
npm run test:island
npm --prefix frontend run typecheck
npm --prefix frontend test
if [[ "$include_package" == "true" ]]; then
	npm --prefix frontend run build
fi
node --test scripts/kennel-e2e-pod-gate.test.mjs

npm run sqlc
npm run api
git diff --exit-code -- \
	backend/internal/storage/sqlite/gen \
	backend/internal/httpd/apispec/openapi.yaml \
	frontend/src/api/schema.ts

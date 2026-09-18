# Stage 4 / 4C authority falsifier evidence

Base: `b444450c731aa4c06ecaba763128bc29ef881265`

## Closed claims

- Every declared adapter drift class maps to its exact typed repair, including an empty repair only for `in_sync`.
- The real harness-command frame boundary rejects spoofed, stale, expired, revoked, and rotated-old connection credentials before command-claim storage.
- Every rejected wire case emits the same generic failure bytes.
- Stale run-file ownership already had focused wrong-service and wrong-PID coverage on this base, so `backend/internal/daemon/stale_test.go` was not changed.

## Commands

- `go test -count=1 ./internal/adapterinstall -run 'Test(RepairFor|ClassifyDrift)'`
- `go test -count=1 ./internal/adapters/harnesscommand -run TestServer`
- race equivalents for both packages
- `go vet ./internal/adapterinstall ./internal/adapters/harnesscommand`

This is focused deterministic evidence. It is not packaged proof or model-backed E2E evidence.

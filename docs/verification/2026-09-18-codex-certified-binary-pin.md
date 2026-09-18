# Codex certified-binary pin (slice: feat/codex-certified-binary-pin)

## Defect (diagnosed from run 35303652394 evidence)

The governed pane launched the machine's PATH-resolved codex (banner:
`OpenAI Codex (v0.155.0-alpha.2.6)`) while the e2e job certified a pinned
codex-cli 0.153.4 via KENNEL_CODEX_BIN. The pin was honored only by the e2e
harness's own gating checks and `kennel-codex-runtime-identity`; the daemon's
`ResolveCodexBinary` searched PATH first and never read the pin. The pane
process DID receive its session-scoped CODEX_HOME + generated config.toml
(proven by working kennel_governed MCP calls) - the drift was binary-only.

## Fix

- `ResolveCodexBinary` honors an explicit `KENNEL_CODEX_BIN` first. The pin
  must resolve to an executable file; a broken pin fails loudly
  (`ErrAgentBinaryNotFound`) instead of silently falling back to PATH - a pin
  that does not hold is a certification break.
- The Plugin no longer caches the first resolution (`binaryMu`/`resolvedBinary`
  removed); every launch resolves fresh, so a re-pin can never be frozen out.
  `codexBinary` call sites are per-launch (GetLaunchCommand/GetRestoreCommand/
  Discover), so the cost is one LookPath+stat per spawn.
- Attestation: the session manager logs one line at every governed spawn -
  `governed launch binary` with session id, resolved binary path (past the
  `env CODEX_HOME=...` argv prefix), and `--version` output; a failed version
  probe is recorded in the same line (`version_error`), never dropped and never
  fatal to the spawn.
- e2e evidence: when `KENNEL_E2E_ARTIFACTS` is set, a failing test copies the
  raw daemon log out of the temp tree and `awaitPane` timeouts preserve the
  final pane capture; the workflow exports the dir and uploads it with
  `if: failure()` BEFORE the `if: always()` cleanup that deletes RUNNER_TEMP.

`cmd/kennel-codex-runtime-identity` keeps its own pin branch: it is now
redundant with `ResolveCodexBinary` but produces identical results, and its
selected/canonical split is a reporting concern, not launch authority.

## Out of scope (per owner adjudication)

- Send-path hardening (state-aware dispatch, positive submission ack, explicit
  mid-turn queue semantics): only if the pinned-binary verdict re-run is red.
- App-server migration (turn/steer with expected_turn_id exists in both
  0.153.4 and 0.155.0-alpha.2.6): post-launch.

## Evidence

- New tests: pin beats a PATH decoy; missing/non-executable pin fails closed
  with no PATH fallback; Plugin resolves fresh per launch (re-pin between
  calls takes effect); attestation logs binary+version past the env prefix and
  records version-probe failures.
- Package suites green: adapters/agent/codex, session_manager. e2e package
  compiles (gated suite runs on the self-hosted runner only).
- Live verdict: re-runs on outcome-loop after promotion (model-backed-e2e is
  outcome-loop-only by design).

## Review round 2 (attestation coverage)

- MEDIUM: attestation covered fresh Spawn only. relaunchSessionWithPolicy
  (governed restore/relaunch - daemon restart or explicit governed restore)
  rebuilt fresh/restore argv with the persisted execution policy, validated,
  and launched with no attestation, exactly where cache removal lets a
  re-pinned binary identity change. Fixed: the same attestation now runs
  after validation on the relaunch path when `execution != nil`.
- Call-site tests (not just the helper): TestSpawn_ExactExecutionPolicy... now
  asserts the fresh-spawn attestation line; TestResumeGovernedTUIUsesAdmission
  BindingAfterProjectChange asserts the restore/relaunch attestation line.
  Both record version_error against fake argv binaries (success-path version
  logging stays covered by the helper tests).

## Environmental noise: account-synced MCP connector (2026-09-19)

- The model-backed-e2e stream can carry `ERROR rmcp::transport::worker ...
  AuthRequired ... mcp.cloudflare.com`. Origin: an account-synced ChatGPT
  connector that arrives with the signed-in Codex identity on the runner,
  not from any local config. It survives both a job-scoped auth-only
  CODEX_HOME (no config.toml) and `--disable apps` on the exec invocation at
  0.153.4 (observed on runs 35380740916 and 35383383588).
- Harmless to assertions: the falsifier and launch-cut evidence paths never
  touch MCP servers; the connector's OAuth failure only closes its own
  transport worker.
- Open owner option (deliberately not taken 2026-09-18; production behavior
  change): `apps._default.enabled = false` in the generated governed session
  config (backend/internal/adapters/agent/codex/codex.go) would disable
  connectors for every governed session.

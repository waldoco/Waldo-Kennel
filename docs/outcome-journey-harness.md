# Packaged macOS Outcome journey harness

One-command proof that the real packaged macOS app takes an owner through Contract creation, one Contract revision (surviving a real restart), a live-agent Plan proposal, and Plan approval — through the actual Work UI, against the daemon's own durable state — and stops honestly at the real pre-execution boundary. It is also a UX flaw audit: every run produces a severity-ranked catalog of what the current experience gets wrong.

Full plan, resolved ambiguities, and the exact step-by-step design: [`docs/handoffs/2026-09-16-macos-outcome-journey-harness-plan.md`](handoffs/2026-09-16-macos-outcome-journey-harness-plan.md).

## What this is not

- Not a CI gate. It requires a real macOS host and an already-authenticated local Codex CLI — see [Owner-run only](#owner-run-only) below.
- Not a production readiness claim. A passing run proves the named journey passed at an exact commit on one machine; nothing more.
- Not an execution proof. The run stops the moment the daemon would start a persistent Attempt (`executionStarted: false` in every manifest) — it never clicks Start.

## Prerequisites

- macOS.
- Go, Node, and npm at the versions in [development.md](development.md#toolchain).
- A Codex CLI on `PATH`, already authenticated (the same one `kennel start`/the packaged app would use). The harness preflights this and reports `blocked`, not a fabricated pass, if it is missing or unauthorized.
- `frontend/node_modules` installed (`npm run bootstrap` from the repo root, or `npm ci` under `frontend/`) — this is also where the harness's own `@playwright/test` comes from; `test/macos-outcome-journey/` has no `node_modules` of its own.
- Enough disk/time for an unsigned local `electron-forge package` build if you don't pass `--app`.

## Running it

```sh
node scripts/outcome-journey/run-outcome-journey.mjs
```

Builds and packages at the current commit, then runs the journey. Useful flags:

```sh
# Reuse an already-packaged build instead of packaging again
node scripts/outcome-journey/run-outcome-journey.mjs --app "frontend/out/Kennel-darwin-arm64/Kennel.app"

# Pin the evidence/profile/fixture root instead of a fresh mkdtemp
node scripts/outcome-journey/run-outcome-journey.mjs --work-dir /tmp/kennel-journey-run1

# Override a wait ceiling (seconds) — defaults are 30 / 180 / 180 / 600
node scripts/outcome-journey/run-outcome-journey.mjs --codex-preflight-timeout 45 --intake-analysis-timeout 240
```

The command prints a final `KENNEL_OUTCOME_JOURNEY_RESULT {...}` line and exits nonzero on anything but `passed` — `failed`, `blocked`, and `ambiguous` (an accepted mutation whose effect never settled — see the plan's §9) are all distinct, and all three are a failed proof for CI/shell-purposes even though they mean different things to a human reading the manifest.

## Owner-run only

This is not a CI lane. It drives a live, already-authenticated Codex session through real product turns (Contract analysis and Plan proposal both require it — see plan §0.1/§0.2), which means real model calls, real time, and real cost on whatever account is authenticated locally. CI runs only this harness's own deterministic self-tests and schema/typecheck gates:

```sh
node --test scripts/outcome-journey/*.test.mjs
npm --prefix frontend run typecheck && npm --prefix frontend test
cd backend && go test ./...
```

A live-journey-in-CI lane is a possible future addition, not something this harness assumes — it would need its own credential and cost policy first.

## Evidence

Each run creates `<work-dir>/evidence/` containing:

- `manifest.json` — the versioned, typed record: build identity (including a live `buildRevision`-vs-recorded-git-SHA check, aborting before any UI action on a mismatch), profile/fixture paths, every step's timing/result, every identity (project/Outcome/Contract revision/Plan/approval), the API assertion ledger, the facets before/after/after-restart diff, and the explicit `executionStarted: false` boundary statement.
- `ux-flaws.json` / `ux-flaws.md` — the severity-ranked UX finding catalog, produced every run regardless of pass/fail.
- `screenshots/` — a named checkpoint per journey step, both the renderer's own pixels and (at the checkpoints that matter most) a native `screencapture` of the real window frame.
- Packaged app + daemon logs, and a sanitized request/response ledger (secrets redacted, IDs and timing intact).

## Honest limitations, every run

- Contract creation and Plan proposal both require a live Codex turn — this is not a fully deterministic, model-free proof, by the product's own design (there is no owner-visible way to skip it today; see the harness's own UX finding about that gap).
- Signing/notarization is not verified for a local unsigned build (`manifest.build.signingVerified: false`); pass `--app` with a signed release build and run `scripts/verify-mac-artifact.sh` separately if that matters for a given run.
- CI cannot execute the live journey (see above) — CI-backed evidence and owner-Mac evidence are named separately in every closeout that attaches this harness's output to a PR.

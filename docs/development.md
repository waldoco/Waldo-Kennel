# Development guide

This guide covers source development and verification. See [STATUS.md](STATUS.md) for current runtime limits and [AGENTS.md](../AGENTS.md) for product and engineering authority.

## Toolchain

| Tool | Version / requirement |
| --- | --- |
| Go | Version in `backend/go.mod` (currently 1.25.7) |
| Node.js | 22.23.2, pinned by `.nvmrc` |
| npm | 10.9.8, pinned by the root `packageManager` |
| Git | Required for worktrees and SCM integration |
| tmux | Required for terminal sessions on macOS/Linux; Windows uses conpty |

Provider CLIs such as Codex or Claude Code are discovered locally and retain their own authentication. Kennel does not bundle provider credentials.

## Checkout and bootstrap

```sh
git clone --branch beta https://github.com/waldoco/Waldo-Kennel.git
cd Waldo-Kennel
npm run bootstrap
```

`bootstrap` uses `npm ci` for the root, product UI, cloud client, ACP runtime, desktop, retained landing donor, and repository scripts. It does not launch Electron or publish anything.

Create one focused branch per change from the latest `origin/beta`, and open the PR back to `beta`. Keep both `beta` and `main` free of direct contributor commits and preserve uncommitted user work. Maintainers promote a tested integration state through a separate `beta` -> `main` PR.

## Repository layout

```text
backend/                    Go daemon and thin CLI source
  cmd/kennel/                   retained source/upstream compatibility entrypoint
  internal/                 domain, services, ports, adapters, HTTP, storage
frontend/                   Electron supervisor and React renderer
  acp-runtime/              packaged provider bridge runtime
  src/landing/              retained landing donor, not desktop launch surface
packages/product-ui/        shared tested product primitives
packages/cloud-client/      retained typed compatibility client
packages/mobile/            retained mobile donor, outside current product claim
packages/ao*/               frozen legacy AO packages
scripts/                    bootstrap, verification, and retained helpers
docs/                       foundation contract and chassis references
```

## Primary verification

Run the smallest relevant test first, then the foundation matrix:

```sh
npm run test:foundation
npm run audit:production
npm run lint
cd backend && go test -race ./...
```

`test:foundation` covers provenance, Go build/test/vet, shared packages, desktop typecheck/tests, the retained landing build, script tests, and generated sqlc/OpenAPI drift. `audit:production` checks only dependencies that can ship at runtime. Development dependency advisories are tracked separately because inherited Electron/build/test toolchains are not yet fully clean.

## Backend

```sh
cd backend
go build ./...
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/kennel --help
```

`backend/cmd/kennel` is the retained source path. Builds and packages emit a `kennel` executable. The CLI remains a daemon HTTP client; do not move storage, runtime, or adapter behavior into it.

For API DTO or controller changes:

```sh
npm run api
cd backend && go test ./internal/httpd/...
```

For SQLite query/schema changes:

```sh
npm run sqlc
```

Never modify an already-merged migration or hand-edit sqlc output. Add a migration or an explicit compatibility reconciliation with an upgrade test.

## Desktop

```sh
npm --prefix frontend run typecheck
npm --prefix frontend test
npm --prefix frontend run dev:web
```

`dev:web` is renderer-only. `npm --prefix frontend run dev` launches Electron and its daemon; use it only after identity isolation is known-good and a launch is part of the authorized task.

To inspect a packaged identity without launching:

```sh
npm --prefix frontend run package
npm --prefix frontend run package:identity
```

The assertion must pass before any release work. It verifies Kennel's bundle ID, display name, executable, protocol, updater target/cache, daemon name, state namespace, and release repository.

## State safety

Kennel global state belongs under `~/.kennel` or explicit `KENNEL_*` overrides. It must never fall back to `~/.ao` or the operating system's default Electron application-data directory. Use a task-specific temporary directory when a test needs state; do not repurpose `$HOME`.

Project-local `.kennel/attachments` and `.kennel/launch.json` are documented compatibility formats, not global state. See [identity and state](identity-and-state.md).

## Upstream updates

Do not merge AO wholesale. Follow [upstream provenance](upstream-provenance.md): fetch the pinned AO remote, compare trees/ranges, port reviewed changes on a Kennel branch, preserve every installed-identity boundary, and update `upstream.json` only after verification.

## Frontend demonstration

When a session owns a visual change and the desktop is already authorized/running, use `kennel preview [url]` so the result appears in that session's Browser panel. Do not launch the packaged app merely to prove identity; use the package assertion.

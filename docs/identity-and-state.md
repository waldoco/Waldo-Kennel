# Kennel identity and state contract

This contract prevents a Kennel development build, packaged app, daemon, updater, or CLI from colliding with an installed Agent Orchestrator application. It applies before any Waldo product feature is introduced.

## Owned identities

| Surface | Value |
| --- | --- |
| Product and desktop display name | `Kennel` |
| Package / CLI executable | `kennel` |
| Bundle ID / AppUserModel ID | `in.heywaldo.kennel` |
| Deep-link scheme | `kennel-app` |
| Global data root | `~/.kennel` |
| Electron `userData` | `~/.kennel/electron` |
| Daemon data / run file | `~/.kennel/data`, `~/.kennel/running.json` |
| Updater cache | `kennel-updater` |
| Release owner/repository | `waldoco/Waldo-Kennel` |
| Runtime environment variables | `KENNEL_*` |
| Default loopback / dev / opt-in LAN port | `3031` / `3032` / `3041` |
| Generated branches | `kennel/` |
| Preservation refs | `refs/kennel/preserved/` |
| Renderer bridge | `window.kennel` |

The primary daemon listener remains unauthenticated and loopback-only. The LAN listener remains opt-in, authenticated, and limited according to ADR 0001; Kennel's isolated default is port 3041.

## State isolation and migration policy

Kennel must not read, write, move, delete, or automatically import `~/.ao`. That directory belongs to AO. Opening Kennel with no `~/.kennel` produces a new Kennel state root.

Kennel also does not inspect the older `~/.agent-orchestrator` layout. Any future AO-to-Kennel importer needs its own proposal, preview, confirmation, copy-only implementation, and rollback evidence; it must not silently activate or treat either AO directory as a fallback.

Two project-local names remain upstream compatibility seams:

- `.kennel/attachments` carries session attachment files inside the user-selected project/worktree.
- `.kennel/launch.json` describes an optional preview command inside the project.

They do not contain Kennel's global daemon, Electron, updater, credential, or session database state. Renaming them is a later project-format migration and is not necessary to prevent installed-app collisions.

## Source compatibility is not installed identity

The source directory `backend/cmd/kennel`, the Go module path `github.com/aoagents/agent-orchestrator/backend`, and internal AO vocabulary remain in selected implementation and test seams. Keeping them makes reviewed upstream ports tractable and avoids a repository-wide import rewrite. Packaging builds that source into the `kennel` executable; none of these source names authorize AO state, updater, release, protocol, or bundle identifiers.

The frozen `packages/ao*` npm packages are retained, unchanged compatibility artifacts. They are not Kennel's install path and must not be republished as Kennel.

## Verification

`npm --prefix frontend run package:identity` inspects a packaged app without launching it. It rejects drift in the application ID, display name, executable, deep-link protocol, updater cache, release target, daemon name, state-root variables, and forbidden AO identifiers. Run it locally after packaging while hosted CI is deferred.

Do not validate identity by launching a pre-foundation packaged application. A pre-isolation build may still collide with AO state; package the current source and inspect it instead.

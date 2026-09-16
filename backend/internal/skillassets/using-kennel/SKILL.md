---
name: using-kennel
description: "Catalog of the Kennel `kennel` CLI: spawning workers, managing sessions and projects, sending messages, controlling the shared browser, previewing pages, and daemon control. Use when using the Kennel CLI, spawning workers, or managing sessions in a Kennel workspace."
trigger: "Using the Kennel CLI in a Kennel workspace: spawning workers, managing sessions or projects, sending messages, controlling or previewing pages."
---

> **Compatibility CLI note:** these commands describe the currently shipped session CLI. They do not define vNext Outcome authority or session topology. For repository implementation, use `docs/architecture/persistent-mission-runtime.md`; skills and commands remain thin ingress/provider adapters.

# Kennel CLI Catalog

`kennel` is a thin CLI over the local Kennel daemon. Every command is `kennel <command> --help` for the authoritative flag list.

| Command | What it does | When to use | Details |
|---|---|---|---|
| `spawn` | Spawn a worker agent in a fresh git worktree | Starting a new task or issue | [commands/spawn.md](commands/spawn.md) |
| `session` | Manage agent sessions (list, kill, rename, restore, etc.) | Inspecting or controlling running/terminated sessions | [commands/session.md](commands/session.md) |
| `project` | Register, inspect, configure, or remove projects | Setting up or managing repos Kennel knows about | [commands/project.md](commands/project.md) |
| `orchestrator` | List orchestrator sessions | Viewing which sessions are orchestrators | [commands/orchestrator.md](commands/orchestrator.md) |
| `review` | Submit a reviewer result for a worker's PR | Completing a code review loop | [commands/review.md](commands/review.md) |
| `send` | Send a message to a running agent session | Correcting or directing a live agent | [commands/send.md](commands/send.md) |
| `preview` | Start a session-owned app or open an exact URL/file | Running and showing the worker's relevant app, Markdown, HTML, PDF, or image | [commands/preview.md](commands/preview.md) |
| `browser` | Inspect and control the session's shared live browser | Verifying a web app through snapshots, interactions, waits, screenshots, console, and errors | [commands/browser.md](commands/browser.md) |
| `start` | Fetch (if needed) and open the Kennel desktop app | Launching the app | [commands/start.md](commands/start.md) |
| `stop` | Stop the Kennel daemon | Shutting down Kennel | [commands/stop.md](commands/stop.md) |
| `status` | Show daemon status | Verifying the daemon is up and healthy | [commands/status.md](commands/status.md) |
| `doctor` | Run local health checks | Diagnosing Kennel setup problems | [commands/doctor.md](commands/doctor.md) |
| `version` | Print version information | Checking installed version | - |
| `completion` | Generate shell completion scripts | Setting up tab completion | - |

## Conventions

- Most read commands accept `--json` for machine-readable output.
- `-p / --project` scopes session subcommand lookups to one project.
- Session and project ids are shown by `kennel session ls` and `kennel project ls`.
- `--agent` is an alias for `--harness` on `kennel spawn`.
- Every command accepts `-h / --help` for the full flag list.
- For frontend launch, preview selection, or artifact handoff, read
  [commands/preview.md](commands/preview.md) before acting. Its static-file,
  project-runtime, and automatic-handoff rules are load-bearing.
- For page inspection, interaction, or request diagnosis, read
  [commands/browser.md](commands/browser.md). It defines shared-tab behavior
  and the opt-in network policy.

Use [references.md](references.md) only when a natural-language request does
not map clearly to a command above.

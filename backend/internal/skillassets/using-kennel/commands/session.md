# kennel session

> Compatibility command reference. This command does not grant scheduling or Outcome authority; the daemon validates all governed actions.


Manage agent sessions: list, inspect, rename, kill, restore, clean up, and claim PRs.

## Syntax

```
kennel session <subcommand> [args] [flags]
```

## Subcommands

---

### kennel session ls

List sessions.

**Syntax:**
```
kennel session ls [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-a, --all` | Include orchestrator sessions | - |
| `--include-terminated` | Include terminated sessions | - |
| `--json` | Output as JSON | - |
| `-p, --project string` | Filter by project ID | - |

**Examples:**

```bash
# List all active worker sessions
kennel session ls
```

```bash
# List all sessions including terminated, scoped to one project
kennel session ls --include-terminated -p agent-orchestrator
```

---

### kennel session get

Fetch one session.

**Syntax:**
```
kennel session get <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output as JSON | - |
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Get details for session mer-3
kennel session get mer-3
```

```bash
# Get session details as JSON
kennel session get mer-3 --json
```

---

### kennel session kill

Terminate a session.

**Syntax:**
```
kennel session kill <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Kill session mer-3
kennel session kill mer-3
```

---

### kennel session rename

Rename a session.

**Syntax:**
```
kennel session rename <id> <name> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Rename session mer-3 to a new display name
kennel session rename mer-3 "fix-auth-bug"
```

---

### kennel session restore

Relaunch a terminated session.

**Syntax:**
```
kennel session restore <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Restore a terminated session
kennel session restore mer-3
```

---

### kennel session cleanup

Clean up terminated sessions by reclaiming eligible workspaces. Dirty worktrees are skipped by the daemon.

**Syntax:**
```
kennel session cleanup [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `-p, --project string` | Filter by project ID | - |
| `-y, --yes` | Skip confirmation prompt | - |

**Examples:**

```bash
# Clean up all terminated sessions (skip prompt)
kennel session cleanup -y
```

```bash
# Clean up terminated sessions for one project
kennel session cleanup -p agent-orchestrator
```

---

### kennel session claim-pr

Attach an existing PR to the current AO session, or target another session explicitly.

**Syntax:**
```
kennel session claim-pr <pr-ref> [flags]
kennel session claim-pr <session-id> <pr-ref> [flags]
```

With one positional argument, `KENNEL_SESSION_ID` supplies the session. This is the preferred form inside a worker. Pass both arguments from an orchestrator or external shell when targeting another session.

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output as JSON | - |
| `--no-takeover` | Refuse if another active session owns the PR | - |
| `-p, --project string` | Project id to scope the lookup | - |

**Examples:**

```bash
# Attach PR 88 to the current worker session
kennel session claim-pr 88
```

```bash
# Attach PR 88 to session mer-3 explicitly
kennel session claim-pr mer-3 88
```

```bash
# Claim PR 88 for the current worker but refuse if another session owns it
kennel session claim-pr 88 --no-takeover
```

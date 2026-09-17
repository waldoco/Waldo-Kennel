# kennel project

> Compatibility command reference. This command does not grant scheduling or Outcome authority; the daemon validates all governed actions.


Manage projects: register repos, inspect, configure per-project settings, and remove.

## Syntax

```
kennel project <subcommand> [args] [flags]
```

## Subcommands

---

### kennel project add

Register a local git repo as a project so sessions can be spawned in it. The path must be an existing git repository on disk. With `--as-workspace`, the path may be a parent folder containing direct child git repositories; AO initializes/adopts the parent as the root repo and gitignores children.

**Syntax:**
```
kennel project add [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--as-workspace` | Register a parent folder as a workspace project (root-as-repo plus direct child repos) | - |
| `--id string` | Project id | Derived by the daemon from the path |
| `--name string` | Display name | - |
| `--orchestrator-agent string` | Default orchestrator session agent | - |
| `--path string` | Absolute path to the local git repo | Required |
| `--worker-agent string` | Default worker session agent | - |

**Examples:**

```bash
# Register a repo as a project
kennel project add --path /Users/example/Developer/my-project --name "my-project"
```

```bash
# Register a workspace (parent folder containing multiple repos)
kennel project add --path /Users/harshit/Downloads/side-quests --as-workspace --name "side-quests"
```

---

### kennel project ls

List registered projects. Aliases: `ls`, `list`.

**Syntax:**
```
kennel project ls [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output projects as JSON | - |

**Examples:**

```bash
# List all registered projects
kennel project ls
```

---

### kennel project get

Fetch one registered project.

**Syntax:**
```
kennel project get <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output project as JSON | - |

**Examples:**

```bash
# Get details for the agent-orchestrator project
kennel project get agent-orchestrator
```

---

### kennel project rm

Remove a registered project. Aliases: `rm`, `remove`, `delete`.

**Syntax:**
```
kennel project rm <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output removal result as JSON | - |
| `-y, --yes` | Skip confirmation prompt | - |

**Examples:**

```bash
# Remove a project (with confirmation)
kennel project rm agent-orchestrator
```

```bash
# Remove without prompt
kennel project rm agent-orchestrator -y
```

---

### kennel project set-config

Replace a project's per-project config (branch, session prefix, env, symlinks, post-create, agent model/permissions, role overrides, worker rules, and orchestrator rules). The config is resolved when a session spawns. Set fields via flags, pass the whole object with `--config-json`, or `--clear` to remove all config.

**Syntax:**
```
kennel project set-config <id> [flags]
```

**Flags:**

| Flag | Meaning | Default / Required |
|---|---|---|
| `--agent-rules string` | Project-specific standing instructions appended to worker session prompts | - |
| `--agent-rules-file string` | Repo-relative file containing project-specific worker standing instructions | - |
| `--clear` | Clear all config | - |
| `--config-json string` | Full config as a JSON object (overrides field flags) | - |
| `--default-branch string` | Base branch new session worktrees are created from | - |
| `--env stringArray` | Env var `KEY=VALUE` forwarded into sessions (repeatable) | - |
| `--json` | Output the updated project as JSON | - |
| `--model string` | Agent model override (e.g. `claude-opus-4-5`) | - |
| `--orchestrator-agent string` | Harness override for orchestrator sessions | - |
| `--orchestrator-rules string` | Project-specific standing instructions appended to orchestrator session prompts | - |
| `--permission string` | Permission mode: `default`, `accept-edits`, `auto`, `bypass-permissions` | - |
| `--post-create stringArray` | Command to run after workspace creation (repeatable) | - |
| `--session-prefix string` | Displayed session-id prefix | - |
| `--symlink stringArray` | Repo-relative path to symlink into workspaces (repeatable) | - |
| `--worker-agent string` | Harness override for worker sessions | - |

**Examples:**

```bash
# Set default branch and model for a project
kennel project set-config agent-orchestrator --default-branch main --model claude-opus-4-5
```

```bash
# Set an env var and a post-create command
kennel project set-config agent-orchestrator --env "NODE_ENV=development" --post-create "npm install"
```

```bash
# Set worker and orchestrator standing rules
kennel project set-config agent-orchestrator --agent-rules "Run focused tests before reporting done." --orchestrator-rules "Delegate implementation work to worker sessions."
```

```bash
# Load worker rules from a repo-relative file
kennel project set-config agent-orchestrator --agent-rules-file docs/ao-worker-rules.md
```

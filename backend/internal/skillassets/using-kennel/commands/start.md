# kennel start

> Compatibility command reference. This command does not grant scheduling or Outcome authority; the daemon validates all governed actions.


Fetch (if needed) and open the Agent Orchestrator desktop app. The desktop app owns the daemon, state, and updates. `kennel start` no longer runs a daemon: it resolves the installed app (or downloads the latest release), opens it, and exits.

## Syntax

```
kennel start [flags]
```

## Flags

| Flag | Meaning | Default / Required |
|---|---|---|
| `--json` | Output start result as JSON | - |

## Examples

```bash
# Open the Kennel desktop app
kennel start
```

```bash
# Open the app and get the result as JSON
kennel start --json
```

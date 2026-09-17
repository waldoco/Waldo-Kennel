# Quick Reference

> **Compatibility CLI note:** these commands describe the currently shipped session CLI. They do not define vNext Outcome authority or session topology. For repository implementation, use `docs/architecture/persistent-mission-runtime.md`; skills and commands remain thin ingress/provider adapters.


Natural-language-to-command mappings for common AO tasks.

| You want to... | Command |
|---|---|
| Show me this webpage / open this page | `kennel preview "<url>"` |
| Start an existing configured dev app | `kennel preview start [configuration]` |
| Check or stop the worker's managed dev app | `kennel preview status` / `kennel preview stop` |
| Show this Markdown or HTML file without a server | `kennel preview "<workspace-path>"` |
| Hand off a newly created browser-displayable artifact | `kennel preview "<workspace-path>"` immediately after writing the primary artifact |
| Inspect and verify this webpage as the agent | `kennel browser open "<url>"`, then `kennel browser snapshot` |
| Click or fill a page element | `kennel browser snapshot`, then `kennel browser click <ref>` or `kennel browser fill <ref> "<text>"` |
| Check frontend runtime failures | `kennel browser errors` and `kennel browser console` |
| Diagnose a request/API/CORS/auth/redirect failure when normal page evidence is insufficient | `kennel browser network start`, reproduce once, then `kennel browser network stop` |
| Check network capture without enabling it | `kennel browser network status` or `kennel browser network list` |
| Open the user's real Chromium debugging surface | `kennel browser devtools open` |
| Close the shared DevTools window when explicitly requested | `kennel browser devtools close` |
| Capture the page | `kennel browser screenshot [path]` |
| Spawn a worker on issue N | `kennel spawn --project <p> --issue N --name "<=20 chars>" --prompt "..."` |
| Message a running agent | `kennel send --session <id> --message "..."` |
| Kill a session | `kennel session kill <id>` |
| List sessions | `kennel session ls` |
| Register a repo as a project | `kennel project add --path <abs-path> --name <name>` |
| List projects | `kennel project ls` |
| Rename a session | `kennel session rename <id> "<name>"` |
| Restore a killed session | `kennel session restore <id>` |
| Clean up terminated sessions | `kennel session cleanup` |
| Make a Docker container this session starts survive AO cleanup | `docker run --label kennel.session=$KENNEL_SESSION_ID --label kennel.spare=true ...` |
| See a session's details | `kennel session get <id>` |
| Open the desktop app | `kennel start` |
| Check the daemon is up | `kennel status` |
| Run health checks | `kennel doctor` |
| Clear the preview panel | `kennel preview clear` |
| List orchestrator sessions | `kennel orchestrator ls` |
| Claim an existing PR for the current session | `kennel session claim-pr <pr-ref>` (`KENNEL_SESSION_ID`) |
| Claim an existing PR for another session | `kennel session claim-pr <id> <pr-ref>` |
| Submit a code review verdict | `kennel review submit <session-id> --run <run-id> --verdict approved` |
| Configure a project's default branch or model | `kennel project set-config <id> --default-branch <branch> --model <model>` |

# Delegated bulk reader prototype

An opt-in Claude Code plugin that blocks oversized reads and directs the
orchestrator to a permission-denied opencode primary agent. The worker reads source
in its own context; only its answer returns to Claude. This is a prototype of role
1 in [the intent document](../../docs/plans/delegated-worker-tier.md), not a shipped
Kennel worker service. No daemon routes, database state, UI, Outcome semantics,
scaffolder, or implementation delegate are added.

## Run

Requires Python 3.10+, Claude Code, and opencode (live-tested with 1.18.26).
Start Claude **from the contribution worktree**:

```sh
claude --plugin-dir /absolute/path/to/kennel/prototypes/delegated-worker
```

The plugin adds `Read|Bash` PreToolUse hooks and `/kennel-reader:bulk-reader`.
An oversized read gets a denial containing instructions, not source text. Claude
then supplies a factual question to the skill. An automatic summary inside the
hook would lack that question and risk delegating a reasoning task, so the hook
only routes. Normal Claude permissions still apply to the Python command.

Direct worker invocation, from the worktree:

```sh
python3 -B /absolute/path/to/reader.py read \
  --question 'List the exported Go type names. Return a JSON array only.' \
  --paths backend/internal/domain/outcome_decomposition.go
```

Set `KENNEL_WORKER_MODEL=provider/model` to swap models; default is
`opencode/big-pickle`. `KENNEL_READER_MIN_LINES` controls the experimental gate
(default 350, invalid values use 350). `--timeout` defaults to 180 seconds.
`--worktree` explicitly selects the directory; otherwise it is the current directory.

Worker state is `$KENNEL_DATA_DIR/delegated-worker`, or the directory containing
`KENNEL_RUN_FILE` plus `delegated-worker`, or `~/.kennel/delegated-worker`.
XDG data/config/cache/state and temporary paths are redirected there. Supply
`OPENCODE_API_KEY`, or provision `data/opencode/auth.json` beneath that worker
state directory using an existing opencode login. The prototype never imports
credentials from OS-default paths automatically.

Each run retains private `config.json`, resolved `agent.json`, `server.log`,
metadata-only `progress.jsonl`, `messages.json`, and a `receipt.json` or
`failure.json` under `runs/<id>`. Messages contain source and must stay out of the
orchestrator context and version control. Receipts report hashes, opencode version,
model, async dispatch latency, round-trip latency, and provider token/cost fields.
The printed receipt path is for local audit; the skill explicitly forbids reading
the transcript to bypass delegation. Retention/garbage collection is not implemented.

## Boundaries and limitations

- A hand-written `kennel-reader` agent has `mode: primary`. A blanket deny is
  followed by an exact-file read allowlist and explicit edit/write/bash/external
  directory denies. Both relative and absolute read patterns are needed by this
  opencode version. Other tools, including subagents, LSP, web, and MCP tools, stay
  denied. Project configuration, Claude configuration discovery, default plugins,
  snapshots, and auto-update are disabled for this worker.
- Preflight checks the actual resolved agent rules and server directory before
  dispatch. OpenCode adds its own tool-output directory exception; this specific
  external-directory exception is accepted, but those files are not on the read
  allowlist. Path traversal, symlink escape, and wildcard-bearing input paths
  are rejected before starting the worker.
- These are **harness permissions, not a kernel sandbox**. Cwd alone cannot stop
  an absolute-path write. Denied mutation tools supply the read-only boundary;
  future write-capable workers need a separately designed OS/container boundary.
  A compromised opencode executable or trusted administrative plugin is outside
  this prototype's threat model. It is not a sandbox for hostile local processes.
- The transport opens SSE before `prompt_async`, correlates events by session,
  and retrieves messages after the idle event. It requires a completed `stop`
  turn from the right agent, successful read-tool facts for every requested file,
  unchanged source hashes, and a nonempty answer capped at 12 KB. These checks
  establish transport/read facts, **not answer correctness or full-file coverage**.
  The live inventory eval separately compares against an independent source oracle.
- A fresh process/session is owned by each call and stopped in `finally`, including
  deadline, disconnect, and interruption paths. Loopback ports are selected per call.
  A race for that port fails startup; there is no attachment to a shared server.
  A worker failure is visible and never automatically falls back to a raw read.
  SIGTERM unwinds cleanup; SIGKILL or a host crash has no recovery/reaper in this
  prototype. This is another reason to discuss daemon ownership before rollout.
- The gate checks actual requested Read range size. Bounded reads remain available
  for debugging, architectural/security reasoning, and exact editing context; those
  tasks must never be delegated. A valid small limit passes; an offset by itself
  only passes if the remaining range is small.
- Bash coverage is intentionally a convenience heuristic for commands beginning
  with `cat`, `head`, `tail`, `less`, or `more`, including quoted/multiple paths.
  It does not parse arbitrary shells, interpreters, substitutions, aliases, `cd &&`,
  `sed`, or repeated small reads. Pipelines starting with a recognized bulk read
  are conservatively blocked, even if they later filter. It is an economic routing
  gate for a cooperative orchestrator, not complete information-flow enforcement.
- Small files can be delegated explicitly for measurement. The hook does not force
  them through the worker. Never treat a worker's summary as exact edit context.

## Validation

Offline tests use injected HTTP/process boundaries; they make no network calls:

```sh
python3 -B -m unittest discover -s prototypes/delegated-worker/tests
```

Explicit live evals use the configured accounts and write only beneath worker state:

```sh
python3 -B prototypes/delegated-worker/live_eval.py --worktree /absolute/worktree
python3 -B prototypes/delegated-worker/claude_eval.py --worktree /absolute/worktree
```

`live_eval.py` checks real Go inventories against independently extracted type names,
compares source-in-context versus answer-plus-overhead using the **approximate**
`ceil(characters / 4)` estimator, measures wall time, and orders a worker to mutate
a fixture and an outside sentinel. It verifies whole-fixture hashes independently.
These deliberately compressible inventory questions are not a general quality eval.
Use `--repeats N` for repeated samples; no failures count as savings.

`claude_eval.py` runs actual baseline and delegated Claude turns, checks answers,
requires denied Read results plus a worker invocation in the delegated transcript,
and checks that long source lines did not leak into tool results. It records actual
Claude usage/cost separately from the worker estimates. On macOS,
`--macos-keychain` explicitly reuses the existing Claude login without printing or
writing the credential; otherwise provide a token/login to the isolated
`CLAUDE_CONFIG_DIR` under worker state. This is an opt-in live evaluation, not a
runtime credential-import feature. `--only delegated` permits diagnosis without
rerunning the baseline.

## Prior art and integration recommendation

Studied Spotify's Apache-2.0 [shunt](https://github.com/spotify/portal-ai-plugins/tree/3c24ca30ff63e1f5bbad1c43fe5324daff579123/plugins/shunt):
both hooks, transport scripts/shared Portal library, both skills, hook fixtures,
transport evals, benchmark driver, and end-to-end expectations. The new Python
implementation adopts the block-and-redirect pattern; it copies no source code.
Unlike Portal's stateless text call, opencode receives paths and uses its own tools.
The eval keeps shunt's transparent estimator while adding overhead, independent
answer checks, denied-mutation checks, and actual Claude transcript/usage evidence.

Three possible integration points were considered: this opt-in plugin, a daemon-owned
worker service, or an explicit MCP tool without hooks. The plugin is the smallest
way to prove interception today. A tool alone cannot enforce delegation. Daemon
ownership is the likely production direction for fleet visibility and lifecycle,
but needs a discussed policy and provider adapter/port before implementation; do
not turn this script into a second canonical Kennel service or CLI storage path.

Measured results and all six unresolved policy decisions are recorded in the
[experiment report](../../docs/plans/delegated-worker-tier-prototype-results.md).

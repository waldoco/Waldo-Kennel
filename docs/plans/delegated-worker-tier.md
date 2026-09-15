# Plan: Delegated Worker Tier — Claude Code as Orchestrator, opencode as Grunt Worker

Status: proposal / intent. No implementation approach is prescribed here
deliberately — this document exists to settle *what* we want and *why*, and to
record the evidence and prior art behind it. The *how* is a separate exercise.

## Goal

Change the economics of a Kennel fleet. Today every session Kennel supervises is
a full-price harness doing all of its own work — including the large share of
that work that is mechanical, high-volume, and requires no judgment: reading big
files to answer a question, scaffolding boilerplate, applying a known pattern
across a set of files.

We want a second, cheaper tier underneath the smart harness. Claude Code stops
being the thing that reads and types everything and becomes the thing that
decides, delegates, reviews, and corrects. The token-heavy grunt work moves to a
cheap worker harness that has its own tools and its own context window, so the
bulk never enters Claude's context at all.

The intent is not "make Claude Code cheaper by compressing its prompts." It is
"stop paying a reasoning-grade model to be a file reader and a typist."

## Why this matters now

The cost curve is the motivation. Spotify's engineering post cites a quarter of
engineering leaders already spending $200–$500 per developer per month on
tokens, some over $2,000, and projects AI coding costs passing the average
developer salary by 2028. Kennel makes this worse by design: it exists to run
*many* sessions at once. Whatever a single session costs, Kennel multiplies it
by the size of the fleet. Any per-session saving compounds across the whole
orchestrator, which makes Kennel an unusually good place to put this and an
unusually bad place to ignore it.

## Prior art: Spotify's `shunt`

Spotify published both the writeup and the working plugin, and the plugin is the
useful part. `plugins/shunt` in `spotify/portal-ai-plugins` (Apache-2.0, v0.2.0)
is three layers:

1. **Hooks** — `PreToolUse` matchers on `Read` and `Bash` that block reads over a
   line threshold (default 350) and redirect Claude to a delegation skill.
   Targeted reads (offset/limit set), small files, nonexistent files, piped
   commands and redirections all pass through untouched.
2. **Transport** — shell scripts that package the work and hand it to a cheap
   worker (`bulk-read --question --paths`, `code-write --spec --reference
   --target`), via one `aika:invoke-chat` call through the Portal CLI.
3. **Skills** — markdown telling Claude when and how to call the scripts.

Their reported results, benchmarked against a 162K-line Java monorepo:

| Scenario | Lines | Without | With | Saving |
|---|---|---|---|---|
| Single large file | 4,014 | 33,684 tok | 5,737 tok | 82% |
| Source + test pair | 7,408 | 75,990 tok | 4,148 tok | 94% |
| Multi-file cross-service | 1,281 | 16,221 tok | 821 tok | 94% |

Mean bulk-read saving: **90%**.

What we take from it: the *shape* of the idea, the threshold heuristic, the
discipline of a hard gate rather than a polite suggestion, and — importantly —
their honest limitations section. What we do not take: the worker itself.

## Why opencode is the right worker for us

Spotify's worker is a stateless chat completion with no tools. Ours does not
have to be, and that difference is the whole reason this is worth building
rather than just installing `shunt`.

- **It is a real harness, not a summarizer.** opencode's worker has its own
  agent loop with `read`/`glob`/`grep`/`edit`/`bash`/`lsp`. It can decide what
  to look at rather than being handed a fixed file list, and it can write files
  itself rather than returning text for someone else to write.
- **It is free at the tier we need.** OpenCode Zen exposes models like
  `opencode/big-pickle` and `opencode/deepseek-v4-flash` without billing
  enabled. Verified, not assumed — see the evidence log below.
- **It is model-agnostic.** The worker model is a config value. If Big Pickle
  regresses, or a cheaper/better option appears, that is a one-line change and
  not an architecture change.
- **It is local and controllable.** A headless server with a typed API on
  loopback fits how Kennel already thinks about processes, worktrees, and
  session state. This is not a SaaS dependency; it is a subprocess.
- **It has a real permission engine.** We can construct a worker that is
  *incapable* of writing, rather than one we have merely asked nicely not to.

## Evidence log — what we actually verified

Recorded because the interesting failures are not in anyone's documentation.
All of this was tested directly against opencode `1.18.26` on macOS.

**Auth and cost**
- `opencode auth login` prompts for a provider (OpenCode Zen is the default),
  then points at `opencode.ai/auth` and asks for one API key. Nothing else.
- That URL resolves to an OpenAuth screen offering only *Continue with GitHub* /
  *Continue with Google*. It is a real account tied to an existing identity —
  "no account at all" is not achievable.
- The key works **without enabling billing**. Confirmed by running real prompts
  against `opencode/big-pickle` and getting `cost: 0` back in the session
  accounting.

**Capability**
- The worker genuinely uses tools. Asked to review a file, it ran `Glob` then
  `Read` and returned an accurate answer grounded in the real contents.
- Asked to fix a planted off-by-one bug, it produced a correct `Edit` and a
  clean diff. It is competent at the class of work we intend to give it.

**Read-only enforcement — the important finding**
- A prompt-level "READ-ONLY MODE, do not edit" instruction *was* respected: only
  `Glob`/`Read`, no mutations, clean `git status`. A control run without the
  instruction edited the file, confirming the instruction was the cause.
- But prompt compliance is a behaviour, not a guarantee. A worker agent declared
  with `permission: {edit: deny, write: deny, bash: deny}` refused to mutate
  anything *even when directly ordered to fix the bug*, replying that as a
  read-only reviewer it could not modify the file.
- **Intent: any read-only worker role is defined by denied permissions, never by
  prompt wording.** This is the one place we should be stricter than Spotify,
  because their worker is safe by construction (no tools) and ours is not.

**Transport characteristics**
- Sync message delivery blocks the HTTP connection for the entire duration of
  the worker's turn — ~18 seconds for a *trivial* prompt. Unusable as the
  primary path for real work.
- Async dispatch returns in ~20ms, with progress and completion observable on a
  separate server-sent event stream (`message.updated`, `message.part.updated`,
  `session.status`, `session.diff`, and so on, correlated by session id).
- **There is no WebSocket for agent traffic.** The event channels are SSE only
  and explicitly ignore upgrade requests. The single genuine WebSocket endpoint
  is for raw PTY/terminal streaming and is unrelated. Long-running work is made
  safe by asynchronous dispatch plus a progress channel, not by a socket.

**Quirks worth remembering**
- opencode's built-in `explore` agent is registered as a *subagent*. It cannot
  be dispatched directly as a primary agent; attempts silently fall back to the
  default. Any read-only worker role of ours has to be its own declared agent.
- `opencode agent create`'s LLM-assisted generation step fails on the free tier
  with `OpenCode's free tier can only be used in OpenCode`. Normal session and
  message traffic on free models is unaffected. Only that convenience command is
  gated.

## What we want to build

Three worker roles, in increasing order of trust. The first two are Spotify's;
the third is ours and is the actual reason for doing this inside Kennel.

**1. Bulk context reader.** Claude has a question that requires ingesting a lot
of text. The worker ingests it; Claude receives the answer. The corpus never
enters Claude's context. This is the role with a hard gate in front of it — a
read over the threshold should be *blocked and redirected*, not left to Claude's
discretion, because the whole saving evaporates if the expensive model reads the
file anyway. This worker must be permission-denied from writing.

**2. Scaffolder.** Predictable, pattern-following output: tests, config, type
stubs, fixtures — the cases where most of the result is derivable from an
existing reference file. The worker writes to disk directly so that neither the
prompt nor the generated output crosses Claude's context. Claude reviews the
result and makes the small share of surgical edits that need judgment.

**3. Supervised implementation delegate.** *(New — Spotify does not do this.)*
For well-scoped mechanical coding work — apply this refactor across N files,
implement this fully-determined spec, fix this known class of lint error —
Claude writes the spec and acceptance criteria, the worker does the work in a
sandboxed worktree with its own tool loop, and Claude reviews the resulting
diff and verifies it independently. Correction is a follow-up turn to the same
worker, which is cheap; reclaiming the task entirely is always available when
the worker is out of its depth.

### Division of labour — the standing rule

Delegate work that is **high-volume and low-judgment**. Keep work that is
**low-volume and high-judgment**.

Never delegated, regardless of size:
- Debugging. Spotify's worker found surface patterns but missed a subtle
  thread-safety bug that the smart model caught immediately. Our own testing
  showed competence on a *planted, obvious* bug — that is not the same thing.
- Architectural decisions and design tradeoffs.
- Safety-critical or security-sensitive reasoning.
- Anything where exact line-level fidelity matters for a subsequent edit — a
  worker's summary is not a reliable substitute for having read the code.

Also not delegated on economic grounds: small reads. Below some threshold the
round-trip latency costs more than the tokens saved. Spotify's 350-line default
is a starting hypothesis for us, not a settled number — ours should be measured
against real Kennel sessions.

### What Kennel gets that a plain plugin does not

- **Delegation becomes visible.** Kennel already tracks sessions, worktrees and
  activity. Delegated work should appear as observable sub-work with its own
  progress, not as an opaque subprocess call inside someone's shell. The
  progress channel exists to make this possible.
- **Sandboxing is real.** The worker's working directory can be pinned to the
  contribution's worktree, so even an unbounded write cannot escape it. Spotify
  gets this for free by having a worker with no filesystem at all; we get it by
  scoping, and scoping is strictly more useful.
- **Cost accounting becomes a first-class number.** The worker reports token
  counts and cost per turn. Kennel is the natural place to surface "what did
  this contribution cost, and how much of it was delegated" — which is also how
  we prove this tier is earning its keep.
- **It applies fleet-wide.** A per-session saving multiplied by every session in
  the fleet is the entire argument for putting this at the orchestrator layer
  instead of leaving each session to fend for itself.

## Non-goals

- Replacing Claude Code. The smart harness remains the one that plans, reviews,
  decides and is accountable for the result.
- Making the worker autonomous. Every delegated result is reviewed. "The worker
  said it was done" is not evidence that it is done.
- Squeezing savings out of prompt compression, context trimming, or caching
  tricks. Those are different projects; this one is about *not doing the work at
  the expensive tier in the first place*.
- Depending on a specific model. Big Pickle is today's default because it is
  free and it worked, not because anything is built around it.
- Building our own agent runtime. opencode already is one.

## Open questions

These are genuinely undecided and should not be resolved by whoever implements
first without discussion.

1. **Worker lifecycle.** One long-lived worker process per worktree, or one per
   delegation? Warm reuse avoids repeated startup cost; per-call isolation is
   simpler and has no state to reason about. This is a real tradeoff, not an
   obvious call.
2. **Stateless or conversational delegation.** Spotify re-sends the corpus every
   call because their worker cannot remember. Ours can. Reusing a worker session
   for follow-up questions would be cheaper and faster — at the cost of having
   to track which worker holds which context, and of the worker's context window
   filling up. Worth doing eventually; probably not worth doing first.
3. **Where the threshold lives.** A global line count is crude. Kennel could
   plausibly tune it per project, per language, or per observed saving.
4. **Whether the scaffolder gets a hard gate.** Spotify explicitly does not have
   one, and flags it as a known limitation: nothing forces delegation of
   boilerplate, it relies on Claude noticing. We could do better, or we could
   accept the same limitation. Undecided.
5. **Free-tier durability.** We are building on models that are currently free
   under someone else's commercial decision. The design should assume the
   worker model is swappable; we should not assume the price stays zero.
6. **Failure semantics.** What happens when the worker is wrong, slow, or
   unavailable? Falling back to Claude doing the work directly is correct but
   silently expensive — it needs to be visible, not invisible.

## References

**Spotify — the prior art**
- Engineering writeup: *Portal by Spotify cut my Claude Code token usage by 90%*
  — https://engineering.atspotify.com/2026/9/portal-by-spotify-cut-my-claude-code-token-usage-by-90
- Plugin source (Apache-2.0): https://github.com/spotify/portal-ai-plugins
  — specifically `plugins/shunt/` (hooks, transport scripts, skills, and a
  51-case eval suite covering hook routing and transport plumbing).
- Secondary coverage: Analytics India Magazine —
  https://analyticsindiamag.com/ai-news/spotify-cuts-claude-code-token-usage-by-90
  and Techzine —
  https://www.techzine.eu/news/devops/144093/spotify-reduces-claude-code-token-usage-by-90-percent/

**opencode — the intended worker**
- CLI reference: https://opencode.ai/docs/cli/
- Headless server and API: https://opencode.ai/docs/server/
- Agents, modes and permissions: https://opencode.ai/v2/docs/agents/
- Source: https://github.com/anomalyco/opencode
- Zen sign-in (GitHub/Google OAuth): https://opencode.ai/auth

**Local verification**
- Everything in the evidence log above was produced against opencode `1.18.26`
  installed via Homebrew, on macOS, in a throwaway scratch repository. None of
  it is quoted from documentation; where documentation and observed behaviour
  disagreed, observed behaviour is what is recorded here.

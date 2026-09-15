# Delegated bulk reader experiment — 10 September 2026

The opt-in [prototype](../../prototypes/delegated-worker/README.md) proves the
permission-denied opencode reader and async/SSE transport on real Kennel files.
It is an experimental Claude plugin, not a daemon integration or a production
policy decision. The [intent document](delegated-worker-tier.md) remains the
source of truth; only its bulk context reader role is implemented.

## Worker measurements

macOS, opencode **1.18.26**, model **opencode/big-pickle**, Kennel source at beta
commit `670a422` (full revision is retained in the local eval report). Every answer
was checked against independently extracted exported Go type declarations. Every
requested file also had a successful worker `read` event and unchanged SHA-256.
These questions are factual inventories, not debugging or architectural analysis.

| Kennel source | Lines | Without: estimated context tokens | With: estimated context tokens | Saving | Worker round trip |
| --- | ---: | ---: | ---: | ---: | ---: |
| `backend/internal/domain/outcome_proof.go` | 343 | 3,466 | 582 | 83.2% | 33.53 s |
| `backend/internal/domain/outcome_decomposition.go` | 353 | 3,208 | 625 | 80.5% | 19.89 s |
| `backend/internal/service/project/service.go` | 909 | 7,948 | 609 | 92.3% | 32.16 s |
| `backend/internal/adapters/agent/opencode/opencode.go` + `opencode_test.go` | 1,549 | 12,565 | 613 | 95.1% | 61.77 s |

Mean saving: **87.8%**. Corpus-weighted saving: **91.1%**. Async POST dispatch:
**8.7–9.1 ms**. Provider-reported worker cost: **0 for every call**, including the
mutation test. This describes these calls, not a future price guarantee.

**Estimator, not billing:** the table uses `ceil(unicode characters / 4)`, as in
shunt's benchmark. Without is source text. With includes the full skill, hook
rejection, question, paths, answer and answer wrapper. It does not include all
Claude system prompts, repeated context across turns, or model reasoning. Tool
JSON framing and the complete shell invocation are not fully represented by this
estimator; actual Claude usage is reported separately below. The 343-line case is
an explicit worker call for threshold comparison; the default hook lets it through.

There is one final sample per row. Earlier exploratory successful samples ranged
from 7.5 to 20.8 seconds; the final rerun was slower. Other live evaluation work ran
on the same machine during this experiment. This is not a controlled latency
distribution, a fair cold-cache dollar comparison, or evidence of general summary
quality. The source/test pair has very few exported types, making its question
especially compressible. Do not market 87.8% as whole-session/fleet savings.

## Actual Claude end-to-end result

The final paired test used Claude Code with `--model sonnet`, which reported
`claude-sonnet-5`, on the 353-line `outcome_decomposition.go`. Both answers matched
the external oracle. Baseline: one successful Read. Delegated: **one denied Read,
one Skill, one Bash worker invocation, no subsequent source reads**. The retained
tool results were checked for long source-line leakage. The worker's answer was
the only source-derived inventory returned to the delegated orchestrator.

| Actual main-model usage, summed across API requests | Without delegation | With delegation |
| --- | ---: | ---: |
| Uncached input tokens | 4 | 8 |
| Cache creation input tokens | 24,376 | 6,183 |
| Cache read input tokens | 17,937 | 71,945 |
| **Total input tokens including cache traffic** | **42,317** | **78,136** |
| Output tokens | 1,148 | 1,835 |
| End-to-end wall time | 17.50 s | 37.71 s |
| CLI-reported total USD, including auxiliary model | $0.1699821 | $0.0873435 |

**Total input traffic increased 84.6% in this small-file case.** Extra hook/skill/
worker turns repeatedly submit the existing context. Cached input is cheaper but
still token traffic. The lower reported dollars are confounded by different cache
creation/read mixes and sequential trial order; they are not a defensible 48.6%
cost-saving claim. These are reported API cost equivalents, not a subscription
invoice. Both runs also reported the same auxiliary Haiku use: 1,023 input and
18 output tokens, included in the USD row but excluded from the main-model rows.

This is a scripted **path test**, not a naturalistic delegation-compliance study:
both prompts explicitly request the initial full Read, and the final system
instruction assigns factual verification to the external evaluator. Earlier
trials exposed two important failures: one stalled at the Claude API and hit the
300-second harness timeout; another got a correct worker answer but reconstructed
the file with bounded Reads. The latter failed the no-source-ingestion assertion.
The targeted-read exception therefore remains a real production limitation.
The final skill explicitly distinguishes external inventory verification from
minimal exact-context reads before consequential decisions; it does not claim to
make arbitrary context exfiltration impossible. The final skill wording is slightly
longer than the measurement snapshot used in the worker estimator table.

## Read-only and failure evidence

The live adversarial test first requested a real read, then explicitly ordered:
change a fixture constant using edit/write, use Bash if needed, create another
file, and overwrite a sentinel outside the worktree. The full fixture's file
hash map and outside sentinel were unchanged. The recorded successful tool was
`read`; mutation tools were unavailable under denied permissions. The test took
60.75 seconds in the final run. This proves the tested permission configuration's
behavior; refusal text alone was not used as evidence.

The primary agent is hand-declared. The transport inspects its **resolved** rules
and confirms the server directory before dispatch. Blanket-denied tools plus an
exact-file read allowlist are stronger than a prompt-only reader. Cwd is scoped
to the contribution worktree, but **cwd is not an OS sandbox**. Future workers
that can write require a separate containment design.

Observed failures were useful:

- Opencode appends a tool-output external-directory exception. An initial strict
  preflight rejected it before any model call. The final check admits only that
  known exception, retaining the read allowlist and write/shell denies.
- Read permissions match relative paths in this version even for absolute tool
  inputs. The initial absolute-only allowlist caused denied reads. The transport
  rejected the resulting answer because no successful read facts existed. The
  final allowlist includes both exact forms.
- A headless Claude invocation rejected a shell command containing `$PWD` and
  then stopped with a visible fallback question. It did not ingest the raw file.
  The skill now relies on cwd without shell expansion; normal orchestrator shell
  permission still needs to admit the reader command.

Offline tests cover threshold boundaries and ranges, shell quoting/multiple paths,
symlink/traversal rejection, SSE framing and disconnects, provider/permission errors,
wrong or incomplete agent turns, absent reads, paths-only async dispatch, and owned
process cleanup. Live evals are explicit commands, not network calls in unit tests.

## Six decisions remain open

| Decision | Prototype choice and tradeoff | Recommendation for discussion |
| --- | --- | --- |
| Worker lifecycle | Fresh process per call, killed on success/failure. Simple ownership; repeated startup and no warm reuse. | Keep this for isolation experiments. Measure a warm process per worktree before choosing production ownership; put eventual ownership/progress behind daemon services and ports. |
| Stateless vs conversational | Fresh opencode session every call, although opencode itself is stateful. No stale context mapping; loses follow-up reuse. | Defer conversational reuse until session/worktree/source-revision identity and context limits have an explicit contract. |
| Threshold location/value | Opt-in plugin environment variable, 350-line default hypothesis. Actual requested range size controls Read gating. | Do not adopt 350 fleet-wide. Similar savings immediately below/above it and substantial latency show no special breakpoint. Compare byte/token estimates and observed whole-turn costs across real tasks before selecting a project/global/adaptive policy. |
| Scaffolder hard gate | No scaffolder or write gate implemented. | Leave undecided. This read-only experiment offers no evidence for a writing policy. |
| Free-tier durability | Model is a `provider/model` setting; observed worker costs were zero. | Keep the provider swappable. Discuss pricing/budget admission and paid-model fallback before rollout; never assume future free availability. |
| Failure semantics | Deadline, SSE disconnect, missing/incorrect agent, denied permission, incomplete turn, missing read, changed source, or oversized/empty answer fails visibly. No automatic raw-read fallback or retry. | Retain explicit cost-visible fallback. Discuss retry budgets and human versus orchestrator authority for reclaiming work. A successful transport does not establish factual correctness; independent verification remains necessary. |

The recommended next step is a broader opt-in experiment, not fleet deployment:
sample genuine low-judgment questions at multiple sizes, measure total Claude
usage/latency with cache conditions recorded, and grade answer correctness without
reading the bulk corpus back into Claude. Debugging, architecture, safety reasoning,
and exact edit context remain excluded regardless of measured savings.

## Reproduce and inspect

See the [prototype README](../../prototypes/delegated-worker/README.md) for the
plugin invocation, offline tests, live benchmarks, and paired Claude eval.
Private transcripts and receipts live under `~/.kennel/delegated-worker` (or the
documented Kennel overrides); they are deliberately not committed. The committed
artifact is this measurement report and the reproducible evaluation code.

Reference behavior was checked against [opencode's server API](https://opencode.ai/docs/server/),
[permission semantics](https://opencode.ai/docs/permissions/), and
[Claude's hook contract](https://code.claude.com/docs/en/hooks). Spotify shunt was
studied at `3c24ca30ff63e1f5bbad1c43fe5324daff579123`; its Portal transport is replaced,
not reused. No WebSocket is involved.

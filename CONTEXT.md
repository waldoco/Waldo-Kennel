# Domain glossary

The canonical topology and diagrams are in [docs/architecture/persistent-mission-runtime.md](docs/architecture/persistent-mission-runtime.md).

- **Outcome:** durable owner goal and completion boundary.
- **Contract:** frozen, versioned statement of desired result, constraints, authority, and acceptance criteria.
- **Planning conversation:** fresh `/mission` Codex thread that receives a frozen Contract and proposes a Plan.
- **Plan:** approved, versioned WorkUnit DAG, edges, checks, profiles, and effect limits.
- **WorkUnit:** bounded unit of execution and verification.
- **Attempt:** one execution lineage for one WorkUnit. In vNext it owns one exclusive worktree lease, one Kennel Session, and one persistent primary Codex thread.
- **Kennel Session:** durable controller identity around a provider conversation; subordinate to the Attempt.
- **Provider thread:** Codex app-server conversation containing many turns. It is not a WorkUnit or authority source.
- **Turn:** one unit of provider activity inside a thread. Turn completion is not WorkUnit completion.
- **Mission Supervisor:** persistent mission-level intelligence for the active Plan revision. It reads typed events and emits recommendations or bounded commands.
- **Kernel daemon:** deterministic state, authority validation, scheduler, custody, checks, recovery, and projection layer.
- **S1:** source-accepted authenticated local owner-command authority and compatibility ingress; packaged macOS/runtime proof is pending.
- **MissionProjection:** the single durable UI/plugin projection of graph, Attempts, attention, supervision, evidence, Result, and next action.
- **Input manifest:** versioned, hashed context selected for one Attempt.
- **Output manifest:** structured claim, revisions, changes, artifacts, checks, risks, and handoff from an Attempt.
- **`needs_you`:** nonterminal attention state that retains the same Attempt/session/thread and worktree custody.
- **Ready for verification:** explicit worker claim that causes daemon checks; not process or turn exit.
- **Accept:** owner decision on the assembled verified Result.
- **Legacy one-shot:** historical execution generation, readable but never resumed into vNext.

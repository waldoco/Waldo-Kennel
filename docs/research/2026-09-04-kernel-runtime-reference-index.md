# Kernel runtime and provider reference index

> **Classification for vNext (2026-09-15):** runtime evidence reference, not product authority. Use [persistent mission runtime](../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../roadmap/persistent-session-execution-map.md).


- **Status:** Active engineering reference; not product ontology authority
- **Date:** 2026-09-04
- **Purpose:** give implementation agents high-signal primary/reference sources for provider control, workspace orchestration, runtime observability, and context patterns without re-running broad product research

Use this document when implementing adapters, scheduler/workspaces, receipts, external ingress, or runtime reconciliation. The canonical Kennel ontology remains `docs/product/kennel-v1-product-architecture.md`.

## 1. Rule for borrowing from other systems

> **Borrow proven runtime primitives and failure discipline; do not inherit another product's task/session ontology.**

Kennel should converge toward:

> Emdash/Conductor-level workspace reliability + Herdr-level explainability + Paseo/Superset-style programmatic orchestration + Xirp/LifeOS-style structured context + Waldo's Outcome/Contract/Evidence/Acceptance model.

## 2. Provider primary/control references

### Codex

References:

- https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md
- https://github.com/openai/codex/tree/main/codex-rs/app-server-protocol
- https://github.com/openai/codex/blob/main/codex-rs/docs/codex_mcp_interface.md

Use for structured thread start/resume/fork/list/read, turn/item event streaming, approvals/sandbox/permission state, native identity, child metadata where exposed, and MCP ingress. Persist Attempt/Outcome state independently.

### Claude Code / Claude Agent SDK

References:

- https://code.claude.com/docs/en/cli-usage
- https://code.claude.com/docs/en/headless
- https://code.claude.com/docs/en/agent-sdk/overview
- https://code.claude.com/docs/en/agent-sdk/claude-code-features
- https://code.claude.com/docs/en/team
- https://github.com/agentclientprotocol/claude-agent-acp

Use for user-owned local authentication, resume/continue/fork, headless structured output, hooks/MCP, permission/subagent surfaces. Kennel should not become a Claude subscription credential broker.

### OpenCode

References:

- https://opencode.ai/docs/server/
- https://opencode.ai/docs/sdk/
- https://opencode.ai/docs/cli/
- https://opencode.ai/docs/plugins/
- https://opencode.ai/docs/tools

Use structured server/SDK/session/event/plugin surfaces where stronger than PTY fallback.

### Cursor

Reference:

- https://cursor.com/docs/cli/acp

Use ACP stdio JSON-RPC/session updates and extensions where available. ACP existence alone does not admit a Kennel role; write conformance tests first.

### Pi

References:

- https://github.com/earendil-works/pi
- `packages/coding-agent/README.md`
- `packages/coding-agent/docs/rpc.md`
- `packages/coding-agent/docs/sdk.md`
- `packages/coding-agent/docs/extensions.md`
- `packages/coding-agent/docs/security.md`

Use RPC/SDK/events/session persistence/extensions. Kennel still owns workspace, sandbox/authority, and effect containment.

## 3. Agent Client Protocol

- https://github.com/agentclientprotocol/agent-client-protocol
- https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v2/overview.mdx
- https://github.com/agentclientprotocol/agent-client-protocol/releases

Use ACP as an adapter protocol. Do not make ACP update/session types the canonical Kennel domain model. Pin/negotiate versions and preserve unknown events for forward-compatible reconciliation.

## 4. Workspace and parallel execution references

### Emdash

- https://github.com/generalaction/emdash
- https://github.com/generalaction/emdash/blob/main/packages/core/src/runtimes/workspace-registry/node/create-worktree.ts

Study staged provisioning:

```text
inspect → resolve base/fetch → add worktree → configure branch → verify
```

Important: target validation, narrow fetches, bounded Git-lock retry, stage-specific errors, rollback only owned artifacts, visible debris.

### Conductor

- https://www.conductor.build/docs/reference/scripts

Treat workspace setup/run environment, generated/ignored files, ports, processes, and preview state as real workspace facts, not only a directory or prompt.

## 5. Runtime/control-plane references

### Paseo

- https://github.com/getpaseo/paseo

Study daemon/API/SDK/MCP symmetry: UI and agents/scripts ultimately use the same control operations.

### OpenHands Agent Server

- https://docs.openhands.dev/sdk/guides/agent-server/overview

Study separation of long-lived runtime/workspace from the client/renderer.

### Herdr

- https://github.com/herdrdev/herdr
- https://herdr.dev/docs/agents/

Study explainable runtime state: blocked/failed/recovered should have inspectable reasons/evidence.

### AO / Agent Orchestrator

- https://github.com/Untrivial-ai/agent-orchestrator

Keep useful worktree/terminal/browser/diff/PR/session machinery already inherited. Do not restore AO task/session ontology or broad donor provider surface.

### Other orchestration references

- Nimbalyst: https://github.com/Nimbalyst/nimbalyst
- Proliferate: https://github.com/proliferate-ai/proliferate
- T3 Code: https://github.com/pingdotgg/t3code
- OpenAI Symphony: https://openai.com/index/open-source-codex-orchestration-symphony/

Compare execution ergonomics/self-hosting discipline; do not copy topology merely because another orchestrator shows many agents.

## 6. Structured context / continuity

- LifeOS: https://github.com/danielmiessler/LifeOS

Use the pattern of structured attributed context—goals, decisions, architecture, conventions—rather than giant transcript replay.

Kennel adaptation:

```text
Project Brief
+ canonical Outcome/Plan facts
+ SessionReceipt / WorkUnitReceipt
+ retained artifacts/evidence
+ explicitly promoted Project Context
→ bounded RunBrief / Waldo brief
```

## 7. Core algorithms/patterns

### DAG validation

At authorization:

1. require unique WorkUnit IDs;
2. require every dependency to exist in the same Plan;
3. reject self-dependency;
4. run deterministic cycle/topological validation;
5. validate authority/capability/evidence/workspace requirements;
6. freeze deterministic ordering/digest.

Use a standard graph algorithm such as Kahn topological sort or DFS cycle detection with deterministic ordering for reproducible tests/digests.

### Runnable frontier

Derive runnable candidates from authorized DAG + retained canonical results. Prefer event-driven recalculation when dependency/Attempt/lease/provider-readiness facts change, plus periodic reconciliation for external/runtime signals.

### Lease/fence separation

Separate at least:

- workspace/write custody;
- shared repository integration;
- consequential external effect/idempotency.

A global Project mutex is safe but defeats parallel execution; no fence is unsafe. Scope fences to the actual shared resource/effect.

### Restart reconciliation

On daemon start:

1. load non-terminal Attempts/leases/effects;
2. probe workspace/process/provider identity using the strongest reliable signal;
3. reconcile without declaring silence as death;
4. mark ambiguity `unconfirmed`;
5. fence duplicate writes/effects;
6. only then create recovery Attempt if policy permits.

The process must be idempotent over repeated restarts.

### Provider event normalization

Normalize native events with provider/protocol version, native identity, canonical Attempt binding, ordering confidence, tool/file/approval/child/usage/result facts, and unknown-event preservation. Do not promote unknown native events directly into product semantics.

### Receipt aggregation

`WorkUnitReceipt` should explain retained result across Attempts, not concatenate transcripts: retained Attempt, superseded/recovery lineage, artifacts/changes, dependency inputs, checks, unresolved contradictions, Evidence candidates, provenance.

A model may summarize this structure for the user; structured facts remain independently inspectable.

## 8. What not to copy

- provider-brand routing folklore;
- task/session as durable user responsibility;
- one global Project execution mutex;
- graph concurrency not backed by scheduler truth;
- transcript parsing as durable state;
- provider completion as acceptance;
- forced cleanup of uncertain work;
- automatic trace → skill promotion;
- ACP or any single provider protocol as Kennel ontology.

When provider documentation changes, update conformance tests and this index if admitted capability changes materially. Do not silently expand product claims based on documentation alone.

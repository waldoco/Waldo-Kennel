# Outcome loop - canonical pointer

The canonical Outcome loop is now [Persistent mission runtime](persistent-mission-runtime.md).

The durable loop remains:

```text
clarify Contract -> plan and approve DAG -> release persistent WorkUnit sessions
-> supervise -> claim -> check -> rework as needed -> Result -> owner Accept
```

The 2026-09-15 architecture adds a persistent Mission Supervisor, one persistent Codex thread per Attempt, nonterminal `needs_you`, typed authority, versioned context/artifacts, and restart reconciliation. Historical versions of this file remain in Git history for provenance.

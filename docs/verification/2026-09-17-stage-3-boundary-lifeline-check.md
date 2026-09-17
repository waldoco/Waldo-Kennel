# Stage 3 boundary lifeline check

- Baseline: `outcome-loop` at `c65051bce745ba05587446da259a2cb19b700f92`
- Scope: architecture and competitor/library comparison only
- Status: design audit, not implementation authority

## Product logic being tested

Kennel's Stage 3 job is narrower than “build a plugin manager.” It must authenticate a Kennel-owned adapter transport without laundering adapter content into owner authority; bind that transport to one installation, digest, harness, mission, app run and generation; reconnect the desktop to the same durable mission; expose one truthful connection state and one repair action; and eventually stage an adapter update without silently changing an active mission's profile or losing the last-known-good adapter.

## External checks

| Pattern | What it proves | Mapping to Kennel | Judgment |
| --- | --- | --- | --- |
| Kubernetes controllers reconcile observed state toward desired state; Operator SDK calls for idempotent reconciliation | Reconnect is an evaluator/reconciler, not a one-time “socket connected” flag | Derive state from durable binding plus fresh observed facts; repeat evaluation safely | Strong fit. Keep one evaluator and avoid a second session controller |
| Kubernetes conditions separate high-level state from evidence and generation | “Running” alone cannot explain stale or degraded state | Keep the simple user state, but retain observed generation and typed reason/evidence internally | Current direction is right; generation evidence must not disappear behind a green badge |
| systemd readiness is an explicit daemon report, not process existence | A process/PID is not proof of readiness | Reconnect must complete a daemon readiness/identity exchange before `connected` | Gap to freeze in S3.5; PID or open socket alone is false confidence |
| VS Code extensions execute with the host's permissions and therefore require publisher/runtime trust controls | Adapter installation is a material local-code boundary | Verify Kennel ownership and exact artifact identity before activation; never treat compatibility as trust | Our content-addressed manifest is necessary but not sufficient if artifact provenance/signing is absent |
| TUF names rollback, freeze, fast-forward and mix-and-match attacks | Hash equality alone does not establish freshness or authorized release lineage | Install/upgrade needs trusted monotonic metadata and expiry, not only artifact digest/version comparison | Largest security gap in current S3.2 boundary; do not describe a bare manifest as a secure updater |
| Nix keeps immutable versions and switches generations atomically, preserving rollback | In-place overwrite creates mixed-version and crash windows | Stage beside active version, activate by atomic pointer/symlink switch, retain last-known-good | Strong pattern for the adapter installer; simpler than inventing a transaction framework |

## Ego and hindrance divergences

1. **Do not invent a generic provider-neutral plugin platform.** Codex-first discovery plus explicit seams is a strength. Multi-provider symmetry now would increase abstraction while S3.3 has not proved one protected transport.
2. **Do not overload `HarnessConnection` into mission ownership.** It is transport identity. Mission lineage belongs to the existing durable mission model; reconnect joins them by IDs and facts rather than creating a new “connection session” timeline.
3. **Do not call a digest-only manifest “signed” or “secure update.”** The architecture prose currently says “signed or content-addressed.” For executable code, content addressing detects substitution only after an expected digest is trusted. It does not establish publisher, freshness, or authorized release order. Stage the first implementation around a pinned Kennel release source and name this limitation honestly.
4. **Do not expose a dashboard of conditions.** The owner asked for one truthful state and one repair. Keep rich evidence internally and return a deterministic primary reason/action. A conditions framework in UI would be copied complexity.
5. **Do not auto-restart a healthy daemon during desktop reconnect.** It forks control and can hide a stale client. First attach, verify readiness/identity, load the mission, then repair only the failed boundary.
6. **Do not silently migrate an active mission after adapter upgrade.** The existing pinned-profile rule is a real safety advantage. Activation may update the installed default, but an active mission remains on its negotiated profile until explicit reconnect migration or a new Attempt.
7. **Do not turn `delivery_unknown` into a retry button.** Industry reconciliation patterns support our conservative stance: state may remain blocked until positive acknowledgement or positive no-effect evidence exists. “Reconnect succeeded” is not delivery evidence.
8. **Do not build a home-grown TUF clone in Stage 3.** Freeze a minimal trusted-release input and monotonic checks now; adopt a standard metadata framework later if distribution scale requires it.

## Recommended boundary

Proceed with two docs-only freezes before code:

- S3.5 owns daemon attach, durable mission/profile selection, deterministic connection evaluation and the blocked-unknown operator projection. It consumes S3.3 transport but does not redefine it.
- Install/upgrade staging owns trusted input, prepare/verify/activate/health/commit/rollback and evidence. It consumes S3.2 classification but grants neither transport nor owner authority.

The smallest honest architecture remains: one existing daemon/controller, one durable mission lineage, one adapter transport identity, one evaluator, one active adapter pointer plus last-known-good generation, and typed evidence at every uncertain boundary.

## Sources

- Kubernetes, “Controllers”: https://kubernetes.io/docs/concepts/architecture/controller/
- Operator SDK, “Common recommendations and suggestions”: https://sdk.operatorframework.io/docs/best-practices/common-recommendation/
- systemd `sd_notify`: https://www.freedesktop.org/software/systemd/man/sd_notify.html
- VS Code, “Extension runtime security”: https://code.visualstudio.com/docs/configure/extensions/extension-runtime-security
- The Update Framework specification: https://theupdateframework.github.io/specification/latest/index.html
- Nix, “How Nix Works” (atomic upgrades and rollbacks): https://nixos.org/guides/how-nix-works/

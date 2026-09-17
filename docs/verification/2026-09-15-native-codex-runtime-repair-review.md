# Native Codex runtime-package repair review

- Baseline: `outcome-loop` at `288bc992bf6c5eb523ffb1652f8d6069596e1314`
- Scope: Stage 1 proof repair only; no live rerun, cutover, or Stage 2 work

## Failed-run lesson

The first Mac run proved protocol negotiation but failed before its first local command because the proof launched a PATH symlink while the installed Codex package locates `codex-code-mode-host` beside the running executable. Production discovery already canonicalizes Unix symlinks, so this patch makes the proof consume that resolver rather than changing it. Protocol compatibility and runtime-companion viability are separate gates.

## Comparative grounding

| Mature pattern | Observed source fact | Kennel invariant adopted |
|---|---|---|
| Codex release packaging | Codex PR #30202 says `codex-code-mode-host` is built, signed, shipped beside `codex`, and sibling lookup is the runtime contract. The reported ChatGPT.app migration defect #32495 shows a PATH symlink can make the same binary look beside the shim; launching the app-bundle target restores commands. | Runtime identity is the canonical installed package executable, not its PATH alias. Resolve once; version, hash, probe, and launch the same target. |
| Apple application bundles | Apple's Bundle Programming Guide defines the app bundle as the unit containing the executable, resources, and integral support files. | Treat bundled companions as part of package viability. Never create a global sidecar symlink to paper over a broken identity. |
| VS Code on macOS | Official setup places the `code` launcher under the app bundle's `Contents/Resources/app/bin`, warns stale aliases must be removed/reinstalled after changes, and documents updates separately. | Record selected launcher and canonical target; stale launchers produce a truthful repair/degraded result instead of silent fallback. |
| JetBrains IDE/Toolbox | Official CLI docs use installation-owned launchers; Toolbox generates per-installation scripts and gives multiple installed versions unique names. | Multiple installations require an explicit, pinned selection and provenance. Do not collapse versions into an unversioned mutable assumption. |

Sources:
- https://github.com/openai/codex/pull/30202
- https://github.com/openai/codex/issues/32495
- https://developer.apple.com/library/archive/documentation/CoreFoundation/Conceptual/CFBundles/BundleTypes/BundleTypes.html
- https://code.visualstudio.com/docs/setup/mac
- https://www.jetbrains.com/help/idea/working-with-the-ide-features-from-command-line.html

The comparison validates the frozen contract's four rules: canonical runtime-package identity, a fast companion viability probe, pinned executable/protocol/profile provenance, and a truthful failed/degraded gate on drift. It does not import another product's updater or launcher architecture.

## Repair checklist

- A small proof helper calls production `ResolveCodexBinary` by default. An explicit `KENNEL_CODEX_BIN` override is resolved and canonicalized once for an intentional test installation. It outputs selected and canonical paths.
- The wrapper versions, hashes, and launches only the canonical target, while retaining both paths in the manifest and log.
- A local-command canary runs alone before the expensive journey. Protocol negotiation cannot substitute for its completed command and sentinel.
- The wrapper refuses an existing nonempty evidence directory and finalizes every accepted run directory with PASS, FAIL, or SKIP, exact exit status, source identity, binary identity, and complete-log hash.
- Deliberate test failure parses real newline-delimited Go test framing, including `=== RUN/PAUSE/CONT/NAME`. It supports both actual orders: verbose/parallel diagnostics buffered before the exact `--- FAIL: TestValue` record, and non-verbose `go test ./...` diagnostics immediately following that exact FAIL record. Source diagnostics must retain real indentation, a nonempty filename/location, and at least one decimal line digit. Exact FAIL records are checked from the original untrimmed line, begin at column zero, and allow no suffix or one valid Go duration in parentheses. Go framing recognizes only exact RUN/NAME/PAUSE/CONT separators; malformed/prose prefixes and suffixes are rejected. Independent fragments, unrelated prose, another test's diagnostics, unindented/re-serialized lines, aggregate/re-serialized, and cross-activity matches are rejected. Repair must run successfully and contain the corrected source.
- Filesystem has an admitted in-worktree write control. Its negative target is canonicalized and proven outside the leased worktree. A failed-command activity counts only when its parsed command arguments contain the boundary-exact target and its status/summary/output carries an explicit enforcement result. An approval activity uses the same exact-argument binding but gets approval semantics only from its status/summary/output; arbitrary command text never supplies semantics. Exact path and URL arguments use canonical equality only; parent, child, prefix-collision, generic OS error, and bare `sandbox` evidence cannot qualify. If directory semantics are needed, the probe must name that exact directory as its operation. Canonical-runtime unit fixtures first resolve the temporary directory itself, avoiding macOS `/var` versus `/private/var` alias failures.
- Network has a local `curl --version` control. DNS, missing-tool, generic connection failures, and failed commands that merely echo `approval required` cannot count. The same failed-command activity must contain the exact URL argument plus an explicit enforcement result in status/summary/output, or the same approval activity must contain that exact URL argument and explicit approval-request/policy semantics outside command text. An unrelated approval cannot upgrade a DNS failure.
- Sanitized evidence retains provider turn ID, item/activity ID, status, summary, command/cwd/exit/duration, and at most 1 KiB bounded output. It never exports raw session history or hidden reasoning.

## Review boundary

This source package does not claim Mac success. A fresh evidence directory and unchanged reviewed source must run on the authenticated Mac. The canary, journey, and steering tests must pass; the resulting manifest and log then require independent acceptance before Stage 2.

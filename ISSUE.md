# Implementation issues and follow-ups

Last updated: 2026-08-26 during issue #77 verification.

This file records verified follow-ups discovered while implementing a feature. Independent work that is large enough to schedule and review separately must also have a dedicated GitHub issue; this file does not replace the canonical issue tracker.

## Escalated: repository-wide lint baseline

- GitHub issue: [#75 Restore the clean repo-wide lint baseline](https://github.com/waldoco/Waldo-Kennel/issues/75)
- Status: open; not caused by issue #32.
- Evidence: `npm run lint` exits 1 with 20 findings on both the issue #32 branch and a clean checkout of `origin/beta` at `296b92a4d822c221ed586a24e13cfa110381419c`.
- Breakdown: 1 errorlint, 1 goimports, 1 prealloc, 16 revive, and 1 staticcheck finding.
- Boundary: fix this in its own coordinated lane. Do not absorb unrelated Home, Outcome, agent-role, or DeepSeek cleanup into issue #32.

## Existing integration follow-ups

These observations are already covered by [#40 Integrate Work with Home through shared intake and ResponsibilityLink](https://github.com/waldoco/Waldo-Kennel/issues/40), so they do not need duplicate GitHub issues:

- An unconfirmed `IntakeSession` is durable and readable after a daemon restart, but the Work board does not yet discover and resume it automatically.
- `ResponsibilityLink` accepts the Home source responsibility as an opaque identifier. Resolving and presenting the canonical Home-side responsibility belongs to the separate Work/Home consumption layer.

## Escalated: governed Project Waldo provider execution

- GitHub issue: [#82 Wire governed Project Waldo provider replies and same-Attempt continuation](https://github.com/waldoco/Waldo-Kennel/issues/82)
- Status: open; discovered while verifying issue #77 against the locked Task 2 continuation contract.
- Evidence: the durable Project conversation, context, policy, and receipt contracts are implemented, but production constructs the continuation service without a canonical-facts or replacement executor adapter. The live Electron send path therefore persists the user turn and explicitly starts no provider response.
- Boundary: issue #77 must remain fail-closed and must not fabricate provider replies, fencing, replacement identity, or caller-supplied canonical bindings. Issue #82 owns the daemon/session-manager integration for provider-backed replies and safe same-Attempt rollover.
- Sequencing: #82 starts from the merged #77 contracts and blocks full Task 2/#77 closure and the real #38 evaluation. It does not block review and merge of the truthful #77 foundation PR or independent #78 renderer composition.

## Security-triage escalation

- During the branch push, GitHub reported unresolved Dependabot alerts on the default branch, including critical and high-severity alerts.
- These alerts predate issue #32 and were not introduced by this branch.
- Triage belongs in the repository's maintainer-only [Dependabot security dashboard](https://github.com/waldoco/Waldo-Kennel/security/dependabot). Do not copy untriaged vulnerability details into a public GitHub issue; open a scoped public issue only after maintainers determine that disclosure is safe.

## Closed by issue #32

- Offline capture failure now keeps the entered statement visible and explicitly marks it unsaved.
- Simple Outcomes advance without a clarification; adaptive intake asks no more than one material question.
- Confirmation and ResponsibilityLink creation are idempotent, stale revisions are rejected, provenance stores references instead of transcript copies, and CDC remains trigger-backed.
- Invalid analyzer output and daemon-interrupted analysis now become explicit retryable `analysis_failed` state instead of remaining stuck in `analyzing`.
- Users can consciously release an unconfirmed intake with a durable cancellation reason; cancellation creates no Outcome.
- Capture and confirmation retries reuse stable request keys, while a confirmed intake remains one Outcome even if a client retries with a new key.
- ResponsibilityLink end conflicts remain distinct from internal storage failures, preserving accurate daemon error envelopes and request IDs.
- The retired pre-#32 understand surface and its obsolete tests were removed; Work now has one Outcome-first intake implementation.

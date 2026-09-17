# Worker slice handoff template

Copy this file for a bounded implementation slice. Delete instructional text before review.

## Slice

- **Milestone / slice:**
- **Owner:**
- **Baseline branch and SHA:**
- **Depends on:**
- **Blocks:**
- **Status:** not started | active | blocked | review

## Outcome

One sentence describing the observable result of this slice.

## Root cause

State the product or correctness failure this fixes. Do not describe the proposed code as the problem.

## Canonical references

- Product contract:
- Architecture contract:
- Admission/event contract:
- Experience contract:
- ADRs or evidence specific to this slice:

## Invariants

List the authority, lifecycle, data, recovery, compatibility, accessibility, and evidence rules that must remain true.

## In scope

- Exact behavior to add, change, redirect, or remove.
- Exact user-visible states, if any.
- Persisted data or migration treatment.

## Out of scope

Name adjacent work the worker must not absorb. Include no broad refactor, formatter churn, unrelated cleanup, issue changes, release changes, push, or PR unless separately authorized.

## File ownership

| Path or package | Allowed change | Other active owner / collision risk |
| --- | --- | --- |
|  |  |  |

A worker must stop and report before editing outside this table or colliding with another slice.

## Implementation constraints

- Use the smallest correct change and existing repository primitives.
- Do not add a second source of truth, policy interpretation, state vocabulary, or compatibility alias.
- Preserve generated-code, migration, and rollback rules.
- Comments explain only non-obvious invariants, safety constraints, or deliberate tradeoffs with a revisit trigger.

## Acceptance criteria

Use observable, binary statements tied to the Outcome. Include failure and restart/recovery behavior, not only the happy path.

## Test and evidence plan

- Focused root regression:
- Adjacent unit/contract tests:
- Integration/E2E gate:
- Full relevant gate:
- Generated/migration checks:
- Visual states and pixel inspection:
- Packaged/native proof:
- Explicitly untested / not claimed:

## Handback

Return one review package containing:

1. baseline and final diff;
2. files added, modified, deleted, or generated;
3. behavior and rationale;
4. all commands and exact outcomes;
5. screenshots/artifacts and what was visually inspected;
6. pre-existing failures separated from regressions;
7. unresolved decisions and blockers;
8. documentation/status updates;
9. proposed commit and PR title/body;
10. confirmation that issues and release state were not changed.

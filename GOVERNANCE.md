# Governance

Waldo Kennel uses maintainer-led, evidence-based governance while the project is
in active beta. Repository history, issues, pull requests, reviews and security
advisories are the durable decision record.

## Roles

- **Contributors** submit issues, documentation, code, tests and reviews.
- **Triagers** help reproduce, label and scope reports without merge authority.
- **Maintainers** review changes, protect project invariants and merge approved PRs.
- **Release conductors** verify and publish an explicitly authorized release.
- **Security maintainers** receive private reports and coordinate disclosure.

Repository administrators grant or remove elevated roles based on sustained,
constructive contributions, sound judgment, security practice and current
capacity. Access is least-privilege and may be reduced when it is no longer
needed. A role does not override branch protection or the product authority in
[AGENTS.md](AGENTS.md).

## Decisions and changes

Implementation branches start from and target `beta`. Required checks and a
reviewing maintainer must pass before merge. `main` accepts only a current
`beta` promotion through a separate pull request, with its promotion and
security gates passing. Direct pushes, force pushes and branch deletion are
blocked for both protected branches, including for administrators.

Routine changes are decided in focused issues and pull requests. Changes to
product authority, durable state, provider boundaries or recovery need the ADR
and verification discipline in [AGENTS.md](AGENTS.md). Maintainers should state
conflicts of interest and avoid being the sole reviewer of changes where their
judgment is materially conflicted.

Only an explicitly authorized release conductor may publish a release. A green
CI run, merge to `beta`, provider completion or verifier success is evidence;
none independently authorizes release or user Acceptance.

## Becoming a maintainer

There is no contribution-count threshold. Prospective maintainers should show a
consistent record of bounded contributions, respectful review, truthful
verification and care with user authority and data. Existing maintainers discuss
the scope privately, then record the granted repository role. CODEOWNERS will be
expanded only after the affected maintainers agree to that responsibility.


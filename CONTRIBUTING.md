# Contributing

We welcome focused contributions: code, documentation, triage, examples and
tests. GitHub issues and pull requests are the durable coordination record.
For non-trivial work, open or comment on an issue before implementation so the
scope, authority documents and shared-file ownership are clear.

## Ways to contribute

| Type             | Examples                                       |
| ---------------- | ---------------------------------------------- |
| Code             | Fixes, features, adapters, performance         |
| Docs             | README, `docs/`, architecture notes            |
| Triage           | Repro bugs, tighten reports, label suggestions |
| Examples / tests | Recipes, edge cases, flaky-test hunts          |

## Quick start

1. **Read the contract** — [AGENTS.md](AGENTS.md) (layout, commands, hard rules, PR hygiene)
2. **Check current truth** — [docs/STATUS.md](docs/STATUS.md), then use [ROADMAP.md](ROADMAP.md) for direction
3. **Pick something focused** — [open issues](https://github.com/waldoco/Waldo-Kennel/issues); check the [milestones](https://github.com/waldoco/Waldo-Kennel/milestones) and issue dependencies
4. **Coordinate scope** — comment on the issue with your proposed slice and check for overlapping work
5. **Open a clear PR** — narrow change, link the issue, user-visible impact, tests
6. **Iterate** — address review; maintainers merge

Need the product/run overview first? Start with [README.md](README.md),
[docs/architecture.md](docs/architecture.md), and
[docs/development.md](docs/development.md).

## Local checks

Install the [pinned toolchain](docs/development.md#toolchain), then run
`npm run bootstrap` from your checkout. Start with the test nearest your change.
The required repository gates and generation rules are in
[AGENTS.md](AGENTS.md#required-commands); the
[development guide](docs/development.md) explains packaging and the full
foundation check. Do not describe a check as passing unless you ran it at the
submitted revision; report failures and unavailable checks with their cause.

For API changes, regenerate OpenAPI and frontend TypeScript together with
`npm run api`. For SQLite changes, add migrations and regenerate with
`npm run sqlc`. User-visible changes also need a real-daemon desktop journey;
fixture tests alone do not prove runtime behavior.

### Bugs and features

Use the GitHub issue forms (**Bug report** / **Feature request**) so reports stay reproducible.
Bug reports should include the Kennel version or commit, environment, repro steps, and expected vs actual behavior.

Feature proposals should describe the user Outcome and current limitation before
suggesting implementation. Roadmap milestones are not assignments; keep each
issue to one falsifiable slice and cite the governing ADR/spec when applicable.

### Pull requests

Follow **PR hygiene** in [AGENTS.md](AGENTS.md): one issue per PR, conventional commits, explicit dependencies and shared-file ownership, intentional omissions, and verification evidence.

Kennel uses `beta` as its integration branch:

1. Refresh `origin/beta` before starting an assigned issue.
2. Create one issue branch from that commit; do not commit directly to `beta` or `main`.
3. Open the implementation PR against `beta` and wait for its required tests and review.
4. Coordinate shared files, generated API artifacts, and migration numbers with the integration owner before editing.
5. Maintainers test the integrated `beta` branch and promote it through a separate `beta` -> `main` PR.

Hosted foundation CI is intentionally opt-in to preserve the project's limited
Actions budget. Open implementation PRs as drafts; after local narrow checks and
review are complete, mark the final head **Ready for review** to request one full
gate: platform-neutral foundation checks run on Linux, while macOS is reserved
for Seatbelt and packaged-app checks. Additional commits do not spend more
minutes automatically. For an already-ready or bot PR, a maintainer can dispatch
`foundation` manually on the exact branch only when a complete run has genuine
decision value. Enable the manual `deep_race` input only for release candidates
or changes to concurrency, scheduling, storage, recovery, or other load-bearing
daemon paths; it runs the full Go race suite before allowing macOS packaging.

Releases are cut only from tested `main` by the designated release conductor. A merge to `beta` is not a release or deployment authorization.

## Community standards

Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md).
Questions belong in [Discussions](https://github.com/waldoco/Waldo-Kennel/discussions),
and vulnerabilities must use the private process in [SECURITY.md](SECURITY.md).
Repository roles and the `beta` to `main` promotion boundary are described in
[GOVERNANCE.md](GOVERNANCE.md).

Thanks for making Waldo Kennel better for the next person who shows up.

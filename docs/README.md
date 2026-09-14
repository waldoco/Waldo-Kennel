# Kennel documentation map

The repository keeps a small canonical set and preserves narrower decisions and evidence without letting dated reports compete with current truth.

## Canonical product and build truth

| Question | Source |
| --- | --- |
| What problem and experience are we building? | [PRODUCT.md](../PRODUCT.md) |
| What is the stable, quality-gated sequence? | [ROADMAP.md](../ROADMAP.md) |
| What is integrated and proven on `beta`? | [STATUS.md](STATUS.md) |
| How does the authoritative Outcome loop work? | [architecture/outcome-loop.md](architecture/outcome-loop.md) |
| What is the proposed W1.0 admission/event seam? | [architecture/admission-and-events.md](architecture/admission-and-events.md) |
| What must the UI do? | [product/experience.md](product/experience.md) |
| What are the regression-prone decisions? | [decisions/product-and-architecture.md](decisions/product-and-architecture.md) |
| What is the September 18 integration view? | [roadmap/launch-2026-09-18.md](roadmap/launch-2026-09-18.md) |
| How is a worker slice handed off? | [handoffs/worker-slice-template.md](handoffs/worker-slice-template.md) |

## Supporting records

- `adr/`: accepted narrow architecture decisions. ADRs record why; canonical docs describe the current whole.
- `contracts/`: detailed durable lifecycle contracts that have not yet been absorbed into implementation or an ADR.
- `verification/`: dated evidence. It proves only its named baseline and tested layer.
- `handoffs/`: temporary execution context and evidence for one bounded slice.
- `research/`: reference material and benchmarks, not product authority.
- `superpowers/plans/` and `superpowers/specs/`: historical implementation plans/specs. They are not current status unless linked from the canonical roadmap.
- `product/` dated files: historical product decisions and audits. [product/experience.md](product/experience.md) is the current UX contract.
- operational guides such as development, daemon, telemetry, CLI, and user docs describe their named surface.

## Document rules

1. Put stable product truth in `PRODUCT.md`, architecture truth in the two canonical architecture docs, sequence in `ROADMAP.md`, and proof/status in `STATUS.md`.
2. A dated report must name its branch/SHA, tested layer, evidence, and limits.
3. Do not create another active roadmap, status ledger, product thesis, or state vocabulary.
4. When a decision changes, update the canonical file and add/revise an ADR when the rationale needs durable review.
5. Preserve migration, audit, and verification history. Archive or replace with a redirect only after references are traced.
6. Each implementation package updates status only after its evidence passes.

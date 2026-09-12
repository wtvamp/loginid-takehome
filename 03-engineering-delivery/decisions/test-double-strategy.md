# Decision: Test-double strategy (S5)

Pattern: rotating devil's advocate, decision 3 of 3 (seat: Marisol Ferran). Proposer: Renata Cole.

## Proposal

Hand-written interface fakes (not generated mocks) for DAO/handler tests — the `Repository` composite and its three sub-interfaces are small and stable enough that a fake struct with configurable fields/funcs per test is less overhead than adopting a codegen tool (`gomock`/`mockery`) for this scope.

## Objections — Marisol Ferran (Enthusiast)

1. **Interface drift is silent with hand-written fakes.** A new interface method still compiles against a stale fake, which silently zero-values it instead of failing a test that should catch the gap. Generated mocks fail loudly or leave a visibly regenerated stub. Marisol's own mitigation: enforce fake regeneration in the same PR as any interface change, as a review checklist item — makes drift a procedural risk, not a tooling one.
2. **Call-count/ordering assertions are free with mocks, hand-rolled with fakes.** Conceded this is likely moot here — S2–S4's test cases are table-driven input/output, not call-sequencing assertions.
3. **Legibility to a reviewer** — conceded weak by Marisol herself: a 3-method, ~20-line fake is faster to read and trust than looking up `mockgen` syntax, for a project this size.

Marisol's own assessment: objection 3 doesn't hold at this scale; objections 1 and 2 are real at scale but this project may not have that scale.

## Ruling (Renata Cole)

**Hand-written fakes stand** — objection 3 was conceded by its own author, and objection 2 doesn't apply given the table-driven test shape already committed to in S5. Objection 1 is real but has a cheap procedural fix, which is adopted rather than reaching for a codegen dependency to solve it.

Adopted: any change to `Repository` or its three sub-interfaces must update the corresponding hand-written fake in the same PR — added as a stated review-checklist item for S2/S5, not left as an assumed discipline. This is the review-time equivalent of what a generated mock would catch automatically, at zero added dependency cost.

## Status

Ruled. No codegen/mocking library added to the module. Carries into S5 as a review-checklist line item.

---
Model: sonnet (Marisol Ferran, objection seat; Renata Cole, lead, proposer + ruling). Turns consumed: 1 hire turn + lead synthesis.

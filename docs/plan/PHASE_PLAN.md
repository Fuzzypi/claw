# Phase Plan

> Binding execution plan for the current project.
> Each phase is locked: no Phase N+1 work begins until Phase N gates pass (Law 1).
> Implementation must conform to the Architecture Blueprint (Law 2).

## Phase 0 — Foundation

**GOAL:** Governance skeleton in place. Verify gates passing.

**VERIFY GATES:** `verify/repo.verify.cjs`, `verify/blueprint.verify.cjs`

## Rules

- Phase N+1 does not begin until Phase N gates pass.
- Phases are append-only once locked.
- Gate criteria cannot be weakened after lock.

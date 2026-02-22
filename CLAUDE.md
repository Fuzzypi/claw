# claw — Claude Instructions

## Governance

Read and obey before any work:
1. `.aos/CHARTER.md` — project purpose and principles
2. `.aos/LAWS.md` — immutable rules
3. `.aos/GATES.md` — verify gate registry

## Roles

Load the appropriate prompt from `.aos/PROMPTS/` based on your assigned role:
- `ARCHITECT.md` — design decisions, blueprint ownership
- `BUILDER.md` — implementation within blueprint boundaries
- `VERIFIER.md` — gate definitions, pass/fail criteria

## Architecture

The binding architecture contract lives at:
`docs/blueprint/ARCHITECTURE_BLUEPRINT.md`

## Phase Plan

`docs/plan/PHASE_PLAN.md` contains the locked phase plan.
No phase N+1 work begins until phase N gates pass.

## Verify Gates

Run `npm run verify:all` before considering any phase complete.

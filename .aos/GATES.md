# Verify Gate Registry — Claw

## Foundation Gates

| Gate | Script | Phase |
|------|--------|-------|
| repo-structure | `verify/repo.verify.cjs` | 0 |
| blueprint | `verify/blueprint.verify.cjs` | 0 |

## Project Gates

| Gate | Script | Phase |
|------|--------|-------|
| store | `verify/claw_phase01_store.verify.cjs` | 1 |
| dispatch | `verify/claw_phase02_dispatch.verify.cjs` | 2 |
| manual | `verify/claw_phase03_manual.verify.cjs` | 3 |
| context | `verify/claw_phase04_context.verify.cjs` | 4 |
| aos | `verify/claw_phase05_aos.verify.cjs` | 5 |
| polish | `verify/claw_phase06_polish.verify.cjs` | 6 |

## Running Gates

```bash
npm run verify:all       # Run all gates
npm run verify:repo      # Repo structure gate only
npm run verify:blueprint # Blueprint gate only
```

## Adding Gates

1. Create `verify/<name>.verify.cjs`
2. Register in this file
3. Add npm script `verify:<name>` to `package.json`
4. Add to the `verify:all` chain

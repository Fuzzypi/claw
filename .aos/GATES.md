# AOS Verify Gate Registry

## Foundation Gates (always active)

| Gate | Script | Phase | Checks |
|------|--------|-------|--------|
| repo-structure | `verify/repo.verify.cjs` | 0 | Required governance files and directories exist |
| blueprint | `verify/blueprint.verify.cjs` | 0 | Architecture blueprint is present, non-empty, has heading |

## Project Gates

Project-specific gates are added here as phases are defined.
Each gate must have:

- A script in `verify/`
- A phase assignment
- Deterministic pass/fail criteria

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

# Phase Plan — Claw

> Binding execution plan for Claw.
> Each phase is locked: no Phase N+1 work begins until Phase N gates pass (Law 1).
> Implementation must conform to the Architecture Blueprint (Law 2).

---

## Phase 0 — Foundation _(complete)_

**GOAL:** AOS governance skeleton in place.

**VERIFY GATES:** `verify/repo.verify.cjs`, `verify/blueprint.verify.cjs` — both passing.

---

## Phase 1 — Store + Job Model

**GOAL:** Initialize Go module, set up SQLite, implement the core data layer
for jobs, pipelines, agents, and context entries. No dispatch, no CLI beyond
`claw version` — just the store with working tests.

**NON-GOALS:**
- No dispatcher
- No gate runner
- No CLI commands (beyond version)
- No context extraction
- No AOS file parsing

### Deliverables

```
src/claw/
├── go.mod                            — module: github.com/fuzzypi/claw
├── go.sum
├── cmd/
│   └── claw/
│       └── main.go                   — Root command (cobra), version subcommand
├── internal/
│   └── store/
│       ├── db.go                     — Open, migrate, close. Creates all tables.
│       ├── jobs.go                   — Job CRUD: Create, Get, List, UpdateStatus, SetOutput
│       ├── pipelines.go              — Pipeline CRUD: Create, Get, List, UpdateStatus
│       ├── agents.go                 — Agent CRUD: Register, Get, List, UpdateStatus
│       ├── context.go                — Context entry CRUD: Add, ListByPipeline
│       ├── dependencies.go           — Add dependency, resolve ready jobs
│       └── store_test.go             — Tests for all store operations
├── Makefile
```

### Schema

All tables as defined in Blueprint sections 4.1–4.5.

Dependency resolution query (ready jobs):
```sql
SELECT j.id FROM jobs j
WHERE j.pipeline_id = ?
  AND j.status = 'pending'
  AND NOT EXISTS (
    SELECT 1 FROM job_dependencies d
    JOIN jobs dep ON dep.id = d.depends_on
    WHERE d.job_id = j.id
      AND dep.status != 'completed'
  )
ORDER BY j.id
```

### Verify Gate

**Script:** `verify/claw_phase01_store.verify.cjs`

Checks:
- `src/claw/go.mod` exists and contains module name
- `src/claw/internal/store/db.go` exists and contains "jobs"
- `src/claw/internal/store/jobs.go` exists and contains "Create" and "Status"
- `src/claw/internal/store/pipelines.go` exists and contains "Pipeline"
- `src/claw/internal/store/agents.go` exists and contains "Register"
- `src/claw/internal/store/dependencies.go` exists and contains "depends"
- `src/claw/internal/store/store_test.go` exists and contains "func Test"
- `src/claw/Makefile` exists and contains "test"
- Running `cd src/claw && make test` exits 0

### DOD

- [ ] Go module initialized with sqlite3 + cobra dependencies
- [ ] SQLite creates all 5 tables + dependency table on first run
- [ ] Job CRUD: create, get, list by pipeline, update status, set output
- [ ] Pipeline CRUD: create, get, list, update status
- [ ] Agent CRUD: register, get, list, update status + current job
- [ ] Context CRUD: add entry, list by pipeline
- [ ] Dependency resolution: query returns jobs whose deps are all completed
- [ ] Cycle detection: adding a dependency that creates a cycle returns error
- [ ] All tests pass: `make test` exits 0
- [ ] `verify/claw_phase01_store.verify.cjs` passes

---

## Phase 2 — Dispatch Engine + Shell Executor

**GOAL:** Job dispatcher that assigns pending jobs to idle agents, and a shell
executor that invokes the agent's configured command with the prompt piped via
stdin. Single-agent execution works end-to-end.

**NON-GOALS:**
- No manual executor (Phase 3)
- No context extraction (Phase 4)
- No gate automation (Phase 4)
- No AOS file parsing (Phase 5)
- No CLI beyond `claw run` and `claw status`

### Deliverables

```
src/claw/
├── internal/
│   ├── dispatch/
│   │   ├── dispatcher.go             — Scheduling loop: assign ready jobs to idle agents
│   │   └── shell.go                  — Shell executor: run command, capture output
│   ├── cli/
│   │   └── commands.go               — run, status commands
│   └── store/                        — (from Phase 1, unchanged)
├── cmd/
│   └── claw/
│       └── main.go                   — (modify) Register run, status subcommands
```

### Dispatcher Loop

1. Query ready jobs (dependencies satisfied, status=pending)
2. Query idle agents (status=idle)
3. For each (job, agent) pair: dispatch
4. Sleep 1 second, repeat
5. Exit when all jobs completed or any job failed

### Shell Executor

1. Build the full prompt: accumulated context + job.prompt
2. Invoke the agent's configured command: `agent.command [agent.args...]`
3. Pipe the full prompt to the agent process via stdin
4. Capture stdout+stderr combined
5. If `rtk` on PATH: pipe output through `rtk --stats`
6. Truncate output to 1MB limit before storage
7. Store output in job.output, exit code in job.exit_code
8. Update status based on exit code
9. Clear `agents.current_job_id` and return agent status to `idle` (or `offline`
   if liveness checks fail)

### Verify Gate

**Script:** `verify/claw_phase02_dispatch.verify.cjs`

Checks:
- `src/claw/internal/dispatch/dispatcher.go` exists and contains "dispatch" or "Dispatch"
- `src/claw/internal/dispatch/shell.go` exists and contains "Command" or "exec"
- `src/claw/internal/cli/commands.go` exists and contains "run" and "status"
- Running `cd src/claw && make test` exits 0

### DOD

- [ ] Dispatcher assigns ready jobs to idle agents
- [ ] Shell executor runs commands and captures output
- [ ] Timed-out/failed jobs clear agent assignment and do not leave agents stuck busy
- [ ] `claw run <pipeline>` executes all jobs in dependency order
- [ ] `claw status <pipeline>` shows job status table
- [ ] Sequential pipeline (A → B → C) executes in order
- [ ] Parallel-eligible jobs dispatch to separate agents concurrently
- [ ] Failed job halts pipeline
- [ ] Retry policy uses `CLAW_MAX_RETRIES` (default 1) for automatic retries
- [ ] All tests pass
- [ ] `verify/claw_phase02_dispatch.verify.cjs` passes

---

## Phase 3 — Manual Executor + Agent Management

**GOAL:** Manual executor for human-in-the-loop dispatch (print prompt, accept
paste-back). Full agent registration and management CLI.

**NON-GOALS:**
- No context extraction (Phase 4)
- No gate automation (Phase 4)
- No AOS file parsing (Phase 5)

### Deliverables

```
src/claw/
├── internal/
│   ├── dispatch/
│   │   └── manual.go                 — Manual executor: print prompt, read response
│   └── cli/
│       └── commands.go               — (modify) Add agent, job, pipeline commands
```

### Manual Executor

1. Print to stdout: `--- JOB: <name> ---`
2. Print full prompt (job prompt + any accumulated context)
3. Print: `--- Paste agent output below (Ctrl+D to end) ---`
4. Read stdin until EOF
5. Store as job output, update status

### New CLI Commands

- `claw pipeline create <name> --project <dir>`
- `claw job add <pipeline> <name> --prompt-file <path> [--depends-on <job-name>]`
- `claw agent register <name> --type <shell|manual> [--command <cmd>] [--cwd <dir>] [--timeout <secs>]`
- `claw agent list`
- `claw output <job-id>`

### Verify Gate

**Script:** `verify/claw_phase03_manual.verify.cjs`

Checks:
- `src/claw/internal/dispatch/manual.go` exists and contains "manual" or "Manual"
- `src/claw/internal/cli/commands.go` contains "agent" and "pipeline"
- Running `cd src/claw && make test` exits 0
- Running `cd src/claw && make build && ./claw --help` exits 0 and output contains "agent"

### DOD

- [ ] Manual executor prints prompt and reads paste-back
- [ ] `claw agent register` adds agent to database
- [ ] `claw agent list` shows all agents with status
- [ ] `claw pipeline create` creates empty pipeline
- [ ] `claw job add` adds jobs with optional dependencies
- [ ] `claw output` shows captured output for a job
- [ ] Mixed pipeline: shell + manual agents can coexist
- [ ] All tests pass
- [ ] `verify/claw_phase03_manual.verify.cjs` passes

---

## Phase 4 — Context Engine + Gate Automation

**GOAL:** Automatic context extraction from job output, context injection into
subsequent jobs, gate execution after job completion, and handoff summary
generation.

**NON-GOALS:**
- No AOS file parsing (Phase 5)

### Deliverables

```
src/claw/
├── internal/
│   ├── context/
│   │   └── extractor.go              — Extract key info from job output
│   ├── gates/
│   │   └── runner.go                 — Run verify gate command, parse result
│   ├── dispatch/
│   │   ├── dispatcher.go             — (modify) Inject context, run gates after jobs
│   │   └── shell.go                  — (modify) Gate execution integration
│   └── cli/
│       └── commands.go               — (modify) Add summary, context, gate commands
```

### Context Extraction Rules

After a job completes, extract:
1. **Status line:** "Job <name>: PASSED|FAILED"
2. **Files changed:** Scan output for file paths (heuristic: lines starting with
   "Created:", "Modified:", "Removed:", or git diff-style output)
3. **Test results:** Scan for "X passed", "X failed" patterns
4. **Gate result:** "Gate <name>: PASSED|FAILED"
5. **Error lines:** First 5 lines matching error patterns (if failed)

Store as a structured context entry.

### Context Injection Format

```
--- PIPELINE CONTEXT (auto-generated by Claw) ---
[1] Job "store" (Phase 1): PASSED — 11 tests, gate passed
    Files: internal/store/db.go, internal/store/jobs.go
[2] Job "mcp" (Phase 2): PASSED — 12 tests, gate passed
    Files: internal/mcp/server.go, internal/mcp/tools.go
---

[original job prompt follows]
```

### Gate Runner

1. Receive gate command string (e.g., "npm run verify:phase01")
2. Execute via shell, capture output and exit code
3. Determine pass/fail: **exit code is authoritative** (0 = pass, non-zero = fail)
4. Store gate_status, gate_exit_code, and gate_output on the job
5. Return pass/fail to dispatcher

Note: checkmarks (✓) in gate output are informational only and never override exit code.

### New CLI Commands

- `claw summary <pipeline>` — Generate handoff summary from context entries
- `claw context <pipeline>` — Show raw accumulated context
- `claw gate <job-id>` — Show gate result for a specific job

### Verify Gate

**Script:** `verify/claw_phase04_context.verify.cjs`

Checks:
- `src/claw/internal/context/extractor.go` exists and contains "extract" or "Extract"
- `src/claw/internal/gates/runner.go` exists and contains "gate" or "Gate"
- `src/claw/internal/cli/commands.go` contains "summary" and "context"
- Running `cd src/claw && make test` exits 0

### DOD

- [ ] Context extracted automatically after each job completes
- [ ] Context injected into subsequent job prompts
- [ ] Gate runs after job completion, result stored
- [ ] Gate pass unlocks dependents, gate fail halts pipeline
- [ ] `claw summary` generates readable handoff from pipeline context
- [ ] `claw context` shows accumulated context entries
- [ ] `claw gate` shows gate result for a job
- [ ] All tests pass
- [ ] `verify/claw_phase04_context.verify.cjs` passes

---

## Phase 5 — AOS Integration + Pipeline Generator

**GOAL:** Read AOS governance files to auto-generate pipelines. `claw init`
scans a project directory and creates a pipeline with jobs, dependencies,
and gates derived from GATES.md and the phase plan.

### Deliverables

```
src/claw/
├── internal/
│   ├── aos/
│   │   └── reader.go                 — Parse GATES.md, PHASE_PLAN.md, package.json
│   └── cli/
│       └── commands.go               — (modify) Add init command
```

### AOS Reader

Parse `<project>/.aos/GATES.md`:
- Extract gate table rows: gate name, script path, phase number
- Map gate scripts to npm verify commands

Parse `<project>/docs/plan/PHASE_PLAN.md` (or project-specific plan):
- Extract phase numbers and names
- Determine phase ordering (Phase N depends on Phase N-1)

Parse `<project>/package.json`:
- Extract verify:* scripts
- Match to gates from GATES.md

### Pipeline Generation

`claw init <project-dir>` produces:
1. One pipeline named after the project
2. One job per phase, named "phase-N-<gate-name>"
3. Each job's gate_command set to the verify npm script
4. Dependencies: phase N depends on phase N-1
5. Job prompts: empty (user fills them in, or references prompt files)

### Verify Gate

**Script:** `verify/claw_phase05_aos.verify.cjs`

Checks:
- `src/claw/internal/aos/reader.go` exists and contains "GATES" or "gates"
- `src/claw/internal/cli/commands.go` contains "init"
- Running `cd src/claw && make test` exits 0

### DOD

- [ ] `claw init ~/aos-platform` generates pipeline from AOS governance files
- [ ] Generated pipeline has correct phase ordering and dependencies
- [ ] Gate commands mapped from GATES.md to package.json scripts
- [ ] Handles projects with no gates (creates pipeline with no gates attached)
- [ ] All tests pass
- [ ] `verify/claw_phase05_aos.verify.cjs` passes

---

## Phase 6 — CLI Polish + README

**GOAL:** Complete CLI experience, log command, persistence configuration,
and full README documentation.

### Deliverables

```
src/claw/
├── internal/
│   └── cli/
│       └── commands.go               — (modify) Add log command, polish all output
├── README.md                         — Full usage docs
```

### Log Command

`claw log [--pipeline <id>] [--limit <n>]`

Shows chronological activity:
```
2026-02-22 14:30:01  Pipeline "rtk-build" created
2026-02-22 14:30:02  Job "phase-1-pipeline" dispatched → agent "builder-1"
2026-02-22 14:32:15  Job "phase-1-pipeline" completed (17 tests passed)
2026-02-22 14:32:16  Gate "rtk_phase01_pipeline" PASSED
2026-02-22 14:32:17  Job "phase-2-filters" dispatched → agent "builder-1"
```

### README Contents

- What Claw does (one paragraph)
- Install (go install, binary)
- Quick start (init project, register agent, run pipeline)
- All CLI commands
- AOS integration guide
- Manual vs shell agents
- Context engine explanation
- Configuration (env vars)

### Verify Gate

**Script:** `verify/claw_phase06_polish.verify.cjs`

Checks:
- `src/claw/internal/cli/commands.go` contains "log"
- `src/claw/README.md` exists and length > 500 chars
- `src/claw/README.md` contains "install" or "Install"
- Running `cd src/claw && make test` exits 0
- Running `cd src/claw && make build` exits 0

### DOD

- [ ] `claw log` shows chronological activity
- [ ] All CLI commands have --help text
- [ ] CLAW_DB_PATH env var respected
- [ ] CLAW_MAX_RETRIES env var respected
- [ ] README covers install, usage, all commands, AOS integration
- [ ] All tests pass
- [ ] `verify/claw_phase06_polish.verify.cjs` passes

---

## Verify Gate Registry

| Gate | Script | Phase |
|------|--------|-------|
| repo-structure | `verify/repo.verify.cjs` | 0 |
| blueprint | `verify/blueprint.verify.cjs` | 0 |
| store | `verify/claw_phase01_store.verify.cjs` | 1 |
| dispatch | `verify/claw_phase02_dispatch.verify.cjs` | 2 |
| manual | `verify/claw_phase03_manual.verify.cjs` | 3 |
| context | `verify/claw_phase04_context.verify.cjs` | 4 |
| aos | `verify/claw_phase05_aos.verify.cjs` | 5 |
| polish | `verify/claw_phase06_polish.verify.cjs` | 6 |

## Dependencies by Phase

| Phase | New Go Dependencies |
|-------|-------------------|
| 1 | `modernc.org/sqlite` (or `github.com/mattn/go-sqlite3`), `github.com/spf13/cobra` |
| 2 | (none) |
| 3 | (none) |
| 4 | (none) |
| 5 | (none) |
| 6 | (none) |

# Architecture Blueprint — Claw

> This document is the binding contract for system design (Law 2).
> Implementation must conform to this blueprint. Amend here first, then change code.

## 1. System Overview

Claw is an orchestration engine for AI agent workflows. It replaces the manual
dispatch loop — where a human pastes builder prompts, tracks parallel agents,
writes handoff summaries, and runs verify gates — with a single Go binary that
manages the full lifecycle.

Claw does NOT replace the agents themselves. It is the coordinator that tells
agents what to do, watches them work, passes context between them, and verifies
results. Think of it as the foreman, not the builders.

### Design Principles

- **One binary, one database.** Go + SQLite. No Docker, no message queue, no cloud.
- **Agent-agnostic.** Works with any agent that accepts a prompt on stdin and produces output on stdout (Claude Code, terminal agents, shell scripts).
- **Convention over configuration.** Reads AOS governance files (GATES.md, phase plans) directly. If you follow AOS conventions, Claw understands your project.
- **Observable.** Every job, every dispatch, every gate result is logged and queryable.
- **Local-first.** All state on disk. No telemetry, no external calls.

## 2. Core Concepts

### 2.1 Job

A unit of work dispatched to an agent. Contains:
- A prompt (the instructions the agent receives)
- Dependencies (jobs that must complete first)
- A verify gate (optional — run after job completes)
- Status: pending → dispatched → running → completed | failed
- Result: captured output from the agent
- Attempt tracking for crash recovery

### 2.2 Pipeline

An ordered set of jobs with dependency edges. Represents a full build
(e.g., "RTK Phase 1 → Phase 2 → Phase 3 → Phase 4"). Jobs without
dependencies can run in parallel.

### 2.3 Agent Slot

A registered execution target. Claw tracks:
- Name (e.g., "builder-1", "builder-2")
- Type: shell (invokes a configured command), manual (prints prompt, waits for paste-back)
- Execution config (shell only): command, args, working directory, timeout
- Status: idle, busy, offline
- Current job assignment

### 2.4 Context

Accumulated state passed between jobs. After each job completes, Claw
extracts key information (files changed, test results, gate status) and
appends it to a running context document. Subsequent jobs receive this
context automatically.

### 2.5 Gate

A verify command that runs after a job completes. Pass/fail is determined
by exit code (0 = pass, non-zero = fail). Exit code is authoritative.
Checkmarks or other output markers are informational only and never
override exit code.

## 3. Component Registry

```
claw (single binary)
├── cmd/
│   └── claw/
│       └── main.go               — CLI entry (cobra), subcommands
├── internal/
│   ├── store/
│   │   ├── db.go                  — SQLite connection, migrations
│   │   ├── jobs.go                — Job CRUD
│   │   ├── pipelines.go           — Pipeline CRUD + dependency resolution
│   │   ├── agents.go              — Agent slot registration + status
│   │   └── context.go             — Context accumulation + retrieval
│   ├── dispatch/
│   │   ├── dispatcher.go          — Job→Agent assignment engine
│   │   ├── shell.go               — Shell executor (invoke agent command, pipe prompt via stdin)
│   │   └── manual.go              — Manual executor (print prompt, accept result)
│   ├── gates/
│   │   └── runner.go              — Verify gate execution + pass/fail
│   ├── context/
│   │   └── extractor.go           — Extract key info from job output
│   ├── cli/
│   │   └── commands.go            — CLI command implementations
│   └── aos/
│       └── reader.go              — Parse AOS governance files (GATES.md, phase plans)
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## 4. Data Model

### 4.1 Jobs Table

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | `INTEGER` | PK, autoincrement |
| `pipeline_id` | `INTEGER` | FK to pipelines |
| `name` | `TEXT` | NOT NULL |
| `prompt` | `TEXT` | NOT NULL |
| `status` | `TEXT` | NOT NULL (pending, dispatched, running, completed, failed) |
| `agent_id` | `INTEGER` | nullable, FK to agents |
| `gate_command` | `TEXT` | nullable (e.g., "npm run verify:phase01") |
| `gate_status` | `TEXT` | nullable (passed, failed, skipped) |
| `gate_exit_code` | `INTEGER` | nullable |
| `gate_output` | `TEXT` | nullable |
| `output` | `TEXT` | nullable (captured agent output, max 1MB, truncated if exceeded) |
| `exit_code` | `INTEGER` | nullable |
| `attempt_count` | `INTEGER` | NOT NULL, default 0 |
| `lease_expires_at` | `DATETIME` | nullable (stale job recovery) |
| `started_at` | `DATETIME` | nullable |
| `completed_at` | `DATETIME` | nullable |
| `created_at` | `DATETIME` | NOT NULL, default CURRENT_TIMESTAMP |

### 4.2 Job Dependencies Table

| Column | Type | Constraints |
|--------|------|-------------|
| `job_id` | `INTEGER` | FK to jobs |
| `depends_on` | `INTEGER` | FK to jobs |

Unique constraint on (job_id, depends_on). No cycles allowed.

### 4.3 Pipelines Table

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | `INTEGER` | PK, autoincrement |
| `name` | `TEXT` | NOT NULL |
| `project_dir` | `TEXT` | NOT NULL (absolute path to project) |
| `status` | `TEXT` | NOT NULL (active, completed, failed, cancelled) |
| `created_at` | `DATETIME` | NOT NULL, default CURRENT_TIMESTAMP |
| `completed_at` | `DATETIME` | nullable |

### 4.4 Agents Table

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | `INTEGER` | PK, autoincrement |
| `name` | `TEXT` | NOT NULL, unique |
| `type` | `TEXT` | NOT NULL (shell, manual) |
| `command` | `TEXT` | nullable (required for shell type — e.g., "claude", "bash") |
| `args` | `TEXT` | nullable (JSON array of additional args) |
| `cwd` | `TEXT` | nullable (working directory for agent execution) |
| `timeout_secs` | `INTEGER` | nullable (max execution time per job, default 600) |
| `status` | `TEXT` | NOT NULL (idle, busy, offline) |
| `current_job_id` | `INTEGER` | nullable, FK to jobs |
| `registered_at` | `DATETIME` | NOT NULL, default CURRENT_TIMESTAMP |

### 4.5 Context Entries Table

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | `INTEGER` | PK, autoincrement |
| `pipeline_id` | `INTEGER` | FK to pipelines |
| `job_id` | `INTEGER` | FK to jobs |
| `content` | `TEXT` | NOT NULL (extracted context summary) |
| `created_at` | `DATETIME` | NOT NULL, default CURRENT_TIMESTAMP |

### 4.6 Database Location

Default: `~/.claw/claw.db`
Override: `CLAW_DB_PATH` environment variable.

### 4.7 Output Limits

Job output stored in `output` and `gate_output` is truncated to 1MB before
storage. If output exceeds 1MB, keep the first 256KB and last 256KB with a
`[... truncated N bytes ...]` marker. This prevents database bloat from
verbose build logs.

### 4.8 Runtime Configuration

- `CLAW_MAX_RETRIES` controls automatic retry count for failed/timed-out jobs.
  Default: `1` (one retry after the initial attempt).
- Retry eligibility rule: if `attempt_count <= max_retries`, reset job status
  to `pending`; otherwise leave as `failed`.

## 5. Dispatch Engine

### 5.1 Scheduling Algorithm

When an agent becomes idle:
1. Query all pending jobs whose dependencies are ALL completed
2. Sort by pipeline order (lower job ID first)
3. Assign first eligible job to the idle agent
4. Set lease_expires_at to now + agent.timeout_secs
5. Increment attempt_count
6. Update job status to dispatched, agent status to busy

### 5.2 Stale Job Recovery

On each scheduling tick, check for jobs where:
- status = dispatched or running
- lease_expires_at < now

These jobs have exceeded their timeout. Mark them as failed with
output: "[claw] job timed out after {timeout} seconds", clear
`lease_expires_at`, clear `agents.current_job_id`, and set the agent status
back to `idle` (or `offline` if a liveness check fails).

Use `CLAW_MAX_RETRIES` (default `1`) for retry policy. If
`attempt_count <= max_retries`, reset status to `pending` for automatic retry.

### 5.3 Shell Executor

For type=shell agents:
1. Build the full prompt: accumulated context + job.prompt
2. Invoke the agent's configured command: `agent.command [agent.args...]`
3. Pipe the full prompt to the agent process via stdin
4. Capture stdout+stderr combined
5. If `rtk` is on PATH: pipe output through `rtk --stats`
6. Truncate output to 1MB limit before storage
7. Store output in job.output, exit code in job.exit_code
8. Run gate command if specified
9. Update status: exit_code 0 + gate passed = completed, else failed
10. Always clear `agents.current_job_id` and return agent status to `idle`
    (or `offline` if process/liveness checks fail)

### 5.4 Manual Executor

For type=manual agents:
- Print to stdout: `--- JOB: <name> ---`
- Print full prompt (accumulated context + job.prompt)
- Print: `--- Paste agent output below (Ctrl+D to end) ---`
- Read stdin until EOF
- Store as job.output
- Run gate command if specified
- Update status

## 6. Context Engine

### 6.1 Automatic Context Extraction

After each job completes, extract a context summary:
- Job name and status (passed/failed)
- Files changed (parse from output if available)
- Gate result (passed/failed + key output lines)
- Error lines (if failed)

Store as a context entry linked to the pipeline.

### 6.2 Context Injection

When dispatching a job, prepend accumulated context:
```
--- PIPELINE CONTEXT (auto-generated by Claw) ---
[1] Job "store" (Phase 1): PASSED — 11 tests, gate passed
    Files: internal/store/db.go, internal/store/jobs.go
[2] Job "mcp" (Phase 2): PASSED — 12 tests, gate passed
    Files: internal/mcp/server.go, internal/mcp/tools.go
---

[original job prompt follows]
```

### 6.3 Handoff Summary Generation

`claw summary <pipeline-id>` generates a handoff summary from all
context entries. This replaces manually written handoff documents.

## 7. Gate Automation

### 7.1 AOS Integration

`claw init <project-dir>` reads:
- `.aos/GATES.md` → extracts gate scripts and phase assignments
- `docs/plan/PHASE_PLAN.md` → extracts phase order and dependencies
- `package.json` → extracts verify npm scripts

Generates a pipeline with jobs for each phase, gates attached automatically.

### 7.2 Gate Execution

After job output is captured:
1. Run gate command (e.g., `npm run verify:phase01`)
2. Capture gate output and exit code
3. Determine pass/fail: **exit code is authoritative** (0 = pass, non-zero = fail)
4. Store gate_exit_code and gate_output on the job
5. If passed: mark job completed, unlock dependents
6. If failed: mark job failed, attach gate output, halt pipeline
   (unless --continue-on-failure)

Note: checkmarks (✓) or other markers in gate output are informational
only and never override exit code determination.

## 8. CLI Interface

| Command | Description |
|---------|-------------|
| `claw init <dir>` | Scan AOS project, generate pipeline from phase plan |
| `claw pipeline create <name> --project <dir>` | Create empty pipeline |
| `claw job add <pipeline> <name> --prompt-file <path>` | Add job to pipeline |
| `claw job add <pipeline> <name> --depends-on <job>` | Add with dependency |
| `claw agent register <name> --type <shell\|manual> [--command <cmd>] [--cwd <dir>] [--timeout <secs>]` | Register agent slot |
| `claw agent list` | Show all agents and status |
| `claw run <pipeline>` | Start pipeline execution |
| `claw status [pipeline]` | Show pipeline/job status |
| `claw output <job-id>` | Show job output |
| `claw gate <job-id>` | Show gate result |
| `claw summary <pipeline>` | Generate handoff summary |
| `claw context <pipeline>` | Show accumulated context |
| `claw log` | Show recent activity |

## 9. Build & Dependencies

### 9.1 Build

```bash
go build -o claw ./cmd/claw
```

Single binary. Pure Go — no CGo required. Uses `modernc.org/sqlite` for
CGo-free SQLite, or `github.com/mattn/go-sqlite3` if CGo is available.
No FTS5 needed — Claw uses plain SQL queries.

### 9.2 Makefile

```makefile
.PHONY: build test

build:
	go build -o claw ./cmd/claw

test:
	go test -vet=off -v ./internal/...
```

### 9.3 Go Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/mattn/go-sqlite3` | SQLite driver (CGo) — OR — |
| `modernc.org/sqlite` | SQLite driver (pure Go, CGo-free) |
| `github.com/spf13/cobra` | CLI framework |

Prefer `modernc.org/sqlite` for zero-CGo builds. Fall back to mattn if
modernc causes issues.

Minimal dependency tree. No MCP, no HTTP — this is a local orchestrator.

## 10. Security Model

- **Local-only control plane.** Claw itself exposes no network server APIs
  (no HTTP server) and sends no telemetry.
- **Agent trust.** Claw trusts registered agents. It dispatches prompts and
  reads output. No sandboxing of agent execution.
- **File access scope.** Claw itself reads/writes:
  - `~/.claw/` (database, config)
  - AOS governance files in project directories (read-only: .aos/, docs/, package.json)
  - Dispatched jobs execute with the permissions of the invoking user. Shell
    agents can access any files the user can access. Claw does not restrict
    this — the agent's configured `cwd` determines the working directory,
    but no filesystem sandbox is enforced.
- **Network scope for dispatched jobs.** Shell-agent commands may perform
  network I/O if the host/user permits it. Claw does not sandbox or firewall
  child processes.
- **Output limits.** Job output truncated to 1MB to prevent storage abuse.

## 11. RTK Integration

If `rtk` is on PATH, shell executor pipes command output through
`rtk --stats` automatically. This compresses noisy build output before
storing in job.output, saving context window space for subsequent jobs.

# Claw

Orchestration engine for AI agent workflows. Claw replaces the manual
dispatch loop — where a human pastes builder prompts, tracks parallel
agents, writes handoff summaries, and runs verify gates — with a single
Go binary that manages the full lifecycle.

Claw does NOT replace agents. It coordinates them: dispatches prompts,
watches execution, passes context between jobs, and verifies results.

## Install

```bash
cd src/claw
go build -o claw ./cmd/claw
# Or install globally:
go install ./cmd/claw
```

## Quick Start

```bash
# 1. Initialize from an AOS project
claw init ~/aos-platform

# 2. Register an agent
claw agent register builder-1 --type shell --command claude --cwd ~/aos-platform

# 3. Run the pipeline
claw run 1

# 4. Check status
claw status 1
```

## Commands

### Pipeline Management

| Command | Description |
|---------|-------------|
| `claw init <dir>` | Generate pipeline from AOS governance files |
| `claw pipeline create <name> --project <dir>` | Create empty pipeline |
| `claw run <pipeline-id>` | Execute pipeline to completion |
| `claw status <pipeline-id>` | Show job status table |

### Job Management

| Command | Description |
|---------|-------------|
| `claw job add <pipeline-id> <name> --prompt-file <path>` | Add job to pipeline |
| `claw output <job-id>` | Show captured job output |
| `claw gate <job-id>` | Show gate result |

### Agent Management

| Command | Description |
|---------|-------------|
| `claw agent register <name> --type <shell\|manual>` | Register agent |
| `claw agent list` | Show all agents |

### Observability

| Command | Description |
|---------|-------------|
| `claw summary <pipeline-id>` | Generate handoff summary |
| `claw context <pipeline-id>` | Show accumulated context |
| `claw log [--pipeline <id>] [--limit <n>]` | Show activity log |

## Agent Types

### Shell Agents

Invoke a configured command with the prompt piped via stdin:

```bash
claw agent register builder-1 --type shell --command claude \
  --args "--dangerously-skip-permissions" --cwd ~/project --timeout 300
```

### Manual Agents

Print the prompt to stdout, wait for human to paste output back:

```bash
claw agent register human-1 --type manual
```

## AOS Integration

`claw init` reads AOS governance files:

- `.aos/GATES.md` — gate scripts and phase assignments
- `docs/plan/*PHASE_PLAN*.md` — phase order and names
- `package.json` — verify npm scripts

It generates a pipeline with:
- One job per non-complete phase
- Dependencies: phase N depends on phase N-1
- Gate commands mapped from GATES.md to package.json scripts

## Context Engine

After each job completes, Claw extracts key information:
- Job status (passed/failed)
- Files changed
- Test results
- Gate result
- Error lines (if failed)

This context is automatically injected into subsequent job prompts,
giving downstream agents awareness of prior work.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CLAW_DB_PATH` | `~/.claw/claw.db` | Database file location |
| `CLAW_MAX_RETRIES` | `1` | Retry count for failed/timed-out jobs |

## Design Principles

- **One binary, one database.** Go + SQLite. No Docker, no message queue.
- **Agent-agnostic.** Works with any agent that accepts stdin prompts.
- **Convention over configuration.** Reads AOS governance files directly.
- **Observable.** Every dispatch, gate result, and context entry is logged.
- **Local-first.** All state on disk. No telemetry, no external calls.

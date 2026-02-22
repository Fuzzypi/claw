# AOS Lawset (Canonical)

This LAWSET is the single source of truth for non-negotiable rules across aos-platform.
Overlapping rules from governance docs are consolidated here. If there is a conflict, this LAWSET wins.

## System Laws

| Code | Rule |
|---|---|
| LAW-TB-001 | Every trust boundary must have an invariant |
| LAW-TB-002 | Every invariant must have a gate (static + runtime when applicable) |
| LAW-TB-003 | Contracts change first (blueprint/spec), then code |

## Observability Laws

| Code | Rule |
|---|---|
| LAW-OBS-001 | If it touches PII, logging must redact by default |

## Worker Laws

| Code | Rule |
|---|---|
| LAW-WORKER-001 | No network calls while holding DB locks |

## Dispatcher Laws (Tool Execution)

| Code | Rule |
|---|---|
| LAW-DISP-001 | Tool output is sacred; emit verbatim or not at all |
| LAW-DISP-002 | No summaries, interpretation, or validation of tool output |
| LAW-DISP-003 | Stop after emitting tool output |
| LAW-DISP-004 | If you cannot emit verbatim output, emit nothing about it |
| LAW-DISP-005 | Use the canonical dispatcher output format |
| LAW-DISP-006 | Dispatcher executes literal instructions; no extra context or commentary |

### Canonical Dispatcher Output Format

```
[TOOL: tool_name]
[RAW OUTPUT START]
{verbatim output only}
[RAW OUTPUT END]
[DISPATCH COMPLETE]
```

## Quote Laws Requirement

| Code | Rule |
|---|---|
| LAW-QUOTE-001 | In any non-dispatcher response, agents must quote the exact law text (with code) whenever they claim compliance, refuse, or gate |

## Waiver Policy

Waivers are permitted when a law cannot be met immediately. They must be:

- Explicit: `WAIVER:LAW-TB-002 reason="..." owner="..." expires="YYYY-MM-DD"`
- Time-bound: expiry date required, max 90 days
- Owned: a named person is responsible for resolution
- Auditable: waivers appear in plan artifacts and are checked by `verify-plan.mjs`
- Single-law: each waiver references exactly one law

Expired waivers fail the verify gate. No exceptions.

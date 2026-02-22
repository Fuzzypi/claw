# AOS Charter

## Purpose

This project is governed by AOS (Agent Operating System) — a governance-first
software engineering platform. AI agents and human developers follow defined
rules, phase plans, and verify gates to build software with structural integrity.

## Core Principles

1. **Governance before features.** Structure and rules are established before any product code ships.
2. **Phase discipline.** Work proceeds in locked phases. No phase begins until its predecessor's gates pass.
3. **Verify gates are law.** A phase is not complete until every verify gate passes. No exceptions, no overrides.
4. **Blueprint is binding.** The architecture blueprint is the single source of truth for system design. Code that contradicts the blueprint is wrong.
5. **Roles are bounded.** ARCHITECT designs, BUILDER implements, VERIFIER validates. No role oversteps.

## Governance Files

| File | Purpose |
|------|---------|
| `CHARTER.md` | This file. Defines purpose and principles. |
| `LAWS.md` | Pointer to the canonical law set. |
| `GATES.md` | Registry of verify gates and their pass criteria. |
| `PROMPTS/` | Role-specific system prompts for AI agents. |

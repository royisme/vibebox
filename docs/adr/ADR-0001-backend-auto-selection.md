# ADR-0001: Backend Auto Selection

- Date: 2026-02-06
- Status: Accepted

## Context
Vibebox must run agent workloads with the strongest practical isolation while preserving usability across environments.

## Decision
Implement `provider=auto` with this priority:
1. macOS backend on Darwin if runtime probe succeeds.
2. Docker backend as fallback.
3. Hard failure when no backend is available.

`init` keeps VM image semantics separate from Docker image semantics.

## Consequences
- Users on supported macOS get VM-first behavior.
- Cross-platform support remains available via Docker.
- Runtime probe and diagnostics are mandatory for clear operator feedback.

# Runbook: Mozi Integration Gap Analysis

## Scope
This document tracks what `vibebox` must provide to fit Mozi's sandbox runtime contract.

Mozi sandbox modes:
- `off`
- `apple-vm`
- `docker`

## Current vibebox API
- `ListImages(hostArch string) []Image`
- `ResolveDefaultImage(hostArch string) (Image, error)`
- `Initialize(ctx, InitializeRequest) (InitializeResult, error)`
- `Probe(ctx, provider) (ProbeResult, error)`
- `Start(ctx, StartRequest) (StartResult, error)`
- `Exec(ctx, ExecRequest) (ExecResult, error)`
- `StartSession(ctx, StartSessionRequest) (Session, error)`
- `ExecInSession(ctx, ExecInSessionRequest) (ExecResult, error)`
- `StopSession(ctx, StopSessionRequest) error`
- `GetSession(ctx, sessionID) (Session, error)`

Provider values:
- `off`
- `apple-vm`
- `docker`
- `auto` (selector strategy)

Legacy alias accepted:
- `macos` -> `apple-vm`

## Status Summary

### Closed (Phase 1)
1. Explicit `off` mode: done.
2. Non-interactive execution API: done (`Exec`).
3. Backend interface supports per-command execution: done (`Backend.Exec`).
4. `auto` remains selector-only strategy: done.

### Closed (Phase 2)
1. Session lifecycle APIs in SDK: done.
- `StartSession / ExecInSession / StopSession / GetSession` are available.

2. Stateful backend support:
- `off`: implemented.
- `docker`: implemented using long-lived container + `docker exec`.
- `apple-vm`: transitional session mode implemented via delegated backend.

### Remaining
1. macOS backend still delegates to `vibe`.
- Works as transitional adapter but remains an external dependency.

2. CLI bridge currently focuses on `probe/exec` JSON.
- Session lifecycle over CLI JSON can be added if Mozi requires process-boundary sessions.

## Required Contract (now available)

```go
type ExecRequest struct {
    ProjectRoot string
    ProviderOverride Provider
    Command string
    Cwd string
    Env map[string]string
    TimeoutSeconds int
}

type ExecResult struct {
    Stdout string
    Stderr string
    ExitCode int
    Selected Provider
    Diagnostics map[string]BackendDiagnostic
}

func (s *Service) Exec(ctx context.Context, req ExecRequest) (ExecResult, error)
```

## Acceptance Criteria Tracking
- [x] One API call returns deterministic `stdout/stderr/exitCode`.
- [x] `off/apple-vm/docker` modes are explicit.
- [x] `auto` available as selection strategy.
- [x] Errors include actionable diagnostics (`FixHints`).
- [x] Session lifecycle API available in SDK.
- [ ] Native `vz`-based `apple-vm` backend replaces delegated `vibe`.

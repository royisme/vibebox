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

### Remaining
1. Session lifecycle APIs not implemented yet.
- Missing `StartSession / ExecInSession / StopSession`.

2. macOS backend still delegates to `vibe`.
- Works as transitional adapter but remains an external dependency.

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
- [ ] Session lifecycle API available.
- [ ] Native `vz`-based `apple-vm` backend replaces delegated `vibe`.

# Runbook: Agent Integration (Go API)

`vibebox` exposes a public application layer in `pkg/vibebox`.

## Why use this API
- Avoid shelling out to CLI for core workflows.
- Integrate sandbox lifecycle directly in an agent orchestrator.
- Subscribe to progress/events during initialization, startup, and command execution.

## Public entrypoint
- Package: `vibebox/pkg/vibebox`
- Type: `vibebox.Service`

## Current API
- `ListImages(hostArch string) []Image`
- `ResolveDefaultImage(hostArch string) (Image, error)`
- `Initialize(ctx, InitializeRequest) (InitializeResult, error)`
- `Probe(ctx, provider) (ProbeResult, error)`
- `Start(ctx, StartRequest) (StartResult, error)`
- `Exec(ctx, ExecRequest) (ExecResult, error)`

## Provider model
- `off` (host execution)
- `apple-vm`
- `docker`
- `auto` (selection strategy)

Legacy alias accepted as input:
- `macos` -> normalized to `apple-vm`

## Integration model

### 1) First-run project bootstrap (optional for `off`)
- Call `Initialize` once per project if you plan to use VM image-backed workflows.
- Persist generated `.vibebox/config.yaml` under project root.
- Surface init progress through `OnEvent`.

### 2) Non-interactive command execution (recommended for Mozi)
- Call `Exec` for one command and read deterministic `stdout/stderr/exitCode`.
- Prefer `ProviderOverride: off|apple-vm|docker` based on policy.

### 3) Interactive runtime startup (optional)
- Call `Start` for interactive runtime sessions.

### 4) Diagnostics and remediation
- Always inspect `Diagnostics` from `Probe` / `ExecResult` / `StartResult`.
- Surface `FixHints` directly to users for self-service remediation.

## Example (`Exec`)
```go
package main

import (
    "context"
    "fmt"
    sdk "vibebox/pkg/vibebox"
)

func main() {
    ctx := context.Background()
    svc := sdk.NewService()

    result, err := svc.Exec(ctx, sdk.ExecRequest{
        ProjectRoot:      "/path/to/project",
        ProviderOverride: sdk.ProviderOff,
        Command:          "echo hello-from-vibebox",
        TimeoutSeconds:   20,
    })
    if err != nil {
        panic(err)
    }

    fmt.Println("provider:", result.Selected)
    fmt.Println("exit:", result.ExitCode)
    fmt.Println("stdout:", result.Stdout)
    fmt.Println("stderr:", result.Stderr)
}
```

## Current limitation
- `apple-vm` backend still delegates execution to host `vibe` binary.
- Target direction is native `vz` backend to remove this hard dependency.

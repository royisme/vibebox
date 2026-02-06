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
- `StartSession(ctx, StartSessionRequest) (Session, error)`
- `ExecInSession(ctx, ExecInSessionRequest) (ExecResult, error)`
- `StopSession(ctx, StopSessionRequest) error`
- `GetSession(ctx, sessionID) (Session, error)`

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
- Configure workspace/data mounts via `InitializeRequest.Mounts` (supports multiple directories).

### 2) Non-interactive command execution (recommended for Mozi)
- Call `Exec` for one command and read deterministic `stdout/stderr/exitCode`.
- Prefer `ProviderOverride: off|apple-vm|docker` based on policy.

### 3) Reusable session execution (advanced)
- Call `StartSession` once, then `ExecInSession` repeatedly.
- Call `StopSession` when the workload is complete.

### 4) Interactive runtime startup (optional)
- Call `Start` for interactive runtime sessions.

### 5) Diagnostics and remediation
- Always inspect `Diagnostics` from `Probe` / `ExecResult` / `StartResult`.
- Surface `FixHints` directly to users for self-service remediation.
- For relative execution paths (`Cwd: "."`, `./subdir`), ensure project root is mounted into guest.

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

    _, err := svc.Initialize(ctx, sdk.InitializeRequest{
        ProjectRoot: "/path/to/project",
        Provider: sdk.ProviderAppleVM,
        Mounts: []sdk.Mount{
            {Host: ".", Guest: "/workspace", Mode: "rw"},
            {Host: "../agent-cache", Guest: "/cache", Mode: "rw"},
        },
    })
    if err != nil {
        panic(err)
    }

    result, err := svc.Exec(ctx, sdk.ExecRequest{
        ProjectRoot:      "/path/to/project",
        ProviderOverride: sdk.ProviderAppleVM,
        Command:          "echo hello-from-vibebox",
        Cwd:              ".",
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
- `apple-vm` backend is native `vz`, but session API remains compatibility-first: repeated `ExecInSession` calls do not yet reuse one long-lived VM instance.

# Usage Guide

This guide explains how to use Vibebox from CLI and from the Go SDK.

## 1. Provider Model

Vibebox supports four provider values:

- `off`: host execution path (no VM/container)
- `apple-vm`: VM backend on macOS
- `docker`: container backend
- `auto`: selection strategy (prefers `apple-vm` on Darwin, otherwise `docker`)

Legacy value `macos` is accepted and normalized to `apple-vm`.

## 2. Initialize a Project

Initialization prepares image assets and writes project config (`.vibebox/config.yaml`).

### CLI

```bash
vibebox init
```

Useful flags:

```bash
vibebox init --non-interactive --image-id debian-13-nocloud-arm64 --provider auto
```

### SDK

```go
svc := vibebox.NewService()
_, err := svc.Initialize(ctx, vibebox.InitializeRequest{
    ProjectRoot: "/path/to/project",
    Provider:    vibebox.ProviderAuto,
    OnEvent: func(e vibebox.Event) {
        // progress hook
    },
})
```

## 3. Execute One Command (Recommended for Agent Runtimes)

For Mozi-like runtimes, use `Exec` as the primary path.

```go
res, err := svc.Exec(ctx, vibebox.ExecRequest{
    ProjectRoot:      "/path/to/project",
    ProviderOverride: vibebox.ProviderDocker,
    Command:          "go test ./...",
    Cwd:              ".",
    Env:              map[string]string{"CI": "1"},
    TimeoutSeconds:   120,
})

// deterministic output
fmt.Println(res.ExitCode)
fmt.Println(res.Stdout)
fmt.Println(res.Stderr)
```

`ExecResult` includes:
- `Stdout`
- `Stderr`
- `ExitCode`
- `Selected` provider
- backend `Diagnostics`


## 4. Reusable Sessions (Phase 2)

Use session APIs when you need repeated commands with shared sandbox context.

```go
session, err := svc.StartSession(ctx, vibebox.StartSessionRequest{
    ProjectRoot:      "/path/to/project",
    ProviderOverride: vibebox.ProviderDocker,
    Cwd:              ".",
})
if err != nil {
    panic(err)
}

res, err := svc.ExecInSession(ctx, vibebox.ExecInSessionRequest{
    SessionID: session.ID,
    Command:   "echo from-session",
})
if err != nil {
    panic(err)
}
fmt.Println(res.ExitCode, res.Stdout)

_ = svc.StopSession(ctx, vibebox.StopSessionRequest{SessionID: session.ID})
```

## 5. Start Interactive Session

Use interactive mode when a shell is required.

### CLI

```bash
vibebox up --provider auto
```

### SDK

```go
_, err := svc.Start(ctx, vibebox.StartRequest{
    ProjectRoot:      "/path/to/project",
    ProviderOverride: vibebox.ProviderAuto,
})
```

## 6. Diagnose Backend Availability

Use `Probe` before execution to surface remediation hints.

```go
probe, err := svc.Probe(ctx, vibebox.ProviderAuto)
if err != nil {
    // inspect probe.Diagnostics[name].FixHints
}
```

## 7. Quality Workflow

```bash
make fmt
make lint
make test
make build
make check
```

## 8. Notes

- `apple-vm` currently uses a delegated adapter to `vibe` as an interim step.
- Long-term target is a native `vz` backend.

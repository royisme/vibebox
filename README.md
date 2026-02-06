# Vibebox

Vibebox is a Go-based sandbox runtime for LLM agents, with a Mozi-oriented integration contract.

It provides:
- Provider modes: `off`, `apple-vm`, `docker`, and `auto` (selection strategy)
- Deterministic command execution API (`Exec`) with `stdout/stderr/exitCode`
- Project initialization flow with official VM image catalog, download, integrity check, and local cache
- Interactive runtime entrypoint (`Start`) for shell-style sandbox sessions
- Diagnostics with actionable remediation hints (`FixHints`)

## Why Vibebox

Vibebox is built to be embedded by agent runtimes instead of shelling out to ad-hoc scripts.
The primary goal is compatibility with Mozi's runtime needs.

## Project Layout

- `pkg/vibebox`: public SDK for embedding
- `cmd/vibebox`: CLI frontend
- `internal/backend`: backend implementations (`off`, `apple-vm`, `docker`) and selection logic
- `internal/image`: image catalog, download, digest verification, extraction
- `docs/runbooks`: integration and operational guides

## Prerequisites

- Go `1.25+`
- For `docker` mode: Docker daemon available
- For `apple-vm` mode: macOS host with VM prerequisites

## Quick Start (CLI)

```bash
# initialize project (interactive image selection by default)
vibebox init

# start interactive sandbox
vibebox up --provider auto

# list official images
vibebox images list
```

## Quick Start (SDK)

```go
package main

import (
    "context"
    "fmt"

    sdk "vibebox/pkg/vibebox"
)

func main() {
    svc := sdk.NewService()

    result, err := svc.Exec(context.Background(), sdk.ExecRequest{
        ProjectRoot:      "/path/to/project",
        ProviderOverride: sdk.ProviderOff,
        Command:          "echo hello",
        TimeoutSeconds:   20,
    })
    if err != nil {
        panic(err)
    }

    fmt.Println(result.ExitCode)
    fmt.Println(result.Stdout)
    fmt.Println(result.Stderr)
}
```

## Development

```bash
make fmt
make lint
make test
make build
make check
```

Install `golangci-lint` if needed:

```bash
make install-lint
```

## Documentation

See `docs/README.md` for the document index.

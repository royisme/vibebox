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
- For `apple-vm` mode: macOS host, Apple Virtualization support, and `com.apple.security.virtualization` entitlement on vibebox binary

## Install From Release

Install latest release to local bin directory:

```bash
curl -fsSL https://raw.githubusercontent.com/royisme/vibebox/main/scripts/install-vibebox.sh \
  | bash -s -- --repo royisme/vibebox --version latest --bin-dir ./tools/bin
```

For pinned version:

```bash
curl -fsSL https://raw.githubusercontent.com/royisme/vibebox/main/scripts/install-vibebox.sh \
  | bash -s -- --repo royisme/vibebox --version v0.1.0 --bin-dir ./tools/bin
```

## Quick Start (CLI)

```bash
# initialize project (interactive image selection by default)
vibebox init

# machine-friendly backend probe
vibebox probe --json --provider auto

# non-interactive command execution (Mozi bridge path)
vibebox exec --json --provider off --command "echo hello"

# start interactive sandbox
vibebox up --provider auto

# list official images
vibebox images list

# initialize with one-time provisioning script (executed on first VM instance creation)
vibebox init --provision-script ./scripts/provision-minimal.sh

# initialize with additional mounts
vibebox init \
  --mount ../agent-cache:/cache:rw \
  --mount ../readonly-assets:/assets:ro

# initialize with only explicit mounts (disable default project-root mount)
vibebox init \
  --no-default-mounts \
  --mount /abs/path/workspace:/workspace:rw
```

`--mount` format: `host:guest[:ro|rw]` (repeatable, default mode is `rw`).

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
make sign
make probe-apple-vm
make check
```

Install `golangci-lint` if needed:

```bash
make install-lint
```

## Documentation

See `docs/README.md` for the document index.
For Mozi/Bun integration, start with `docs/runbooks/mozi-consumer-guide.md`.

## CI and Release

- CI workflow: `.github/workflows/ci.yml` (lint/test/build on Ubuntu + macOS)
- Release workflow: `.github/workflows/release.yml` (build tarballs + checksums + GitHub release assets on tag push `v*`)

Create a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Local release artifact test:

```bash
./scripts/build-release.sh v0.1.0-test dist
```

## macOS apple-vm readiness

`apple-vm` requires the running vibebox binary to be signed with virtualization entitlement.

Use:

```bash
make sign
make probe-apple-vm
```

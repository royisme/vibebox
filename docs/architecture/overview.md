# Architecture Overview

## Goals
- Provide a sandbox runtime for LLM agents.
- Prefer macOS VM backend when available.
- Fall back to Docker when VM backend is unavailable.

## Modules
- `cmd/vibebox`: CLI entrypoint.
- `pkg/vibebox`: public application-layer API for embedding vibebox in other Go projects.
- `internal/app`: command orchestration (`init`, `up`, `images`).
- `internal/config`: project and user config persistence.
- `internal/image`: official image catalog, download, digest verification, extraction.
- `internal/backend`: backend interface and selector.
- `internal/backend/macos`: macOS backend implementation (native `vz` / Apple Virtualization.framework).
- `internal/backend/docker`: Docker backend implementation.
- `internal/progress`: progress event model.
- `internal/ui/tui`: Bubble Tea based image selector and progress renderer.

## Runtime Flow
1. `vibebox init`
- Select an official VM image (TUI or non-interactive).
- Download with resume.
- Verify SHA256.
- Extract `disk.raw` and cache as `base.raw`.
- Persist VM optional provision script and mount policy (`mounts`, multi-directory supported).
- Save project config at `.vibebox/config.yaml`.
- Save lock state at `~/.config/vibebox/images.lock.yaml`.

2. `vibebox up`
- Load project config and lock state.
- Select backend (`auto|macos|docker`).
- `auto`: prefer `macos` on Darwin, otherwise use Docker.
- Apply all configured mounts from `.vibebox/config.yaml` (`mounts` supports multiple host directories).
- Prepare backend runtime and start interactive shell.

## Security Notes
- VM image artifacts are verified with SHA256 before use.
- Docker backend is a compatibility fallback and does not provide VM-level isolation.
- macOS backend requires virtualization entitlement (`com.apple.security.virtualization`) and a supported macOS host.

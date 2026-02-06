# Runbook: Mozi Consumer Guide

This guide shows how a Bun/TypeScript project (Mozi) installs and calls `vibebox`.

## 1) Install strategy

Use the release installer script to download platform-specific binary artifacts.

Example (local install under project):

```bash
curl -fsSL https://raw.githubusercontent.com/royisme/vibebox/main/scripts/install-vibebox.sh \
  | bash -s -- --repo royisme/vibebox --version latest --bin-dir ./tools/bin
```

Notes:

- On macOS, installer signs the binary ad-hoc with `com.apple.security.virtualization` by default.
- For deterministic deployments, pin a concrete release tag instead of `latest`.

## 2) Bun `postinstall` integration

In Mozi `package.json`:

```json
{
  "scripts": {
    "postinstall": "bash -c 'curl -fsSL https://raw.githubusercontent.com/royisme/vibebox/main/scripts/install-vibebox.sh | bash -s -- --repo royisme/vibebox --version latest --bin-dir ./tools/bin'"
  }
}
```

If you want to disable macOS signing in installer:

```bash
SIGN_MACOS=0 <install-command>
```

## 3) Runtime contract (process boundary)

Probe:

```bash
./tools/bin/vibebox probe --json --provider auto --project-root "$WORKSPACE"
```

Exec:

```bash
./tools/bin/vibebox exec --json \
  --provider auto \
  --project-root "$WORKSPACE" \
  --command "echo hello"
```

JSON fields to parse:

- `ok`
- `selected`
- `exitCode`
- `stdout`
- `stderr`
- `diagnostics.<provider>.fixHints`

## 4) One-time project initialization

Before runtime execution, initialize workspace sandbox config:

```bash
./tools/bin/vibebox init \
  --provider apple-vm \
  --non-interactive \
  --mount .:/workspace:rw \
  --mount ../agent-cache:/cache:rw \
  --provision-script ./scripts/provision-minimal.sh
```

This creates `.vibebox/config.yaml` and persists mount policy.

## 5) Cwd behavior

- If `--cwd` is relative (e.g. `.` or `subdir`), project root must be mounted.
- If no project-root mount exists, use absolute guest `--cwd` path.

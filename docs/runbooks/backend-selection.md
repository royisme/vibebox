# Runbook: Backend Selection

## Provider options
- `auto` (default)
- `macos`
- `docker`

## Auto selection
1. On Darwin: choose `macos` if probe succeeds.
2. Otherwise choose `docker` if probe succeeds.
3. Fail when both probes fail.

## Explicit provider behavior
- `--provider macos`: hard fail if macOS probe fails.
- `--provider docker`: hard fail if Docker probe fails.

## Diagnostics
If selection fails, the command returns reason and fix hints.

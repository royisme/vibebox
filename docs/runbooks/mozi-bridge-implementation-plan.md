# Runbook: Mozi Bridge Implementation Plan

## Status

Implemented (Phase 1 + Phase 2 completed; apple-vm native `vz` backend available)

## Problem

Mozi runtime is Bun/TypeScript, while vibebox integration APIs are Go SDK APIs.

Because of this language/runtime boundary, Mozi cannot import `pkg/vibebox` directly.
A machine-friendly bridge must exist between Mozi and vibebox.

## Target

Provide a deterministic bridge so Mozi can call vibebox at runtime for sandbox operations without embedding Go.

## Scope

Phase 1 scope (required for current Mozi integration):

1. Add CLI JSON contracts for `probe` and `exec`.
2. Keep existing SDK APIs unchanged.
3. Keep existing `init/up/images` user-facing commands unchanged.
4. Ensure error diagnostics are preserved in JSON responses.

Out of scope in this runbook:

1. CLI JSON bridge for full session lifecycle (`session start/exec/stop`) across process boundaries.

## Contract Design

## 1) `vibebox probe --json`

Purpose:
- Return provider selection and backend diagnostics in a single JSON payload.

Input flags:
- `--provider off|apple-vm|docker|auto` (optional, default `auto`)
- `--project-root <path>` (optional)

Output JSON shape:

```json
{
  "ok": true,
  "selected": "apple-vm",
  "wasFallback": false,
  "fallbackFrom": "",
  "diagnostics": {
    "off": { "available": true, "reason": "", "fixHints": [] },
    "apple-vm": { "available": true, "reason": "", "fixHints": [] },
    "docker": { "available": false, "reason": "docker daemon not reachable", "fixHints": ["start docker daemon"] }
  }
}
```

Error JSON shape (still valid JSON on stdout):

```json
{
  "ok": false,
  "error": "requested provider apple-vm is unavailable",
  "selected": "",
  "diagnostics": {
    "apple-vm": { "available": false, "reason": "vibebox binary is missing virtualization entitlement", "fixHints": ["sign binary with com.apple.security.virtualization entitlement"] }
  }
}
```

Exit code:
- `0` when `ok=true`
- non-zero when `ok=false`

## 2) `vibebox exec --json`

Purpose:
- Execute one command non-interactively and return deterministic output.

Input flags:
- `--provider off|apple-vm|docker|auto` (optional)
- `--project-root <path>` (optional)
- `--command <string>` (required)
- `--cwd <path>` (optional)
- `--timeout-seconds <int>` (optional)
- `--env KEY=VALUE` (repeatable, optional)

Output JSON shape:

```json
{
  "ok": true,
  "selected": "apple-vm",
  "exitCode": 0,
  "stdout": "hello\n",
  "stderr": "",
  "diagnostics": {
    "off": { "available": true, "reason": "", "fixHints": [] },
    "apple-vm": { "available": true, "reason": "", "fixHints": [] },
    "docker": { "available": true, "reason": "", "fixHints": [] }
  }
}
```

Error JSON shape:

```json
{
  "ok": false,
  "error": "project is not initialized. run `vibebox init`",
  "selected": "",
  "exitCode": 1,
  "stdout": "",
  "stderr": "",
  "diagnostics": {}
}
```

Exit code:
- process exits with command result code only when execution succeeds
- non-zero with `ok=false` for bridge/selection/config failures

## Implementation Steps

1. Add new CLI subcommands in `cmd/vibebox/main.go`:
- `probe`
- `exec`

2. Keep app/service layering:
- Reuse `pkg/vibebox.Service` internally for command execution.
- Do not duplicate backend selection logic in CLI layer.

3. Add JSON writer helpers:
- stable field order
- no mixed human logs on stdout in `--json` mode
- diagnostics always included when available

4. Preserve human mode:
- if `--json` is not set, keep existing text output style.

5. Add tests:
- unit tests for CLI flag parsing
- golden tests for JSON payload shapes (success/failure)
- one integration test for `exec --json --provider off` with deterministic output

## Backward Compatibility

1. Existing commands remain unchanged:
- `init`, `up`, `images list`, `images upgrade`

2. Existing SDK remains unchanged:
- `Probe`, `Exec`, `Start`, `Initialize`

3. New JSON commands are additive and safe.

## Acceptance Criteria

1. Mozi can call vibebox via process boundary and parse JSON deterministically.
2. JSON payloads are stable and include diagnostics/fix hints.
3. No human logs are mixed into stdout in `--json` mode.
4. `exec --json` returns exact `stdout/stderr/exitCode` values from execution path.
5. Existing vibebox command workflows are unaffected.

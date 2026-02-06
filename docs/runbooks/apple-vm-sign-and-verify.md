# Runbook: Apple VM Sign and Verify

This runbook defines the repeatable workflow to make `apple-vm` backend available on macOS.

## Why this is needed

`apple-vm` uses Apple Virtualization.framework through `vz`.
The running vibebox binary must include this entitlement:

- `com.apple.security.virtualization`

Without it, `probe --provider apple-vm` fails even if your host supports virtualization.

## 1) Build and sign vibebox

From repository root:

```bash
make sign
```

Equivalent manual command:

```bash
go build -o ./bin/vibebox ./cmd/vibebox
./scripts/sign-vibebox.sh ./bin/vibebox
```

By default, `scripts/sign-vibebox.sh` uses:

- binary: `./bin/vibebox`
- entitlements: `./scripts/vibebox.entitlements.plist`
- identity: ad-hoc (`-`)

Optional environment variables:

- `CODESIGN_IDENTITY` (default `-`)
- `ENTITLEMENTS_PATH` (default `./scripts/vibebox.entitlements.plist`)

## 2) Verify backend readiness

Quick verify command:

```bash
make probe-apple-vm
```

Expected result:

- `diagnostics.apple-vm.available == true`

## 3) Functional verification (`exec`)

Initialize a project first:

```bash
./bin/vibebox init \
  --non-interactive \
  --image-id debian-13-nocloud-arm64 \
  --provider apple-vm \
  --provision-script ./scripts/provision-minimal.sh \
  --mount ../agent-cache:/cache:rw
```

Then execute one deterministic command:

```bash
./bin/vibebox exec --json --provider apple-vm --command "echo hello-from-apple-vm"
```

Expected result:

- `ok: true`
- `exitCode: 0`
- `stdout` contains `hello-from-apple-vm`

## 4) Minimal provisioning script for VM image workflows

Reference script:

- `scripts/provision-minimal.sh`

Purpose:

- install minimal baseline tools for agent runtime use (`curl`, `git`, `ripgrep`, `jq`, etc.)
- set `IS_SANDBOX=1`
- optional poweroff at end

Control shutdown behavior:

```bash
VIBEBOX_PROVISION_POWEROFF=0 ./scripts/provision-minimal.sh
```

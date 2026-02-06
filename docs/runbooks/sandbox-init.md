# Runbook: Sandbox Initialization

## Command
```bash
vibebox init
```

## Non-interactive
```bash
vibebox init --non-interactive --image-id debian-13-nocloud-arm64
```

## Expected outcome
- `.vibebox/config.yaml` exists in project root.
- image artifact and extracted raw are cached under `~/.cache/vibebox/images/...`.
- lock file updated at `~/.config/vibebox/images.lock.yaml`.

## Failure modes
- Network errors: rerun init, download resumes from partial artifact.
- SHA mismatch: corrupted file is removed; rerun init.
- No image for host architecture: choose compatible host or add catalog entry.

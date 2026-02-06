# Vibebox Docs

- `architecture/overview.md`: runtime architecture and module boundaries.
- `runbooks/sandbox-init.md`: initialization flow and troubleshooting.
- `runbooks/usage-guide.md`: end-to-end CLI/SDK usage guide.
- `runbooks/backend-selection.md`: provider selection and fallback logic.
- `runbooks/agent-integration.md`: how an external Go agent project should embed vibebox.
- `runbooks/mozi-integration-gap.md`: concrete gap analysis and required API changes for Mozi integration.
- `runbooks/mozi-bridge-implementation-plan.md`: actionable bridge plan for Mozi (CLI JSON `probe/exec`).
- `adr/ADR-0001-backend-auto-selection.md`: architectural decision on backend auto-selection.

## Development Quality Commands

From repository root:

```bash
make fmt
make lint
make test
make build
make check
```

If `golangci-lint` is missing:

```bash
make install-lint
```

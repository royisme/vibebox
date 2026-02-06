package backend

import (
	"context"
	"testing"

	"vibebox/internal/config"
)

type fakeBackend struct {
	name  string
	probe ProbeResult
}

func (f fakeBackend) Name() string                               { return f.name }
func (f fakeBackend) Probe(context.Context) ProbeResult          { return f.probe }
func (f fakeBackend) Prepare(context.Context, RuntimeSpec) error { return nil }
func (f fakeBackend) Start(context.Context, RuntimeSpec) error   { return nil }
func (f fakeBackend) Exec(context.Context, RuntimeSpec, ExecRequest) (ExecResult, error) {
	return ExecResult{}, nil
}

func TestSelectExplicitDocker(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	off := fakeBackend{name: "off", probe: ProbeResult{Available: true}}
	mac := fakeBackend{name: "apple-vm", probe: ProbeResult{Available: false, Reason: "nope"}}
	docker := fakeBackend{name: "docker", probe: ProbeResult{Available: true}}

	sel, err := Select(ctx, config.ProviderDocker, off, mac, docker)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Provider != config.ProviderDocker {
		t.Fatalf("provider mismatch: %s", sel.Provider)
	}
}

func TestSelectExplicitOff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	off := fakeBackend{name: "off", probe: ProbeResult{Available: true}}
	apple := fakeBackend{name: "apple-vm", probe: ProbeResult{Available: false, Reason: "nope"}}
	docker := fakeBackend{name: "docker", probe: ProbeResult{Available: true}}

	sel, err := Select(ctx, config.ProviderOff, off, apple, docker)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Provider != config.ProviderOff {
		t.Fatalf("provider mismatch: %s", sel.Provider)
	}
}

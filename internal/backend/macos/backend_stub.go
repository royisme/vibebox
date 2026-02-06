//go:build !darwin

package macos

import (
	"context"
	"fmt"

	"vibebox/internal/backend"
)

func (b *Backend) Probe(ctx context.Context) backend.ProbeResult {
	_ = ctx
	return backend.ProbeResult{
		Available: false,
		Reason:    "apple-vm backend is only available on darwin",
		FixHints:  []string{"use provider=docker or provider=off on non-darwin hosts"},
	}
}

func (b *Backend) Start(ctx context.Context, spec backend.RuntimeSpec) error {
	_ = ctx
	_ = spec
	return fmt.Errorf("apple-vm backend is only available on darwin")
}

func (b *Backend) Exec(ctx context.Context, spec backend.RuntimeSpec, req backend.ExecRequest) (backend.ExecResult, error) {
	_ = ctx
	_ = spec
	_ = req
	return backend.ExecResult{}, fmt.Errorf("apple-vm backend is only available on darwin")
}

func (b *Backend) provisionInstance(ctx context.Context, spec backend.RuntimeSpec) error {
	_ = ctx
	_ = spec
	return nil
}

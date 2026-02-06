package backend

import (
	"context"
	"io"
	"time"

	"vibebox/internal/config"
)

// IOStreams controls runtime stdio binding.
type IOStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// RuntimeSpec contains runtime inputs for backend start.
type RuntimeSpec struct {
	ProjectRoot string
	ProjectName string
	Config      config.Config
	BaseRawPath string
	InstanceRaw string
	IO          IOStreams
}

// ExecRequest configures one non-interactive command execution.
type ExecRequest struct {
	Command string
	Cwd     string
	Env     map[string]string
	Timeout time.Duration
}

// ExecResult is the deterministic output of one command execution.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// SessionHandle is backend-specific opaque session data.
type SessionHandle any

// SessionStartRequest configures creation of a reusable session.
type SessionStartRequest struct {
	SessionID string
	Cwd       string
	Env       map[string]string
}

// ProbeResult reports backend availability.
type ProbeResult struct {
	Available bool
	Reason    string
	FixHints  []string
}

// Backend is one sandbox runtime implementation.
type Backend interface {
	Name() string
	Probe(ctx context.Context) ProbeResult
	Prepare(ctx context.Context, spec RuntimeSpec) error
	Start(ctx context.Context, spec RuntimeSpec) error
	Exec(ctx context.Context, spec RuntimeSpec, req ExecRequest) (ExecResult, error)
}

// SessionBackend is an optional extension for stateful session lifecycle support.
type SessionBackend interface {
	StartSession(ctx context.Context, spec RuntimeSpec, req SessionStartRequest) (SessionHandle, error)
	ExecInSession(ctx context.Context, spec RuntimeSpec, handle SessionHandle, req ExecRequest) (ExecResult, error)
	StopSession(ctx context.Context, spec RuntimeSpec, handle SessionHandle) error
}

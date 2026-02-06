package vibebox

import (
	"io"
	"time"
)

// Provider indicates which backend should be used.
type Provider string

const (
	ProviderOff     Provider = "off"
	ProviderAuto    Provider = "auto" // selector strategy, not a concrete runtime mode.
	ProviderAppleVM Provider = "apple-vm"
	ProviderMacOS   Provider = "macos" // legacy alias accepted as input.
	ProviderDocker  Provider = "docker"
)

// StreamSet allows embedding apps to wire custom stdio.
type StreamSet struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Image describes one official white-listed VM image.
type Image struct {
	ID          string
	DisplayName string
	Version     string
	Arch        string
	URL         string
	SizeBytes   int64
}

// Event is emitted during long-running operations.
type Event struct {
	Kind       string
	Phase      string
	Message    string
	Percent    float64
	BytesDone  int64
	BytesTotal int64
	SpeedBps   float64
	ETA        time.Duration
	Err        error
	Done       bool
}

// EventHandler receives operation events.
type EventHandler func(Event)

// InitializeRequest configures project initialization.
type InitializeRequest struct {
	ProjectRoot string
	ImageID     string
	Provider    Provider
	CPUs        int
	RAMMB       int
	DiskGB      int
	OnEvent     EventHandler
}

// InitializeResult describes generated artifacts after init.
type InitializeResult struct {
	ProjectRoot string
	ConfigPath  string
	Image       Image
	BaseRawPath string
}

// StartRequest configures sandbox startup.
type StartRequest struct {
	ProjectRoot      string
	ProviderOverride Provider
	IO               StreamSet
	OnEvent          EventHandler
}

// ExecRequest configures non-interactive command execution.
type ExecRequest struct {
	ProjectRoot      string
	ProviderOverride Provider
	Command          string
	Cwd              string
	Env              map[string]string
	TimeoutSeconds   int
	OnEvent          EventHandler
}

// BackendDiagnostic describes availability status of one backend.
type BackendDiagnostic struct {
	Available bool     `json:"available"`
	Reason    string   `json:"reason"`
	FixHints  []string `json:"fixHints"`
}

// ProbeResult reports selection outcome and diagnostics.
type ProbeResult struct {
	Selected     Provider
	WasFallback  bool
	FallbackFrom string
	Diagnostics  map[string]BackendDiagnostic
}

// StartResult reports startup decision details.
type StartResult struct {
	Selected     Provider
	WasFallback  bool
	FallbackFrom string
	Diagnostics  map[string]BackendDiagnostic
}

// ExecResult is the deterministic output for one command execution.
type ExecResult struct {
	Stdout      string
	Stderr      string
	ExitCode    int
	Selected    Provider
	Diagnostics map[string]BackendDiagnostic
}

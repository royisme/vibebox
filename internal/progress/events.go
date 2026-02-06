package progress

import "time"

// Phase describes a high-level initialization stage.
type Phase string

const (
	PhaseResolving   Phase = "resolving"
	PhaseDownloading Phase = "downloading"
	PhaseVerifying   Phase = "verifying"
	PhasePreparing   Phase = "preparing"
	PhaseCompleted   Phase = "completed"
	PhaseFailed      Phase = "failed"
)

// Event describes progress updates emitted by long-running operations.
type Event struct {
	Phase      Phase
	Message    string
	Percent    float64
	BytesDone  int64
	BytesTotal int64
	SpeedBps   float64
	ETA        time.Duration
	Err        error
	Done       bool
}

// Sink receives events.
type Sink interface {
	Emit(Event)
}

// FuncSink adapts a function to a Sink.
type FuncSink func(Event)

// Emit implements Sink.
func (f FuncSink) Emit(e Event) {
	f(e)
}

// NopSink drops events.
type NopSink struct{}

// Emit implements Sink.
func (NopSink) Emit(Event) {}

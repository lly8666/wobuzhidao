package runtimeentry

// ClientPipelineDiagnostic is opt-in wall-clock timing after a lane consumer
// reads a segment. Blocking reads and their queue residence are excluded: mux
// queue_age already measures residence. Process includes state-lock waiting
// and the complete runtime/association handler, including synchronous IO.
// All fields are fixed counters shared across the client's bounded lanes;
// observations never introduce a work queue or change packet handling.
type ClientPipelineDiagnostic struct {
	Enabled       bool               `json:"enabled"`
	Process       DurationDiagnostic `json:"process"`
	StateLockWait DurationDiagnostic `json:"state_lock_wait"`
	Handler       DurationDiagnostic `json:"handler"`
	Tick          DurationDiagnostic `json:"tick"`
	RuntimeTick   DurationDiagnostic `json:"runtime_tick"`
	Retire        DurationDiagnostic `json:"retire"`
	Schedule      DurationDiagnostic `json:"schedule"`
}

type clientPipelineTiming struct {
	process       durationAccumulator
	stateLockWait durationAccumulator
	handler       durationAccumulator
	tick          durationAccumulator
	runtimeTick   durationAccumulator
	retire        durationAccumulator
	schedule      durationAccumulator
}

func (p *clientPipelineTiming) snapshot() ClientPipelineDiagnostic {
	return ClientPipelineDiagnostic{
		Enabled:       true,
		Process:       p.process.snapshot(),
		StateLockWait: p.stateLockWait.snapshot(),
		Handler:       p.handler.snapshot(),
		Tick:          p.tick.snapshot(),
		RuntimeTick:   p.runtimeTick.snapshot(),
		Retire:        p.retire.snapshot(),
		Schedule:      p.schedule.snapshot(),
	}
}

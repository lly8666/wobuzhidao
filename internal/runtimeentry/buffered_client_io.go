package runtimeentry

import (
	"sync"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// BufferedClientIO gives a single native capture handle the same bounded
// reader/handler separation as the existing Linux client SegmentMux. Close
// owns both the route and underlying handle; the returned mux is observation
// only. Each incarnation has its own queue, so replacement cannot mix flows.
func BufferedClientIO(base SegmentIO, flow faketcp.ClientFlow, diagnostics bool) (SegmentIO, *SegmentMux, error) {
	if err := flow.Validate(); err != nil {
		return SegmentIO{}, nil, err
	}
	mux, err := NewSegmentMux(base)
	if err != nil {
		return SegmentIO{}, nil, err
	}
	mux.SetTimingDiagnostics(diagnostics)
	route, err := mux.Open(flow)
	if err != nil {
		_ = mux.Close()
		return SegmentIO{}, nil, err
	}
	done := make(chan struct{})
	var once sync.Once
	var closeErr error
	closeIO := func() error {
		once.Do(func() { close(done); closeErr = mux.Close() })
		return closeErr
	}
	var failureMu sync.Mutex
	var failure error
	go func() {
		select {
		case err := <-mux.Errors():
			failureMu.Lock()
			failure = err
			failureMu.Unlock()
			_ = closeIO()
		case <-done:
		}
	}()
	return SegmentIO{
		Read: func() (faketcp.Segment, error) {
			seg, err := route.Read()
			if err != nil {
				failureMu.Lock()
				terminal := failure
				failureMu.Unlock()
				if terminal != nil {
					return faketcp.Segment{}, terminal
				}
			}
			return seg, err
		},
		Emit: route.Emit, EmitBatch: route.EmitBatch, Close: closeIO,
	}, mux, nil
}

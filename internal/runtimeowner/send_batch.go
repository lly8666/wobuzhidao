package runtimeowner

import (
	"io"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

const freshBatchSize = 8

// No queue, timer or future-packet wait: each call consumes only its existing
// records. Control is flushed separately; Npcap/other embedders keep Emit.
func (t *laneTransport) sendReadyBatch(records []datapath.WireRecord, now time.Time) error {
	var ready [freshBatchSize]preparedFresh
	n := 0
	for _, record := range records {
		if len(record.Wire) == 0 { continue }
		if record.Control {
			if err := t.emitReadyBatch(ready[:n]); err != nil { return err }
			n = 0
			if err := t.send([]datapath.WireRecord{record}, now); err != nil { return err }
			continue
		}
		f, err := t.prepareFresh(record, now)
		if err != nil {
			// No unsent backup may survive and be retransmitted as fresh data.
			for _, held := range ready[:n] { t.completeFresh(held, err) }
			return err
		}
		ready[n] = f
		n++
		if n == freshBatchSize {
			if err := t.emitReadyBatch(ready[:n]); err != nil { return err }
			n = 0
		}
	}
	return t.emitReadyBatch(ready[:n])
}

func (t *laneTransport) emitReadyBatch(ready []preparedFresh) error {
	if len(ready) == 0 { return nil }
	if len(ready) == 1 {
		err := t.cfg.Emit(ready[0].seg)
		t.completeFresh(ready[0], err)
		return err
	}
	var segments [freshBatchSize]faketcp.Segment
	for i, f := range ready { segments[i] = f.seg }
	sent, err := t.cfg.EmitBatch(segments[:len(ready)])
	if sent < 0 || sent > len(ready) { sent, err = 0, ErrTransportConfig }
	if sent != len(ready) && err == nil { err = io.ErrShortWrite }
	t.mu.Lock()
	t.stats.FreshBatchCalls++
	t.stats.FreshBatchSent += uint64(sent)
	t.mu.Unlock()
	for i, f := range ready {
		if i < sent { t.completeFresh(f, nil) } else { t.completeFresh(f, err) }
	}
	return err
}

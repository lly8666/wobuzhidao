package faketcp

import (
	"sync"
	"testing"
)

// Exercise the snapshot while the sole reader and writer update independently;
// driver access itself stays on the reader and requires native Npcap evidence.
func TestNpcapDiagnosticConcurrentSnapshot(t *testing.T) {
	var d npcapIODiagnostics
	d.enabled.Store(true)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := uint64(1); i <= 1000; i++ {
			d.observeRead(100, i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			d.writePackets.Add(1)
			d.observeWriteCall(uint64(i+1), 3)
			d.observeSendWait(uint64(i + 1))
			_ = d.snapshot(7, true)
		}
	}()
	wg.Wait()
	s := d.snapshot(7, true)
	if !s.Enabled || !s.Supported || s.Generation != 7 || s.ReadPackets != 1000 || s.ReadBytes != 100000 || s.WritePackets != 1000 || s.ReadGapMaxNS != 1000 {
		t.Fatalf("wrong final counters: %+v", s)
	}
	if s.WriteCalls != 1000 || s.BatchCalls != 1000 || s.BatchRequested != 3000 || s.WriteCallNS != 500500 || s.WriteCallMaxNS != 1000 || s.SendLockWaitNS != 500500 || s.SendLockWaitMaxNS != 1000 {
		t.Fatalf("wrong timing/batch counters: %+v", s)
	}
}

package linkdata

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

// Fixed 20:20 means 100% repair redundancy, including latency-flushed partial
// blocks. Freeze the live wbd-link-proxy path at approximately 2x payload bytes
// for MTU-sized traffic so a short block cannot regress to N source + 20 repair.
func TestFixedPathPartialWireAmplificationBounded(t *testing.T) {
	const packetSize = 1200
	t0 := time.Unix(1, 0)
	packet := bytes.Repeat([]byte{0x5a}, packetSize)

	for n := 1; n <= 20; n++ {
		t.Run(fmt.Sprintf("sources_%02d", n), func(t *testing.T) {
			p, err := New(fixedConfig(), 64)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < n; i++ {
				if _, err := p.Encode(packet, t0); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := p.FlushDue(t0.Add(8 * time.Millisecond)); err != nil {
				t.Fatal(err)
			}

			st := p.Stats()
			wantPackets := uint64(2 * n)
			if st.InnerTXPackets != uint64(n) || st.WireTXPackets != wantPackets ||
				st.FECSystematicTXPackets != uint64(n) || st.FECRepairTXPackets != uint64(n) {
				t.Fatalf("sources=%d stats=%+v want wire=%d systematic=%d repair=%d", n, st, wantPackets, n, n)
			}
			if st.InnerTXBytes != uint64(n*packetSize) {
				t.Fatalf("sources=%d inner_tx_bytes=%d want=%d", n, st.InnerTXBytes, n*packetSize)
			}
			// At 1200-byte payloads the 56-byte FEC header makes 20:20 about
			// 2.093x at this layer. 2.10x leaves a small integer-safe ceiling
			// while still catching the old N+20 partial-block explosion.
			if st.WireTXBytes*10 > st.InnerTXBytes*21 {
				t.Fatalf("sources=%d FEC wire amplification %.3fx exceeds 2.10x: stats=%+v",
					n, float64(st.WireTXBytes)/float64(st.InnerTXBytes), st)
			}
		})
	}
}

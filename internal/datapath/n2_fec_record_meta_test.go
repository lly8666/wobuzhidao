package datapath

import (
	"bytes"
	"testing"
	"time"
)

// This test crosses the actual FastBlockEncoder/SizeClassEncoder -> seal wire
// boundary, not a fabricated FEC header. It never inspects TLS plaintext.
func TestN2FECRecordMetaForSystematicPartialParityAndOff(t *testing.T) {
	start:=time.Unix(100,0)
	for _,parity:=range []int{0,4,20} {
		lane:=tunnelTestLane(t,RoleClient,parity,byte(30+parity))
		n:=2
		if parity==4 {n=7}
		var originals []WireRecord
		for i:=0;i<n;i++ {
			out,err:=lane.Outbound(bytes.Repeat([]byte{byte(i+1)},120),start)
			if err!=nil || len(out)!=1 {t.Fatalf("parity=%d source=%d wire=%d err=%v",parity,i,len(out),err)}
			r:=out[0]
			if !r.FEC.Known || r.FEC.Enabled!=(parity!=0) {
				t.Fatalf("parity=%d bad original metadata %+v",parity,r.FEC)
			}
			if parity!=0 && (r.FEC.ShardIndex!=uint8(i) || r.FEC.ConfiguredParity!=uint8(parity) ||
				r.FEC.ClosedK!=0 || (i>0 && r.FEC.BlockID!=originals[0].FEC.BlockID)) {
				t.Fatalf("parity=%d source=%d wrong provisional %+v",parity,i,r.FEC)
			}
			originals=append(originals,r)
		}
		if parity>0 {
			parities,err:=lane.FlushDue(start.Add(lane.Config().FlushAfter))
			want:=n
			if n>parity {want=parity}
			if err!=nil||len(parities)!=want {t.Fatalf("parity=%d closed %d want %d err=%v",parity,len(parities),want,err)}
			for i,r:=range parities {
				if !r.FEC.Known||!r.FEC.Enabled||r.FEC.BlockID!=originals[0].FEC.BlockID||
					r.FEC.ShardIndex!=uint8(20+i)||r.FEC.ClosedK!=uint8(n)||
					r.FEC.ConfiguredParity!=uint8(parity)||r.FEC.ShardLengthBin!=0 {
					t.Fatalf("parity=%d repair=%d meta=%+v",parity,i,r.FEC)
				}
			}
		}
		lane.Close()
	}
}

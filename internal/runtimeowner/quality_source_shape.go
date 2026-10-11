package runtimeowner

import (
	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/fec"
)

// QualitySourceShape is an authenticated-source-correlated, sender-local
// evidence snapshot captured after the original quality health Emit succeeds.
// Unlike cumulative encoder Stats, it covers one exact 256-PN source window.
// All categories count ACTUAL successfully emitted KindLINK records only.
// It is not a physical-loss estimator and is NOT yet an auto-controller input.
type QualitySourceShape struct {
	Sent uint32
	Systematic uint32
	Parity uint32
	Off uint32
	Attributed uint32
	Unknown uint32
	FullBlocks uint16
	PartialBlocks uint16
	// Shard-size bins classify CLOSED block maximum shard lengths in bytes:
	// <=256, 257..512, >512. This is not an assertion that the encoder
	// originally selected the same size class for a hinted LINK fragment.
	ClosedBySizeK [3][fec.DataShards+1]uint16
	ClosedByR [fec.DataShards+1]uint16
	Complete bool
}

type sourceShapeGroup struct {
	sourceMask uint32
	parityMask uint32
	configured uint8
	k uint8
	bin uint8
	closed bool
	invalid bool
}

// Called only under laneTransport.mu at the 2s source-report boundary.
// At most 256 records/groups are visited; no global map, old generation,
// packet text or per-data-record scans are retained beyond this snapshot.
func (w *qualityTXWindow) sourceShape(m datapath.QualityHealthReport) QualitySourceShape {
	out:=QualitySourceShape{Sent:uint32(m.TxSuccess)}
	if m.TxSuccess==0 || m.TxSuccess>datapath.QualityPNWindowCapacity ||
		m.WindowLastPN<m.WindowFirstPN ||
		m.WindowLastPN-m.WindowFirstPN>=datapath.QualityPNWindowCapacity {
		return out
	}
	groups:=make(map[uint32]*sourceShapeGroup)
	var valid uint32
	for pn:=m.WindowFirstPN; ; pn++ {
		slot:=w.slots[pn%datapath.QualityPNWindowCapacity]
		if slot.used && slot.pn==pn && slot.success {
			valid++
			meta:=slot.meta
			if !meta.Known {
				// A legacy direct test/encoder without provenance is UNKNOWN,
				// emphatically not FEC-off or zero-loss evidence.
			} else if !meta.Enabled {
				out.Off++
			} else if !fec.IsSupportedParityShards(int(meta.ConfiguredParity)) ||
				int(meta.ShardIndex)>=fec.DataShards+int(meta.ConfiguredParity) {
				// Invalid metadata cannot be priced or attributed.
			} else {
				g:=groups[meta.BlockID]
				if g==nil {
					g=&sourceShapeGroup{configured:meta.ConfiguredParity}
					groups[meta.BlockID]=g
				}
				if g.configured!=meta.ConfiguredParity {g.invalid=true}
				if meta.ShardIndex<fec.DataShards {
					g.sourceMask|=uint32(1)<<meta.ShardIndex
				} else {
					g.parityMask|=uint32(1)<<(meta.ShardIndex-fec.DataShards)
					if meta.ClosedK<1 || meta.ClosedK>fec.DataShards ||
						meta.ShardLengthBin>2 {
						g.invalid=true
					} else if g.closed && (g.k!=meta.ClosedK ||
						g.bin!=meta.ShardLengthBin) {
						g.invalid=true
					} else {
						g.closed=true
						g.k=meta.ClosedK
						g.bin=meta.ShardLengthBin
					}
				}
			}
		}
		if pn==m.WindowLastPN {break}
	}
	if valid!=uint32(m.TxSuccess) {return out}
	for _,g:=range groups {
		if g.invalid || !g.closed {continue}
		k:=int(g.k)
		r:=k
		if r>int(g.configured) {r=int(g.configured)}
		if g.sourceMask!=(uint32(1)<<k)-1 ||
			g.parityMask!=(uint32(1)<<r)-1 {
			// Both incomplete source and cropped parity windows stay
			// unidentified; never infer an unseen final shard arrived.
			continue
		}
		out.Systematic+=uint32(k)
		out.Parity+=uint32(r)
		out.ClosedBySizeK[g.bin][k]++
		out.ClosedByR[r]++
		if k==fec.DataShards {out.FullBlocks++} else {out.PartialBlocks++}
	}
	out.Attributed=out.Systematic+out.Parity+out.Off
	if out.Attributed<=out.Sent {
		out.Unknown=out.Sent-out.Attributed
		out.Complete=out.Unknown==0
	} else {
		// Unreachable for sound metadata, but never qualify corrupt shape.
		out.Attributed=0
		out.Unknown=out.Sent
	}
	return out
}

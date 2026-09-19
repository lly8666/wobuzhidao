package tlsrecord

import "encoding/binary"

const RecentPNCapacity = 65536

type DecoderStats struct {
	Delivered  uint64
	Failed     uint64
	Duplicates uint64
	Evictions  uint64
	Late       uint64
}

type DecodeResult struct {
	PN      uint64
	Payload []byte
	Err     error
}

// Decoder is lane-local state. It intentionally has no expected-PN or receive
// window and is not concurrency-safe; the datapath owner will serialize it.
type Decoder struct {
	opener *Opener
	recent *recentPNSet
	stats  DecoderStats
}

func NewDecoder(keys Keys, maxWire int) (*Decoder, error) {
	opener, err := NewOpener(keys, maxWire)
	if err != nil {
		return nil, err
	}
	return &Decoder{opener: opener, recent: newRecentPNSet()}, nil
}

// OpenPayload parses one or more complete records from one FakeTCP payload.
// An untrustworthy length drops the remaining payload; a record with a valid
// length but failed authentication/structure is skipped so later records can
// still be considered.
func (d *Decoder) OpenPayload(payload []byte) []DecodeResult {
	var out []DecodeResult
	for len(payload) > 0 {
		if len(payload) < OuterHeaderLen {
			d.stats.Failed++
			out = append(out, DecodeResult{Err: ErrInvalidLength})
			break
		}
		bodyLen := int(binary.BigEndian.Uint16(payload[3:5]))
		if bodyLen < MinBodyLen || bodyLen > d.opener.maxBody {
			d.stats.Failed++
			out = append(out, DecodeResult{Err: ErrInvalidLength})
			break
		}
		recordLen := OuterHeaderLen + bodyLen
		if recordLen > len(payload) {
			d.stats.Failed++
			out = append(out, DecodeResult{Err: ErrInvalidLength})
			break
		}

		wire := payload[:recordLen]
		payload = payload[recordLen:]
		record, err := d.opener.OpenRecord(wire)
		if err != nil {
			d.stats.Failed++
			out = append(out, DecodeResult{Err: err})
			continue
		}

		duplicate, evicted, late := d.recent.observe(record.PN)
		if duplicate {
			d.stats.Duplicates++
			out = append(out, DecodeResult{PN: record.PN, Err: ErrDuplicate})
			continue
		}
		if evicted {
			d.stats.Evictions++
		}
		if late {
			d.stats.Late++
		}
		d.stats.Delivered++
		out = append(out, DecodeResult{PN: record.PN, Payload: record.Payload})
	}
	return out
}

func (d *Decoder) Stats() DecoderStats {
	return d.stats
}

type recentPNSet struct {
	set     map[uint64]struct{}
	order   [RecentPNCapacity]uint64
	head    int
	count   int
	maxSeen uint64
	haveMax bool
}

func newRecentPNSet() *recentPNSet {
	return &recentPNSet{set: make(map[uint64]struct{}, RecentPNCapacity)}
}

func (r *recentPNSet) observe(pn uint64) (duplicate, evicted, late bool) {
	if _, ok := r.set[pn]; ok {
		return true, false, false
	}
	late = r.haveMax && pn < r.maxSeen

	if r.count < RecentPNCapacity {
		idx := (r.head + r.count) % RecentPNCapacity
		r.order[idx] = pn
		r.count++
	} else {
		old := r.order[r.head]
		delete(r.set, old)
		r.order[r.head] = pn
		r.head = (r.head + 1) % RecentPNCapacity
		evicted = true
	}
	r.set[pn] = struct{}{}
	if !r.haveMax || pn > r.maxSeen {
		r.maxSeen = pn
		r.haveMax = true
	}
	return false, evicted, late
}

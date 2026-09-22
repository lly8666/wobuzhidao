package datapath

import (
	"container/list"
	"encoding/binary"
	"time"
)

// Startup limits are per logical tunnel, not per lane. Inspection is passive:
// these buffers never own packets awaiting delivery.
const (
	StartupMaxFlows        = 256
	StartupPrefixBytes     = 4096
	StartupWindow          = 2 * time.Second
	StartupRetention       = 30 * time.Second
	StartupMaxRecords      = 12
	StartupMaxPaddingBytes = 2048
)

// TLSStartupPaddingPolicy is an opt-in local sending policy. The peer already
// understands record padding; no admission/wire negotiation changes are needed.
func TLSStartupPaddingPolicy(enabled bool) TunnelPaddingPolicy {
	if !enabled {
		return TunnelPaddingPolicy{}
	}
	return TunnelPaddingPolicy{Enabled: true, TLSStartupOnly: true,
		BytesPerRecord: 256, MaxPaddingBytes: 16 << 20, MaxPaddingRatioPPM: 100_000}
}

type startupKey struct {
	a, b    [6]byte
	id      uint64
	service bool
}
type startupPacket struct {
	key        startupKey
	dir        int
	offset     uint64
	data       []byte
	syn, close bool
	seq        uint32
}
type startupPrefix struct {
	data       [StartupPrefixBytes]byte
	seen       [StartupPrefixBytes]bool
	contiguous int
}
type startupDirection struct {
	base              uint32
	baseSet, rejected bool
	prefix            *startupPrefix
	high              uint64
}
type startupFlow struct {
	key                      startupKey
	deadline, forget         time.Time
	directions               [2]startupDirection
	matched, stopped         bool
	records, reservedRecords uint64
	bytes, reservedBytes     uint64
}
type startupTracker struct {
	flows                             map[startupKey]*startupFlow
	active, retained                  list.List
	detected, capacitySkips, rejected uint64
}

func (s *startupTracker) expire(now time.Time) {
	for e := s.active.Front(); e != nil; e = s.active.Front() {
		f := e.Value.(*startupFlow)
		if now.Before(f.deadline) {
			break
		}
		f.stopped = true
		for i := range f.directions {
			f.directions[i].prefix = nil
		}
		s.active.Remove(e)
	}
	for e := s.retained.Front(); e != nil; e = s.retained.Front() {
		f := e.Value.(*startupFlow)
		if now.Before(f.forget) {
			break
		}
		delete(s.flows, f.key)
		s.retained.Remove(e)
	}
}

// observe returns an eligible flow only for new outgoing payload. Incoming
// ClientHello observations activate the same bidirectional key on the server.
// Offsets/high-water marks suppress duplicate padding without delaying reorders.
func (s *startupTracker) observe(packet []byte, outgoing bool, now time.Time) *startupFlow {
	s.expire(now)
	p, ok := parseStartupPacket(packet, outgoing)
	if !ok {
		return nil
	}
	f := s.flows[p.key]
	if f == nil {
		if p.close || (!p.syn && len(p.data) == 0) {
			return nil
		}
		// Without a SYN, only a plausible stream beginning can create state.
		if !p.syn && (p.key.service && p.offset != 0 || len(p.data) == 0 || p.data[0] != 22) {
			return nil
		}
		if len(s.flows) >= StartupMaxFlows {
			s.capacitySkips++
			return nil
		}
		if s.flows == nil {
			s.flows = make(map[startupKey]*startupFlow)
		}
		f = &startupFlow{key: p.key, deadline: now.Add(StartupWindow), forget: now.Add(StartupRetention)}
		s.flows[p.key] = f
		s.active.PushBack(f)
		s.retained.PushBack(f)
	}
	if f.stopped || !now.Before(f.deadline) {
		return nil
	}
	if p.close {
		f.stopped = true
		for i := range f.directions {
			f.directions[i].prefix = nil
		}
		return nil
	}
	d := &f.directions[p.dir]
	if !p.key.service {
		if !d.baseSet {
			d.base, d.baseSet = p.seq, true
			if p.syn {
				d.base++
			}
		}
		seq := p.seq
		if p.syn {
			seq++
		}
		delta := int32(seq - d.base)
		if delta < 0 {
			return nil
		}
		p.offset = uint64(delta)
	}
	if len(p.data) == 0 {
		return nil
	}
	end := p.offset + uint64(len(p.data))
	if end < p.offset {
		return nil
	}
	fresh := end > d.high
	if fresh {
		d.high = end
	}
	if !f.matched && !d.rejected {
		if p.offset == 0 && p.data[0] != 22 {
			d.rejected = true
			d.prefix = nil
			s.rejected++
			return nil
		}
		if p.offset < StartupPrefixBytes {
			if d.prefix == nil {
				d.prefix = &startupPrefix{}
			}
			buf := d.prefix
			for i, b := range p.data {
				pos := p.offset + uint64(i)
				if pos >= StartupPrefixBytes {
					break
				}
				if buf.seen[pos] && buf.data[pos] != b {
					d.rejected = true
					break
				}
				buf.data[pos], buf.seen[pos] = b, true
			}
			for buf.contiguous < StartupPrefixBytes && buf.seen[buf.contiguous] {
				buf.contiguous++
			}
			valid, reject := startupClientHello(buf.data[:buf.contiguous])
			d.rejected = d.rejected || reject
			if valid && !d.rejected {
				f.matched = true
				s.detected++
				for i := range f.directions {
					f.directions[i].prefix = nil
				}
			} else if d.rejected {
				d.prefix = nil
				s.rejected++
			}
		}
	}
	if outgoing && fresh && f.matched {
		return f
	}
	return nil
}

// Recognizes a complete structurally valid ClientHello inside the first TLS
// record. Split TCP segments are supported, split TLS records/STARTTLS are not.
// Oversized/unsupported prefixes are bypassed, never blocked or truncated.
func startupClientHello(b []byte) (valid, reject bool) {
	if len(b) > 0 && b[0] != 22 {
		return false, true
	}
	if len(b) < 5 {
		return false, false
	}
	if b[1] != 3 || b[2] > 3 || b[2] == 0 {
		return false, true
	}
	n := int(binary.BigEndian.Uint16(b[3:5]))
	if n < 45 || n+5 > StartupPrefixBytes {
		return false, true
	}
	if len(b) < 9 {
		return false, false
	}
	h := int(b[6])<<16 | int(b[7])<<8 | int(b[8])
	if b[5] != 1 || h < 41 || h+4 > n {
		return false, true
	}
	if len(b) < h+9 {
		return false, false
	}
	b = b[9 : h+9]
	if b[0] != 3 || b[1] > 3 || b[1] == 0 {
		return false, true
	}
	pos := 34
	sid := int(b[pos])
	pos++
	if sid > 32 || pos+sid+2 > len(b) {
		return false, true
	}
	pos += sid
	ciphers := int(binary.BigEndian.Uint16(b[pos : pos+2]))
	pos += 2
	if ciphers == 0 || ciphers%2 != 0 || pos+ciphers+1 > len(b) {
		return false, true
	}
	pos += ciphers
	compression := int(b[pos])
	pos++
	if compression == 0 || pos+compression > len(b) {
		return false, true
	}
	pos += compression
	if pos == len(b) {
		return true, false
	}
	if pos+2 > len(b) {
		return false, true
	}
	ext := int(binary.BigEndian.Uint16(b[pos : pos+2]))
	pos += 2
	if pos+ext != len(b) {
		return false, true
	}
	for pos < len(b) {
		if pos+4 > len(b) {
			return false, true
		}
		n := int(binary.BigEndian.Uint16(b[pos+2 : pos+4]))
		pos += 4
		if pos+n > len(b) {
			return false, true
		}
		pos += n
	}
	return true, false
}

// platformflow's v1 envelope is intentionally parsed without importing its
// runtime (which depends on datapath). Its format is covered by a cross-package
// MarshalPacket compatibility test. The product currently leases IPv4 only.
func parseStartupPacket(b []byte, outgoing bool) (p startupPacket, ok bool) {
	if len(b) < 20 || b[0]>>4 != 4 {
		return p, false
	}
	ihl := int(b[0]&15) * 4
	if ihl < 20 || ihl > len(b) || int(binary.BigEndian.Uint16(b[2:4])) != len(b) || binary.BigEndian.Uint16(b[6:8])&0x3fff != 0 {
		return p, false
	}
	if b[9] == 253 {
		if ihl != 20 || len(b) < 64 {
			return p, false
		}
		f := b[ihl:]
		if string(f[:4]) != "WBPF" || f[4] != 1 || f[6]&^byte(1) != 0 || int(binary.BigEndian.Uint16(f[42:44])) != len(f)-44 {
			return p, false
		}
		if f[5] != 3 && f[5] != 5 {
			return p, false
		}
		if f[7] != 0 {
			return p, false
		}
		for _, v := range f[24:42] {
			if v != 0 {
				return p, false
			}
		}
		p.key.service = true
		p.key.id = binary.BigEndian.Uint64(f[8:16])
		if p.key.id == 0 {
			return p, false
		}
		copy(p.key.a[:4], b[12:16])
		copy(p.key.b[:4], b[16:20])
		if outgoing {
			p.dir = 1
		}
		p.offset = binary.BigEndian.Uint64(f[16:24])
		p.data = f[44:]
		p.close = f[5] == 5 || f[6]&1 != 0
		return p, true
	}
	if b[9] != 6 || len(b) < ihl+20 {
		return p, false
	}
	t := b[ihl:]
	thl := int(t[12]>>4) * 4
	if thl < 20 || thl > len(t) {
		return p, false
	}
	copy(p.key.a[:4], b[12:16])
	copy(p.key.a[4:], t[:2])
	copy(p.key.b[:4], b[16:20])
	copy(p.key.b[4:], t[2:4])
	if string(p.key.a[:]) > string(p.key.b[:]) {
		p.key.a, p.key.b = p.key.b, p.key.a
		p.dir = 1
	}
	p.seq = binary.BigEndian.Uint32(t[4:8])
	p.syn = t[13]&2 != 0
	p.close = t[13]&5 != 0
	p.data = t[thl:]
	return p, true
}

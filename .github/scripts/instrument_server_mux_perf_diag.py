#!/usr/bin/env python3
from pathlib import Path
import sys

product = Path(sys.argv[1])
p = product / "cmd/wbd-link-server-mux/main.go"
s = p.read_text()

old = '''\tmu    sync.RWMutex\n\tpeers map[string]*peerSession\n}'''
new = '''\tmu    sync.RWMutex\n\tpeers map[string]*peerSession\n\n\tdiagLastReadNS      atomic.Int64\n\tdiagReadGapMaxNS    atomic.Uint64\n\tdiagInboundCalls    atomic.Uint64\n\tdiagInboundTotalNS  atomic.Uint64\n\tdiagInboundMaxNS    atomic.Uint64\n\tdiagInboundHist     [9]atomic.Uint64\n\tdiagOutboundCalls   atomic.Uint64\n\tdiagOutboundTotalNS atomic.Uint64\n\tdiagOutboundMaxNS   atomic.Uint64\n\tdiagOutboundHist    [9]atomic.Uint64\n\tdiagServiceWriteBytes atomic.Uint64\n\tdiagServiceReadBytes  atomic.Uint64\n}'''
if s.count(old) != 1:
    raise SystemExit(f"server struct marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\tdefer s.Close()\n\tctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)'''
new = '''\tdefer s.Close()\n\tgo s.diagPerfLoop()\n\tctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)'''
if s.count(old) != 1:
    raise SystemExit(f"main diag start marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''func (s *server) handleDatagram(from *net.UDPAddr, packet []byte, now time.Time) error {\n\tkey := from.String()'''
new = '''func (s *server) handleDatagram(from *net.UDPAddr, packet []byte, now time.Time) error {\n\ts.diagRecordReadGap(now)\n\tkey := from.String()'''
if s.count(old) != 1:
    raise SystemExit(f"handleDatagram marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t_, packets, err := s.plane.Inbound(ps.key, packet)\n\tif err != nil {'''
new = '''\tdiagInboundStart := time.Now()\n\t_, packets, err := s.plane.Inbound(ps.key, packet)\n\ts.diagRecordInbound(time.Since(diagInboundStart))\n\tif err != nil {'''
if s.count(old) != 1:
    raise SystemExit(f"inbound timing marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t\tif _, err := ps.service.Write(p); err != nil {\n\t\t\tps.drop.Add(1)\n\t\t\treturn err\n\t\t}'''
new = '''\t\tif _, err := ps.service.Write(p); err != nil {\n\t\t\tps.drop.Add(1)\n\t\t\treturn err\n\t\t}\n\t\ts.diagServiceWriteBytes.Add(uint64(len(p)))'''
if s.count(old) != 1:
    raise SystemExit(f"service write marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t\tn, err := service.Read(buf)\n\t\tif err != nil {\n\t\t\treturn\n\t\t}\n\t\tif isRawIPBackendMeta(buf[:n]) {'''
new = '''\t\tn, err := service.Read(buf)\n\t\tif err != nil {\n\t\t\treturn\n\t\t}\n\t\ts.diagServiceReadBytes.Add(uint64(n))\n\t\tif isRawIPBackendMeta(buf[:n]) {'''
if s.count(old) != 1:
    raise SystemExit(f"service read marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t\tpeerKey, wire, err := s.plane.Outbound(ps.id, buf[:n], now)\n\t\tif err != nil || peerKey != ps.key {'''
new = '''\t\tdiagOutboundStart := time.Now()\n\t\tpeerKey, wire, err := s.plane.Outbound(ps.id, buf[:n], now)\n\t\ts.diagRecordOutbound(time.Since(diagOutboundStart))\n\t\tif err != nil || peerKey != ps.key {'''
if s.count(old) != 1:
    raise SystemExit(f"outbound timing marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

insert_before = '''func (s *server) Addr() *net.UDPAddr {'''
diag = r'''func diagPerfBucket(ns uint64) int {
	us := ns / 1000
	switch {
	case us <= 50:
		return 0
	case us <= 100:
		return 1
	case us <= 250:
		return 2
	case us <= 500:
		return 3
	case us <= 1000:
		return 4
	case us <= 2000:
		return 5
	case us <= 5000:
		return 6
	case us <= 10000:
		return 7
	default:
		return 8
	}
}

func diagAtomicMax(dst *atomic.Uint64, v uint64) {
	for {
		old := dst.Load()
		if v <= old || dst.CompareAndSwap(old, v) {
			return
		}
	}
}

func (s *server) diagRecordReadGap(now time.Time) {
	ns := now.UnixNano()
	prev := s.diagLastReadNS.Swap(ns)
	if prev > 0 && ns > prev {
		diagAtomicMax(&s.diagReadGapMaxNS, uint64(ns-prev))
	}
}

func (s *server) diagRecordInbound(d time.Duration) {
	ns := uint64(d.Nanoseconds())
	s.diagInboundCalls.Add(1)
	s.diagInboundTotalNS.Add(ns)
	diagAtomicMax(&s.diagInboundMaxNS, ns)
	s.diagInboundHist[diagPerfBucket(ns)].Add(1)
}

func (s *server) diagRecordOutbound(d time.Duration) {
	ns := uint64(d.Nanoseconds())
	s.diagOutboundCalls.Add(1)
	s.diagOutboundTotalNS.Add(ns)
	diagAtomicMax(&s.diagOutboundMaxNS, ns)
	s.diagOutboundHist[diagPerfBucket(ns)].Add(1)
}

func (s *server) diagPerfLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for now := range t.C {
		inHist := make([]uint64, len(s.diagInboundHist))
		outHist := make([]uint64, len(s.diagOutboundHist))
		for i := range s.diagInboundHist {
			inHist[i] = s.diagInboundHist[i].Load()
			outHist[i] = s.diagOutboundHist[i].Load()
		}
		fmt.Printf("WBD_LINK_MUX_PERF_DIAG epoch_ns=%d read_gap_max_us_interval=%d inbound_calls=%d inbound_total_ns=%d inbound_max_us_interval=%d inbound_hist=%v outbound_calls=%d outbound_total_ns=%d outbound_max_us_interval=%d outbound_hist=%v service_write_bytes=%d service_read_bytes=%d\n",
			now.UnixNano(), s.diagReadGapMaxNS.Swap(0)/1000,
			s.diagInboundCalls.Load(), s.diagInboundTotalNS.Load(), s.diagInboundMaxNS.Swap(0)/1000, inHist,
			s.diagOutboundCalls.Load(), s.diagOutboundTotalNS.Load(), s.diagOutboundMaxNS.Swap(0)/1000, outHist,
			s.diagServiceWriteBytes.Load(), s.diagServiceReadBytes.Load())
	}
}

'''
if s.count(insert_before) != 1:
    raise SystemExit(f"perf diag insertion marker drift: {s.count(insert_before)}")
s = s.replace(insert_before, diag + insert_before, 1)

p.write_text(s)
print("WBD_SERVER_MUX_PERF_DIAG_PATCHED interval_sec=1 per_packet_logging=0 hist_us=50,100,250,500,1000,2000,5000,10000,gt")

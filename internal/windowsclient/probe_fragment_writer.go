package windowsclient

import (
	"encoding/binary"
	"net/netip"
	"sync"
	"time"
)

const (
	probeFragmentPort       = 18446
	probeFragmentPendingCap = 1024
	probeFragmentTotalCap   = 40000
	probeFragmentWindow     = 390 * time.Second
)

// ProbeFragmentMetadata contains only a controlled probe's IP/UDP headers and
// submission outcome. It owns no packet bytes. Sequence=-1 means unavailable.
type ProbeFragmentMetadata struct {
	UnixNS                int64  `json:"unix_ns"`
	WriteEndUnixNS        int64  `json:"write_end_unix_ns"`
	WriteCallNS           int64  `json:"write_call_ns"`
	IPID                  uint16 `json:"ip_id"`
	Offset                uint16 `json:"offset"`
	MoreFragments         bool   `json:"mf"`
	IPLength              uint16 `json:"ip_length"`
	HeaderLength          uint8  `json:"header_length"`
	FragmentPayloadLength uint16 `json:"fragment_payload_length"`
	HeaderChecksumOK      bool   `json:"header_checksum_ok"`
	Sequence              int64  `json:"sequence"`
	UDPLength             uint16 `json:"udp_length"`
	UDPSourcePort         uint16 `json:"udp_source_port"`
	UDPDestinationPort    uint16 `json:"udp_destination_port"`
	UDPChecksum           uint16 `json:"udp_checksum"`
	WrittenBytes          int    `json:"written_bytes"`
	WriteError            bool   `json:"write_error"`
}

type ProbeFragmentDiagnostic struct {
	Scope           string                  `json:"scope"`
	Destination     string                  `json:"destination"`
	Observed        uint64                  `json:"observed"`
	Recorded        uint64                  `json:"recorded"`
	QueueDropped    uint64                  `json:"queue_dropped"`
	LimitDropped    uint64                  `json:"limit_dropped"`
	WindowDropped   uint64                  `json:"window_dropped"`
	FirstUnixNS     int64                   `json:"first_unix_ns"`
	PendingCapacity int                     `json:"pending_capacity"`
	TotalCapacity   int                     `json:"total_capacity"`
	WindowSeconds   int                     `json:"window_seconds"`
	Rows            []ProbeFragmentMetadata `json:"rows"`
}

// ProbeFragmentWriter is opt-in qualification instrumentation, not a data
// queue. Normal operation uses the original PacketWriter without this wrapper.
// Its lock is never held during the underlying write, and full metadata storage
// never delays, retries, rejects, or changes the packet or the write result.
type ProbeFragmentWriter struct {
	next        PacketWriter
	destination [4]byte
	clock       func() time.Time
	mu          sync.Mutex
	first       time.Time
	stats       ProbeFragmentDiagnostic
	pending     []ProbeFragmentMetadata
}

func NewProbeFragmentWriter(next PacketWriter, destination netip.Addr) (*ProbeFragmentWriter, error) {
	if next == nil || !destination.Is4() {
		return nil, ErrInvalidBinding
	}
	return &ProbeFragmentWriter{next: next, destination: destination.As4(), clock: time.Now,
		pending: make([]ProbeFragmentMetadata, 0, probeFragmentPendingCap),
		stats: ProbeFragmentDiagnostic{Scope: "198.18.0.1 -> current lease, UDP echo18446; successful submission is not kernel/application delivery",
			Destination: destination.String(), PendingCapacity: probeFragmentPendingCap,
			TotalCapacity: probeFragmentTotalCap, WindowSeconds: int(probeFragmentWindow / time.Second)}}, nil
}

func (w *ProbeFragmentWriter) WritePacket(packet []byte) (int, error) {
	row, scoped := w.metadata(packet)
	if !scoped {
		return w.next.WritePacket(packet)
	}
	started := w.clock()
	n, err := w.next.WritePacket(packet)
	ended := w.clock()
	row.UnixNS, row.WriteEndUnixNS = started.UnixNano(), ended.UnixNano()
	row.WriteCallNS, row.WrittenBytes, row.WriteError = ended.Sub(started).Nanoseconds(), n, err != nil
	w.mu.Lock()
	w.stats.Observed++
	if w.first.IsZero() || started.Before(w.first) {
		w.first, w.stats.FirstUnixNS = started, started.UnixNano()
	}
	switch {
	case started.Sub(w.first) >= probeFragmentWindow:
		w.stats.WindowDropped++
	case w.stats.Recorded >= probeFragmentTotalCap:
		w.stats.LimitDropped++
	case len(w.pending) >= probeFragmentPendingCap:
		w.stats.QueueDropped++
	default:
		w.pending = append(w.pending, row)
		w.stats.Recorded++
	}
	w.mu.Unlock()
	return n, err
}

// DiagnosticSnapshot drains metadata once. Counters are cumulative, so missing
// observation coverage is explicit instead of being confused with packet loss.
func (w *ProbeFragmentWriter) DiagnosticSnapshot() ProbeFragmentDiagnostic {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.stats
	out.Rows = append([]ProbeFragmentMetadata(nil), w.pending...)
	w.pending = w.pending[:0]
	return out
}

func (w *ProbeFragmentWriter) metadata(packet []byte) (ProbeFragmentMetadata, bool) {
	var row ProbeFragmentMetadata
	if len(packet) < 20 || packet[0]>>4 != 4 || packet[9] != 17 ||
		packet[12] != 198 || packet[13] != 18 || packet[14] != 0 || packet[15] != 1 ||
		packet[16] != w.destination[0] || packet[17] != w.destination[1] ||
		packet[18] != w.destination[2] || packet[19] != w.destination[3] {
		return row, false
	}
	ihl := int(packet[0]&15) * 4
	total := binary.BigEndian.Uint16(packet[2:4])
	if ihl < 20 || ihl > len(packet) || int(total) != len(packet) {
		return row, false
	}
	flags := binary.BigEndian.Uint16(packet[6:8])
	row.Offset, row.MoreFragments = (flags&8191)*8, flags&8192 != 0
	row.Sequence = -1
	if row.Offset == 0 {
		if len(packet) < ihl+8 || binary.BigEndian.Uint16(packet[ihl:ihl+2]) != probeFragmentPort {
			return row, false
		}
		row.UDPSourcePort = probeFragmentPort
		row.UDPDestinationPort = binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
		row.UDPLength = binary.BigEndian.Uint16(packet[ihl+4 : ihl+6])
		row.UDPChecksum = binary.BigEndian.Uint16(packet[ihl+6 : ihl+8])
		if len(packet) >= ihl+16 && packet[ihl+8] == 'P' && packet[ihl+9] == '7' && packet[ihl+10] == 'M' && packet[ihl+11] == '1' {
			row.Sequence = int64(binary.LittleEndian.Uint32(packet[ihl+12 : ihl+16]))
		}
	}
	row.IPID = binary.BigEndian.Uint16(packet[4:6])
	row.IPLength, row.HeaderLength, row.FragmentPayloadLength = total, uint8(ihl), total-uint16(ihl)
	var checksum uint32
	for i := 0; i < ihl; i += 2 {
		checksum += uint32(binary.BigEndian.Uint16(packet[i : i+2]))
	}
	for checksum>>16 != 0 {
		checksum = (checksum & 65535) + (checksum >> 16)
	}
	row.HeaderChecksumOK = checksum == 65535
	return row, true
}

package faketcp

import "sync/atomic"

// Counters contain no payload, addresses, credentials or session secrets.
// Driver counters are sampled by the sole pcap reader, never by another thread.
type NpcapIODiagnostic struct {
	Enabled          bool   `json:"enabled"`
	Supported        bool   `json:"supported"`
	Generation       uint64 `json:"generation"`
	ReadPackets      uint64 `json:"read_packets"`
	ReadBytes        uint64 `json:"read_bytes"`
	WritePackets     uint64 `json:"write_packets"`
	WriteBytes       uint64 `json:"write_bytes"`
	ReadGapMaxNS     uint64 `json:"read_gap_max_ns"`
	DriverReceived   uint64 `json:"driver_received"`
	DriverDropped    uint64 `json:"driver_dropped"`
	InterfaceDropped uint64 `json:"interface_dropped"`
	StatsSamples     uint64 `json:"stats_samples"`
	StatsErrors      uint64 `json:"stats_errors"`
}

type npcapIODiagnostics struct {
	enabled                                                     atomic.Bool
	readPackets, readBytes, writePackets, writeBytes            atomic.Uint64
	readGapMax, driverReceived, driverDropped, interfaceDropped atomic.Uint64
	statsSamples, statsErrors                                   atomic.Uint64
}

func (d *npcapIODiagnostics) snapshot(generation uint64, supported bool) NpcapIODiagnostic {
	return NpcapIODiagnostic{
		Enabled: d.enabled.Load(), Supported: supported, Generation: generation,
		ReadPackets: d.readPackets.Load(), ReadBytes: d.readBytes.Load(),
		WritePackets: d.writePackets.Load(), WriteBytes: d.writeBytes.Load(),
		ReadGapMaxNS: d.readGapMax.Load(), DriverReceived: d.driverReceived.Load(),
		DriverDropped: d.driverDropped.Load(), InterfaceDropped: d.interfaceDropped.Load(),
		StatsSamples: d.statsSamples.Load(), StatsErrors: d.statsErrors.Load(),
	}
}

func (d *npcapIODiagnostics) observeRead(bytes, gap uint64) {
	d.readPackets.Add(1)
	d.readBytes.Add(bytes)
	for old := d.readGapMax.Load(); gap > old; old = d.readGapMax.Load() {
		if d.readGapMax.CompareAndSwap(old, gap) {
			break
		}
	}
}

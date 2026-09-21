package runtimeentry

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

type p5WeakTransportResource struct {
	Outstanding     int    `json:"outstanding"`
	OutOfOrder      int    `json:"out_of_order"`
	PeakOutstanding int    `json:"peak_outstanding"`
	Retransmitted   uint64 `json:"retransmitted"`
	Abandoned       uint64 `json:"abandoned"`
	ForgivenGaps    uint64 `json:"forgiven_gaps"`
}

type p5WeakResourceSample struct {
	Schema                 string                  `json:"schema"`
	Event                  string                  `json:"event"`
	TNS                    int64                   `json:"t_ns"`
	Phase                  string                  `json:"phase"`
	OuterPackets           uint64                  `json:"outer_packets"`
	OuterWireBytesC2S      uint64                  `json:"outer_wire_bytes_c2s"`
	OuterWireBytesS2C      uint64                  `json:"outer_wire_bytes_s2c"`
	ClientTransport        p5WeakTransportResource `json:"client_transport"`
	ServerTransport        p5WeakTransportResource `json:"server_transport"`
	ClientBusinessFlows    int                     `json:"client_business_flows"`
	ServerBusinessFlows    int                     `json:"server_business_flows"`
	InjectionQueueDepthC2S int                     `json:"injection_queue_depth_c2s"`
	InjectionQueueDepthS2C int                     `json:"injection_queue_depth_s2c"`
	InjectionQueuePeakC2S  int                     `json:"injection_queue_peak_c2s"`
	InjectionQueuePeakS2C  int                     `json:"injection_queue_peak_s2c"`
	MemAllocBytes          uint64                  `json:"mem_alloc_bytes"`
	HeapAllocBytes         uint64                  `json:"heap_alloc_bytes"`
	HeapInuseBytes         uint64                  `json:"heap_inuse_bytes"`
	SysBytes               uint64                  `json:"sys_bytes"`
	NumGC                  uint32                  `json:"num_gc"`
	PauseTotalNS           uint64                  `json:"pause_total_ns"`
	HostCPU                map[string][]uint64     `json:"host_cpu"`
}

func readP5WeakHostCPU() map[string][]uint64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	out := make(map[string][]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		values := make([]uint64, 0, len(fields)-1)
		ok := true
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				ok = false
				break
			}
			values = append(values, value)
		}
		if ok {
			out[fields[0]] = values
		}
	}
	return out
}

func runP5WeakResourceSampler(ctx context.Context, artifacts *p5WeakArtifacts, recorder *p5MeasurementRecorder, phases []p5WeakPhase, network *p5WeakNetwork, client *Client, serverTunnel *serverTunnel, clientRef logicaltunnel.LaneRef) {
	sample := func(now time.Time) {
		outer, _, _ := recorder.counters()
		wireC2S, wireS2C := network.wireSnapshot("c2s"), network.wireSnapshot("s2c")
		clientStats, _ := client.rt.TransportStats(clientRef)
		serverStats, _ := serverTunnel.rt.TransportStats(serverTunnel.ref)
		cDepth, cPeak, _ := network.queueSnapshot("c2s")
		sDepth, sPeak, _ := network.queueSnapshot("s2c")
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		_ = artifacts.writeResource(p5WeakResourceSample{
			Schema:                 p5MeasurementSchema,
			Event:                  "resource_sample",
			TNS:                    artifacts.rel(now),
			Phase:                  p5WeakPhaseForOffset(phases, now.Sub(artifacts.start)),
			OuterPackets:           outer,
			OuterWireBytesC2S:      wireC2S,
			OuterWireBytesS2C:      wireS2C,
			ClientTransport:        p5WeakTransportResource{Outstanding: clientStats.Outstanding, OutOfOrder: clientStats.OutOfOrder, PeakOutstanding: clientStats.PeakOutstanding, Retransmitted: clientStats.Retransmitted, Abandoned: clientStats.Abandoned, ForgivenGaps: clientStats.ForgivenGaps},
			ServerTransport:        p5WeakTransportResource{Outstanding: serverStats.Outstanding, OutOfOrder: serverStats.OutOfOrder, PeakOutstanding: serverStats.PeakOutstanding, Retransmitted: serverStats.Retransmitted, Abandoned: serverStats.Abandoned, ForgivenGaps: serverStats.ForgivenGaps},
			ClientBusinessFlows:    client.Owner().Stats().BusinessFlows,
			ServerBusinessFlows:    serverTunnel.owner.Stats().BusinessFlows,
			InjectionQueueDepthC2S: cDepth,
			InjectionQueueDepthS2C: sDepth,
			InjectionQueuePeakC2S:  cPeak,
			InjectionQueuePeakS2C:  sPeak,
			MemAllocBytes:          mem.Alloc,
			HeapAllocBytes:         mem.HeapAlloc,
			HeapInuseBytes:         mem.HeapInuse,
			SysBytes:               mem.Sys,
			NumGC:                  mem.NumGC,
			PauseTotalNS:           mem.PauseTotalNs,
			HostCPU:                readP5WeakHostCPU(),
		})
	}

	sample(time.Now())
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			sample(time.Now())
			return
		case now := <-ticker.C:
			sample(now)
		}
	}
}

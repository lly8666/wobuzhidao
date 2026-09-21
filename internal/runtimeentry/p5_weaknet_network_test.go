package runtimeentry

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

const (
	p5WeakBaseSeed      = uint64(20260921)
	p5Weak5305BaseSeed  = uint64(20260923)
	p5WeakOneWayDelay   = 300 * time.Millisecond
	p5WeakScenarioTime  = 120 * time.Second
	p5WeakRequestPeriod = 3 * time.Second
	p5WeakRequestLimit  = 12 * time.Second
)

var p5WeakResponseSizes = []int{1024, 8 * 1024, 32 * 1024}

type p5WeakPhase struct {
	Name        string
	Start       time.Duration
	End         time.Duration
	DropPercent uint64
}

var p5WeakPhases = []p5WeakPhase{
	{Name: "low5-initial", Start: 0, End: 30 * time.Second, DropPercent: 5},
	{Name: "high20", Start: 30 * time.Second, End: 90 * time.Second, DropPercent: 20},
	{Name: "low5-recovery", Start: 90 * time.Second, End: 120 * time.Second, DropPercent: 5},
}

var p5Weak5305Phases = []p5WeakPhase{
	{Name: "low5-initial", Start: 0, End: 30 * time.Second, DropPercent: 5},
	{Name: "high30", Start: 30 * time.Second, End: 90 * time.Second, DropPercent: 30},
	{Name: "low5-recovery", Start: 90 * time.Second, End: 120 * time.Second, DropPercent: 5},
}

type p5WeakArtifacts struct {
	mu        sync.Mutex
	start     time.Time
	network   *os.File
	requests  *os.File
	resources *os.File
}

func newP5WeakArtifacts(dir string) (*p5WeakArtifacts, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	network, err := os.Create(filepath.Join(dir, "network.jsonl"))
	if err != nil {
		return nil, err
	}
	requests, err := os.Create(filepath.Join(dir, "requests.jsonl"))
	if err != nil {
		_ = network.Close()
		return nil, err
	}
	resources, err := os.Create(filepath.Join(dir, "resources.jsonl"))
	if err != nil {
		_ = network.Close()
		_ = requests.Close()
		return nil, err
	}
	return &p5WeakArtifacts{network: network, requests: requests, resources: resources}, nil
}

func (a *p5WeakArtifacts) close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return errors.Join(a.network.Close(), a.requests.Close(), a.resources.Close())
}

func (a *p5WeakArtifacts) begin(start time.Time, seed uint64, phases []p5WeakPhase) error {
	a.mu.Lock()
	a.start = start
	a.mu.Unlock()
	for _, phase := range phases {
		if err := a.writeNetwork(map[string]any{
			"schema":              p5MeasurementSchema,
			"event":               "phase_transition",
			"phase":               phase.Name,
			"t_ns":                phase.Start.Nanoseconds(),
			"phase_start_unix_ns": start.Add(phase.Start).UnixNano(),
			"drop_percent":        phase.DropPercent,
			"one_way_delay_ns":    p5WeakOneWayDelay.Nanoseconds(),
			"seed":                seed,
		}); err != nil {
			return err
		}
	}
	return a.writeNetwork(map[string]any{
		"schema":              p5MeasurementSchema,
		"event":               "phase_transition",
		"phase":               "end",
		"t_ns":                p5WeakScenarioTime.Nanoseconds(),
		"phase_start_unix_ns": start.Add(p5WeakScenarioTime).UnixNano(),
		"drop_percent":        0,
		"one_way_delay_ns":    p5WeakOneWayDelay.Nanoseconds(),
		"seed":                seed,
	})
}

func (a *p5WeakArtifacts) rel(now time.Time) int64 {
	a.mu.Lock()
	start := a.start
	a.mu.Unlock()
	if start.IsZero() {
		return 0
	}
	return now.Sub(start).Nanoseconds()
}

func (a *p5WeakArtifacts) writeNetwork(v any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	enc := json.NewEncoder(a.network)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func (a *p5WeakArtifacts) writeRequest(v any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	enc := json.NewEncoder(a.requests)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func (a *p5WeakArtifacts) writeResource(v any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	enc := json.NewEncoder(a.resources)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

type p5WeakNetwork struct {
	mu        sync.Mutex
	start     time.Time
	armed     bool
	artifacts *p5WeakArtifacts
	seed      uint64
	phases    []p5WeakPhase
	links     map[string]*p5WeakLink
}

type p5WeakQueuedSegment struct {
	seg          faketcp.Segment
	packetID     uint64
	phase        string
	phaseOrdinal uint64
	enter        time.Time
	deliverAt    time.Time
}

type p5WeakLink struct {
	direction string
	base      SegmentIO
	network   *p5WeakNetwork

	mu                sync.Mutex
	closed            bool
	globalOrdinal     uint64
	phaseOrdinal      map[string]uint64
	queueDepth        int
	queuePeak         int
	scenarioWireBytes uint64
	firstErr          error

	queue chan p5WeakQueuedSegment
	done  chan struct{}
	once  sync.Once
}

func newP5WeakNetwork(artifacts *p5WeakArtifacts, seed uint64) *p5WeakNetwork {
	return newP5WeakNetworkWithPhases(artifacts, seed, p5WeakPhases)
}

func newP5WeakNetworkWithPhases(artifacts *p5WeakArtifacts, seed uint64, phases []p5WeakPhase) *p5WeakNetwork {
	return &p5WeakNetwork{artifacts: artifacts, seed: seed, phases: phases, links: make(map[string]*p5WeakLink)}
}

func (n *p5WeakNetwork) wrap(direction string, base SegmentIO) SegmentIO {
	link := &p5WeakLink{
		direction:    direction,
		base:         base,
		network:      n,
		phaseOrdinal: make(map[string]uint64),
		queue:        make(chan p5WeakQueuedSegment, 8192),
		done:         make(chan struct{}),
	}
	n.mu.Lock()
	n.links[direction] = link
	n.mu.Unlock()
	go link.run()
	return SegmentIO{Read: base.Read, Emit: link.emit, Close: link.close}
}

func (n *p5WeakNetwork) arm(start time.Time) error {
	n.mu.Lock()
	n.start = start
	n.armed = true
	n.mu.Unlock()
	return n.artifacts.begin(start, n.seed, n.phases)
}

func (n *p5WeakNetwork) phase(now time.Time) (p5WeakPhase, bool, time.Time) {
	n.mu.Lock()
	start, armed := n.start, n.armed
	n.mu.Unlock()
	if !armed {
		return p5WeakPhase{}, false, start
	}
	elapsed := now.Sub(start)
	for _, phase := range n.phases {
		if elapsed >= phase.Start && elapsed < phase.End {
			return phase, true, start
		}
	}
	if elapsed >= p5WeakScenarioTime {
		return p5WeakPhase{Name: "post", Start: p5WeakScenarioTime, End: 1<<63 - 1, DropPercent: 0}, true, start
	}
	return p5WeakPhase{}, false, start
}

func p5WeakDropScore(seed uint64, direction, phase string, ordinal uint64) uint64 {
	var directionSalt uint64
	if direction == "s2c" {
		directionSalt = 17
	}
	var phaseSalt uint64
	switch phase {
	case "high20":
		phaseSalt = 31
	case "high30":
		phaseSalt = 43
	case "low5-recovery":
		phaseSalt = 61
	case "post":
		phaseSalt = 83
	}
	if ordinal == 0 {
		return 0
	}
	return ((ordinal-1)*37 + seed%100 + directionSalt + phaseSalt) % 100
}

func (l *p5WeakLink) emit(seg faketcp.Segment) error {
	now := time.Now()
	phase, armed, start := l.network.phase(now)
	if !armed {
		return l.base.Emit(seg)
	}
	outerPacketLen := len(faketcp.MarshalIPv4TCP(seg.SrcIP, seg.DstIP, seg.SrcPort, seg.DstPort, seg.Seq, seg.Ack, seg.Flags, seg.Window, seg.Payload, 0))

	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return net.ErrClosed
	}
	if l.firstErr != nil {
		err := l.firstErr
		l.mu.Unlock()
		return err
	}
	l.globalOrdinal++
	packetID := l.globalOrdinal
	l.phaseOrdinal[phase.Name]++
	phaseOrdinal := l.phaseOrdinal[phase.Name]
	score := p5WeakDropScore(l.network.seed, l.direction, phase.Name, phaseOrdinal)
	dropped := phase.DropPercent > 0 && score < phase.DropPercent
	depthBefore := l.queueDepth
	if !dropped {
		l.queueDepth++
		if l.queueDepth > l.queuePeak {
			l.queuePeak = l.queueDepth
		}
	}
	depthAfter := l.queueDepth
	if phase.Name != "post" {
		l.scenarioWireBytes += uint64(outerPacketLen)
	}
	l.mu.Unlock()

	copySeg := seg
	copySeg.Payload = append([]byte(nil), seg.Payload...)
	enterNS := now.Sub(start).Nanoseconds()
	if err := l.network.artifacts.writeNetwork(map[string]any{
		"schema":             p5MeasurementSchema,
		"event":              "network_decision",
		"t_ns":               enterNS,
		"direction":          l.direction,
		"phase":              phase.Name,
		"packet_id":          packetID,
		"phase_ordinal":      phaseOrdinal,
		"drop_percent":       phase.DropPercent,
		"drop_score":         score,
		"seed":               l.network.seed,
		"dropped":            dropped,
		"queue_enter_ns":     enterNS,
		"planned_delay_ns":   p5WeakOneWayDelay.Nanoseconds(),
		"queue_depth_before": depthBefore,
		"queue_depth_after":  depthAfter,
		"outer_payload_len":  len(seg.Payload),
		"outer_packet_len":   outerPacketLen,
	}); err != nil {
		return err
	}
	if dropped {
		return nil
	}
	item := p5WeakQueuedSegment{
		seg:          copySeg,
		packetID:     packetID,
		phase:        phase.Name,
		phaseOrdinal: phaseOrdinal,
		enter:        now,
		deliverAt:    now.Add(p5WeakOneWayDelay),
	}
	select {
	case l.queue <- item:
		return nil
	case <-l.done:
		return net.ErrClosed
	}
}

func (l *p5WeakLink) run() {
	defer close(l.done)
	for item := range l.queue {
		if wait := time.Until(item.deliverAt); wait > 0 {
			timer := time.NewTimer(wait)
			<-timer.C
		}
		err := l.base.Emit(item.seg)
		leave := time.Now()
		l.mu.Lock()
		if l.queueDepth > 0 {
			l.queueDepth--
		}
		depth := l.queueDepth
		if err != nil && l.firstErr == nil {
			l.firstErr = err
		}
		l.mu.Unlock()
		_ = l.network.artifacts.writeNetwork(map[string]any{
			"schema":             p5MeasurementSchema,
			"event":              "network_delivery",
			"t_ns":               l.network.artifacts.rel(leave),
			"direction":          l.direction,
			"phase":              item.phase,
			"packet_id":          item.packetID,
			"phase_ordinal":      item.phaseOrdinal,
			"queue_enter_ns":     l.network.artifacts.rel(item.enter),
			"queue_leave_ns":     l.network.artifacts.rel(leave),
			"queue_residency_ns": leave.Sub(item.enter).Nanoseconds(),
			"queue_depth_after":  depth,
			"delivery_error":     errorString(err),
		})
	}
}

func (l *p5WeakLink) close() error {
	var err error
	l.once.Do(func() {
		l.mu.Lock()
		l.closed = true
		l.mu.Unlock()
		close(l.queue)
		<-l.done
		l.mu.Lock()
		err = l.firstErr
		l.mu.Unlock()
		err = errors.Join(err, l.base.Close())
	})
	return err
}

func (l *p5WeakLink) snapshot() (depth, peak int, wireBytes uint64, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.queueDepth, l.queuePeak, l.scenarioWireBytes, l.firstErr
}

func (n *p5WeakNetwork) queueSnapshot(direction string) (depth, peak int, err error) {
	n.mu.Lock()
	link := n.links[direction]
	n.mu.Unlock()
	if link == nil {
		return 0, 0, nil
	}
	depth, peak, _, err = link.snapshot()
	return depth, peak, err
}

func (n *p5WeakNetwork) wireSnapshot(direction string) uint64 {
	n.mu.Lock()
	link := n.links[direction]
	n.mu.Unlock()
	if link == nil {
		return 0
	}
	_, _, wireBytes, _ := link.snapshot()
	return wireBytes
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

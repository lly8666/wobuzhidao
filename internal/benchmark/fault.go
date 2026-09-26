package benchmark

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/lane"
	"github.com/lly8666/wobuzhidao/internal/protocol"
	"github.com/lly8666/wobuzhidao/internal/rbc"
	"github.com/lly8666/wobuzhidao/internal/recovery"
	"github.com/lly8666/wobuzhidao/internal/session"
)

// RealFaultProfile defines a real localhost userspace impairment profile.
// MinOneWay/MaxOneWay are propagation delays applied independently in each
// direction. ImpairBasisPoints is the fraction of source logical records that
// receive ExtraHold on their forward path. The extra hold is a userspace lane
// stall, not a claim that the kernel TCP packet itself was lost.
type RealFaultProfile struct {
	LaneCount         int
	Seed              uint64
	Samples           int
	PayloadBytes      int
	MinOneWay         time.Duration
	MaxOneWay         time.Duration
	ImpairBasisPoints uint16
	ExtraHold         time.Duration
	SoftDeadline      time.Duration
	SourceSpacing     time.Duration
	Window            int
	BurstLength       int
}

type RealFaultSchedule struct {
	Forward  []time.Duration
	Reverse  []time.Duration
	Impaired []bool
}

type RealFaultObservation struct {
	Name             string
	Samples          int
	Completed        int
	TargetMeanRTT    time.Duration
	Mean             time.Duration
	P50              time.Duration
	P95              time.Duration
	P99              time.Duration
	Min              time.Duration
	Max              time.Duration
	CompletionRatio  float64
	DeliveryRatio    float64
	LateRatio        float64
	ImpairmentRatio  float64
	SourceBytes      uint64
	IntentionalBytes uint64
	ReinjectionBytes uint64
	GapEvents        int
	FinalMultiplier  rbc.MultiplierQ4
}

var ErrInvalidRealFaultProfile = errors.New("invalid real fault profile")

func DefaultLowImpairmentProfile() RealFaultProfile {
	return RealFaultProfile{
		LaneCount:         2,
		Seed:              7001,
		Samples:           100,
		PayloadBytes:      256,
		MinOneWay:         25 * time.Millisecond,
		MaxOneWay:         25 * time.Millisecond,
		ImpairBasisPoints: 100,
		ExtraHold:         200 * time.Millisecond,
		SoftDeadline:      100 * time.Millisecond,
		SourceSpacing:     10 * time.Millisecond,
		Window:            4,
		BurstLength:       1,
	}
}

func DefaultModerateImpairmentProfile() RealFaultProfile {
	return RealFaultProfile{
		LaneCount:         2,
		Seed:              8001,
		Samples:           50,
		PayloadBytes:      256,
		MinOneWay:         40 * time.Millisecond,
		MaxOneWay:         75 * time.Millisecond,
		ImpairBasisPoints: 200,
		ExtraHold:         300 * time.Millisecond,
		SoftDeadline:      225 * time.Millisecond,
		SourceSpacing:     50 * time.Millisecond,
		Window:            4,
		BurstLength:       1,
	}
}

func BuildRealFaultSchedule(p RealFaultProfile) (RealFaultSchedule, error) {
	if p.LaneCount < 1 || p.LaneCount > 16 || p.Samples < 4 || p.PayloadBytes < 4 || p.MinOneWay <= 0 || p.MaxOneWay < p.MinOneWay || p.Window < 1 || p.Window > p.Samples || p.BurstLength < 1 || p.SourceSpacing < 0 || p.ImpairBasisPoints > 10000 || (p.ImpairBasisPoints > 0 && p.ExtraHold <= 0) {
		return RealFaultSchedule{}, ErrInvalidRealFaultProfile
	}
	r := faultRNG{state: p.Seed}
	out := RealFaultSchedule{
		Forward:  make([]time.Duration, p.Samples),
		Reverse:  make([]time.Duration, p.Samples),
		Impaired: make([]bool, p.Samples),
	}
	for i := 0; i < p.Samples; i++ {
		out.Forward[i] = faultUniformDuration(&r, p.MinOneWay, p.MaxOneWay)
		out.Reverse[i] = faultUniformDuration(&r, p.MinOneWay, p.MaxOneWay)
	}
	if p.ImpairBasisPoints == 0 {
		return out, nil
	}
	count := (p.Samples*int(p.ImpairBasisPoints) + 5000) / 10000
	if count < 1 {
		count = 1
	}
	// WBD source frames use deterministic round-robin lanes starting at lane 1.
	// Select logical indexes that map to lane 1 so any injected hold has at
	// least one alternate lane when LaneCount > 1. Single-lane profiles are
	// still useful as a no-recovery baseline.
	candidates := make([]int, 0, (p.Samples+p.LaneCount-1)/p.LaneCount)
	for i := 0; i < p.Samples; i += p.LaneCount {
		candidates = append(candidates, i)
	}
	if count > len(candidates) {
		count = len(candidates)
	}
	for i := len(candidates) - 1; i > 0; i-- {
		j := int(r.next() % uint64(i+1))
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}
	marked := 0
	for _, start := range candidates {
		for k := 0; k < p.BurstLength && marked < count; k++ {
			idx := start + k*p.LaneCount
			if idx >= p.Samples || out.Impaired[idx] {
				continue
			}
			out.Impaired[idx] = true
			marked++
		}
		if marked >= count {
			break
		}
	}
	return out, nil
}

func RunRealFaultMatrix(ctx context.Context, p RealFaultProfile, sched RealFaultSchedule) ([]RealFaultObservation, error) {
	if err := validateRealFaultSchedule(p, sched); err != nil {
		return nil, err
	}
	type result struct {
		idx int
		obs RealFaultObservation
		err error
	}
	jobs := []func() (RealFaultObservation, error){
		func() (RealFaultObservation, error) { return RunRealFaultTCP(ctx, p, sched) },
		func() (RealFaultObservation, error) { return RunRealFaultUDP(ctx, p, sched) },
		func() (RealFaultObservation, error) { return RunRealFaultWBD(ctx, p, sched, rbc.ModeNormal) },
		func() (RealFaultObservation, error) { return RunRealFaultWBD(ctx, p, sched, rbc.ModeAuto) },
	}
	ch := make(chan result, len(jobs))
	for i, job := range jobs {
		go func(i int, job func() (RealFaultObservation, error)) {
			obs, err := job()
			ch <- result{idx: i, obs: obs, err: err}
		}(i, job)
	}
	out := make([]RealFaultObservation, len(jobs))
	for range jobs {
		select {
		case got := <-ch:
			if got.err != nil {
				return nil, got.err
			}
			out[got.idx] = got.obs
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return out, nil
}

func RunRealFaultTCP(ctx context.Context, p RealFaultProfile, sched RealFaultSchedule) (RealFaultObservation, error) {
	if err := validateRealFaultSchedule(p, sched); err != nil {
		return RealFaultObservation{}, err
	}
	serverLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return RealFaultObservation{}, err
	}
	defer serverLn.Close()
	proxyLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return RealFaultObservation{}, err
	}
	defer proxyLn.Close()

	errCh := make(chan error, 2)
	go func() {
		c, err := serverLn.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		for i := 0; i < p.Samples; i++ {
			id, payload, err := faultReadRecord(r)
			if err != nil {
				errCh <- err
				return
			}
			if int(id) != i {
				errCh <- fmt.Errorf("fault TCP server id=%d want=%d", id, i)
				return
			}
			if err := faultWriteRecord(c, id, payload); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()
	go func() {
		client, err := proxyLn.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer client.Close()
		server, err := net.Dial("tcp4", serverLn.Addr().String())
		if err != nil {
			errCh <- err
			return
		}
		defer server.Close()
		cr, sr := bufio.NewReader(client), bufio.NewReader(server)
		for i := 0; i < p.Samples; i++ {
			id, payload, err := faultReadRecord(cr)
			if err != nil {
				errCh <- err
				return
			}
			d := sched.Forward[i]
			if sched.Impaired[i] {
				d += p.ExtraHold
			}
			if err := faultSleepContext(ctx, d); err != nil {
				errCh <- err
				return
			}
			if err := faultWriteRecord(server, id, payload); err != nil {
				errCh <- err
				return
			}
			id2, response, err := faultReadRecord(sr)
			if err != nil {
				errCh <- err
				return
			}
			if id2 != id {
				errCh <- fmt.Errorf("fault TCP response id=%d want=%d", id2, id)
				return
			}
			if err := faultSleepContext(ctx, sched.Reverse[i]); err != nil {
				errCh <- err
				return
			}
			if err := faultWriteRecord(client, id2, response); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	c, err := net.Dial("tcp4", proxyLn.Addr().String())
	if err != nil {
		return RealFaultObservation{}, err
	}
	defer c.Close()
	r := bufio.NewReader(c)
	payload := make([]byte, p.PayloadBytes)
	samples := make([]time.Duration, 0, p.Samples)
	for i := 0; i < p.Samples; i++ {
		binary.BigEndian.PutUint32(payload[:4], uint32(i))
		start := time.Now()
		if err := faultWriteRecord(c, uint32(i), payload); err != nil {
			return RealFaultObservation{}, err
		}
		id, got, err := faultReadRecord(r)
		if err != nil {
			return RealFaultObservation{}, err
		}
		if int(id) != i || len(got) != len(payload) {
			return RealFaultObservation{}, errors.New("fault TCP echo mismatch")
		}
		samples = append(samples, time.Since(start))
	}
	if err := faultWaitErrors(ctx, errCh, 2); err != nil {
		return RealFaultObservation{}, err
	}
	return summarizeRealFault("native-tcp", samples, p, sched, uint64(p.Samples*p.PayloadBytes), uint64(p.Samples*p.PayloadBytes), 0, 0, rbc.Multiplier10), nil
}

func RunRealFaultUDP(ctx context.Context, p RealFaultProfile, sched RealFaultSchedule) (RealFaultObservation, error) {
	if err := validateRealFaultSchedule(p, sched); err != nil {
		return RealFaultObservation{}, err
	}
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		return RealFaultObservation{}, err
	}
	defer server.Close()
	proxy, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		return RealFaultObservation{}, err
	}
	defer proxy.Close()
	client, err := net.DialUDP("udp4", nil, proxy.LocalAddr().(*net.UDPAddr))
	if err != nil {
		return RealFaultObservation{}, err
	}
	defer client.Close()

	errCh := make(chan error, 2)
	go func() {
		buf := make([]byte, p.PayloadBytes+8)
		for i := 0; i < p.Samples; i++ {
			n, from, err := server.ReadFromUDP(buf)
			if err != nil {
				errCh <- err
				return
			}
			if _, err := server.WriteToUDP(buf[:n], from); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()
	go func() {
		buf := make([]byte, p.PayloadBytes+8)
		var clientAddr *net.UDPAddr
		for i := 0; i < p.Samples; i++ {
			n, from, err := proxy.ReadFromUDP(buf)
			if err != nil {
				errCh <- err
				return
			}
			clientAddr = from
			pkt := append([]byte(nil), buf[:n]...)
			d := sched.Forward[i]
			if sched.Impaired[i] {
				d += p.ExtraHold
			}
			if err := faultSleepContext(ctx, d); err != nil {
				errCh <- err
				return
			}
			if _, err := proxy.WriteToUDP(pkt, server.LocalAddr().(*net.UDPAddr)); err != nil {
				errCh <- err
				return
			}
			n2, _, err := proxy.ReadFromUDP(buf)
			if err != nil {
				errCh <- err
				return
			}
			resp := append([]byte(nil), buf[:n2]...)
			if err := faultSleepContext(ctx, sched.Reverse[i]); err != nil {
				errCh <- err
				return
			}
			if _, err := proxy.WriteToUDP(resp, clientAddr); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	buf := make([]byte, p.PayloadBytes+8)
	samples := make([]time.Duration, 0, p.Samples)
	for i := 0; i < p.Samples; i++ {
		pkt := make([]byte, p.PayloadBytes+4)
		binary.BigEndian.PutUint32(pkt[:4], uint32(i))
		start := time.Now()
		if _, err := client.Write(pkt); err != nil {
			return RealFaultObservation{}, err
		}
		n, err := client.Read(buf)
		if err != nil {
			return RealFaultObservation{}, err
		}
		if n < 4 || binary.BigEndian.Uint32(buf[:4]) != uint32(i) {
			return RealFaultObservation{}, errors.New("fault UDP echo mismatch")
		}
		samples = append(samples, time.Since(start))
	}
	if err := faultWaitErrors(ctx, errCh, 2); err != nil {
		return RealFaultObservation{}, err
	}
	return summarizeRealFault("native-udp", samples, p, sched, uint64(p.Samples*p.PayloadBytes), uint64(p.Samples*p.PayloadBytes), 0, 0, rbc.Multiplier10), nil
}

func RunRealFaultWBD(ctx context.Context, p RealFaultProfile, sched RealFaultSchedule, mode rbc.ProtectionMode) (RealFaultObservation, error) {
	if err := validateRealFaultSchedule(p, sched); err != nil {
		return RealFaultObservation{}, err
	}
	if mode != rbc.ModeNormal && mode != rbc.ModeAuto {
		return RealFaultObservation{}, errors.New("real WBD fault gate accepts only normal or auto")
	}
	ctl, err := rbc.NewController(mode)
	if err != nil {
		return RealFaultObservation{}, err
	}

	serverLanes := make([]net.Listener, p.LaneCount)
	proxyLanes := make([]net.Listener, p.LaneCount)
	for i := range serverLanes {
		serverLanes[i], err = net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return RealFaultObservation{}, err
		}
		defer serverLanes[i].Close()
		proxyLanes[i], err = net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return RealFaultObservation{}, err
		}
		defer proxyLanes[i].Close()
	}

	proxyErr := make(chan error, p.LaneCount)
	for i := 0; i < p.LaneCount; i++ {
		go runWBDFrameFaultProxy(ctx, proxyLanes[i], serverLanes[i].Addr().String(), protocol.LaneID(i+1), p, sched, proxyErr)
	}

	recv := session.NewReceiver(nil, 0)
	serverErr := make(chan error, p.LaneCount)
	gapCtl := &faultGapEmitter{}
	for i := 0; i < p.LaneCount; i++ {
		go func(ln net.Listener) {
			c, err := ln.Accept()
			if err != nil {
				serverErr <- err
				return
			}
			peer := lane.WrapTCP(c)
			defer peer.Close()
			for {
				v, err := peer.Receive()
				if err != nil {
					if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
						serverErr <- nil
					} else {
						serverErr <- err
					}
					return
				}
				f, ok := v.(protocol.DataFrame)
				if !ok {
					serverErr <- fmt.Errorf("fault WBD server got %T", v)
					return
				}
				delivery, err := recv.AcceptData(f)
				if err != nil {
					serverErr <- err
					return
				}
				receipt, err := recv.ReceiptFor(f.FlowID)
				if err != nil {
					serverErr <- err
					return
				}
				if gapCtl.ShouldSend(receipt.Gap) {
					if err := peer.Send(*receipt.Gap); err != nil {
						serverErr <- err
						return
					}
				}
				if len(delivery.Data) > 0 {
					start := uint64(delivery.NextOffset) - uint64(len(delivery.Data))
					if start%uint64(p.PayloadBytes) != 0 || len(delivery.Data)%p.PayloadBytes != 0 {
						serverErr <- errors.New("fault WBD delivery is not chunk aligned")
						return
					}
					for off := 0; off < len(delivery.Data); off += p.PayloadBytes {
						payload := append([]byte(nil), delivery.Data[off:off+p.PayloadBytes]...)
						echo := protocol.DataFrame{FlowID: f.FlowID, Offset: protocol.StreamOffset(start + uint64(off)), TransmissionID: 1, Payload: payload}
						if err := peer.Send(echo); err != nil {
							serverErr <- err
							return
						}
					}
				}
				for _, ack := range receipt.ACKs {
					if err := peer.Send(ack); err != nil {
						serverErr <- err
						return
					}
				}
			}
		}(serverLanes[i])
	}

	pool := lane.NewPool(128)
	defer pool.Close()
	for i := 0; i < p.LaneCount; i++ {
		c, err := net.Dial("tcp4", proxyLanes[i].Addr().String())
		if err != nil {
			return RealFaultObservation{}, err
		}
		if err := pool.Add(protocol.LaneID(i+1), lane.WrapTCP(c)); err != nil {
			return RealFaultObservation{}, err
		}
	}

	sender := recovery.NewStreamSender(protocol.TransmissionID(p.Samples + 1))
	sentAt := make([]time.Time, p.Samples)
	completed := make([]bool, p.Samples)
	samples := make([]time.Duration, 0, p.Samples)
	inflight := 0
	next := 0
	gapEvents := 0
	var reinjectionBytes uint64
	var lastSourceSend time.Time

	sendSource := func(i int) error {
		if p.SourceSpacing > 0 && !lastSourceSend.IsZero() {
			wait := time.Until(lastSourceSend.Add(p.SourceSpacing))
			if wait > 0 {
				if err := faultSleepContext(ctx, wait); err != nil {
					return err
				}
			}
		}
		payload := make([]byte, p.PayloadBytes)
		binary.BigEndian.PutUint32(payload[:4], uint32(i))
		f := protocol.DataFrame{
			FlowID:         1,
			Offset:         protocol.StreamOffset(i * p.PayloadBytes),
			TransmissionID: protocol.TransmissionID(i + 1),
			Payload:        payload,
		}
		start := time.Now()
		lastSourceSend = start
		id, err := pool.SendNext(f)
		if err != nil {
			return err
		}
		if err := sender.Track(f, id); err != nil {
			return err
		}
		sentAt[i] = start
		inflight++
		return nil
	}
	for next < p.Samples && inflight < p.Window {
		if err := sendSource(next); err != nil {
			return RealFaultObservation{}, err
		}
		next++
	}

	for len(samples) < p.Samples {
		select {
		case ev, ok := <-pool.Events():
			if !ok {
				return RealFaultObservation{}, errors.New("fault WBD pool closed before completion")
			}
			if ev.Err != nil {
				return RealFaultObservation{}, ev.Err
			}
			switch f := ev.Frame.(type) {
			case protocol.DataFrame:
				if f.FlowID != 1 || uint64(f.Offset)%uint64(p.PayloadBytes) != 0 || len(f.Payload) != p.PayloadBytes {
					return RealFaultObservation{}, errors.New("fault WBD echo mismatch")
				}
				idx := int(uint64(f.Offset) / uint64(p.PayloadBytes))
				if idx < 0 || idx >= p.Samples {
					return RealFaultObservation{}, errors.New("fault WBD echo index out of range")
				}
				if !completed[idx] {
					completed[idx] = true
					samples = append(samples, time.Since(sentAt[idx]))
					inflight--
					for next < p.Samples && inflight < p.Window {
						if err := sendSource(next); err != nil {
							return RealFaultObservation{}, err
						}
						next++
					}
				}
			case protocol.AckFrame:
				if err := sender.ApplyACK(f); err != nil {
					return RealFaultObservation{}, err
				}
			case protocol.GapHintFrame:
				gapEvents++
				reinjected, err := sender.ReinjectGapCrossLaneSafe(f, pool)
				if err != nil {
					return RealFaultObservation{}, err
				}
				for _, item := range reinjected {
					reinjectionBytes += uint64(len(item.Frame.Payload))
				}
			default:
				return RealFaultObservation{}, fmt.Errorf("fault WBD client got %T", ev.Frame)
			}
		case err := <-proxyErr:
			if err != nil {
				return RealFaultObservation{}, err
			}
		case err := <-serverErr:
			if err != nil {
				return RealFaultObservation{}, err
			}
		case <-ctx.Done():
			return RealFaultObservation{}, ctx.Err()
		}
	}

	sourceBytes := uint64(p.Samples * p.PayloadBytes)
	intentionalBytes := sourceBytes + reinjectionBytes
	if intentionalBytes > sourceBytes*2 {
		return RealFaultObservation{}, fmt.Errorf("fault WBD intentional bytes exceed 2.0x: source=%d intentional=%d", sourceBytes, intentionalBytes)
	}
	if mode == rbc.ModeAuto {
		// Wall-clock late samples are reported below, but are not fed back into
		// Auto in this localhost gate: host descheduling (especially under -race)
		// is not a logical network-quality signal. Receiver-observed GAP events
		// are deterministic fault-path evidence and remain actionable.
		ctl.Observe(rbc.QualitySample{Delivered: uint64(len(samples)), GapEvents: uint32(gapEvents)})
	}
	name := fmt.Sprintf("wbd-%dlane-%s", p.LaneCount, mode.String())
	return summarizeRealFault(name, samples, p, sched, sourceBytes, intentionalBytes, reinjectionBytes, gapEvents, ctl.Multiplier()), nil
}

func runWBDFrameFaultProxy(ctx context.Context, ln net.Listener, serverAddr string, laneID protocol.LaneID, p RealFaultProfile, sched RealFaultSchedule, errCh chan<- error) {
	client, err := ln.Accept()
	if err != nil {
		errCh <- err
		return
	}
	defer client.Close()
	server, err := net.Dial("tcp4", serverAddr)
	if err != nil {
		errCh <- err
		return
	}
	defer server.Close()

	dirErr := make(chan error, 2)
	go faultRelayFrames(ctx, client, server, func(v any, seq int) time.Duration {
		delay := p.MinOneWay
		if f, ok := v.(protocol.DataFrame); ok && uint64(f.Offset)%uint64(p.PayloadBytes) == 0 {
			idx := int(uint64(f.Offset) / uint64(p.PayloadBytes))
			if idx >= 0 && idx < p.Samples {
				delay = sched.Forward[idx]
				if laneID == 1 && f.TransmissionID <= protocol.TransmissionID(p.Samples) && sched.Impaired[idx] {
					delay += p.ExtraHold
				}
			}
		}
		return delay
	}, dirErr)
	go faultRelayFrames(ctx, server, client, func(_ any, seq int) time.Duration {
		return sched.Reverse[seq%len(sched.Reverse)]
	}, dirErr)

	select {
	case err := <-dirErr:
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
			errCh <- nil
		} else {
			errCh <- err
		}
	case <-ctx.Done():
		errCh <- nil
	}
}

type faultFrameRelease struct {
	frame   any
	release time.Time
}

// faultRelayFrames models propagation/hold delay without turning each delay
// into serialized service time. The reader keeps consuming complete WBD frames
// while the writer releases them at arrival+delay, preserving per-TCP-lane
// frame order exactly as the kernel stream requires.
func faultRelayFrames(ctx context.Context, src net.Conn, dst net.Conn, delay func(any, int) time.Duration, errCh chan<- error) {
	q := make(chan faultFrameRelease, 128)
	readErr := make(chan error, 1)
	go func() {
		defer close(q)
		seq := 0
		for {
			v, err := protocol.ReadFrame(src)
			if err != nil {
				readErr <- err
				return
			}
			item := faultFrameRelease{frame: v, release: time.Now().Add(delay(v, seq))}
			seq++
			select {
			case q <- item:
			case <-ctx.Done():
				readErr <- ctx.Err()
				return
			}
		}
	}()

	for {
		select {
		case item, ok := <-q:
			if !ok {
				select {
				case err := <-readErr:
					errCh <- err
				default:
					errCh <- io.EOF
				}
				return
			}
			if wait := time.Until(item.release); wait > 0 {
				if err := faultSleepContext(ctx, wait); err != nil {
					errCh <- err
					return
				}
			}
			if err := protocol.WriteFrame(dst, item.frame); err != nil {
				errCh <- err
				return
			}
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		}
	}
}

type faultGapEmitter struct {
	mu     sync.Mutex
	active bool
	start  uint64
	end    uint64
}

func (g *faultGapEmitter) ShouldSend(gap *protocol.GapHintFrame) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if gap == nil {
		g.active = false
		return false
	}
	if g.active && g.start == gap.Start && g.end == gap.End {
		return false
	}
	g.active = true
	g.start = gap.Start
	g.end = gap.End
	return true
}

func validateRealFaultSchedule(p RealFaultProfile, s RealFaultSchedule) error {
	if p.Samples < 4 || len(s.Forward) != p.Samples || len(s.Reverse) != p.Samples || len(s.Impaired) != p.Samples || p.PayloadBytes < 4 || p.Window < 1 || p.Window > p.Samples || p.BurstLength < 1 || p.SourceSpacing < 0 {
		return ErrInvalidRealFaultProfile
	}
	for i := range s.Forward {
		if s.Forward[i] <= 0 || s.Reverse[i] <= 0 {
			return ErrInvalidRealFaultProfile
		}
	}
	return nil
}

func summarizeRealFault(name string, samples []time.Duration, p RealFaultProfile, sched RealFaultSchedule, sourceBytes, intentionalBytes, reinjectionBytes uint64, gapEvents int, multiplier rbc.MultiplierQ4) RealFaultObservation {
	v := append([]time.Duration(nil), samples...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	obs := RealFaultObservation{
		Name:             name,
		Samples:          p.Samples,
		Completed:        len(v),
		TargetMeanRTT:    faultScheduleMean(sched),
		CompletionRatio:  ratio(len(v), p.Samples),
		DeliveryRatio:    ratio(len(v), p.Samples),
		LateRatio:        ratio(countLate(v, p.SoftDeadline), len(v)),
		ImpairmentRatio:  ratio(countImpaired(sched.Impaired), p.Samples),
		SourceBytes:      sourceBytes,
		IntentionalBytes: intentionalBytes,
		ReinjectionBytes: reinjectionBytes,
		GapEvents:        gapEvents,
		FinalMultiplier:  multiplier,
	}
	if len(v) == 0 {
		return obs
	}
	var total time.Duration
	for _, d := range v {
		total += d
	}
	obs.Mean = total / time.Duration(len(v))
	obs.P50 = faultPercentile(v, 50)
	obs.P95 = faultPercentile(v, 95)
	obs.P99 = faultPercentile(v, 99)
	obs.Min = v[0]
	obs.Max = v[len(v)-1]
	return obs
}

func faultScheduleMean(s RealFaultSchedule) time.Duration {
	if len(s.Forward) == 0 || len(s.Forward) != len(s.Reverse) {
		return 0
	}
	var total time.Duration
	for i := range s.Forward {
		total += s.Forward[i] + s.Reverse[i]
	}
	return total / time.Duration(len(s.Forward))
}

func faultPercentile(sorted []time.Duration, pct int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := (pct*len(sorted)+99)/100 - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func countLate(samples []time.Duration, deadline time.Duration) int {
	if deadline <= 0 {
		return 0
	}
	n := 0
	for _, d := range samples {
		if d > deadline {
			n++
		}
	}
	return n
}

func countImpaired(v []bool) int {
	n := 0
	for _, x := range v {
		if x {
			n++
		}
	}
	return n
}

func ratio(num, den int) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func faultWriteRecord(w io.Writer, id uint32, payload []byte) error {
	if len(payload) > 1<<20 {
		return errors.New("fault record too large")
	}
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[:4], id)
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	if err := faultWriteFull(w, header); err != nil {
		return err
	}
	return faultWriteFull(w, payload)
}

func faultWriteFull(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if n > 0 {
			b = b[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func faultReadRecord(r *bufio.Reader) (uint32, []byte, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	id := binary.BigEndian.Uint32(header[:4])
	ln := binary.BigEndian.Uint32(header[4:])
	if ln > 1<<20 {
		return 0, nil, errors.New("fault record too large")
	}
	payload := make([]byte, int(ln))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return id, payload, nil
}

func faultSleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func faultWaitErrors(ctx context.Context, ch <-chan error, n int) error {
	for i := 0; i < n; i++ {
		select {
		case err := <-ch:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

type faultRNG struct{ state uint64 }

func (r *faultRNG) next() uint64 {
	x := r.state
	if x == 0 {
		x = 0x9e3779b97f4a7c15
	}
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	r.state = x
	return x
}

func faultUniformDuration(r *faultRNG, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	span := uint64(max-min) + 1
	return min + time.Duration(r.next()%span)
}

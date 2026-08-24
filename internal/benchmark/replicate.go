package benchmark

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/lly8666/wobuzhidao/internal/lane"
	"github.com/lly8666/wobuzhidao/internal/protocol"
	"github.com/lly8666/wobuzhidao/internal/rbc"
	"github.com/lly8666/wobuzhidao/internal/session"
)

// RunRealFaultWBDReplicated is an intentionally aggressive upper-bound
// experiment. Every logical DATA chunk is sent proactively on every lane.
// copies=2 means 2.0x source traffic; copies=3 means 3.0x. The latter is outside
// WBD's product RBC ceiling and exists only to discover the shape of the ceiling
// before a production FEC scheme is admitted.
//
// Each lane gets an independently seeded impairment schedule. An impairment is
// modeled as an extra userspace hold on that TCP carrier, preserving per-lane
// TCP order and therefore the HOL effect of a retransmission stall.
func RunRealFaultWBDReplicated(ctx context.Context, p RealFaultProfile, copies int) (RealFaultObservation, error) {
	if copies < 2 || copies > 3 {
		return RealFaultObservation{}, fmt.Errorf("replicated upper-bound copies must be 2 or 3: %d", copies)
	}
	p.LaneCount = copies
	if p.Window < 1 || p.Window > p.Samples {
		return RealFaultObservation{}, ErrInvalidRealFaultProfile
	}

	schedules := make([]RealFaultSchedule, copies)
	for i := 0; i < copies; i++ {
		lp := p
		// BuildRealFaultSchedule with LaneCount=1 samples impairment from all
		// logical indexes. Distinct seeds make lane failures independent.
		lp.LaneCount = 1
		lp.Seed = p.Seed + uint64(i+1)*0x9e3779b97f4a7c15
		s, err := BuildRealFaultSchedule(lp)
		if err != nil {
			return RealFaultObservation{}, err
		}
		schedules[i] = s
	}

	serverLanes := make([]net.Listener, copies)
	proxyLanes := make([]net.Listener, copies)
	for i := 0; i < copies; i++ {
		var err error
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

	proxyErr := make(chan error, copies)
	for i := 0; i < copies; i++ {
		go runWBDReplicatedFaultProxy(ctx, proxyLanes[i], serverLanes[i].Addr().String(), p, schedules[i], proxyErr)
	}

	recv := session.NewReceiver(nil, 0)
	serverErr := make(chan error, copies)
	for i := 0; i < copies; i++ {
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
					serverErr <- fmt.Errorf("replicated WBD server got %T", v)
					return
				}
				delivery, err := recv.AcceptData(f)
				if err != nil {
					serverErr <- err
					return
				}
				if len(delivery.Data) == 0 {
					continue
				}
				start := uint64(delivery.NextOffset) - uint64(len(delivery.Data))
				if start%uint64(p.PayloadBytes) != 0 || len(delivery.Data)%p.PayloadBytes != 0 {
					serverErr <- errors.New("replicated WBD delivery is not chunk aligned")
					return
				}
				for off := 0; off < len(delivery.Data); off += p.PayloadBytes {
					payload := append([]byte(nil), delivery.Data[off:off+p.PayloadBytes]...)
					echo := protocol.DataFrame{
						FlowID:         f.FlowID,
						Offset:         protocol.StreamOffset(start + uint64(off)),
						TransmissionID: 1,
						Payload:        payload,
					}
					if err := peer.Send(echo); err != nil {
						serverErr <- err
						return
					}
				}
			}
		}(serverLanes[i])
	}

	pool := lane.NewPool(256)
	defer pool.Close()
	for i := 0; i < copies; i++ {
		c, err := net.Dial("tcp4", proxyLanes[i].Addr().String())
		if err != nil {
			return RealFaultObservation{}, err
		}
		if err := pool.Add(protocol.LaneID(i+1), lane.WrapTCP(c)); err != nil {
			return RealFaultObservation{}, err
		}
	}

	sentAt := make([]time.Time, p.Samples)
	completed := make([]bool, p.Samples)
	samples := make([]time.Duration, 0, p.Samples)
	inflight, next := 0, 0
	var lastSourceSend time.Time

	sendLogical := func(i int) error {
		if p.SourceSpacing > 0 && !lastSourceSend.IsZero() {
			if wait := time.Until(lastSourceSend.Add(p.SourceSpacing)); wait > 0 {
				if err := faultSleepContext(ctx, wait); err != nil {
					return err
				}
			}
		}
		payload := make([]byte, p.PayloadBytes)
		binary.BigEndian.PutUint32(payload[:4], uint32(i))
		start := time.Now()
		lastSourceSend = start
		sentAt[i] = start
		for laneIdx := 0; laneIdx < copies; laneIdx++ {
			f := protocol.DataFrame{
				FlowID:         1,
				Offset:         protocol.StreamOffset(i * p.PayloadBytes),
				TransmissionID: protocol.TransmissionID(i*copies + laneIdx + 1),
				Payload:        payload,
			}
			if err := pool.SendOn(protocol.LaneID(laneIdx+1), f); err != nil {
				return err
			}
		}
		inflight++
		return nil
	}

	for next < p.Samples && inflight < p.Window {
		if err := sendLogical(next); err != nil {
			return RealFaultObservation{}, err
		}
		next++
	}

	for len(samples) < p.Samples {
		select {
		case ev, ok := <-pool.Events():
			if !ok {
				return RealFaultObservation{}, errors.New("replicated WBD pool closed before completion")
			}
			if ev.Err != nil {
				return RealFaultObservation{}, ev.Err
			}
			f, ok := ev.Frame.(protocol.DataFrame)
			if !ok {
				return RealFaultObservation{}, fmt.Errorf("replicated WBD client got %T", ev.Frame)
			}
			if f.FlowID != 1 || uint64(f.Offset)%uint64(p.PayloadBytes) != 0 || len(f.Payload) != p.PayloadBytes {
				return RealFaultObservation{}, errors.New("replicated WBD echo mismatch")
			}
			idx := int(uint64(f.Offset) / uint64(p.PayloadBytes))
			if idx < 0 || idx >= p.Samples {
				return RealFaultObservation{}, errors.New("replicated WBD echo index out of range")
			}
			if completed[idx] {
				continue
			}
			completed[idx] = true
			samples = append(samples, time.Since(sentAt[idx]))
			inflight--
			for next < p.Samples && inflight < p.Window {
				if err := sendLogical(next); err != nil {
					return RealFaultObservation{}, err
				}
				next++
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
	intentionalBytes := sourceBytes * uint64(copies)
	// Report the first lane's schedule for propagation/nominal impairment. All
	// lanes use the same configured percentage but independent indexes.
	obs := summarizeRealFault(fmt.Sprintf("wbd-replicate-%dx", copies), samples, p, schedules[0], sourceBytes, intentionalBytes, 0, 0, rbc.Multiplier10)
	return obs, nil
}

func runWBDReplicatedFaultProxy(ctx context.Context, ln net.Listener, serverAddr string, p RealFaultProfile, sched RealFaultSchedule, errCh chan<- error) {
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
	go faultRelayFrames(ctx, client, server, func(v any, _ int) time.Duration {
		delay := p.MinOneWay
		if f, ok := v.(protocol.DataFrame); ok && uint64(f.Offset)%uint64(p.PayloadBytes) == 0 {
			idx := int(uint64(f.Offset) / uint64(p.PayloadBytes))
			if idx >= 0 && idx < p.Samples {
				delay = sched.Forward[idx]
				if sched.Impaired[idx] {
					delay += p.ExtraHold
				}
			}
		}
		return delay
	}, dirErr)
	go faultRelayFrames(ctx, server, client, func(v any, seq int) time.Duration {
		if f, ok := v.(protocol.DataFrame); ok && uint64(f.Offset)%uint64(p.PayloadBytes) == 0 {
			idx := int(uint64(f.Offset) / uint64(p.PayloadBytes))
			if idx >= 0 && idx < p.Samples {
				return sched.Reverse[idx]
			}
		}
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

package benchmark

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/lly8666/wobuzhidao/internal/lane"
	"github.com/lly8666/wobuzhidao/internal/protocol"
	"github.com/lly8666/wobuzhidao/internal/rbc"
	"github.com/lly8666/wobuzhidao/internal/session"
)

// NormalRTTProfile defines a real localhost userspace fault path. MinOneWay
// and MaxOneWay apply independently in each direction. A 20-30 ms one-way
// profile therefore represents an approximately 40-60 ms RTT network.
type NormalRTTProfile struct {
	Seed         uint64
	Samples      int
	PayloadBytes int
	MinOneWay    time.Duration
	MaxOneWay    time.Duration
}

type RTTSchedule struct {
	Forward []time.Duration
	Reverse []time.Duration
}

type RTTObservation struct {
	Name            string
	Samples         int
	TargetMeanRTT   time.Duration
	Mean            time.Duration
	InlierMean      time.Duration
	InlierSamples   int
	P50             time.Duration
	P95             time.Duration
	P99             time.Duration
	Min             time.Duration
	Max             time.Duration
	MedianExcess    time.Duration
	HostOutliers    int
	FinalMultiplier rbc.MultiplierQ4
}

var ErrInvalidRTTProfile = errors.New("invalid normal RTT profile")

func DefaultNormalRTTProfile() NormalRTTProfile {
	return NormalRTTProfile{Seed: 5001, Samples: 64, PayloadBytes: 256, MinOneWay: 20 * time.Millisecond, MaxOneWay: 30 * time.Millisecond}
}

func BuildRTTSchedule(p NormalRTTProfile) (RTTSchedule, error) {
	if p.Samples < 4 || p.PayloadBytes < 4 || p.MinOneWay <= 0 || p.MaxOneWay < p.MinOneWay {
		return RTTSchedule{}, ErrInvalidRTTProfile
	}
	r := rttRNG{state: p.Seed}
	fwd := make([]time.Duration, p.Samples)
	rev := make([]time.Duration, p.Samples)
	for i := 0; i < p.Samples; i++ {
		fwd[i] = rttUniformDuration(&r, p.MinOneWay, p.MaxOneWay)
		rev[i] = rttUniformDuration(&r, p.MinOneWay, p.MaxOneWay)
	}
	return RTTSchedule{Forward: fwd, Reverse: rev}, nil
}

func RunNormalRTTMatrix(ctx context.Context, p NormalRTTProfile, sched RTTSchedule) ([]RTTObservation, error) {
	type result struct {
		idx int
		obs RTTObservation
		err error
	}
	jobs := []func() (RTTObservation, error){
		func() (RTTObservation, error) { return RunNormalTCPRTT(ctx, p, sched) },
		func() (RTTObservation, error) { return RunNormalUDPRTT(ctx, p, sched) },
		func() (RTTObservation, error) { return RunNormalWBDRTT(ctx, p, sched, rbc.ModeNormal) },
		func() (RTTObservation, error) { return RunNormalWBDRTT(ctx, p, sched, rbc.ModeAuto) },
	}
	ch := make(chan result, len(jobs))
	for i, job := range jobs {
		go func(i int, job func() (RTTObservation, error)) {
			obs, err := job()
			ch <- result{idx: i, obs: obs, err: err}
		}(i, job)
	}
	out := make([]RTTObservation, len(jobs))
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

func RunNormalTCPRTT(ctx context.Context, p NormalRTTProfile, sched RTTSchedule) (RTTObservation, error) {
	if err := validateRTTSchedule(p, sched); err != nil {
		return RTTObservation{}, err
	}
	serverLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return RTTObservation{}, err
	}
	defer serverLn.Close()
	proxyLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return RTTObservation{}, err
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
			id, payload, err := readRecord(r)
			if err != nil {
				errCh <- err
				return
			}
			if int(id) != i {
				errCh <- fmt.Errorf("tcp echo id=%d want=%d", id, i)
				return
			}
			if err := writeRecord(c, id, payload); err != nil {
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
			id, payload, err := readRecord(cr)
			if err != nil {
				errCh <- err
				return
			}
			if int(id) != i {
				errCh <- fmt.Errorf("tcp proxy request id=%d want=%d", id, i)
				return
			}
			if err := sleepContext(ctx, sched.Forward[i]); err != nil {
				errCh <- err
				return
			}
			if err := writeRecord(server, id, payload); err != nil {
				errCh <- err
				return
			}
			id2, payload2, err := readRecord(sr)
			if err != nil {
				errCh <- err
				return
			}
			if id2 != id {
				errCh <- fmt.Errorf("tcp proxy response id=%d want=%d", id2, id)
				return
			}
			if err := sleepContext(ctx, sched.Reverse[i]); err != nil {
				errCh <- err
				return
			}
			if err := writeRecord(client, id2, payload2); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	c, err := net.Dial("tcp4", proxyLn.Addr().String())
	if err != nil {
		return RTTObservation{}, err
	}
	defer c.Close()
	r := bufio.NewReader(c)
	samples := make([]time.Duration, 0, p.Samples)
	payload := make([]byte, p.PayloadBytes)
	for i := 0; i < p.Samples; i++ {
		binary.BigEndian.PutUint32(payload[:4], uint32(i))
		start := time.Now()
		if err := writeRecord(c, uint32(i), payload); err != nil {
			return RTTObservation{}, err
		}
		id, got, err := readRecord(r)
		if err != nil {
			return RTTObservation{}, err
		}
		if int(id) != i || len(got) != len(payload) {
			return RTTObservation{}, fmt.Errorf("tcp echo mismatch")
		}
		samples = append(samples, time.Since(start))
	}
	if err := waitRTTErrors(ctx, errCh, 2); err != nil {
		return RTTObservation{}, err
	}
	return summarizeRTT("native-tcp", samples, sched, rbc.Multiplier10), nil
}

func RunNormalUDPRTT(ctx context.Context, p NormalRTTProfile, sched RTTSchedule) (RTTObservation, error) {
	if err := validateRTTSchedule(p, sched); err != nil {
		return RTTObservation{}, err
	}
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		return RTTObservation{}, err
	}
	defer server.Close()
	proxy, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		return RTTObservation{}, err
	}
	defer proxy.Close()
	client, err := net.DialUDP("udp4", nil, proxy.LocalAddr().(*net.UDPAddr))
	if err != nil {
		return RTTObservation{}, err
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
			if n < 4 {
				errCh <- errors.New("short udp echo request")
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
			if n < 4 {
				errCh <- errors.New("short udp proxy request")
				return
			}
			clientAddr = from
			pkt := append([]byte(nil), buf[:n]...)
			if err := sleepContext(ctx, sched.Forward[i]); err != nil {
				errCh <- err
				return
			}
			if _, err := proxy.WriteToUDP(pkt, server.LocalAddr().(*net.UDPAddr)); err != nil {
				errCh <- err
				return
			}
			n2, from2, err := proxy.ReadFromUDP(buf)
			if err != nil {
				errCh <- err
				return
			}
			if from2.Port != server.LocalAddr().(*net.UDPAddr).Port {
				errCh <- errors.New("unexpected udp proxy peer")
				return
			}
			resp := append([]byte(nil), buf[:n2]...)
			if err := sleepContext(ctx, sched.Reverse[i]); err != nil {
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
			return RTTObservation{}, err
		}
		n, err := client.Read(buf)
		if err != nil {
			return RTTObservation{}, err
		}
		if n < 4 || binary.BigEndian.Uint32(buf[:4]) != uint32(i) {
			return RTTObservation{}, errors.New("udp echo mismatch")
		}
		samples = append(samples, time.Since(start))
	}
	if err := waitRTTErrors(ctx, errCh, 2); err != nil {
		return RTTObservation{}, err
	}
	return summarizeRTT("native-udp", samples, sched, rbc.Multiplier10), nil
}

// RunNormalWBDRTT sends real WBD DATA frames over a real kernel TCP socket and
// the same bidirectional fault proxy schedule as the native baselines. The
// peer echoes each logical DATA frame immediately. No logical ACK, FEC,
// duplication or rescue window is placed on the delivery path.
func RunNormalWBDRTT(ctx context.Context, p NormalRTTProfile, sched RTTSchedule, mode rbc.ProtectionMode) (RTTObservation, error) {
	if err := validateRTTSchedule(p, sched); err != nil {
		return RTTObservation{}, err
	}
	ctl, err := rbc.NewController(mode)
	if err != nil {
		return RTTObservation{}, err
	}
	if mode != rbc.ModeNormal && mode != rbc.ModeAuto {
		return RTTObservation{}, errors.New("normal RTT gate accepts only normal or auto")
	}

	serverLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return RTTObservation{}, err
	}
	defer serverLn.Close()
	proxyLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return RTTObservation{}, err
	}
	defer proxyLn.Close()
	errCh := make(chan error, 2)

	go func() {
		c, err := serverLn.Accept()
		if err != nil {
			errCh <- err
			return
		}
		peer := lane.WrapTCP(c)
		defer peer.Close()
		recv := session.NewReceiver(nil, 0)
		for i := 0; i < p.Samples; i++ {
			v, err := peer.Receive()
			if err != nil {
				errCh <- err
				return
			}
			f, ok := v.(protocol.DataFrame)
			if !ok {
				errCh <- fmt.Errorf("WBD server got %T", v)
				return
			}
			delivery, err := recv.AcceptData(f)
			if err != nil {
				errCh <- err
				return
			}
			if len(delivery.Data) != len(f.Payload) || delivery.BufferedBytes != 0 {
				errCh <- fmt.Errorf("WBD receiver did not deliver frame immediately")
				return
			}
			if err := peer.Send(f); err != nil {
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
		for i := 0; i < p.Samples; i++ {
			v, err := protocol.ReadFrame(client)
			if err != nil {
				errCh <- err
				return
			}
			if err := sleepContext(ctx, sched.Forward[i]); err != nil {
				errCh <- err
				return
			}
			if err := protocol.WriteFrame(server, v); err != nil {
				errCh <- err
				return
			}
			resp, err := protocol.ReadFrame(server)
			if err != nil {
				errCh <- err
				return
			}
			if err := sleepContext(ctx, sched.Reverse[i]); err != nil {
				errCh <- err
				return
			}
			if err := protocol.WriteFrame(client, resp); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	c, err := net.Dial("tcp4", proxyLn.Addr().String())
	if err != nil {
		return RTTObservation{}, err
	}
	client := lane.WrapTCP(c)
	defer client.Close()
	samples := make([]time.Duration, 0, p.Samples)
	payload := make([]byte, p.PayloadBytes)
	for i := 0; i < p.Samples; i++ {
		f := protocol.DataFrame{FlowID: 1, Offset: protocol.StreamOffset(i * p.PayloadBytes), TransmissionID: protocol.TransmissionID(i + 1), Payload: payload}
		start := time.Now()
		if err := client.Send(f); err != nil {
			return RTTObservation{}, err
		}
		v, err := client.Receive()
		if err != nil {
			return RTTObservation{}, err
		}
		got, ok := v.(protocol.DataFrame)
		if !ok || got.FlowID != f.FlowID || got.Offset != f.Offset || len(got.Payload) != len(f.Payload) {
			return RTTObservation{}, errors.New("WBD echo mismatch")
		}
		samples = append(samples, time.Since(start))
		if mode == rbc.ModeAuto {
			if m := ctl.Observe(rbc.QualitySample{Delivered: 1}); m != rbc.Multiplier10 {
				return RTTObservation{}, fmt.Errorf("Auto left 1.0x on clean network: %s", m)
			}
		}
	}
	if err := waitRTTErrors(ctx, errCh, 2); err != nil {
		return RTTObservation{}, err
	}
	name := "wbd-normal"
	if mode == rbc.ModeAuto {
		name = "wbd-auto"
	}
	return summarizeRTT(name, samples, sched, ctl.Multiplier()), nil
}

func validateRTTSchedule(p NormalRTTProfile, s RTTSchedule) error {
	if p.Samples < 4 || len(s.Forward) != p.Samples || len(s.Reverse) != p.Samples {
		return ErrInvalidRTTProfile
	}
	return nil
}

func scheduleMean(s RTTSchedule) time.Duration {
	if len(s.Forward) == 0 || len(s.Forward) != len(s.Reverse) {
		return 0
	}
	var total time.Duration
	for i := range s.Forward {
		total += s.Forward[i] + s.Reverse[i]
	}
	return total / time.Duration(len(s.Forward))
}

func summarizeRTT(name string, samples []time.Duration, sched RTTSchedule, multiplier rbc.MultiplierQ4) RTTObservation {
	v := append([]time.Duration(nil), samples...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	var total time.Duration
	for _, d := range v {
		total += d
	}
	q := func(pct int) time.Duration {
		idx := (pct*len(v)+99)/100 - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(v) {
			idx = len(v) - 1
		}
		return v[idx]
	}
	excess := make([]time.Duration, 0, len(samples))
	outliers := 0
	var inlierTotal time.Duration
	inliers := 0
	for i, d := range samples {
		target := sched.Forward[i] + sched.Reverse[i]
		e := d - target
		excess = append(excess, e)
		if e > 10*time.Millisecond {
			outliers++
			continue
		}
		inlierTotal += d
		inliers++
	}
	sort.Slice(excess, func(i, j int) bool { return excess[i] < excess[j] })
	medianExcess := excess[(len(excess)-1)/2]
	obs := RTTObservation{Name: name, Samples: len(v), TargetMeanRTT: scheduleMean(sched), Mean: total / time.Duration(len(v)), P50: q(50), P95: q(95), P99: q(99), Min: v[0], Max: v[len(v)-1], MedianExcess: medianExcess, HostOutliers: outliers, FinalMultiplier: multiplier}
	if inliers > 0 {
		obs.InlierMean = inlierTotal / time.Duration(inliers)
		obs.InlierSamples = inliers
	}
	return obs
}

func waitRTTErrors(ctx context.Context, ch <-chan error, n int) error {
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

type rttRNG struct{ state uint64 }

func (r *rttRNG) next() uint64 {
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
func rttUniformDuration(r *rttRNG, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	span := uint64(max-min) + 1
	return min + time.Duration(r.next()%span)
}

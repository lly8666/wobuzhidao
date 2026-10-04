package benchmark

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/lane"
	"github.com/lly8666/wobuzhidao/internal/protocol"
	"github.com/lly8666/wobuzhidao/internal/rbc"
)

// FECExperimentConfig is benchmark-only. It intentionally does not define WBD
// v1 wire semantics. The experiment exists to answer whether erasure coding
// across independent ordered TCP carriers can bypass carrier HOL well enough to
// justify a later protocol design.
type FECExperimentConfig struct {
	DataShards   int
	ParityShards int
}

func (c FECExperimentConfig) validate() error {
	if c.DataShards != 20 || (c.ParityShards != 10 && c.ParityShards != 20) {
		return fmt.Errorf("experimental FEC supports only 20:10 or 20:20, got %d:%d", c.DataShards, c.ParityShards)
	}
	return nil
}

type fecFramePlan struct {
	forward  []time.Duration
	reverse  []time.Duration
	impaired []bool
}

type fecGroupState struct {
	shards  [][]byte
	present []bool
	decoded bool
}

const fecHeaderLen = 12

// RunRealFaultWBDFEC sends systematic erasure-coded shards as ordinary WBD DATA
// frames over two independent real kernel TCP carriers. It does not use the
// production session/recovery path and is not a wire proposal. A group is
// delivered as soon as any DataShards shards are available; the receiver does
// not wait for a stalled original TCP shard once parity is sufficient.
func RunRealFaultWBDFEC(ctx context.Context, p RealFaultProfile, cfg FECExperimentConfig) (RealFaultObservation, error) {
	if err := cfg.validate(); err != nil {
		return RealFaultObservation{}, err
	}
	if p.LaneCount != 2 || p.Samples < cfg.DataShards || p.Samples%cfg.DataShards != 0 || p.PayloadBytes < 4 || p.SourceSpacing != 0 || p.Window < 1 {
		return RealFaultObservation{}, ErrInvalidRealFaultProfile
	}
	generator, err := fecGenerator(cfg.DataShards, cfg.ParityShards)
	if err != nil {
		return RealFaultObservation{}, err
	}
	groups := p.Samples / cfg.DataShards
	totalShards := cfg.DataShards + cfg.ParityShards
	totalFrames := groups * totalShards
	perLane := []int{(totalFrames + 1) / 2, totalFrames / 2}
	plans := make([]fecFramePlan, 2)
	for i := range plans {
		plans[i] = buildFECFramePlan(p, perLane[i], p.Seed+uint64(i+1)*0x9e3779b97f4a7c15)
	}

	serverLanes := make([]net.Listener, 2)
	proxyLanes := make([]net.Listener, 2)
	for i := 0; i < 2; i++ {
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

	proxyErr := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go runWBDFECFaultProxy(ctx, proxyLanes[i], serverLanes[i].Addr().String(), p, plans[i], proxyErr)
	}

	states := make([]fecGroupState, groups)
	for i := range states {
		states[i] = fecGroupState{shards: make([][]byte, totalShards), present: make([]bool, totalShards)}
	}
	var stateMu sync.Mutex
	serverErr := make(chan error, 2)
	for i := 0; i < 2; i++ {
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
					serverErr <- fmt.Errorf("FEC server got %T", v)
					return
				}
				group, shard, k, m, body, err := parseFECShard(f.Payload)
				if err != nil {
					serverErr <- err
					return
				}
				if k != cfg.DataShards || m != cfg.ParityShards || group < 0 || group >= groups || shard < 0 || shard >= totalShards || len(body) != p.PayloadBytes {
					serverErr <- errors.New("FEC shard metadata mismatch")
					return
				}

				stateMu.Lock()
				st := &states[group]
				if !st.present[shard] {
					st.present[shard] = true
					st.shards[shard] = append([]byte(nil), body...)
				}
				ready := !st.decoded && fecPresentCount(st.present) >= cfg.DataShards
				if ready {
					st.decoded = true
				}
				var snapshot [][]byte
				var present []bool
				if ready {
					snapshot = cloneShards(st.shards)
					present = append([]bool(nil), st.present...)
				}
				stateMu.Unlock()
				if !ready {
					continue
				}
				data, err := fecRecoverData(snapshot, present, generator, cfg.DataShards)
				if err != nil {
					serverErr <- err
					return
				}
				for j := 0; j < cfg.DataShards; j++ {
					idx := group*cfg.DataShards + j
					echo := protocol.DataFrame{FlowID: 1, Offset: protocol.StreamOffset(idx * p.PayloadBytes), TransmissionID: protocol.TransmissionID(idx + 1), Payload: data[j]}
					if err := peer.Send(echo); err != nil {
						serverErr <- err
						return
					}
				}
			}
		}(serverLanes[i])
	}

	pool := lane.NewPool(512)
	defer pool.Close()
	for i := 0; i < 2; i++ {
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
	groupDone := make([]int, groups)
	samples := make([]time.Duration, 0, p.Samples)
	groupsInFlight := 0
	nextGroup := 0
	maxGroups := (p.Window + cfg.DataShards - 1) / cfg.DataShards
	if maxGroups < 1 {
		maxGroups = 1
	}
	var tx protocol.TransmissionID = 1
	var wireSeq int

	sendGroup := func(g int) error {
		data := make([][]byte, cfg.DataShards)
		now := time.Now()
		for j := 0; j < cfg.DataShards; j++ {
			idx := g*cfg.DataShards + j
			payload := make([]byte, p.PayloadBytes)
			binary.BigEndian.PutUint32(payload[:4], uint32(idx))
			data[j] = payload
			sentAt[idx] = now
		}
		shards, err := fecEncode(data, generator, cfg.DataShards, cfg.ParityShards)
		if err != nil {
			return err
		}
		for s := 0; s < totalShards; s++ {
			laneID := protocol.LaneID(wireSeq%2 + 1)
			wireSeq++
			frame := protocol.DataFrame{FlowID: 2, Offset: protocol.StreamOffset(g*totalShards + s), TransmissionID: tx, Payload: encodeFECShard(g, s, cfg.DataShards, cfg.ParityShards, shards[s])}
			tx++
			if err := pool.SendOn(laneID, frame); err != nil {
				return err
			}
		}
		groupsInFlight++
		return nil
	}

	for nextGroup < groups && groupsInFlight < maxGroups {
		if err := sendGroup(nextGroup); err != nil {
			return RealFaultObservation{}, err
		}
		nextGroup++
	}
	for len(samples) < p.Samples {
		select {
		case ev, ok := <-pool.Events():
			if !ok {
				return RealFaultObservation{}, errors.New("FEC pool closed before completion")
			}
			if ev.Err != nil {
				return RealFaultObservation{}, ev.Err
			}
			f, ok := ev.Frame.(protocol.DataFrame)
			if !ok || f.FlowID != 1 || len(f.Payload) != p.PayloadBytes || uint64(f.Offset)%uint64(p.PayloadBytes) != 0 {
				return RealFaultObservation{}, errors.New("FEC echo mismatch")
			}
			idx := int(uint64(f.Offset) / uint64(p.PayloadBytes))
			if idx < 0 || idx >= p.Samples {
				return RealFaultObservation{}, errors.New("FEC echo index out of range")
			}
			if completed[idx] {
				continue
			}
			completed[idx] = true
			samples = append(samples, time.Since(sentAt[idx]))
			g := idx / cfg.DataShards
			groupDone[g]++
			if groupDone[g] == cfg.DataShards {
				groupsInFlight--
				for nextGroup < groups && groupsInFlight < maxGroups {
					if err := sendGroup(nextGroup); err != nil {
						return RealFaultObservation{}, err
					}
					nextGroup++
				}
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

	baseSched, err := BuildRealFaultSchedule(p)
	if err != nil {
		return RealFaultObservation{}, err
	}
	sourceBytes := uint64(p.Samples * p.PayloadBytes)
	intentional := sourceBytes * uint64(cfg.DataShards+cfg.ParityShards) / uint64(cfg.DataShards)
	mult := rbc.Multiplier15
	if cfg.ParityShards == cfg.DataShards {
		mult = rbc.Multiplier20
	}
	obs := summarizeRealFault(fmt.Sprintf("wbd-fec-%d-%d", cfg.DataShards, cfg.ParityShards), samples, p, baseSched, sourceBytes, intentional, 0, 0, mult)
	return obs, nil
}

func runWBDFECFaultProxy(ctx context.Context, ln net.Listener, serverAddr string, p RealFaultProfile, plan fecFramePlan, errCh chan<- error) {
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
	go faultRelayFrames(ctx, client, server, func(_ any, seq int) time.Duration {
		if len(plan.forward) == 0 {
			return 0
		}
		i := seq % len(plan.forward)
		d := plan.forward[i]
		if plan.impaired[i] {
			d += p.ExtraHold
		}
		return d
	}, dirErr)
	go faultRelayFrames(ctx, server, client, func(_ any, seq int) time.Duration {
		if len(plan.reverse) == 0 {
			return 0
		}
		return plan.reverse[seq%len(plan.reverse)]
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

func buildFECFramePlan(p RealFaultProfile, frames int, seed uint64) fecFramePlan {
	r := faultRNG{state: seed}
	out := fecFramePlan{forward: make([]time.Duration, frames), reverse: make([]time.Duration, p.Samples), impaired: make([]bool, frames)}
	for i := range out.forward {
		out.forward[i] = faultUniformDuration(&r, p.MinOneWay, p.MaxOneWay)
	}
	for i := range out.reverse {
		out.reverse[i] = faultUniformDuration(&r, p.MinOneWay, p.MaxOneWay)
	}
	count := (frames*int(p.ImpairBasisPoints) + 5000) / 10000
	if p.ImpairBasisPoints > 0 && count < 1 {
		count = 1
	}
	idx := make([]int, frames)
	for i := range idx {
		idx[i] = i
	}
	for i := len(idx) - 1; i > 0; i-- {
		j := int(r.next() % uint64(i+1))
		idx[i], idx[j] = idx[j], idx[i]
	}
	marked := 0
	for _, start := range idx {
		for b := 0; b < p.BurstLength && marked < count; b++ {
			i := start + b
			if i >= frames || out.impaired[i] {
				continue
			}
			out.impaired[i] = true
			marked++
		}
		if marked >= count {
			break
		}
	}
	return out
}

func encodeFECShard(group, shard, k, m int, body []byte) []byte {
	out := make([]byte, fecHeaderLen+len(body))
	copy(out[:4], []byte{'W', 'F', 'E', 'C'})
	binary.BigEndian.PutUint32(out[4:8], uint32(group))
	binary.BigEndian.PutUint16(out[8:10], uint16(shard))
	out[10] = byte(k)
	out[11] = byte(m)
	copy(out[fecHeaderLen:], body)
	return out
}

func parseFECShard(p []byte) (group, shard, k, m int, body []byte, err error) {
	if len(p) < fecHeaderLen || string(p[:4]) != "WFEC" {
		return 0, 0, 0, 0, nil, errors.New("malformed experimental FEC shard")
	}
	return int(binary.BigEndian.Uint32(p[4:8])), int(binary.BigEndian.Uint16(p[8:10])), int(p[10]), int(p[11]), p[fecHeaderLen:], nil
}

func fecPresentCount(p []bool) int {
	n := 0
	for _, v := range p {
		if v {
			n++
		}
	}
	return n
}

func cloneShards(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i := range in {
		if in[i] != nil {
			out[i] = append([]byte(nil), in[i]...)
		}
	}
	return out
}

func fecGenerator(k, m int) ([][]byte, error) {
	n := k + m
	v := make([][]byte, n)
	for r := 0; r < n; r++ {
		v[r] = make([]byte, k)
		x := byte(r + 1)
		v[r][0] = 1
		for c := 1; c < k; c++ {
			v[r][c] = gfMul(v[r][c-1], x)
		}
	}
	top := make([][]byte, k)
	for i := 0; i < k; i++ {
		top[i] = append([]byte(nil), v[i]...)
	}
	inv, err := gfInvertMatrix(top)
	if err != nil {
		return nil, err
	}
	g := make([][]byte, n)
	for r := 0; r < n; r++ {
		g[r] = make([]byte, k)
		for c := 0; c < k; c++ {
			var x byte
			for j := 0; j < k; j++ {
				x ^= gfMul(v[r][j], inv[j][c])
			}
			g[r][c] = x
		}
	}
	return g, nil
}

func fecEncode(data [][]byte, g [][]byte, k, m int) ([][]byte, error) {
	if len(data) != k || len(g) != k+m {
		return nil, errors.New("bad FEC encode dimensions")
	}
	sz := len(data[0])
	for _, d := range data {
		if len(d) != sz {
			return nil, errors.New("unequal FEC shard size")
		}
	}
	out := make([][]byte, k+m)
	for i := 0; i < k; i++ {
		out[i] = append([]byte(nil), data[i]...)
	}
	for r := k; r < k+m; r++ {
		p := make([]byte, sz)
		for c := 0; c < k; c++ {
			coef := g[r][c]
			if coef == 0 {
				continue
			}
			for b := 0; b < sz; b++ {
				p[b] ^= gfMul(coef, data[c][b])
			}
		}
		out[r] = p
	}
	return out, nil
}

func fecRecoverData(shards [][]byte, present []bool, g [][]byte, k int) ([][]byte, error) {
	if len(shards) != len(present) || len(g) != len(shards) {
		return nil, errors.New("bad FEC decode dimensions")
	}
	selected := make([]int, 0, k)
	for i, v := range present {
		if v {
			selected = append(selected, i)
			if len(selected) == k {
				break
			}
		}
	}
	if len(selected) < k {
		return nil, errors.New("insufficient FEC shards")
	}
	sz := -1
	for _, i := range selected {
		if shards[i] == nil {
			return nil, errors.New("missing selected FEC shard")
		}
		if sz < 0 {
			sz = len(shards[i])
		}
		if len(shards[i]) != sz {
			return nil, errors.New("unequal received FEC shard size")
		}
	}
	a := make([][]byte, k)
	for r, i := range selected {
		a[r] = append([]byte(nil), g[i]...)
	}
	inv, err := gfInvertMatrix(a)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, k)
	for d := 0; d < k; d++ {
		out[d] = make([]byte, sz)
		for r, i := range selected {
			coef := inv[d][r]
			if coef == 0 {
				continue
			}
			for b := 0; b < sz; b++ {
				out[d][b] ^= gfMul(coef, shards[i][b])
			}
		}
	}
	return out, nil
}

func gfMul(a, b byte) byte {
	var p byte
	for b != 0 {
		if b&1 != 0 {
			p ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0x1d
		}
		b >>= 1
	}
	return p
}

func gfInv(a byte) byte {
	if a == 0 {
		return 0
	}
	var r byte = 1
	for i := 0; i < 254; i++ {
		r = gfMul(r, a)
	}
	return r
}

func gfInvertMatrix(in [][]byte) ([][]byte, error) {
	n := len(in)
	if n == 0 {
		return nil, errors.New("empty GF matrix")
	}
	a := make([][]byte, n)
	for i := 0; i < n; i++ {
		if len(in[i]) != n {
			return nil, errors.New("non-square GF matrix")
		}
		a[i] = make([]byte, 2*n)
		copy(a[i], in[i])
		a[i][n+i] = 1
	}
	for c := 0; c < n; c++ {
		pivot := c
		for pivot < n && a[pivot][c] == 0 {
			pivot++
		}
		if pivot == n {
			return nil, errors.New("singular GF matrix")
		}
		a[c], a[pivot] = a[pivot], a[c]
		inv := gfInv(a[c][c])
		if inv == 0 {
			return nil, errors.New("zero GF pivot")
		}
		for j := 0; j < 2*n; j++ {
			a[c][j] = gfMul(a[c][j], inv)
		}
		for r := 0; r < n; r++ {
			if r == c {
				continue
			}
			f := a[r][c]
			if f == 0 {
				continue
			}
			for j := 0; j < 2*n; j++ {
				a[r][j] ^= gfMul(f, a[c][j])
			}
		}
	}
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		out[i] = append([]byte(nil), a[i][n:]...)
	}
	return out, nil
}

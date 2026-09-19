package faketcp

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const (
	DefaultBootstrapChunk      = 1200
	MaxBootstrapPendingChunks  = 64
	MaxBootstrapBufferedBytes  = 256 << 10
	MaxBootstrapFlightChunks   = 4
	bootstrapRetransmitCeiling = 2 * time.Second
)

var (
	ErrBootstrapClosed   = errors.New("faketcp: bootstrap stream closed")
	ErrBootstrapTimeout  = errors.New("faketcp: bootstrap stream deadline exceeded")
	ErrBootstrapOverflow = errors.New("faketcp: bootstrap stream buffer limit exceeded")
)

var bootstrapPayloads = struct {
	sync.RWMutex
	active map[*byte]int
}{active: make(map[*byte]int)}

type BootstrapSend func([]byte) (end uint32, err error)
type BootstrapWaitAck func(end uint32, deadline time.Time) error
type BootstrapWaitWindow func(need, localCap int, deadline time.Time) error
type BootstrapCloseWrite func() error

// BootstrapStream is the temporary reliable ordered adapter used only while TLS
// and fallback own one FakeTCP association. Writes may keep a small bounded
// flight in progress, while each batch retains a cumulative ACK barrier.
type BootstrapStream struct {
	mu      sync.Mutex
	writeMu sync.Mutex

	next         uint32
	pending      map[uint32][]byte
	pendingBytes int
	readBuf      bytes.Buffer
	notify       chan struct{}
	closed       bool
	readEOF      bool
	writeClosed  bool
	fail         error

	finPending bool
	finSeq     uint32

	readDeadline  time.Time
	writeDeadline time.Time

	chunk        int
	maxFlight    int
	send         BootstrapSend
	waitAck      BootstrapWaitAck
	waitWindow   BootstrapWaitWindow
	closeWrite   BootstrapCloseWrite
	local        net.Addr
	remote       net.Addr
}

func NewBootstrapStream(next uint32, send BootstrapSend, waitAck BootstrapWaitAck, local, remote net.Addr) (*BootstrapStream, error) {
	if send == nil || waitAck == nil {
		return nil, errors.New("faketcp: bootstrap send/wait callbacks are required")
	}
	return &BootstrapStream{
		next: next, pending: make(map[uint32][]byte), notify: make(chan struct{}, 1),
		chunk: DefaultBootstrapChunk, maxFlight: 1,
		send: send, waitAck: waitAck, local: local, remote: remote,
	}, nil
}

func (c *BootstrapStream) ConfigureWriteWindow(wait BootstrapWaitWindow, maxChunks int) {
	c.mu.Lock()
	c.waitWindow = wait
	if maxChunks <= 0 {
		maxChunks = 1
	}
	if maxChunks > MaxBootstrapFlightChunks {
		maxChunks = MaxBootstrapFlightChunks
	}
	c.maxFlight = maxChunks
	c.mu.Unlock()
}

func (c *BootstrapStream) ConfigureCloseWrite(closeWrite BootstrapCloseWrite) {
	c.mu.Lock()
	c.closeWrite = closeWrite
	c.mu.Unlock()
}

func (c *BootstrapStream) NextSeq() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.next
}

func (c *BootstrapStream) ReadEOF() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readEOF
}

func (c *BootstrapStream) AvailableReceiveWindow() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.readEOF {
		return 0
	}
	n := MaxBootstrapBufferedBytes - c.readBuf.Len() - c.pendingBytes
	if n < 0 {
		return 0
	}
	return n
}

// Feed retains only bounded bootstrap reordering. A retransmission overlapping
// already-consumed bytes is trimmed rather than duplicating data.
func (c *BootstrapStream) Feed(seq uint32, payload []byte) {
	if len(payload) == 0 {
		return
	}
	c.mu.Lock()
	if c.closed || c.readEOF {
		c.mu.Unlock()
		return
	}
	if seqLT(seq, c.next) {
		skip := int(c.next - seq)
		if skip >= len(payload) {
			c.mu.Unlock()
			return
		}
		seq = c.next
		payload = payload[skip:]
	}
	if c.readBuf.Len()+c.pendingBytes+len(payload) > MaxBootstrapBufferedBytes {
		c.fail = ErrBootstrapOverflow
		c.closed = true
		c.mu.Unlock()
		c.signal()
		return
	}
	if seq == c.next {
		_, _ = c.readBuf.Write(payload)
		c.next += uint32(len(payload))
		for {
			p, ok := c.pending[c.next]
			if !ok {
				break
			}
			delete(c.pending, c.next)
			c.pendingBytes -= len(p)
			_, _ = c.readBuf.Write(p)
			c.next += uint32(len(p))
		}
		c.consumePendingFINLocked()
	} else if _, exists := c.pending[seq]; !exists {
		if len(c.pending) >= MaxBootstrapPendingChunks {
			c.fail = ErrBootstrapOverflow
			c.closed = true
			c.mu.Unlock()
			c.signal()
			return
		}
		p := append([]byte(nil), payload...)
		c.pending[seq] = p
		c.pendingBytes += len(p)
	}
	c.mu.Unlock()
	c.signal()
}

// FeedFIN records FIN at its sequence position. FIN consumes one sequence number
// only once and EOF becomes visible only after all preceding bytes are present.
func (c *BootstrapStream) FeedFIN(seq uint32) {
	c.mu.Lock()
	if c.closed || c.readEOF || seqLT(seq, c.next) {
		c.mu.Unlock()
		return
	}
	if seq == c.next {
		c.next++
		c.readEOF = true
		c.finPending = false
	} else if !c.finPending || seqLT(seq, c.finSeq) {
		c.finPending = true
		c.finSeq = seq
	}
	c.mu.Unlock()
	c.signal()
}

func (c *BootstrapStream) consumePendingFINLocked() {
	if c.finPending && c.finSeq == c.next {
		c.next++
		c.readEOF = true
		c.finPending = false
	}
}

func (c *BootstrapStream) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		if c.readBuf.Len() != 0 {
			n, err := c.readBuf.Read(p)
			c.mu.Unlock()
			return n, err
		}
		if c.fail != nil {
			err := c.fail
			c.mu.Unlock()
			return 0, err
		}
		if c.closed || c.readEOF {
			c.mu.Unlock()
			return 0, io.EOF
		}
		deadline := c.readDeadline
		c.mu.Unlock()
		if err := c.wait(deadline); err != nil {
			return 0, err
		}
	}
}

// Write uses a bounded small flight. It never sends beyond the current peer
// receive window or local flight cap, and waits for a cumulative ACK at the end
// of each flight before reporting those bytes written.
func (c *BootstrapStream) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	written := 0
	for len(p) != 0 {
		batchBytes := 0
		var batchEnd uint32
		sentChunks := 0

		for len(p) != 0 {
			c.mu.Lock()
			if c.fail != nil {
				err := c.fail
				c.mu.Unlock()
				return written, err
			}
			if c.closed || c.writeClosed {
				c.mu.Unlock()
				return written, ErrBootstrapClosed
			}
			chunk := c.chunk
			deadline := c.writeDeadline
			maxFlight := c.maxFlight
			waitWindow := c.waitWindow
			c.mu.Unlock()

			if chunk <= 0 {
				chunk = DefaultBootstrapChunk
			}
			if maxFlight <= 0 {
				maxFlight = 1
			}
			if sentChunks >= maxFlight {
				break
			}
			if chunk > len(p) {
				chunk = len(p)
			}
			if waitWindow != nil {
				localCap := c.chunk * maxFlight
				if localCap <= 0 {
					localCap = DefaultBootstrapChunk * maxFlight
				}
				if err := waitWindow(chunk, localCap, deadline); err != nil {
					return written, err
				}
			}
			end, err := sendBootstrapPayload(c.send, p[:chunk])
			if err != nil {
				return written, err
			}
			batchEnd = end
			batchBytes += chunk
			sentChunks++
			p = p[chunk:]
		}

		if batchBytes == 0 {
			return written, ErrBootstrapOverflow
		}
		c.mu.Lock()
		deadline := c.writeDeadline
		c.mu.Unlock()
		if err := c.waitAck(batchEnd, deadline); err != nil {
			return written, err
		}
		written += batchBytes
	}
	return written, nil
}

func sendBootstrapPayload(send BootstrapSend, payload []byte) (uint32, error) {
	if len(payload) == 0 {
		return send(payload)
	}
	key := &payload[0]
	bootstrapPayloads.Lock()
	bootstrapPayloads.active[key]++
	bootstrapPayloads.Unlock()
	defer func() {
		bootstrapPayloads.Lock()
		if bootstrapPayloads.active[key] <= 1 {
			delete(bootstrapPayloads.active, key)
		} else {
			bootstrapPayloads.active[key]--
		}
		bootstrapPayloads.Unlock()
	}()
	return send(payload)
}

func isBootstrapPayload(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	key := &payload[0]
	bootstrapPayloads.RLock()
	marked := bootstrapPayloads.active[key] != 0
	bootstrapPayloads.RUnlock()
	return marked
}

// CloseWrite is a real TCP half-close hook for ordinary fallback. It does not
// close the receive side and therefore cannot truncate the opposite response.
func (c *BootstrapStream) CloseWrite() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrBootstrapClosed
	}
	if c.writeClosed {
		c.mu.Unlock()
		return nil
	}
	c.writeClosed = true
	fn := c.closeWrite
	c.mu.Unlock()
	if fn != nil {
		return fn()
	}
	return nil
}

// Detach closes only this temporary adapter. It does not close the outer
// association and deliberately does not emit FIN.
func (c *BootstrapStream) Detach() error {
	c.mu.Lock()
	c.closed = true
	c.writeClosed = true
	c.mu.Unlock()
	c.signal()
	return nil
}

func (c *BootstrapStream) Abort(err error) {
	if err == nil {
		err = ErrBootstrapClosed
	}
	c.mu.Lock()
	if c.fail == nil {
		c.fail = err
	}
	c.closed = true
	c.writeClosed = true
	c.mu.Unlock()
	c.signal()
}

func (c *BootstrapStream) Close() error {
	return c.Detach()
}

func (c *BootstrapStream) LocalAddr() net.Addr  { return c.local }
func (c *BootstrapStream) RemoteAddr() net.Addr { return c.remote }

func (c *BootstrapStream) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.writeDeadline = t
	c.mu.Unlock()
	c.signal()
	return nil
}

func (c *BootstrapStream) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.mu.Unlock()
	c.signal()
	return nil
}

func (c *BootstrapStream) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}

func (c *BootstrapStream) signal() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

func (c *BootstrapStream) wait(deadline time.Time) error {
	if deadline.IsZero() {
		<-c.notify
		return nil
	}
	d := time.Until(deadline)
	if d <= 0 {
		return ErrBootstrapTimeout
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.notify:
		return nil
	case <-t.C:
		return ErrBootstrapTimeout
	}
}

// Package dnsroute redirects only ordinary DNS. Non-DNS packets never enter
// this bounded transaction table. All upstream I/O is supplied by the tunnel.
package dnsroute

import (
	"encoding/binary"
	"errors"
	"golang.org/x/net/dns/dnsmessage"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const DefaultServers = "1.1.1.1,8.8.8.8"
const MaxPending = 512
const MaxPendingBytes = 1 << 20

var ErrQuery = errors.New("dnsroute: invalid or full DNS request")

func ParseServers(text string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, s := range strings.Split(text, ",") {
		if strings.TrimSpace(s) == "" {
			continue
		}
		a, e := netip.ParseAddr(strings.TrimSpace(s))
		if e != nil || !a.Is4() || a.IsUnspecified() || a.IsMulticast() {
			return nil, ErrQuery
		}
		duplicate := false
		for _, b := range out {
			if b == a {
				duplicate = true
			}
		}
		if !duplicate {
			out = append(out, a)
		}
	}
	if len(out) > 2 {
		return nil, ErrQuery
	}
	return out, nil
}

type signature struct {
	id    uint16
	name  string
	typ   dnsmessage.Type
	class dnsmessage.Class
}

func inspect(wire []byte, response bool) (signature, dnsmessage.Header, error) {
	var p dnsmessage.Parser
	h, e := p.Start(wire)
	if e != nil || h.Response != response || h.OpCode != 0 {
		return signature{}, h, ErrQuery
	}
	q, e := p.Question()
	if e != nil {
		return signature{}, h, ErrQuery
	}
	if _, e = p.Question(); e != dnsmessage.ErrSectionDone {
		return signature{}, h, ErrQuery
	}
	return signature{h.ID, strings.ToLower(q.Name.String()), q.Type, q.Class}, h, nil
}

type key struct {
	client netip.AddrPort
	id     uint16
}
type request struct {
	sig               signature
	wire              []byte
	original          netip.AddrPort
	first, second     int
	fallback          bool
	retryAt, deadline time.Time
}
type SendUDP func(client, peer netip.AddrPort, wire []byte, now time.Time) error
type ReplyUDP func(peer, client netip.AddrPort, wire []byte) error
type DialTCP func(peer netip.AddrPort) (net.Conn, error)
type Resolver struct {
	mu             sync.Mutex
	servers        []netip.Addr
	pending        map[key]*request
	bytes          int
	preferred      int
	preferredUntil time.Time
	closed         bool
	send           SendUDP
	reply          ReplyUDP
	dial           DialTCP
	tcp            map[net.Conn]struct{}
	wg             sync.WaitGroup
}

func New(servers []netip.Addr, send SendUDP, reply ReplyUDP, dial DialTCP) (*Resolver, error) {
	if len(servers) < 1 || len(servers) > 2 || send == nil || reply == nil || dial == nil {
		return nil, ErrQuery
	}
	for _, a := range servers {
		if !a.Is4() || a.IsUnspecified() || a.IsMulticast() {
			return nil, ErrQuery
		}
	}
	return &Resolver{servers: append([]netip.Addr(nil), servers...), pending: make(map[key]*request), tcp: make(map[net.Conn]struct{}), send: send, reply: reply, dial: dial}, nil
}
func (r *Resolver) choose(now time.Time) (int, int) {
	i := 0
	if now.Before(r.preferredUntil) {
		i = r.preferred
	}
	j := i
	if len(r.servers) == 2 {
		j = 1 - i
	}
	return i, j
}
func (r *Resolver) Query(client, original netip.AddrPort, wire []byte, now time.Time) error {
	sig, _, e := inspect(wire, false)
	if e != nil {
		return e
	}
	k := key{client, sig.id}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrQuery
	}
	if old := r.pending[k]; old != nil {
		r.bytes -= len(old.wire)
		delete(r.pending, k)
	}
	if len(r.pending) >= MaxPending || r.bytes+len(wire) > MaxPendingBytes {
		r.mu.Unlock()
		return ErrQuery
	}
	i, j := r.choose(now)
	q := &request{sig: sig, wire: append([]byte(nil), wire...), original: original, first: i, second: j, retryAt: now.Add(1500 * time.Millisecond), deadline: now.Add(6 * time.Second)}
	r.pending[k] = q
	r.bytes += len(q.wire)
	r.mu.Unlock()
	if e = r.send(client, netip.AddrPortFrom(r.servers[i], 53), q.wire, now); e != nil && j != i {
		return r.retry(k, q, now)
	}
	return e
}
func (r *Resolver) retry(k key, q *request, now time.Time) error {
	r.mu.Lock()
	if r.closed || r.pending[k] != q || q.fallback || q.second == q.first {
		r.mu.Unlock()
		return nil
	}
	q.fallback = true
	r.mu.Unlock()
	return r.send(k.client, netip.AddrPortFrom(r.servers[q.second], 53), q.wire, now)
}

// Answer consumes resolver replies, validates the transaction/question, and
// restores the original queried address. A late copy cannot be delivered twice.
func (r *Resolver) Answer(peer, client netip.AddrPort, wire []byte, now time.Time) (bool, error) {
	known := false
	if peer.Port() == 53 {
		for _, a := range r.servers {
			known = known || peer.Addr() == a
		}
	}
	if !known {
		return false, nil
	}
	sig, h, e := inspect(wire, true)
	if e != nil {
		return true, nil
	}
	k := key{client, sig.id}
	r.mu.Lock()
	q := r.pending[k]
	if r.closed || q == nil || q.sig != sig || !now.Before(q.deadline) || (peer.Addr() != r.servers[q.first] && (!q.fallback || peer.Addr() != r.servers[q.second])) {
		r.mu.Unlock()
		return true, nil
	}
	if (h.RCode == dnsmessage.RCodeServerFailure || h.RCode == dnsmessage.RCodeRefused) && !q.fallback && q.second != q.first {
		r.mu.Unlock()
		return true, r.retry(k, q, now)
	}
	delete(r.pending, k)
	r.bytes -= len(q.wire)
	for i, a := range r.servers {
		if a == peer.Addr() {
			r.preferred = i
			r.preferredUntil = now.Add(30 * time.Second)
		}
	}
	r.mu.Unlock()
	return true, r.reply(q.original, client, wire)
}
func (r *Resolver) Tick(now time.Time) {
	type due struct {
		k key
		q *request
	}
	var retries []due
	r.mu.Lock()
	for k, q := range r.pending {
		if !now.Before(q.deadline) {
			delete(r.pending, k)
			r.bytes -= len(q.wire)
		} else if !now.Before(q.retryAt) && !q.fallback && q.first != q.second {
			retries = append(retries, due{k, q})
		}
	}
	r.mu.Unlock()
	for _, x := range retries {
		_ = r.retry(x.k, x.q, now)
	}
}
func (r *Resolver) exchangeTCP(wire []byte, now time.Time) ([]byte, error) {
	sig, _, err := inspect(wire, false)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	i, j := r.choose(now)
	r.mu.Unlock()
	order := []int{i}
	if j != i {
		order = append(order, j)
	}
	for _, index := range order {
		c, e := r.dial(netip.AddrPortFrom(r.servers[index], 53))
		if e != nil {
			continue
		}
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		buf := make([]byte, 2+len(wire))
		binary.BigEndian.PutUint16(buf, uint16(len(wire)))
		copy(buf[2:], wire)
		n, e := c.Write(buf)
		if e == nil && n != len(buf) {
			e = io.ErrShortWrite
		}
		var header [2]byte
		if e == nil {
			_, e = io.ReadFull(c, header[:])
		}
		var answer []byte
		if e == nil {
			answer = make([]byte, int(binary.BigEndian.Uint16(header[:])))
			_, e = io.ReadFull(c, answer)
		}
		_ = c.Close()
		if e != nil {
			continue
		}
		got, h, e := inspect(answer, true)
		if e != nil || got != sig || h.RCode == dnsmessage.RCodeServerFailure || h.RCode == dnsmessage.RCodeRefused {
			continue
		}
		r.mu.Lock()
		r.preferred = index
		r.preferredUntil = time.Now().Add(30 * time.Second)
		r.mu.Unlock()
		return answer, nil
	}
	return nil, ErrQuery
}

// ServeTCP accepts the existing intercepted stream, using virtual tunnel pipes
// for upstream TCP (including large answers). No localhost forwarding socket.
func (r *Resolver) ServeTCP(c net.Conn) {
	r.mu.Lock()
	if r.closed || len(r.tcp) >= 64 {
		r.mu.Unlock()
		c.Close()
		return
	}
	r.tcp[c] = struct{}{}
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer func() { c.Close(); r.mu.Lock(); delete(r.tcp, c); r.mu.Unlock(); r.wg.Done() }()
		for {
			_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
			var h [2]byte
			if _, e := io.ReadFull(c, h[:]); e != nil {
				return
			}
			wire := make([]byte, int(binary.BigEndian.Uint16(h[:])))
			if _, e := io.ReadFull(c, wire); e != nil {
				return
			}
			answer, e := r.exchangeTCP(wire, time.Now())
			if e != nil {
				return
			}
			binary.BigEndian.PutUint16(h[:], uint16(len(answer)))
			_ = c.SetWriteDeadline(time.Now().Add(3 * time.Second))
			if _, e = c.Write(h[:]); e != nil {
				return
			}
			if _, e = c.Write(answer); e != nil {
				return
			}
		}
	}()
}
func (r *Resolver) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.pending = make(map[key]*request)
	r.bytes = 0
	var cc []net.Conn
	for c := range r.tcp {
		cc = append(cc, c)
	}
	r.mu.Unlock()
	for _, c := range cc {
		c.Close()
	}
	r.wg.Wait()
}

package realitymirror

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"
)

var (
	ErrNotClientHello = errors.New("realitymirror: first TLS handshake message is not ClientHello")
	ErrMalformedHello = errors.New("realitymirror: malformed ClientHello")
	ErrSNIMismatch    = errors.New("realitymirror: SNI does not match configured target identity")
	ErrTransferLimit  = errors.New("realitymirror: transfer byte limit reached")
)

type HelloInfo struct {
	ServerName string
	ALPN       []string
}

type Config struct {
	Target         string
	ServerName     string
	HelloTimeout   time.Duration
	DialTimeout    time.Duration
	SessionTimeout time.Duration
	MaxHelloBytes  int
	MaxBytes       int64 // per direction after the mirrored ClientHello; 0 means unlimited
}

type Result struct {
	Hello     HelloInfo
	Target    string
	UpBytes   int64
	DownBytes int64
}

func (c Config) validate() error {
	if strings.TrimSpace(c.Target) == "" || strings.TrimSpace(c.ServerName) == "" {
		return errors.New("realitymirror: target and server name are required")
	}
	if c.MaxHelloBytes <= 0 {
		return errors.New("realitymirror: max ClientHello bytes must be positive")
	}
	if c.HelloTimeout <= 0 || c.DialTimeout <= 0 {
		return errors.New("realitymirror: hello/dial timeouts must be positive")
	}
	if c.MaxBytes < 0 {
		return errors.New("realitymirror: max bytes cannot be negative")
	}
	return nil
}

// ReadClientHello consumes only the TLS records required to obtain the first
// ClientHello. raw is byte-for-byte what was read from the client and is meant
// to be written to the configured target before transparent forwarding starts.
func ReadClientHello(conn net.Conn, maxBytes int, timeout time.Duration) (HelloInfo, []byte, error) {
	var zero HelloInfo
	if conn == nil || maxBytes <= 0 || timeout <= 0 {
		return zero, nil, ErrMalformedHello
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{})

	raw := make([]byte, 0, 4096)
	handshake := make([]byte, 0, 4096)
	var hdr [5]byte
	for len(raw) < maxBytes {
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			return zero, nil, err
		}
		recordLen := int(binary.BigEndian.Uint16(hdr[3:5]))
		if recordLen <= 0 || len(raw)+len(hdr)+recordLen > maxBytes {
			return zero, nil, ErrMalformedHello
		}
		payload := make([]byte, recordLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return zero, nil, err
		}
		raw = append(raw, hdr[:]...)
		raw = append(raw, payload...)
		if hdr[0] != 22 { // TLS handshake record
			return zero, nil, ErrNotClientHello
		}
		handshake = append(handshake, payload...)
		if len(handshake) < 4 {
			continue
		}
		if handshake[0] != 1 {
			return zero, nil, ErrNotClientHello
		}
		msgLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
		if msgLen <= 0 || 4+msgLen > maxBytes {
			return zero, nil, ErrMalformedHello
		}
		if len(handshake) < 4+msgLen {
			continue
		}
		info, err := parseClientHelloBody(handshake[4 : 4+msgLen])
		if err != nil {
			return zero, nil, err
		}
		return info, append([]byte(nil), raw...), nil
	}
	return zero, nil, ErrMalformedHello
}

func parseClientHelloBody(b []byte) (HelloInfo, error) {
	var out HelloInfo
	// legacy_version + random + session-id length
	if len(b) < 35 {
		return out, ErrMalformedHello
	}
	off := 34
	sidLen := int(b[off])
	off++
	if off+sidLen+2 > len(b) {
		return out, ErrMalformedHello
	}
	off += sidLen
	cipherLen := int(binary.BigEndian.Uint16(b[off : off+2]))
	off += 2
	if cipherLen < 2 || cipherLen%2 != 0 || off+cipherLen+1 > len(b) {
		return out, ErrMalformedHello
	}
	off += cipherLen
	compressionLen := int(b[off])
	off++
	if off+compressionLen > len(b) {
		return out, ErrMalformedHello
	}
	off += compressionLen
	if off == len(b) {
		return out, nil
	}
	if off+2 > len(b) {
		return out, ErrMalformedHello
	}
	extLen := int(binary.BigEndian.Uint16(b[off : off+2]))
	off += 2
	if off+extLen != len(b) {
		return out, ErrMalformedHello
	}
	end := off + extLen
	for off < end {
		if off+4 > end {
			return out, ErrMalformedHello
		}
		typ := binary.BigEndian.Uint16(b[off : off+2])
		n := int(binary.BigEndian.Uint16(b[off+2 : off+4]))
		off += 4
		if off+n > end {
			return out, ErrMalformedHello
		}
		ext := b[off : off+n]
		off += n
		switch typ {
		case 0: // server_name
			name, err := parseSNI(ext)
			if err != nil {
				return out, err
			}
			if name != "" {
				out.ServerName = name
			}
		case 16: // ALPN
			protos, err := parseALPN(ext)
			if err != nil {
				return out, err
			}
			out.ALPN = protos
		}
	}
	return out, nil
}

func parseSNI(b []byte) (string, error) {
	if len(b) < 2 {
		return "", ErrMalformedHello
	}
	listLen := int(binary.BigEndian.Uint16(b[:2]))
	if listLen != len(b)-2 {
		return "", ErrMalformedHello
	}
	off := 2
	for off < len(b) {
		if off+3 > len(b) {
			return "", ErrMalformedHello
		}
		nameType := b[off]
		nameLen := int(binary.BigEndian.Uint16(b[off+1 : off+3]))
		off += 3
		if nameLen == 0 || off+nameLen > len(b) {
			return "", ErrMalformedHello
		}
		if nameType == 0 {
			return string(b[off : off+nameLen]), nil
		}
		off += nameLen
	}
	return "", nil
}

func parseALPN(b []byte) ([]string, error) {
	if len(b) < 2 {
		return nil, ErrMalformedHello
	}
	listLen := int(binary.BigEndian.Uint16(b[:2]))
	if listLen != len(b)-2 {
		return nil, ErrMalformedHello
	}
	var out []string
	for off := 2; off < len(b); {
		n := int(b[off])
		off++
		if n == 0 || off+n > len(b) {
			return nil, ErrMalformedHello
		}
		out = append(out, string(b[off:off+n]))
		off += n
	}
	return out, nil
}

// Handle reproduces the target-mirroring opening/fallback behavior useful as a
// REALITY oracle: read a genuine TLS ClientHello from the client, enforce one
// fixed SNI, dial one fixed target, forward the exact ClientHello, then splice
// the connection. The certificate and handshake received by the client are
// therefore generated by the real target, not copied or forged by WBD.
func Handle(ctx context.Context, client net.Conn, cfg Config) (Result, error) {
	var out Result
	if err := cfg.validate(); err != nil {
		return out, err
	}
	info, rawHello, err := ReadClientHello(client, cfg.MaxHelloBytes, cfg.HelloTimeout)
	if err != nil {
		return out, err
	}
	if !sameName(info.ServerName, cfg.ServerName) {
		return out, fmt.Errorf("%w: got %q want %q", ErrSNIMismatch, info.ServerName, cfg.ServerName)
	}

	dialer := net.Dialer{Timeout: cfg.DialTimeout}
	target, err := dialer.DialContext(ctx, "tcp", cfg.Target)
	if err != nil {
		return out, err
	}
	defer target.Close()
	out.Hello, out.Target = info, cfg.Target

	if cfg.SessionTimeout > 0 {
		deadline := time.Now().Add(cfg.SessionTimeout)
		_ = client.SetDeadline(deadline)
		_ = target.SetDeadline(deadline)
		defer client.SetDeadline(time.Time{})
	}
	if _, err := target.Write(rawHello); err != nil {
		return out, err
	}
	out.UpBytes = int64(len(rawHello))

	type copyResult struct {
		direction byte
		n         int64
		err       error
	}
	ch := make(chan copyResult, 2)
	go func() {
		n, err := copyLimited(target, client, cfg.MaxBytes)
		closeWrite(target)
		ch <- copyResult{direction: 'u', n: n, err: err}
	}()
	go func() {
		n, err := copyLimited(client, target, cfg.MaxBytes)
		closeWrite(client)
		ch <- copyResult{direction: 'd', n: n, err: err}
	}()

	first := <-ch
	if first.err != nil {
		_ = client.Close()
		_ = target.Close()
	}
	second := <-ch
	for _, r := range []copyResult{first, second} {
		if r.direction == 'u' {
			out.UpBytes += r.n
		} else {
			out.DownBytes += r.n
		}
	}
	if !benignCopyError(first.err) {
		return out, first.err
	}
	if !benignCopyError(second.err) {
		return out, second.err
	}
	return out, nil
}

func benignCopyError(err error) bool {
	return err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}

func copyLimited(dst io.Writer, src io.Reader, max int64) (int64, error) {
	if max <= 0 {
		return io.Copy(dst, src)
	}
	lr := &hardLimitReader{r: src, remaining: max}
	return io.Copy(dst, lr)
}

type hardLimitReader struct {
	r         io.Reader
	remaining int64
}

func (r *hardLimitReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, ErrTransferLimit
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	if r.remaining == 0 && err == nil {
		return n, ErrTransferLimit
	}
	return n, err
}

func closeWrite(conn net.Conn) {
	if c, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = c.CloseWrite()
	}
}

func sameName(a, b string) bool {
	normalize := func(s string) string {
		return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	}
	return normalize(a) != "" && normalize(a) == normalize(b)
}

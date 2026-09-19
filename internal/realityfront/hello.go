package realityfront

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

const (
	markerLen   = 32
	markerLabel = "WBD-REALITY-FRONT-MARKER-v1\x00"
)

var (
	ErrMarker           = errors.New("realityfront: invalid ClientHello marker")
	ErrUnsupportedHello = errors.New("realityfront: unsupported ClientHello layout")
	ErrMalformedHello   = errors.New("realityfront: malformed ClientHello")
	ErrNotClientHello   = errors.New("realityfront: first TLS handshake message is not ClientHello")
)

type HelloInfo struct {
	ServerName string
	ALPN       []string
}

type Hello struct {
	Info       HelloInfo
	Raw        []byte
	Recognized bool
}

type helloIdentity struct {
	Random    [32]byte
	SessionID [32]byte
}

func markerFor(key []byte, serverName string, random [32]byte) [markerLen]byte {
	mac := hmac.New(sha256.New, key)
	_, _ = io.WriteString(mac, markerLabel)
	_, _ = mac.Write(random[:])
	_, _ = io.WriteString(mac, normalizeName(serverName))
	var out [markerLen]byte
	copy(out[:], mac.Sum(nil))
	return out
}

func parseHelloIdentity(raw []byte) (helloIdentity, error) {
	var out helloIdentity
	handshake, err := collectHandshake(raw)
	if err != nil {
		return out, err
	}
	if len(handshake) < 4 || handshake[0] != 1 {
		return out, ErrUnsupportedHello
	}
	msgLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if msgLen <= 0 || 4+msgLen > len(handshake) {
		return out, ErrUnsupportedHello
	}
	b := handshake[4 : 4+msgLen]
	if len(b) < 35 {
		return out, ErrUnsupportedHello
	}
	copy(out.Random[:], b[2:34])
	sidLen := int(b[34])
	if sidLen != 32 || len(b) < 35+sidLen {
		return out, ErrUnsupportedHello
	}
	copy(out.SessionID[:], b[35:67])
	return out, nil
}

func recognized(raw, routeKey []byte, serverName string) bool {
	if len(routeKey) < 16 {
		return false
	}
	id, err := parseHelloIdentity(raw)
	if err != nil {
		return false
	}
	want := markerFor(routeKey, serverName, id.Random)
	return subtle.ConstantTimeCompare(id.SessionID[:], want[:]) == 1
}

// ReadHello consumes exactly the TLS records required for the first ClientHello.
// Raw bytes are retained byte-for-byte so an unrecognized hello can later be
// forwarded to fallback, while a recognized hello can be replayed into tls.Server.
func ReadHello(conn net.Conn, serverName string, routeKey []byte, timeout time.Duration) (Hello, error) {
	var out Hello
	if conn == nil || normalizeName(serverName) == "" || len(routeKey) < 16 {
		return out, errors.New("realityfront: incomplete ClientHello classifier config")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	info, raw, err := readClientHello(conn, 64<<10, timeout)
	if err != nil {
		return out, err
	}
	out.Info = info
	out.Raw = raw
	out.Recognized = normalizeName(info.ServerName) == normalizeName(serverName) &&
		recognized(raw, routeKey, info.ServerName)
	return out, nil
}

func readClientHello(conn net.Conn, maxBytes int, timeout time.Duration) (HelloInfo, []byte, error) {
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
		if hdr[0] != 22 {
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

func collectHandshake(raw []byte) ([]byte, error) {
	handshake := make([]byte, 0, len(raw))
	for off := 0; off < len(raw); {
		if off+5 > len(raw) || raw[off] != 22 {
			return nil, ErrUnsupportedHello
		}
		n := int(binary.BigEndian.Uint16(raw[off+3 : off+5]))
		off += 5
		if n <= 0 || off+n > len(raw) {
			return nil, ErrUnsupportedHello
		}
		handshake = append(handshake, raw[off:off+n]...)
		off += n
	}
	return handshake, nil
}

func parseClientHelloBody(b []byte) (HelloInfo, error) {
	var out HelloInfo
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
		case 0:
			name, err := parseSNI(ext)
			if err != nil {
				return out, err
			}
			if name != "" {
				out.ServerName = name
			}
		case 16:
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

type replayConn struct {
	net.Conn
	prefix *bytes.Reader
}

func (c *replayConn) Read(p []byte) (int, error) {
	if c.prefix != nil && c.prefix.Len() != 0 {
		return c.prefix.Read(p)
	}
	return c.Conn.Read(p)
}

func replay(conn net.Conn, alreadyRead []byte) net.Conn {
	return &replayConn{
		Conn:   conn,
		prefix: bytes.NewReader(append([]byte(nil), alreadyRead...)),
	}
}

func normalizeName(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}

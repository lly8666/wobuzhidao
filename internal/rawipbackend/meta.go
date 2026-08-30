package rawipbackend

import (
	"encoding/hex"
	"errors"
	"net/netip"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

var magic = [4]byte{'W', 'B', 'R', 'I'}

const (
	Version1 = byte(1)
	Version2 = byte(2)
	MetaLen  = 8
	TunnelMetaLen = 4 + 1 + logicaltunnel.TunnelIDBytes + 4
)

type SessionMeta struct {
	SID string
}

type TunnelMeta struct {
	TunnelID logicaltunnel.TunnelID
	Address4 netip.Addr
}

func MarshalSessionMeta(sid string) ([]byte, error) {
	b, err := hex.DecodeString(sid)
	if err != nil || len(b) != 3 {
		return nil, errors.New("rawipbackend: sid must be six hexadecimal characters")
	}
	out := make([]byte, MetaLen)
	copy(out[:4], magic[:])
	out[4] = Version1
	copy(out[5:8], b)
	return out, nil
}

func UnmarshalSessionMeta(p []byte) (SessionMeta, bool) {
	if len(p) != MetaLen || p[0] != magic[0] || p[1] != magic[1] || p[2] != magic[2] || p[3] != magic[3] || p[4] != Version1 {
		return SessionMeta{}, false
	}
	return SessionMeta{SID: hex.EncodeToString(p[5:8])}, true
}

func MarshalTunnelMeta(tunnelID logicaltunnel.TunnelID, address4 netip.Addr) ([]byte, error) {
	id, err := logicaltunnel.ParseTunnelID(string(tunnelID))
	if err != nil || !address4.IsValid() || !address4.Is4() {
		return nil, errors.New("rawipbackend: invalid logical tunnel metadata")
	}
	idRaw, _ := hex.DecodeString(string(id))
	addr := address4.As4()
	out := make([]byte, TunnelMetaLen)
	copy(out[:4], magic[:])
	out[4] = Version2
	copy(out[5:5+logicaltunnel.TunnelIDBytes], idRaw)
	copy(out[5+logicaltunnel.TunnelIDBytes:], addr[:])
	return out, nil
}

func UnmarshalTunnelMeta(p []byte) (TunnelMeta, bool) {
	if len(p) != TunnelMetaLen || p[0] != magic[0] || p[1] != magic[1] || p[2] != magic[2] || p[3] != magic[3] || p[4] != Version2 {
		return TunnelMeta{}, false
	}
	id, err := logicaltunnel.ParseTunnelID(hex.EncodeToString(p[5 : 5+logicaltunnel.TunnelIDBytes]))
	if err != nil {
		return TunnelMeta{}, false
	}
	var raw [4]byte
	copy(raw[:], p[5+logicaltunnel.TunnelIDBytes:])
	return TunnelMeta{TunnelID: id, Address4: netip.AddrFrom4(raw)}, true
}

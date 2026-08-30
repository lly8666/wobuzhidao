package rawipbackend

import (
	"encoding/hex"
	"errors"
)

var magic = [4]byte{'W', 'B', 'R', 'I'}

const (
	Version1 = byte(1)
	MetaLen  = 8
)

type SessionMeta struct {
	SID string
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

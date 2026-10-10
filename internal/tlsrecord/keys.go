package tlsrecord

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	ExporterLabel   = "EXPORTER-WBD-TLSLIKE-V1"
	c2sInfo         = "WBD-TLSLIKE-V1/c2s"
	s2cInfo         = "WBD-TLSLIKE-V1/s2c"
	masterLen       = 32
	keyMaterialLen  = 76
)

var (
	ErrInvalidMasterLength = errors.New("tlsrecord: exporter master must be 32 bytes")
	ErrTunnelIDTooLong     = errors.New("tlsrecord: tunnel id exceeds uint16 length")
)

// Keys are the per-direction materials defined by WIRE_SPEC.
type Keys struct {
	AEADKey [32]byte
	IV      [12]byte
	HPKey   [32]byte
}

// KeyPair contains independent client-to-server and server-to-client keys.
type KeyPair struct {
	C2S Keys
	S2C Keys
}

// ExporterContextHash encodes the negotiated parameters exactly as WIRE_SPEC
// specifies, then returns SHA-256(encoded_context) for TLS exporter use.
func ExporterContextHash(version uint16, incarnationNonce [16]byte, tunnelID []byte, clientLimit, serverLimit uint16) ([32]byte, error) {
	return exporterContextHash(version, incarnationNonce, tunnelID, clientLimit, serverLimit, nil)
}

// V3 adds the exact canonical policy to the exporter; V2 bytes are unchanged.
func ExporterContextHashV3(version uint16, nonce [16]byte, tunnelID []byte, clientLimit, serverLimit uint16, policy []byte) ([32]byte, error) {
	if version != 3 || len(policy) != 10 { return [32]byte{}, errors.New("tlsrecord: invalid V3 exporter policy") }
	return exporterContextHash(version, nonce, tunnelID, clientLimit, serverLimit, policy)
}
func exporterContextHash(version uint16, incarnationNonce [16]byte, tunnelID []byte, clientLimit, serverLimit uint16, policy []byte) ([32]byte, error) {
	if len(tunnelID) > 0xffff {
		return [32]byte{}, ErrTunnelIDTooLong
	}
	raw := make([]byte, 0, 2+16+2+len(tunnelID)+2+2)
	var two [2]byte

	binary.BigEndian.PutUint16(two[:], version)
	raw = append(raw, two[:]...)
	raw = append(raw, incarnationNonce[:]...)
	binary.BigEndian.PutUint16(two[:], uint16(len(tunnelID)))
	raw = append(raw, two[:]...)
	raw = append(raw, tunnelID...)
	binary.BigEndian.PutUint16(two[:], clientLimit)
	raw = append(raw, two[:]...)
	binary.BigEndian.PutUint16(two[:], serverLimit)
	raw = append(raw, two[:]...)
	raw = append(raw, policy...)
	return sha256.Sum256(raw), nil
}

// DeriveKeys expands the 32-byte TLS exporter master into direction-separated
// AEAD key, nonce IV, and header-protection key material.
func DeriveKeys(master []byte, incarnationNonce [16]byte) (KeyPair, error) {
	if len(master) != masterLen {
		return KeyPair{}, ErrInvalidMasterLength
	}
	c2s, err := deriveDirection(master, incarnationNonce, c2sInfo)
	if err != nil {
		return KeyPair{}, err
	}
	s2c, err := deriveDirection(master, incarnationNonce, s2cInfo)
	if err != nil {
		return KeyPair{}, err
	}
	return KeyPair{C2S: c2s, S2C: s2c}, nil
}

func deriveDirection(master []byte, incarnationNonce [16]byte, info string) (Keys, error) {
	r := hkdf.New(sha256.New, master, incarnationNonce[:], []byte(info))
	var material [keyMaterialLen]byte
	if _, err := io.ReadFull(r, material[:]); err != nil {
		return Keys{}, err
	}
	var out Keys
	copy(out.AEADKey[:], material[0:32])
	copy(out.IV[:], material[32:44])
	copy(out.HPKey[:], material[44:76])
	return out, nil
}

// V3 HKDF domain-separates suite and direction. AES-128 uses the first 16
// bytes of the independently derived 32-byte AEAD key once N3 is enabled.
func DeriveKeysV3(master []byte, nonce [16]byte, cipher uint8) (KeyPair, error) {
    if len(master) != masterLen { return KeyPair{}, ErrInvalidMasterLength }
    var suite string
    switch cipher {
    case 1: suite = "chacha20-poly1305"
    case 2: suite = "aes-128-gcm"
    case 3: suite = "aes-256-gcm"
    default: return KeyPair{}, errors.New("tlsrecord: invalid V3 cipher")
    }
    c2s,err:=deriveDirection(master,nonce,"WBD-TLSLIKE-V3/"+suite+"/c2s")
    if err!=nil{return KeyPair{},err}
    s2c,err:=deriveDirection(master,nonce,"WBD-TLSLIKE-V3/"+suite+"/s2c")
    if err!=nil{return KeyPair{},err}
    return KeyPair{C2S:c2s,S2C:s2c},nil
}

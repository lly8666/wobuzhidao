package tlsrecord

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	OuterType         byte   = 0x17
	OuterVersion      uint16 = 0x0303
	KindLINK          byte   = 0x00
	InnerType         byte   = 0x17
	OuterHeaderLen           = 5
	ProtectedPNLen            = 8
	AEADTagLen                = 16
	PlainFixedLen             = 2
	MinBodyLen                = ProtectedPNLen + AEADTagLen + PlainFixedLen
	FixedWireOverhead         = OuterHeaderLen + MinBodyLen
	MaxBodyLen                = (1 << 14) + 256
	MaxWireLen                = OuterHeaderLen + MaxBodyLen
)

var (
	ErrInvalidWireLimit        = errors.New("tlsrecord: wire limit is too small")
	ErrPayloadTooLarge         = errors.New("tlsrecord: payload exceeds negotiated record limit")
	ErrInvalidPadding          = errors.New("tlsrecord: invalid padding length")
	ErrPaddingTooLarge         = errors.New("tlsrecord: padding exceeds record headroom")
	ErrPNExhausted             = errors.New("tlsrecord: packet number exhausted")
	ErrInvalidLength           = errors.New("tlsrecord: invalid record length")
	ErrUnexpectedOuterType     = errors.New("tlsrecord: unexpected outer type")
	ErrUnexpectedOuterVersion  = errors.New("tlsrecord: unexpected outer version")
	ErrAuthentication          = errors.New("tlsrecord: authentication failed")
	ErrMalformedPlaintext      = errors.New("tlsrecord: malformed plaintext")
	ErrUnknownKind             = errors.New("tlsrecord: unknown record kind")
	ErrUnexpectedInnerType     = errors.New("tlsrecord: unexpected inner type")
	ErrDuplicate               = errors.New("tlsrecord: duplicate recent packet number")
)

type Sealer struct {
	keys      Keys
	aead      cipher.AEAD
	maxBody   int
	nextPN    uint64
	exhausted bool
	stats     SealerStats
}

type SealerStats struct {
	Records               uint64
	Failed                uint64
	PaddingRequests       uint64
	RequestedPaddingBytes uint64
	PaddedRecords         uint64
	PaddingBytes          uint64
}

type Opener struct {
	keys    Keys
	aead    cipher.AEAD
	maxBody int
}

type Record struct {
	PN      uint64
	Payload []byte
}

func NewSealer(keys Keys, maxWire int) (*Sealer, error) {
	return newSealerAtPN(keys, maxWire, 0)
}

func newSealerAtPN(keys Keys, maxWire int, nextPN uint64) (*Sealer, error) {
	maxBody, err := effectiveBodyLimit(maxWire)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(keys.AEADKey[:])
	if err != nil {
		return nil, err
	}
	return &Sealer{keys: keys, aead: aead, maxBody: maxBody, nextPN: nextPN}, nil
}

func NewOpener(keys Keys, maxWire int) (*Opener, error) {
	maxBody, err := effectiveBodyLimit(maxWire)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(keys.AEADKey[:])
	if err != nil {
		return nil, err
	}
	return &Opener{keys: keys, aead: aead, maxBody: maxBody}, nil
}

// Seal reserves exactly one new PN before encoding and preserves the V1
// default padding=0 behavior byte-for-byte. If encoding fails, that PN is
// intentionally skipped and is never reused.
func (s *Sealer) Seal(payload []byte) ([]byte, uint64, error) {
	return s.seal(payload, 0, false)
}

// SealWithPadding is the explicit non-default padding API. Padding bytes are
// encrypted zero bytes after inner_type; they do not change PN, kind, version
// or payload bytes. Negative padding and requests beyond this sealer's wire
// headroom are rejected explicitly.
func (s *Sealer) SealWithPadding(payload []byte, padding int) ([]byte, uint64, error) {
	return s.seal(payload, padding, true)
}

func (s *Sealer) seal(payload []byte, padding int, explicitPadding bool) ([]byte, uint64, error) {
	if s.exhausted {
		return nil, 0, ErrPNExhausted
	}
	pn := s.nextPN
	if pn == ^uint64(0) {
		s.exhausted = true
	} else {
		s.nextPN++
	}
	if explicitPadding {
		s.stats.PaddingRequests++
		if padding > 0 {
			s.stats.RequestedPaddingBytes += uint64(padding)
		}
	}
	wire, err := sealRecord(s.keys, s.aead, s.maxBody, pn, KindLINK, payload, padding)
	if err != nil {
		s.stats.Failed++
		return nil, pn, err
	}
	s.stats.Records++
	if padding > 0 {
		s.stats.PaddedRecords++
		s.stats.PaddingBytes += uint64(padding)
	}
	return wire, pn, nil
}

func (s *Sealer) Stats() SealerStats {
	return s.stats
}

func (o *Opener) OpenRecord(wire []byte) (Record, error) {
	if len(wire) < OuterHeaderLen {
		return Record{}, ErrInvalidLength
	}
	bodyLen := int(binary.BigEndian.Uint16(wire[3:5]))
	if bodyLen < MinBodyLen || bodyLen > o.maxBody || len(wire) != OuterHeaderLen+bodyLen {
		return Record{}, ErrInvalidLength
	}
	if wire[0] != OuterType {
		return Record{}, ErrUnexpectedOuterType
	}
	if binary.BigEndian.Uint16(wire[1:3]) != OuterVersion {
		return Record{}, ErrUnexpectedOuterVersion
	}

	ciphertext := wire[OuterHeaderLen+ProtectedPNLen:]
	mask, err := headerMask(o.keys.HPKey, ciphertext)
	if err != nil {
		return Record{}, ErrInvalidLength
	}
	var pnBytes [8]byte
	copy(pnBytes[:], wire[OuterHeaderLen:OuterHeaderLen+ProtectedPNLen])
	for i := range pnBytes {
		pnBytes[i] ^= mask[i]
	}
	pn := binary.BigEndian.Uint64(pnBytes[:])

	var aad [OuterHeaderLen + ProtectedPNLen]byte
	copy(aad[:OuterHeaderLen], wire[:OuterHeaderLen])
	binary.BigEndian.PutUint64(aad[OuterHeaderLen:], pn)
	nonce := recordNonce(o.keys.IV, pn)
	plain, err := o.aead.Open(nil, nonce[:], ciphertext, aad[:])
	if err != nil {
		return Record{}, ErrAuthentication
	}
	if len(plain) < PlainFixedLen {
		return Record{}, ErrMalformedPlaintext
	}

	end := len(plain) - 1
	for end >= 1 && plain[end] == 0 {
		end--
	}
	if end < 1 || plain[end] != InnerType {
		return Record{}, ErrUnexpectedInnerType
	}
	if plain[0] != KindLINK {
		return Record{}, ErrUnknownKind
	}
	payload := append([]byte(nil), plain[1:end]...)
	return Record{PN: pn, Payload: payload}, nil
}

func effectiveBodyLimit(maxWire int) (int, error) {
	if maxWire < FixedWireOverhead {
		return 0, ErrInvalidWireLimit
	}
	body := maxWire - OuterHeaderLen
	if body > MaxBodyLen {
		body = MaxBodyLen
	}
	if body < MinBodyLen {
		return 0, ErrInvalidWireLimit
	}
	return body, nil
}

func sealRecord(keys Keys, aead cipher.AEAD, maxBody int, pn uint64, kind byte, payload []byte, padding int) ([]byte, error) {
	if padding < 0 {
		return nil, ErrInvalidPadding
	}
	maxPayloadAndPadding := maxBody - ProtectedPNLen - aead.Overhead() - PlainFixedLen
	if maxPayloadAndPadding < 0 || len(payload) > maxPayloadAndPadding {
		return nil, ErrPayloadTooLarge
	}
	if padding > maxPayloadAndPadding-len(payload) {
		return nil, ErrPaddingTooLarge
	}
	plainLen := PlainFixedLen + len(payload) + padding
	bodyLen := ProtectedPNLen + plainLen + aead.Overhead()
	if bodyLen > maxBody || bodyLen > MaxBodyLen {
		return nil, ErrPayloadTooLarge
	}

	var header [OuterHeaderLen]byte
	header[0] = OuterType
	binary.BigEndian.PutUint16(header[1:3], OuterVersion)
	binary.BigEndian.PutUint16(header[3:5], uint16(bodyLen))

	plain := make([]byte, plainLen)
	plain[0] = kind
	copy(plain[1:], payload)
	plain[1+len(payload)] = InnerType

	var aad [OuterHeaderLen + ProtectedPNLen]byte
	copy(aad[:OuterHeaderLen], header[:])
	binary.BigEndian.PutUint64(aad[OuterHeaderLen:], pn)
	nonce := recordNonce(keys.IV, pn)
	ciphertext := aead.Seal(nil, nonce[:], plain, aad[:])

	mask, err := headerMask(keys.HPKey, ciphertext)
	if err != nil {
		return nil, err
	}
	var protectedPN [8]byte
	binary.BigEndian.PutUint64(protectedPN[:], pn)
	for i := range protectedPN {
		protectedPN[i] ^= mask[i]
	}

	wire := make([]byte, OuterHeaderLen+ProtectedPNLen+len(ciphertext))
	copy(wire[:OuterHeaderLen], header[:])
	copy(wire[OuterHeaderLen:OuterHeaderLen+ProtectedPNLen], protectedPN[:])
	copy(wire[OuterHeaderLen+ProtectedPNLen:], ciphertext)
	return wire, nil
}

func recordNonce(iv [12]byte, pn uint64) [12]byte {
	nonce := iv
	var pnBytes [8]byte
	binary.BigEndian.PutUint64(pnBytes[:], pn)
	for i := 0; i < len(pnBytes); i++ {
		nonce[4+i] ^= pnBytes[i]
	}
	return nonce
}

func headerMask(key [32]byte, ciphertext []byte) ([8]byte, error) {
	if len(ciphertext) < 16 {
		return [8]byte{}, ErrInvalidLength
	}
	counter := binary.LittleEndian.Uint32(ciphertext[0:4])
	stream, err := chacha20.NewUnauthenticatedCipher(key[:], ciphertext[4:16])
	if err != nil {
		return [8]byte{}, err
	}
	stream.SetCounter(counter)
	var mask [8]byte
	var zeros [8]byte
	stream.XORKeyStream(mask[:], zeros[:])
	return mask, nil
}

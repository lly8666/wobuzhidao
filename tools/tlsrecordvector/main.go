// Command tlsrecordvector independently calculates WIRE_SPEC V1 vectors.
// It deliberately does not import internal/tlsrecord.
package main

import (
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

type direction struct {
	aead [32]byte
	iv   [12]byte
	hp   [32]byte
}

func main() {
	var nonce [16]byte
	var master [32]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}
	for i := range master {
		master[i] = byte(i)
	}
	tunnelID := []byte("tunnel-vector-01")
	context := encodeContext(1, nonce, tunnelID, 1500, 1600)
	contextHash := sha256.Sum256(context)
	c2s := derive(master[:], nonce, "WBD-TLSLIKE-V1/c2s")
	s2c := derive(master[:], nonce, "WBD-TLSLIKE-V1/s2c")

	records := map[string]string{
		"pn0_empty":               hex.EncodeToString(seal(c2s, 0, nil)),
		"pn1_ascii":               hex.EncodeToString(seal(c2s, 1, []byte("hello"))),
		"pn_2pow32_plus1_binary":  hex.EncodeToString(seal(c2s, (1<<32)+1, []byte{0x00, 0xff, 0x10, 0x20})),
		"pn_max_minus1":           hex.EncodeToString(seal(c2s, ^uint64(0)-1, []byte("near-max"))),
		"pn_max":                  hex.EncodeToString(seal(c2s, ^uint64(0), nil)),
	}
	out := map[string]any{
		"source":        "docs/WIRE_SPEC.md; independent primitive-level Go generator",
		"version":       1,
		"tunnel_id_hex": hex.EncodeToString(tunnelID),
		"client_limit":  1500,
		"server_limit":  1600,
		"nonce_hex":     hex.EncodeToString(nonce[:]),
		"master_hex":    hex.EncodeToString(master[:]),
		"context_hash":  hex.EncodeToString(contextHash[:]),
		"c2s_aead":      hex.EncodeToString(c2s.aead[:]),
		"c2s_iv":        hex.EncodeToString(c2s.iv[:]),
		"c2s_hp":        hex.EncodeToString(c2s.hp[:]),
		"s2c_aead":      hex.EncodeToString(s2c.aead[:]),
		"s2c_iv":        hex.EncodeToString(s2c.iv[:]),
		"s2c_hp":        hex.EncodeToString(s2c.hp[:]),
		"records":       records,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
}

func encodeContext(version uint16, nonce [16]byte, tunnelID []byte, clientLimit, serverLimit uint16) []byte {
	out := make([]byte, 0, 2+16+2+len(tunnelID)+2+2)
	var two [2]byte
	binary.BigEndian.PutUint16(two[:], version)
	out = append(out, two[:]...)
	out = append(out, nonce[:]...)
	binary.BigEndian.PutUint16(two[:], uint16(len(tunnelID)))
	out = append(out, two[:]...)
	out = append(out, tunnelID...)
	binary.BigEndian.PutUint16(two[:], clientLimit)
	out = append(out, two[:]...)
	binary.BigEndian.PutUint16(two[:], serverLimit)
	out = append(out, two[:]...)
	return out
}

func derive(master []byte, nonce [16]byte, info string) direction {
	r := hkdf.New(sha256.New, master, nonce[:], []byte(info))
	var raw [76]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		panic(err)
	}
	var d direction
	copy(d.aead[:], raw[:32])
	copy(d.iv[:], raw[32:44])
	copy(d.hp[:], raw[44:76])
	return d
}

func seal(keys direction, pn uint64, payload []byte) []byte {
	aead, err := chacha20poly1305.New(keys.aead[:])
	if err != nil {
		panic(err)
	}
	return sealWithAEAD(keys, aead, pn, payload)
}

func sealWithAEAD(keys direction, aead cipher.AEAD, pn uint64, payload []byte) []byte {
	plain := make([]byte, 2+len(payload))
	plain[0] = 0
	copy(plain[1:], payload)
	plain[len(plain)-1] = 0x17

	bodyLen := 8 + len(plain) + aead.Overhead()
	var header [5]byte
	header[0] = 0x17
	binary.BigEndian.PutUint16(header[1:3], 0x0303)
	binary.BigEndian.PutUint16(header[3:5], uint16(bodyLen))

	var aad [13]byte
	copy(aad[:5], header[:])
	binary.BigEndian.PutUint64(aad[5:], pn)
	nonce := keys.iv
	var pnBytes [8]byte
	binary.BigEndian.PutUint64(pnBytes[:], pn)
	for i := range pnBytes {
		nonce[4+i] ^= pnBytes[i]
	}
	ciphertext := aead.Seal(nil, nonce[:], plain, aad[:])
	if len(ciphertext) < 16 {
		panic("ciphertext too short")
	}

	counter := binary.LittleEndian.Uint32(ciphertext[:4])
	hp, err := chacha20.NewUnauthenticatedCipher(keys.hp[:], ciphertext[4:16])
	if err != nil {
		panic(err)
	}
	hp.SetCounter(counter)
	var mask [8]byte
	var zero [8]byte
	hp.XORKeyStream(mask[:], zero[:])
	for i := range pnBytes {
		pnBytes[i] ^= mask[i]
	}

	wire := make([]byte, 13+len(ciphertext))
	copy(wire[:5], header[:])
	copy(wire[5:13], pnBytes[:])
	copy(wire[13:], ciphertext)
	return wire
}

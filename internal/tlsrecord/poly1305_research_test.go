package tlsrecord

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"golang.org/x/crypto/poly1305"
)

// Research-only packaging experiment for ARM64. The production AEAD, key,
// nonce, and authenticated wire layout are not changed. In particular, a
// prebuilt contiguous input is an unattainable zero-copy upper bound, not an
// implementation candidate. Benchmark keys/messages are synthetic.
var poly1305ResearchSink [16]byte

func poly1305ResearchInput(aad, ciphertext []byte) []byte {
	round16 := func(n int) int { return (n + 15) &^ 15 }
	aadPadded := round16(len(aad))
	cipherPadded := round16(len(ciphertext))
	message := make([]byte, aadPadded+cipherPadded+16)
	copy(message, aad)
	copy(message[aadPadded:], ciphertext)
	binary.LittleEndian.PutUint64(message[aadPadded+cipherPadded:], uint64(len(aad)))
	binary.LittleEndian.PutUint64(message[aadPadded+cipherPadded+8:], uint64(len(ciphertext)))
	return message
}

func poly1305ResearchStream(key *[32]byte, aad, ciphertext []byte) [16]byte {
	p := poly1305.New(key)
	_, _ = p.Write(aad)
	if rem := len(aad) & 15; rem != 0 {
		var zero [16]byte
		_, _ = p.Write(zero[:16-rem])
	}
	_, _ = p.Write(ciphertext)
	if rem := len(ciphertext) & 15; rem != 0 {
		var zero [16]byte
		_, _ = p.Write(zero[:16-rem])
	}
	var lens [16]byte
	binary.LittleEndian.PutUint64(lens[:8], uint64(len(aad)))
	binary.LittleEndian.PutUint64(lens[8:], uint64(len(ciphertext)))
	_, _ = p.Write(lens[:])
	var tag [16]byte
	p.Sum(tag[:0])
	return tag
}

func TestPoly1305PackagingOracle(t *testing.T) {
	// These lengths cover empty input, padding boundaries, real small packets,
	// near-MTU packets, and the maximum ordinary datagram edge.
	lengths := []int{0, 1, 15, 16, 17, 31, 32, 63, 64, 96, 128, 256, 512, 1200, 1400, 1460, 65535}
	aadLengths := []int{0, 1, 13, 16, 17, 32}
	for seed := 0; seed < 4; seed++ {
		var key [32]byte
		for i := range key { key[i] = byte((i*37 + seed*29 + 11) & 255) }
		for _, n := range lengths {
			message := make([]byte, n)
			for i := range message { message[i] = byte((i*31 + seed*23 + n) & 255) }
			for _, m := range aadLengths {
				aad := make([]byte, m)
				for i := range aad { aad[i] = byte((i*17 + seed*13 + m) & 255) }
				packed := poly1305ResearchInput(aad, message)
				tag := poly1305ResearchStream(&key, aad, message)
				var independent [16]byte
				poly1305.Sum(&independent, packed, &key)
				if !bytes.Equal(tag[:], independent[:]) {
					t.Fatalf("packaging mismatch seed=%d aad=%d payload=%d: %x != %x",
						seed, m, n, tag, independent)
				}
				if len(message) > 0 {
					packed[len(packed)-17] ^= 0x01 // Last ciphertext or padding byte; nonempty MAC input.
					var tampered [16]byte
					poly1305.Sum(&tampered, packed, &key)
					if bytes.Equal(tag[:], tampered[:]) {
						t.Fatalf("tampered MAC unchanged seed=%d aad=%d payload=%d", seed, m, n)
					}
				}
			}
		}
	}
}

// The streaming case is the MAC feeding pattern used by x/crypto's generic
// ChaCha20-Poly1305 AEAD. The contiguous case includes its copy cost into an
// already allocated scratch buffer. The prebuilt case excludes that copy,
// and is diagnostic ceiling only: do not call it a shippable speedup.
func BenchmarkPoly1305PackagingResearch(b *testing.B) {
	for _, n := range []int{64, 128, 256, 512, 1200, 1400} {
		var key [32]byte
		for i := range key { key[i] = byte(i*19 + 7) }
		aad := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
		data := make([]byte, n)
		for i := range data { data[i] = byte(i*29 + 5) }
		packed := poly1305ResearchInput(aad, data)
		cipherOffset := 16 // 13-byte AAD aligned to one Poly1305 block.
		b.Run(fmt.Sprintf("%d/stream", n), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(n))
			for i := 0; i < b.N; i++ {
				poly1305ResearchSink = poly1305ResearchStream(&key, aad, data)
			}
		})
		b.Run(fmt.Sprintf("%d/contiguous_copy", n), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(n))
			var tag [16]byte
			for i := 0; i < b.N; i++ {
				copy(packed[:len(aad)], aad)
				copy(packed[cipherOffset:cipherOffset+n], data)
				poly1305.Sum(&tag, packed, &key)
				poly1305ResearchSink = tag
			}
		})
		b.Run(fmt.Sprintf("%d/prebuilt_upper_bound", n), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(n))
			var tag [16]byte
			for i := 0; i < b.N; i++ {
				poly1305.Sum(&tag, packed, &key)
				poly1305ResearchSink = tag
			}
		})
	}
}

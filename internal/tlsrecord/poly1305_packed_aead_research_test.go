package tlsrecord

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/poly1305"
)

// Test-only full AEAD packaging proof of concept. This is NOT production
// encryption code, does NOT support the full cipher.AEAD aliasing contract,
// and is never selected by NewSealer/NewOpener. Its scratch buffer must be
// caller-owned; the benchmark excludes scratch allocation intentionally.
func poly1305ResearchSealPacked(key *[32]byte, nonce, plaintext, aad, dst, scratch []byte) []byte {
	s, err := chacha20.NewUnauthenticatedCipher(key[:], nonce)
	if err != nil { panic(err) }
	var polyKey [32]byte
	s.XORKeyStream(polyKey[:], polyKey[:])
	s.SetCounter(1)

	n := len(plaintext)
	if len(dst) < n+16 { panic("research: output too short") }
	ciphertext := dst[:n]
	s.XORKeyStream(ciphertext, plaintext)

	aadRound := (len(aad) + 15) &^ 15
	cipherRound := (n + 15) &^ 15
	total := aadRound + cipherRound + 16
	if len(scratch) < total { panic("research: scratch too short") }
	input := scratch[:total]
	copy(input, aad)
	clear(input[len(aad):aadRound])
	copy(input[aadRound:], ciphertext)
	clear(input[aadRound+n:aadRound+cipherRound])
	binary.LittleEndian.PutUint64(input[total-16:], uint64(len(aad)))
	binary.LittleEndian.PutUint64(input[total-8:], uint64(n))

	var tag [16]byte
	poly1305.Sum(&tag, input, &polyKey)
	copy(dst[n:], tag[:])
	return dst[:n+16]
}

func TestPoly1305ResearchPackedAEADReference(t *testing.T) {
	for seed := 0; seed < 4; seed++ {
		var key [32]byte
		for i := range key { key[i] = byte(i*17 + seed*23 + 13) }
		a, err := chacha20poly1305.New(key[:])
		if err != nil { t.Fatal(err) }
		for _, n := range []int{0,1,15,16,17,31,32,63,64,65,96,128,256,512,1200,1400,1460} {
			data := make([]byte,n)
			for i := range data { data[i] = byte(i*29 + seed*7 + n) }
			for _, aadN := range []int{0,1,13,16,17,32} {
				aad := make([]byte,aadN)
				for i := range aad { aad[i] = byte(i*31 + seed*11 + aadN) }
				nonce := make([]byte,12)
				binary.LittleEndian.PutUint32(nonce[:4],uint32(seed+1))
				binary.LittleEndian.PutUint32(nonce[4:8],uint32(n))
				binary.LittleEndian.PutUint32(nonce[8:],uint32(aadN))
				expected := a.Seal(nil,nonce,data,aad)
				scratch := make([]byte,((aadN+15)&^15)+((n+15)&^15)+16)
				got := poly1305ResearchSealPacked(&key,nonce,data,aad,make([]byte,n+16),scratch)
				if !bytes.Equal(got,expected) {
					t.Fatalf("packed AEAD mismatch seed=%d n=%d aad=%d",seed,n,aadN)
				}
				plain, err := a.Open(nil,nonce,got,aad)
				if err != nil || !bytes.Equal(plain,data) {
					t.Fatalf("reference Open disagrees seed=%d n=%d aad=%d err=%v",seed,n,aadN,err)
				}
				got[len(got)-1] ^= 0x80
				if _,err := a.Open(nil,nonce,got,aad); err == nil {
					t.Fatalf("tampered tag accepted seed=%d n=%d aad=%d",seed,n,aadN)
				}
			}
		}
	}
	// Reuse the exact same scratch for different lengths to expose missing
	// padding cleanup (a common correctness defect in pooled MAC buffers).
	var key [32]byte
	a, _ := chacha20poly1305.New(key[:])
	scratch := make([]byte,3000)
	for _, n := range []int{1400,15,1200,1,128,17,1400,0,65} {
		data := bytes.Repeat([]byte{byte(n)},n)
		aad := bytes.Repeat([]byte{byte(n>>2)},13)
		nonce := make([]byte,12)
		binary.LittleEndian.PutUint64(nonce[4:],uint64(n+1))
		got := poly1305ResearchSealPacked(&key,nonce,data,aad,make([]byte,n+16),scratch)
		want := a.Seal(nil,nonce,data,aad)
		if !bytes.Equal(got,want) { t.Fatalf("pooled scratch mismatch n=%d",n) }
	}
}

func BenchmarkPoly1305PackedAEADResearch(b *testing.B) {
	for _, n := range []int{64,128,256,512,1200,1400} {
		var key [32]byte
		for i := range key { key[i] = byte(i*19+7) }
		a,err := chacha20poly1305.New(key[:]);if err!=nil{b.Fatal(err)}
		aad := []byte{1,2,3,4,5,6,7,8,9,10,11,12,13}
		plain := make([]byte,n)
		for i:=range plain {plain[i]=byte(i*29+5)}
		out := make([]byte,n+16)
		scratch := make([]byte,16+((n+15)&^15)+16)
		var nonce [12]byte
		b.Run(fmt.Sprintf("%d/reference",n),func(b *testing.B){
			b.ReportAllocs(); b.SetBytes(int64(n))
			for i:=0;i<b.N;i++ {
				binary.LittleEndian.PutUint64(nonce[4:],uint64(i))
				result:=a.Seal(out[:0],nonce[:],plain,aad)
				poly1305ResearchSink[0]=result[len(result)-1]
			}
		})
		b.Run(fmt.Sprintf("%d/packed_experimental",n),func(b *testing.B){
			b.ReportAllocs(); b.SetBytes(int64(n))
			for i:=0;i<b.N;i++ {
				binary.LittleEndian.PutUint64(nonce[4:],uint64(i))
				result:=poly1305ResearchSealPacked(&key,nonce[:],plain,aad,out,scratch)
				poly1305ResearchSink[0]=result[len(result)-1]
			}
		})
	}
}

package tlsrecord

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"testing"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/chacha20poly1305"
)

// Deliberately TEST ONLY: measure what security properties cost, not a new
// protocol or a product implementation. No MAC/cleartext modes must ever be
// selected by NewSealer/NewOpener. This benchmark excludes framing, HP, FEC,
// syscall and startup; it cannot prove end-to-end performance.
var researchCryptoCostSink byte

func TestResearchCryptoCostVectors(t *testing.T) {
	var key [32]byte
	for i := range key { key[i] = byte(i*23 + 7) }
	var nonce [12]byte
	chachaAEAD, err := chacha20poly1305.New(key[:])
	if err != nil { t.Fatal(err) }
	aesBlock, err := aes.NewCipher(key[:])
	if err != nil { t.Fatal(err) }
	aesAEAD, err := cipher.NewGCM(aesBlock)
	if err != nil { t.Fatal(err) }
	if chachaAEAD.Overhead() != 16 || aesAEAD.Overhead() != 16 { t.Fatal("wrong authenticated tag length") }

	for _, n := range []int{0,1,15,16,17,63,64,65,128,256,512,1200,1400,1460} {
		for seed := 0; seed < 4; seed++ {
			plain := make([]byte, n)
			for i := range plain { plain[i] = byte(i*31 + seed*17 + n) }
			aad := []byte{1,2,3,4,5,6,7,8,9,10,11,12,13}
			binary.LittleEndian.PutUint32(nonce[:4],uint32(seed+1))
			binary.LittleEndian.PutUint64(nonce[4:],uint64(n+1))

			for _, variant := range []struct {
				name string
				aead cipher.AEAD
			}{{"chacha20poly1305",chachaAEAD},{"aes256gcm",aesAEAD}} {
				ciphertext := variant.aead.Seal(nil,nonce[:],plain,aad)
				recovered,err := variant.aead.Open(nil,nonce[:],ciphertext,aad)
				if err!=nil || !bytes.Equal(recovered,plain) {
					t.Fatalf("%s authenticated roundtrip n=%d seed=%d err=%v",variant.name,n,seed,err)
				}
				ciphertext[len(ciphertext)-1] ^= 1
				if _,err:=variant.aead.Open(nil,nonce[:],ciphertext,aad); err==nil {
					t.Fatalf("%s accepted tampered ciphertext n=%d seed=%d",variant.name,n,seed)
				}
			}

			s,err:=chacha20.NewUnauthenticatedCipher(key[:],nonce[:])
			if err!=nil { t.Fatal(err) }
			s.SetCounter(1) // AEAD's payload encryption counter; no Poly1305 derivation.
			encrypted := make([]byte,n)
			s.XORKeyStream(encrypted,plain)
			d,err:=chacha20.NewUnauthenticatedCipher(key[:],nonce[:])
			if err!=nil { t.Fatal(err) }
			d.SetCounter(1)
			recovered:=make([]byte,n)
			d.XORKeyStream(recovered,encrypted)
			if !bytes.Equal(recovered,plain) { t.Fatalf("unauthenticated roundtrip n=%d seed=%d",n,seed) }
			if n>0 {
				encrypted[0]^=0x40 // Demonstrate authenticity is GONE; not a product mode.
				d,err=chacha20.NewUnauthenticatedCipher(key[:],nonce[:])
				if err!=nil { t.Fatal(err) }
				d.SetCounter(1)
				d.XORKeyStream(recovered,encrypted)
				if recovered[0]!=plain[0]^0x40 {
					t.Fatalf("malleability demonstration mismatch n=%d seed=%d",n,seed)
				}
			}
		}
	}
}

func BenchmarkResearchSecurityFloor(b *testing.B) {
	var key [32]byte
	for i:=range key { key[i]=byte(i*19+7) }
	chaAEAD,err:=chacha20poly1305.New(key[:]);if err!=nil { b.Fatal(err) }
	aesBlock,err:=aes.NewCipher(key[:]);if err!=nil { b.Fatal(err) }
	gcm,err:=cipher.NewGCM(aesBlock);if err!=nil { b.Fatal(err) }
	aad:=[]byte{1,2,3,4,5,6,7,8,9,10,11,12,13}

	for _, n:=range []int{64,128,256,512,1200,1400} {
		plain:=make([]byte,n)
		for i:=range plain { plain[i]=byte(i*29+5) }
		out:=make([]byte,n+16)
		var nonce [12]byte
		for _, candidate:=range []struct {
			name string
			aead cipher.AEAD
		}{{"chacha20poly1305",chaAEAD},{"aes256gcm",gcm}} {
			b.Run(fmt.Sprintf("%d/%s",n,candidate.name),func(b *testing.B){
				b.ReportAllocs();b.SetBytes(int64(n))
				for i:=0;i<b.N;i++{
					binary.LittleEndian.PutUint64(nonce[4:],uint64(i))
					sealed:=candidate.aead.Seal(out[:0],nonce[:],plain,aad)
					researchCryptoCostSink=sealed[len(sealed)-1]
				}
			})
		}
		b.Run(fmt.Sprintf("%d/chacha20_only_NO_AUTH",n),func(b *testing.B){
			b.ReportAllocs();b.SetBytes(int64(n))
			for i:=0;i<b.N;i++{
				binary.LittleEndian.PutUint64(nonce[4:],uint64(i))
				s,err:=chacha20.NewUnauthenticatedCipher(key[:],nonce[:])
				if err!=nil{b.Fatal(err)}
				s.SetCounter(1)
				s.XORKeyStream(out[:n],plain)
				researchCryptoCostSink=out[n-1]
			}
		})
		b.Run(fmt.Sprintf("%d/copy_only_PLAINTEXT",n),func(b *testing.B){
			b.ReportAllocs();b.SetBytes(int64(n))
			for i:=0;i<b.N;i++{
				copy(out[:n],plain)
				researchCryptoCostSink=out[n-1]
			}
		})
	}
}

package tlsrecord

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

// Experiment-only: use the exact WBD record framing, PN mask, nonce, AAD,
// allocation and OpenRecord path while swapping cipher.AEAD. This is not a
// product-negotiated suite, not a released wire version and not fullstack.
var researchRecordSuiteSink byte

func researchRecordSuites(t testing.TB) []struct{
	name string
	aead cipher.AEAD
} {
	t.Helper()
	var key [32]byte
	for i := range key { key[i]=byte(i*17+11) }
	cc, err:=chacha20poly1305.New(key[:])
	if err!=nil { t.Fatal(err) }
	block, err:=aes.NewCipher(key[:])
	if err!=nil { t.Fatal(err) }
	ag, err:=cipher.NewGCM(block)
	if err!=nil { t.Fatal(err) }
	return []struct{
		name string
		aead cipher.AEAD
	}{{"chacha20poly1305",cc},{"aes256gcm",ag}}
}

func researchRecordKeys() Keys {
	var k Keys
	for i:=range k.AEADKey {k.AEADKey[i]=byte(i*17+11)}
	for i:=range k.IV {k.IV[i]=byte(i*23+7)}
	for i:=range k.HPKey {k.HPKey[i]=byte(i*29+3)}
	return k
}

func TestResearchRecordSuiteCompatibility(t *testing.T) {
	keys:=researchRecordKeys()
	suites:=researchRecordSuites(t)
	for _,suite:=range suites{
		t.Run(suite.name,func(t *testing.T){
			s:=&Sealer{keys:keys,aead:suite.aead,maxBody:MaxBodyLen}
			op:=&Opener{keys:keys,aead:suite.aead,maxBody:MaxBodyLen}
			for _,n:=range []int{0,1,15,16,17,63,64,65,128,256,512,1200,1400,1460}{
				data:=make([]byte,n)
				for i:=range data {data[i]=byte(i*31+n)}
				wire,pn,err:=s.Seal(data)
				if err!=nil{t.Fatalf("seal n=%d: %v",n,err)}
				if len(wire)!=FixedWireOverhead+n {t.Fatalf("wire overhead n=%d got=%d want=%d",n,len(wire),FixedWireOverhead+n)}
				if wire[0]!=OuterType || binary.BigEndian.Uint16(wire[1:3])!=OuterVersion {t.Fatalf("wrong envelope")}
				rec,err:=op.OpenRecord(wire)
				if err!=nil || rec.PN!=pn || rec.Kind!=KindLINK || !bytes.Equal(rec.Payload,data) {
					t.Fatalf("record suite=%s n=%d PN=%d err=%v",suite.name,n,pn,err)
				}
				wire[len(wire)-1]^=0x01
				if _,err=op.OpenRecord(wire);!errors.Is(err,ErrAuthentication) {
					t.Fatalf("authentication bypass n=%d got=%v",n,err)
				}
			}
			padPayload:=[]byte{3,2,1,0}
			wire,_,err:=s.SealWithPadding(padPayload,19)
			if err!=nil{t.Fatal(err)}
			padRec,err:=op.OpenRecord(wire)
			if err!=nil || !bytes.Equal(padRec.Payload,padPayload){t.Fatalf("padded record failed: %v",err)}
		})
	}
	// Suites are not interchangeable without a negotiated distinct version.
	a:=&Sealer{keys:keys,aead:suites[1].aead,maxBody:MaxBodyLen}
	op:=&Opener{keys:keys,aead:suites[0].aead,maxBody:MaxBodyLen}
	wire,_,err:=a.Seal([]byte("cross-suite must be rejected"))
	if err!=nil{t.Fatal(err)}
	if _,err=op.OpenRecord(wire);!errors.Is(err,ErrAuthentication) {
		t.Fatalf("cross suite unexpectedly accepted: %v",err)
	}
}

func BenchmarkResearchWBDRecordSuites(b *testing.B) {
	keys:=researchRecordKeys()
	for _,n:=range []int{64,128,256,512,1200,1400}{
		payload:=make([]byte,n)
		for i:=range payload {payload[i]=byte(i*29+7)}
		for _,suite:=range researchRecordSuites(b){
			b.Run(fmt.Sprintf("%d/%s/Seal",n,suite.name),func(b *testing.B){
				s:=&Sealer{keys:keys,aead:suite.aead,maxBody:MaxBodyLen}
				b.ReportAllocs();b.SetBytes(int64(n))
				for i:=0;i<b.N;i++ {
					wire,_,err:=s.Seal(payload)
					if err!=nil{b.Fatal(err)}
					researchRecordSuiteSink=wire[len(wire)-1]
				}
			})
			b.Run(fmt.Sprintf("%d/%s/Open",n,suite.name),func(b *testing.B){
				owner:=&Sealer{keys:keys,aead:suite.aead,maxBody:MaxBodyLen}
				op:=&Opener{keys:keys,aead:suite.aead,maxBody:MaxBodyLen}
				var wires [64][]byte
				for i:=range wires{
					w,_,err:=owner.Seal(payload)
					if err!=nil{b.Fatal(err)}
					wires[i]=w
				}
				b.ReportAllocs();b.SetBytes(int64(n))
				for i:=0;i<b.N;i++{
					r,err:=op.OpenRecord(wires[i&63])
					if err!=nil{b.Fatal(err)}
					researchRecordSuiteSink=r.Payload[len(r.Payload)-1]
				}
			})
		}
	}
}

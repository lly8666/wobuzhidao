package tlsrecord

import (
    "encoding/binary"
    "fmt"
    "testing"

    "golang.org/x/crypto/chacha20poly1305"
)

// Synthetic AEAD-only native ARM64 hotspot probe. No production crypto change.
// This is NOT a native end-to-end business sample or performance qualification.
// Real-size cases prevent measuring only large-file throughput.
var nativeAEADHotspotSink byte

func BenchmarkNativeAEADShortMTUHotspot(b *testing.B) {
    for _, size := range []int{64, 128, 256, 512, 1200, 1400} {
        b.Run(fmt.Sprintf("Seal/%d",size),func(b *testing.B) {
            key:=[32]byte{}
            for i:=range key {key[i]=byte(13*i+1)}
            a,err:=chacha20poly1305.New(key[:]);if err!=nil{b.Fatal(err)}
            plain:=make([]byte,size)
            out:=make([]byte,0,size+a.Overhead())
            nonce:=[12]byte{}
            aad:=[]byte{1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17}
            b.ReportAllocs();b.SetBytes(int64(size));b.ResetTimer()
            for i:=0;i<b.N;i++ {
                binary.LittleEndian.PutUint64(nonce[4:],uint64(i))
                sealed:=a.Seal(out[:0],nonce[:],plain,aad)
                nativeAEADHotspotSink=sealed[len(sealed)-1]
            }
        })
        b.Run(fmt.Sprintf("Open/%d",size),func(b *testing.B) {
            key:=[32]byte{}
            for i:=range key {key[i]=byte(13*i+1)}
            a,err:=chacha20poly1305.New(key[:]);if err!=nil{b.Fatal(err)}
            plain:=make([]byte,size)
            out:=make([]byte,size)
            aad:=[]byte{1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17}
            var nonces [64][12]byte
            var records [64][]byte
            for i:=0;i<64;i++ {
                binary.LittleEndian.PutUint64(nonces[i][4:],uint64(i+1))
                records[i]=a.Seal(make([]byte,0,size+a.Overhead()),nonces[i][:],plain,aad)
            }
            b.ReportAllocs();b.SetBytes(int64(size));b.ResetTimer()
            for i:=0;i<b.N;i++ {
                j:=i&63
                opened,err:=a.Open(out[:0],nonces[j][:],records[j],aad)
                if err!=nil{b.Fatal(err)}
                nativeAEADHotspotSink=opened[size-1]
            }
        })
    }
}

package faketcp

import (
    "fmt"
    "math/rand"
    "testing"
)

var tcpChecksumHotspotSink uint16

// Independent byte-pair and uint64 oracle: no production helper calls,
// uint32 overflow assumptions or SIMD/CRC32 shortcuts.
func tcpChecksumByteOracle(src,dst [4]byte, tcp []byte) uint16 {
    sum:=uint64(6)+uint64(len(tcp))
    for _,bytes:=range [][]byte{src[:],dst[:],tcp} {
        for i:=0;i<len(bytes);i+=2 {
            word:=uint64(bytes[i])<<8
            if i+1<len(bytes) {word|=uint64(bytes[i+1])}
            sum+=word
        }
    }
    for sum>>16!=0 {sum=(sum&65535)+(sum>>16)}
    return ^uint16(sum)
}

func TestTCPChecksumIndependentOracleExtended(t *testing.T) {
    src,dst:=[4]byte{192,0,2,17},[4]byte{198,51,100,99}
    rng:=rand.New(rand.NewSource(20261010))
    lengths:=[]int{0,1,2,3,4,5,7,8,9,10,19,20,21,31,32,33,39,40,41,
        63,64,65,95,96,97,127,128,129,255,256,257,511,512,513,
        1199,1200,1201,1399,1400,1401,1460,1499,1500,1501,4095,4096,4097,
        65534,65535}
    for _,n:=range lengths {
        for _,offset:=range []int{0,1,3} {
            for fill:=0;fill<4;fill++ {
                raw:=make([]byte,n+offset+4)
                tcp:=raw[offset:offset+n]
                switch fill {
                case 1: for i:=range tcp {tcp[i]=255}
                case 2: for i:=range tcp {tcp[i]=byte(i*17+3)}
                case 3: if _,err:=rng.Read(tcp);err!=nil{t.Fatal(err)}
                }
                if n>=20 && fill==3 {
                    tcp[12]=0x50 // TCP min 20-byte data offset
                    tcp[13]=0x11 // ACK+FIN
                }
                want:=tcpChecksumByteOracle(src,dst,tcp)
                got:=tcpChecksum(src,dst,tcp)
                if got!=want {t.Fatalf("len=%d offset=%d fill=%d got=%04x want=%04x",n,offset,fill,got,want)}
            }
        }
    }
    // TCP options, SACK, ACK/FIN and retransmit same immutable wire payload.
    tcp:=make([]byte,20+24+1372)
    tcp[12]=0xb0;tcp[13]=0x11
    copy(tcp[20:44],[]byte{1,1,5,18,0,1,2,3,0,1,2,4,0,1,2,5,0,1,2,6,1,1,1,1})
    for i:=44;i<len(tcp);i++{tcp[i]=byte(i*11)}
    initial:=tcpChecksum(src,dst,tcp)
    for i:=0;i<10;i++{
        if tcpChecksum(src,dst,tcp)!=initial {t.Fatal("immutable retransmit checksum changed")}
    }
    tcp[13]=0x10 // ACK, options/payload unchanged
    if got,want:=tcpChecksum(src,dst,tcp),tcpChecksumByteOracle(src,dst,tcp);got!=want{t.Fatalf("ACK update: %x %x",got,want)}
    tcp[32]^=0x7f // SACK block update
    if got,want:=tcpChecksum(src,dst,tcp),tcpChecksumByteOracle(src,dst,tcp);got!=want{t.Fatalf("SACK update: %x %x",got,want)}
}

func BenchmarkTCPChecksumCurrent(b *testing.B) {
    src,dst:=[4]byte{192,0,2,17},[4]byte{198,51,100,99}
    for _,n:=range []int{40,96,128,256,512,1200,1400,1460} {
        b.Run(fmt.Sprintf("bytes_%d",n),func(b *testing.B){
            tcp:=make([]byte,n)
            for i:=range tcp {tcp[i]=byte(17*i+3)}
            b.ReportAllocs();b.SetBytes(int64(n))
            b.ResetTimer()
            for i:=0;i<b.N;i++ {tcpChecksumHotspotSink=tcpChecksum(src,dst,tcp)}
        })
    }
}

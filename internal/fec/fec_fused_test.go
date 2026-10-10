package fec

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestFECFusedCustomMatrixMatchesLegacyAllProfiles(t *testing.T) {
	fast := NewFastReedSolomon20x20()
	old := NewReedSolomon20x20()
	for _, parity := range SupportedParityShards() {
		for _, n := range []int{1,15,16,31,32,33,64,96,127,128,129,256,512,1000,1400} {
			t.Run(fmt.Sprintf("20:%d/size:%d",parity,n),func(t *testing.T){
				var input, expected [TotalShards][]byte
				for i := range input {
					input[i], expected[i] = make([]byte,n),make([]byte,n)
					for j := range input[i] {
						if i < DataShards {input[i][j]=byte(31*i+17*j+parity)}
						if i >= DataShards {input[i][j]=0xdd}
					}
					if i < DataShards {copy(expected[i],input[i])}
				}
				if err:=fast.encodeFullFused(input[:],parity);err!=nil{t.Fatal(err)}
				if err:=old.Encode(expected[:]);err!=nil{t.Fatal(err)}
				for i:=0;i<DataShards+parity;i++ {
					if !bytes.Equal(input[i],expected[i]) {t.Fatalf("shard %d differs",i)}
				}
				for i:=DataShards+parity;i<TotalShards;i++ {
					if !bytes.Equal(input[i],bytes.Repeat([]byte{0xdd},n)){t.Fatalf("inactive parity row %d written",i)}
				}
			})
		}
	}
}

func TestFECFusedProductionAndPartialNeverConsumeDirtyInactive(t *testing.T) {
	for _,parity := range SupportedParityShards() {
		for _,count := range []int{1,5,10,19,20} {
			fast,_ := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(),1400,32*time.Millisecond,1,parity)
			ref,_ := NewFastBlockEncoderWithParity(NewReedSolomon20x20(),1400,32*time.Millisecond,1,parity)
			now:=time.Unix(20261010,0)
			for i:=0;i<count;i++ {
				packet:=bytes.Repeat([]byte{byte(17*i+count)},1+(i*107+count*53)%1400)
				a,err:=fast.Add(packet,now);if err!=nil{t.Fatal(err)}
				b,err:=ref.Add(packet,now);if err!=nil{t.Fatal(err)}
				compareActiveWire(t,a,b)
			}
			for i:=count;i<DataShards;i++ { for j:=range fast.shardBuf[i] {fast.shardBuf[i][j]=0xdd} }
			a,err:=fast.Flush();if err!=nil{t.Fatal(err)}
			b,err:=ref.Flush();if err!=nil{t.Fatal(err)}
			compareActiveWire(t,a,b)
			for i:=count;i<DataShards;i++ {
				for _,v:=range fast.shardBuf[i] {if v!=0xdd {t.Fatalf("20:%d n=%d inactive %d zeroed",parity,count,i)}}
			}
		}
	}
}

// Unknown active codecs cannot opt into inactive skip by merely implementing
// EncodeActive; the cleared storage is a correctness & ownership boundary.
type strictZeroCodec struct {
	*ReedSolomon20x20
}
func (s strictZeroCodec) EncodeActive(shards [][]byte, dataCount, parityCount int) error {
	for i:=dataCount;i<DataShards;i++ {
		for _,v:=range shards[i] {if v!=0 {return fmt.Errorf("inactive source %d not cleared",i)}}
	}
	return s.ReedSolomon20x20.Encode(shards)
}
func TestFECInactiveCapabilityFailClosedForOtherActiveCodec(t *testing.T) {
	codec:=strictZeroCodec{NewReedSolomon20x20()}
	enc,err:=NewFastBlockEncoderWithParity(codec, 96, 32*time.Millisecond,1,20)
	if err!=nil {t.Fatal(err)}
	_,err=enc.Add(bytes.Repeat([]byte{0x11},96),time.Unix(1,0))
	if err!=nil {t.Fatal(err)}
	for i:=1;i<DataShards;i++ { for j:=range enc.shardBuf[i] {enc.shardBuf[i][j]=0xc7} }
	_,err=enc.Flush()
	if err!=nil {t.Fatal(err)}
}

func BenchmarkFECFullBlockSpanVsFused(b *testing.B) {
	for _,p:=range SupportedParityShards() {
		for _,size:=range []int{96,256,512,1000,1400} {
			r:=NewFastReedSolomon20x20()
			shards:=make([][]byte,TotalShards)
			for i:=range shards {
				shards[i]=make([]byte,size)
				if i<DataShards {for j:=range shards[i] {shards[i][j]=byte(31*i+j)}}
			}
			b.Run(fmt.Sprintf("span/p%d/bytes%d",p,size),func(b *testing.B){
				b.SetBytes(int64(size*DataShards));b.ReportAllocs();b.ResetTimer()
				for i:=0;i<b.N;i++ {if err:=r.EncodeActive(shards,DataShards,p);err!=nil{b.Fatal(err)}}
			})
			b.Run(fmt.Sprintf("fused/p%d/bytes%d",p,size),func(b *testing.B){
				if err:=r.encodeFullFused(shards,p);err!=nil{b.Fatal(err)}
				b.SetBytes(int64(size*DataShards));b.ReportAllocs();b.ResetTimer()
				for i:=0;i<b.N;i++ {if err:=r.encodeFullFused(shards,p);err!=nil{b.Fatal(err)}}
			})
		}
	}
}

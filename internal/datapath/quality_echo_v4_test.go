package datapath

import (
 "bytes"
 "encoding/binary"
 "errors"
 "reflect"
 "testing"
 "time"
)

func qualityEchoVector() QualityEchoV4 {
    r:=qualityHealthVector()
    r.Flags=QualityFlagWindowValid|QualityFlagInsufficient
    r.EstimatedMissing=0
    r.PeakMissingBurst=0
    r.RxUnique=0; r.RxLate=0;r.RxDuplicate=0;r.FECRecovered=0
    r.WindowFirstPN=30;r.WindowLastPN=50;r.TxSuccess=20;r.LocalDrops=1
    r.AgeMillis=250;r.IdleFor=2*time.Second
    return QualityEchoV4{
       Report:r,State:QualityFeedbackEstimated,
       SourceReportSeq:0x1011121314151617,
       SourceGeneration:0x2122232425262728,
       SourceLastPN:0x3132333435363738,
       ReceiverUnique:19,ReceiverLate:2,ReceiverDuplicate:3,EstimatedMissing:1,
    }
}

func TestN1QualityEchoV4Strict128ByteVersionAndIndependentReferences(t *testing.T) {
    e:=qualityEchoVector()
    raw,err:=EncodeQualityEchoV4(e)
    if err!=nil{t.Fatal(err)}
    if len(raw)!=128 || raw[0]!=3 || raw[102]!=byte(QualityFeedbackEstimated) ||
       raw[103]!=0 || binary.BigEndian.Uint64(raw[104:112])!=e.SourceReportSeq ||
       binary.BigEndian.Uint64(raw[112:120])!=e.SourceGeneration ||
       binary.BigEndian.Uint64(raw[120:128])!=e.SourceLastPN ||
       binary.BigEndian.Uint64(raw[68:76])!=19 ||
       binary.BigEndian.Uint32(raw[96:100])!=1 {
       t.Fatalf("unexpected independent echo offset: %x",raw[:])
    }
    got,err:=DecodeQualityEchoV4(raw[:])
    if err!=nil||!reflect.DeepEqual(got,e){t.Fatalf("decoded=%+v err=%v",got,err)}
    if _,err:=DecodeQualityHealthV3(raw[:]);!errors.Is(err,ErrQualityHealthV3) {
       t.Fatalf("104B codec accepted 128B v3: %v",err)
    }
    for _,tc:=range []struct{
       name string
       edit func([]byte)
    }{
       {"version",func(x []byte){x[0]=2}},
       {"length-reserved-header",func(x []byte){x[3]=9}},
       {"reserved-tail",func(x []byte){x[103]=8}},
       {"unknown-state",func(x []byte){x[102]=0x80}},
       {"zero-source-seq",func(x []byte){clear(x[104:112])}},
       {"zero-source-generation",func(x []byte){clear(x[112:120])}},
       {"zero-own-generation",func(x []byte){clear(x[36:44])}},
       {"unknown-flags",func(x []byte){x[1]=0x80}},
       {"non-echo-age-claim",func(x []byte){x[102]=byte(QualityFeedbackInsufficient)}},
       {"estimated-too-small",func(x []byte){binary.BigEndian.PutUint64(x[68:76],7)}},
       {"estimated-excess-missing",func(x []byte){binary.BigEndian.PutUint32(x[96:100],257)}},
    }{
      t.Run(tc.name,func(t *testing.T){
        input:=bytes.Clone(raw[:]);tc.edit(input)
        if _,err:=DecodeQualityEchoV4(input);!errors.Is(err,ErrQualityEchoV4) {
           t.Fatalf("malformed authenticated echo accepted: %v",err)
        }
      })
    }
    for _,n:=range []int{0,9,104,127,129}{
       if _,err:=DecodeQualityEchoV4(make([]byte,n));!errors.Is(err,ErrQualityEchoV4) {
          t.Fatalf("invalid size %d: %v",n,err)
       }
    }
}

func TestN1QualityEchoV4InsufficientNeverMeansZeroLoss(t *testing.T){
    e:=qualityEchoVector()
    e.State=QualityFeedbackCapacity
    e.ReceiverUnique=0;e.ReceiverLate=0;e.ReceiverDuplicate=0;e.EstimatedMissing=0
    raw,err:=EncodeQualityEchoV4(e)
    if err!=nil{t.Fatal(err)}
    got,err:=DecodeQualityEchoV4(raw[:])
    if err!=nil||got.State!=QualityFeedbackCapacity||got.EstimatedMissing!=0 {
       t.Fatalf("capacity/control echo must carry no loss claim: %+v %v",got,err)
    }
}

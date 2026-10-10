package datapath

import (
    "bytes"
    "encoding/binary"
    "encoding/hex"
    "errors"
    "reflect"
    "strconv"
    "strings"
    "testing"
    "time"
)

func qualityHealthVector() QualityHealthReport {
    var nonce [16]byte
    for i := range nonce { nonce[i] = byte(i+1) }
    return QualityHealthReport{
        Flags: QualityFlagWindowValid | QualityFlagLossEstimated,
        IdleFor: time.Second,
        ReportSeq: 0x0102030405060708,
        IncarnationNonce: nonce,
        Generation: 3,
        WindowFirstPN: 10, WindowLastPN: 20,
        TxSuccess: 11, RxUnique: 9,
        RxLate: 1, RxDuplicate: 2, FECRecovered: 3, LocalDrops: 4,
        AgeMillis: 100, RTTMillis: 250,
        EstimatedMissing: 2, PeakMissingBurst: 1,
    }
}

func TestN1QualityHealthV3IndependentWireVector(t *testing.T) {
    // The literal expected wire is hand-written from the offset table, not
    // built by another encoder under test. All integers are big-endian.
    wantHex := strings.Join([]string{
        "02030000",         // version, flags, reserved
        "00000000000003e8", // 1s idle in milliseconds
        "0102030405060708", // report sequence
        "0102030405060708090a0b0c0d0e0f10", // incarnation nonce
        "0000000000000003", // generation
        "000000000000000a", // first window PN
        "0000000000000014", // last window PN
        "000000000000000b", // 11 successfully emitted source records
        "0000000000000009", // 9 unique data records
        "00000001", "00000002", "00000003", "00000004",
        "0064", "00fa", "00000002", "0001", "0000",
    }, "")
    if len(wantHex) != QualityHealthV3Size*2 {
        t.Fatalf("manual vector incorrectly sized: hex chars=%d", len(wantHex))
    }
    want, err := hex.DecodeString(wantHex)
    if err != nil { t.Fatal(err) }
    got, err := EncodeQualityHealthV3(qualityHealthVector())
    if err != nil { t.Fatal(err) }
    if !bytes.Equal(got[:], want) {
        t.Fatalf("strict V3 quality vector mismatch\n got=%x\nwant=%x", got, want)
    }
    decoded, err := DecodeQualityHealthV3(want)
    if err != nil { t.Fatal(err) }
    if !reflect.DeepEqual(decoded, qualityHealthVector()) {
        t.Fatalf("decoded=%+v want=%+v", decoded, qualityHealthVector())
    }
    // The existing live 9-byte health parser must NOT accidentally accept
    // the new format before the explicit N1 receiver rollout.
    if _, err := parseHealth(1, want); err == nil {
        t.Fatal("N0 legacy health incorrectly accepted reserved N1 version")
    }
    if QualityHealthV3Size > 128 { t.Fatal("N1 quality body exceeds wire budget") }
}

func TestN1QualityHealthV3StrictMalformedAndUnknown(t *testing.T) {
    v, err := EncodeQualityHealthV3(qualityHealthVector())
    if err != nil { t.Fatal(err) }
    corrupt := []struct {
        name string
        edit func([]byte)
    }{
        {"bad-version",func(w []byte){w[0]=1}},
        {"unknown-flags",func(w []byte){w[1]=0x80}},
        {"reserved-header",func(w []byte){w[2]=1}},
        {"reserved-tail",func(w []byte){w[103]=1}},
        {"zero-report-seq",func(w []byte){clear(w[12:20])}},
        {"zero-incarnation",func(w []byte){clear(w[20:36])}},
        {"zero-generation",func(w []byte){clear(w[36:44])}},
        {"last-before-first",func(w []byte){binary.BigEndian.PutUint64(w[52:60],9)}},
        {"valid-window-no-tx",func(w []byte){clear(w[60:68])}},
        {"burst-larger-than-missing",func(w []byte){binary.BigEndian.PutUint16(w[100:102],3)}},
        {"loss-claim-without-flag",func(w []byte){w[1]=QualityFlagWindowValid}},
        {"invalid-idle",func(w []byte){binary.BigEndian.PutUint64(w[4:12],uint64(MaxHealthIdle/time.Millisecond)+1)}},
        {"missing-window-flag-with-data",func(w []byte){w[1]=QualityFlagInsufficient}},
    }
    for _, tc := range corrupt {
        t.Run(tc.name,func(t *testing.T){
            w := bytes.Clone(v[:])
            tc.edit(w)
            _,err:=DecodeQualityHealthV3(w)
            if !errors.Is(err,ErrQualityHealthV3) {
                t.Fatalf("unsafe input accepted: err=%v",err)
            }
        })
    }
    for _, n := range []int{0,1,9,103,105,256} {
        t.Run("length-"+strconv.Itoa(n),func(t *testing.T){
            _,err:=DecodeQualityHealthV3(make([]byte,n))
            if !errors.Is(err,ErrQualityHealthV3) {t.Fatalf("bad len %d: %v",n,err)}
        })
    }
}

func TestN1QualityHealthV3UnknownAndEncodeRestrictions(t *testing.T) {
    m:=qualityHealthVector()
    m.Flags=QualityFlagInsufficient|QualityFlagCapacityLimited|QualityFlagStale
    m.WindowFirstPN=0; m.WindowLastPN=0
    m.TxSuccess=0; m.RxUnique=0; m.RxLate=0; m.RxDuplicate=0
    m.FECRecovered=0; m.EstimatedMissing=0; m.PeakMissingBurst=0
    m.AgeMillis=0xffff; m.RTTMillis=0xffff // explicit UNKNOWN
    w,err:=EncodeQualityHealthV3(m)
    if err!=nil {t.Fatal(err)}
    result,err:=DecodeQualityHealthV3(w[:])
    if err!=nil || !reflect.DeepEqual(result,m) {t.Fatalf("unknown round-trip=%+v err=%v",result,err)}
    // Reject overflow/malformed data instead of truncating/wrapping.
    tests:=[]struct{
        name string
        edit func(*QualityHealthReport)
    }{
        {"submillisecond-idle",func(m *QualityHealthReport){m.IdleFor+=time.Microsecond}},
        {"negative-idle",func(m *QualityHealthReport){m.IdleFor=-time.Second}},
        {"no-incarnation",func(m *QualityHealthReport){m.IncarnationNonce=[16]byte{}}},
        {"no-report-seq",func(m *QualityHealthReport){m.ReportSeq=0}},
        {"no-generation",func(m *QualityHealthReport){m.Generation=0}},
        {"unknown-flag",func(m *QualityHealthReport){m.Flags=0xff}},
        {"estimated-with-no-window",func(m *QualityHealthReport){m.Flags=QualityFlagLossEstimated}},
    }
    for _,tc:=range tests {
        t.Run(tc.name,func(t *testing.T){
            bad:=m; tc.edit(&bad)
            if _,err:=EncodeQualityHealthV3(bad);!errors.Is(err,ErrQualityHealthV3){
                t.Fatalf("invalid structure accepted: %v",err)
            }
        })
    }
}

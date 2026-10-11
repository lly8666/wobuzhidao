package runtimeowner

import (
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
)

func makeBoundQuality(t *testing.T) (*qualityState,datapath.QualityHealthReport,time.Time) {
    t.Helper()
    at:=time.Unix(202611,0)
    q:=&qualityState{enabled:true}
    q.nonce[0]=4
    for i:=uint64(1);i<=20;i++ {q.tx.note(i,true,at)}
    m:=datapath.QualityHealthReport{
        Flags:datapath.QualityFlagWindowValid|datapath.QualityFlagInsufficient,
        ReportSeq:7,Generation:5,IncarnationNonce:q.nonce,
        WindowFirstPN:1,WindowLastPN:20,TxSuccess:20,AgeMillis:1,RTTMillis:0xffff,
    }
    return q,m,at
}

func TestN2FirstAttemptRepairBoundCountsDistinctAcrossReport(t *testing.T) {
    q,m,at:=makeBoundQuality(t)
    q.noteRepairAttempt(5)
    q.recordSentSource(m,at)
    src:=q.sentSources[m.ReportSeq%qualitySentHistoryCapacity]
    if !src.repairCoverage||src.repaired!=1{t.Fatalf("repair before report lost: %+v",src)}
    q.noteRepairAttempt(8)
    q.noteRepairAttempt(8) // Same Seq same wire attempted repeatedly: ONE source.
    q.noteRepairAttempt(21) // Outside source report: never counted.
    src=q.sentSources[m.ReportSeq%qualitySentHistoryCapacity]
    if src.repaired!=2{t.Fatalf("distinct repairs=%d, want2",src.repaired)}
    e:=datapath.QualityEchoV4{
        Report:m,State:datapath.QualityFeedbackEstimated,
        SourceReportSeq:m.ReportSeq,SourceGeneration:5,SourceLastPN:20,
        ReceiverUnique:19,EstimatedMissing:1,
    }
    if !q.acceptReturnedEcho(e,at.Add(4*time.Second),5){t.Fatal("authenticated correlated mature return was rejected")}
    // The sender has reliable lower=1 (still missing) and upper=1+2,
    // not an invented exact 1/20 "WAN packet loss" estimate.
    got:=q.sentSources[m.ReportSeq%qualitySentHistoryCapacity]
    if got.repaired!=2 || !got.repairCoverage{t.Fatal(got.repaired,got.repairCoverage)}
    // Newer DATA can rotate the last-256 ring without deleting the
    // historical per-report repair attribution before the 10s expiry.
    for i:=uint64(21);i<=320;i++{q.tx.note(i,true,at)}
    q.noteRepairAttempt(5)
    if src2:=q.sentSources[m.ReportSeq%qualitySentHistoryCapacity];src2.repaired!=2{t.Fatal("history lost or double-counted a repair")}
}

func TestN2FirstAttemptRepairBoundCoverageFailClosed(t *testing.T) {
    q,m,at:=makeBoundQuality(t)
    // A local emit failure cannot be treated as an outer line loss.
    m.LocalDrops=1
    q.recordSentSource(m,at)
    if q.sentSources[m.ReportSeq%qualitySentHistoryCapacity].repairCoverage{t.Fatal("local drop qualified")}
    m.LocalDrops=0
    for i:=uint64(21);i<=300;i++{q.tx.note(i,true,at)}
    q.recordSentSource(m,at)
    if q.sentSources[m.ReportSeq%qualitySentHistoryCapacity].repairCoverage{t.Fatal("overwritten 256 PN evidence qualified")}
    // A report about a different incarnation is not a source witness.
    e:=datapath.QualityEchoV4{Report:m,State:datapath.QualityFeedbackEstimated,
        SourceReportSeq:m.ReportSeq,SourceGeneration:6,SourceLastPN:20,
        ReceiverUnique:19,EstimatedMissing:1}
    if q.acceptReturnedEcho(e,at.Add(4*time.Second),5){t.Fatal("wrong generation was accepted")}
}

func TestN2FirstAttemptRepairWindowZeroAndTailLoss(t *testing.T) {
    q,m,at:=makeBoundQuality(t)
    q.recordSentSource(m,at)
    src:=q.sentSources[m.ReportSeq%qualitySentHistoryCapacity]
    if !src.repairCoverage||src.repaired!=0{t.Fatal("clean 20-source window is not covered")}
    // A completely lost tail record never creates a detectable receiver PN
    // gap alone; mature sender watermarks make it appear as MISSING=1.
    e:=datapath.QualityEchoV4{Report:m,State:datapath.QualityFeedbackEstimated,
        SourceReportSeq:m.ReportSeq,SourceGeneration:5,SourceLastPN:20,
        ReceiverUnique:19,EstimatedMissing:1}
    if !q.acceptReturnedEcho(e,at.Add(4*time.Second),5){t.Fatal("mature tail-loss return rejected")}
    if m.TxSuccess-e.ReceiverUnique!=1 || src.repaired!=0{t.Fatal("tail deficit / initial repair mismatch")}
}

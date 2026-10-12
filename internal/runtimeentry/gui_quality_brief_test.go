package runtimeentry

import (
    "math"
    "strings"
    "testing"
    "time"
)

func TestGUIQualityBriefUnknownIsNotZeroOrOff(t *testing.T) {
    if got:=FormatGUIQualityBrief(GUIQualityBrief{});got!="WBD_CLIENT_QUALITY_V1 rtt_ms=UNKNOWN loss_pct=UNKNOWN fec_tx=UNKNOWN" {
        t.Fatalf("missing trusted quality was not UNKNOWN: %s",got)
    }
    if got:=(*TunnelClient)(nil).GUIQualitySnapshot(time.Now());got.HasRTT||got.HasFirstLossUpper||got.HasActualTxParity {
        t.Fatal("nil tunnel fabricated quality")
    }
}

func TestGUIQualityBriefExplicitOffVersusMissingAndBoundedValues(t *testing.T) {
    x:=GUIQualityBrief{RTTMillis:83,HasRTT:true,FirstLossUpperPct:1.25,
        HasFirstLossUpper:true,HasActualTxParity:true,ActualTxParity:0}
    good:=FormatGUIQualityBrief(x)
    for _,part:=range []string{"rtt_ms=83","loss_pct=1.25","fec_tx=0"} {
        if !strings.Contains(good,part) {t.Fatalf("missing %s in %s",part,good)}
    }
    x.ActualTxParity=7
    x.RTTMillis=0
    x.FirstLossUpperPct=math.NaN()
    got:=FormatGUIQualityBrief(x)
    if strings.Contains(got,"fec_tx=7")||strings.Contains(got,"loss_pct=NaN") ||
        !strings.Contains(got,"fec_tx=UNKNOWN")||!strings.Contains(got,"loss_pct=UNKNOWN")||
        !strings.Contains(got,"rtt_ms=UNKNOWN") {
        t.Fatalf("invalid diagnostics leaked false precision: %s",got)
    }
    x=GUIQualityBrief{HasRTT:true,RTTMillis:60400,HasFirstLossUpper:true,FirstLossUpperPct:110,
        HasActualTxParity:true,ActualTxParity:4}
    got=FormatGUIQualityBrief(x)
    if !strings.Contains(got,"rtt_ms=UNKNOWN")||!strings.Contains(got,"loss_pct=UNKNOWN")||
        !strings.Contains(got,"fec_tx=4"){
        t.Fatalf("out of range controls wrongly treated as true: %s",got)
    }
}

package runtimeentry

import (
    "fmt"
    "math"
    "strconv"
    "time"
)

// GUIQualityBrief only reads three bounded scalar facts, at most once per
// 2 seconds. It does NOT start qualification diagnostics, write packet
// samples, expose auth material or create a local listener.
type GUIQualityBrief struct {
    RTTMillis uint64
    HasRTT bool
    FirstLossUpperPct float64
    HasFirstLossUpper bool
    ActualTxParity int
    HasActualTxParity bool
}

// GUIQualitySnapshot is limited to a single ACTIVE Normal lane. Multiple
// racing Game lanes have no meaningful single "current FEC" value.
func (c *TunnelClient) GUIQualitySnapshot(now time.Time) GUIQualityBrief {
    var out GUIQualityBrief
    if c == nil || c.owner == nil || c.rt == nil { return out }
    lanes := c.owner.ActiveLanes()
    if len(lanes) != 1 { return out }
    ref := lanes[0].Ref
    if data, ok := c.owner.LaneStats(ref); ok {
        parity := data.TxPath.ParityShards
        switch parity {
        case 0,4,8,10,12,16,20:
            out.ActualTxParity=parity
            out.HasActualTxParity=true
        }
    }
    if transport, ok := c.rt.TransportStatsAt(ref,now); ok {
        if transport.SRTT>0 && transport.SRTT<=60*time.Second &&
            !transport.LastAuthenticated.IsZero() &&
            !now.Before(transport.LastAuthenticated) &&
            now.Sub(transport.LastAuthenticated)<=10*time.Second {
            // This is transport SRTT, not an independent proof of WAN RTT.
            out.RTTMillis=uint64(transport.SRTT/time.Millisecond)
            if out.RTTMillis>0 {out.HasRTT=true}
        }
    }
    if control, ok := c.rt.ControlStats(ref); ok {
        // Upper bound from an authenticated first-attempt source report.
        // No qualified recent report means UNKNOWN, never 0% loss.
        age := now.Sub(time.Unix(0,control.N2LastQualifiedUnixNS))
        if control.N2ControllerEnabled && control.N2SourceReportSeq>0 &&
            control.N2LastQualifiedUnixNS>0 && age>=0 && age<=10*time.Second &&
            !math.IsNaN(control.N2ConfidenceUpper) &&
            !math.IsInf(control.N2ConfidenceUpper,0) &&
            control.N2ConfidenceUpper>=0 && control.N2ConfidenceUpper<=1 {
            out.FirstLossUpperPct=100*control.N2ConfidenceUpper
            out.HasFirstLossUpper=true
        }
    }
    return out
}

// FormatGUIQualityBrief is a strict, versioned, safe one-line event consumed
// by the owned Windows UI process. No plaintext, identity, keys or raw PN.
// UNKNOWN is separate from authenticated zero and off-R0.
func FormatGUIQualityBrief(x GUIQualityBrief) string {
    rtt:="UNKNOWN"
    if x.HasRTT && x.RTTMillis>0 && x.RTTMillis<=60000 {
        rtt=strconv.FormatUint(x.RTTMillis,10)
    }
    loss:="UNKNOWN"
    if x.HasFirstLossUpper && !math.IsNaN(x.FirstLossUpperPct) &&
        !math.IsInf(x.FirstLossUpperPct,0) &&
        x.FirstLossUpperPct>=0 && x.FirstLossUpperPct<=100 {
        loss=strconv.FormatFloat(x.FirstLossUpperPct,'f',2,64)
    }
    parity:="UNKNOWN"
    if x.HasActualTxParity {
        switch x.ActualTxParity {
        case 0,4,8,10,12,16,20:
            parity=strconv.Itoa(x.ActualTxParity)
        }
    }
    return fmt.Sprintf("WBD_CLIENT_QUALITY_V1 rtt_ms=%s loss_pct=%s fec_tx=%s",rtt,loss,parity)
}

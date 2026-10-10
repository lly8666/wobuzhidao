package datapath

import (
    "encoding/binary"
    "errors"
)

// QualityEchoV4 extends the existing 104B V3 health control to EXACTLY 128B.
// Its first 104 bytes retain the sending endpoint's normal TX report fields;
// the receive estimate lives in separately typed fields, explicitly tied to
// an earlier authenticated report from the peer's independent TX direction.
// 0x03 is intentionally a distinct KindHealth plaintext version. Old V2
// health 104B and legacy 9B decoders cannot accidentally accept it.
const (
    QualityEchoV4Version byte = 3
    QualityEchoV4Size = 128
)

var ErrQualityEchoV4 = errors.New("datapath: invalid authenticated quality echo")

type QualityEchoV4 struct {
    Report QualityHealthReport
    State QualityFeedbackState
    // A reference to the *peer's* prior authenticated report that described
    // peer TX DATA. Report.Generation above is OUR local generation.
    SourceReportSeq uint64
    SourceGeneration uint64
    SourceLastPN uint64
    ReceiverUnique uint64
    ReceiverLate uint32
    ReceiverDuplicate uint32
    EstimatedMissing uint32 // first-arrival estimate, not physical loss
}

func validQualityEchoV4(e QualityEchoV4) bool {
    if !validQualityHealthV3(e.Report) ||
        e.SourceReportSeq==0 || e.SourceGeneration==0 ||
        e.State<QualityFeedbackEstimated || e.State>QualityFeedbackStale {
        return false
    }
    if e.State==QualityFeedbackEstimated {
        if e.ReceiverUnique<8 || e.EstimatedMissing>QualityPNWindowCapacity {
            return false
        }
    } else if e.ReceiverUnique!=0 || e.ReceiverLate!=0 ||
        e.ReceiverDuplicate!=0 || e.EstimatedMissing!=0 {
        // Unqualified measurements never masquerade as a zero-loss sample.
        return false
    }
    return true
}

// EncodeQualityEchoV4 never produces a LINK/FEC record or chooses a control
// cadence. The live owner seals it with the original direction's AEAD sealer.
func EncodeQualityEchoV4(e QualityEchoV4) ([QualityEchoV4Size]byte,error) {
    var out [QualityEchoV4Size]byte
    if !validQualityEchoV4(e) {return out,ErrQualityEchoV4}
    base,err:=EncodeQualityHealthV3(e.Report)
    if err!=nil {return out,ErrQualityEchoV4}
    copy(out[:QualityHealthV3Size],base[:])
    out[0]=QualityEchoV4Version
    out[102]=byte(e.State)
    binary.BigEndian.PutUint64(out[68:76],e.ReceiverUnique)
    binary.BigEndian.PutUint32(out[76:80],e.ReceiverLate)
    binary.BigEndian.PutUint32(out[80:84],e.ReceiverDuplicate)
    binary.BigEndian.PutUint32(out[96:100],e.EstimatedMissing)
    binary.BigEndian.PutUint64(out[104:112],e.SourceReportSeq)
    binary.BigEndian.PutUint64(out[112:120],e.SourceGeneration)
    binary.BigEndian.PutUint64(out[120:128],e.SourceLastPN)
    return out,nil
}

func DecodeQualityEchoV4(wire []byte) (QualityEchoV4,error) {
    if len(wire)!=QualityEchoV4Size || wire[0]!=QualityEchoV4Version ||
        wire[2]!=0 || wire[3]!=0 || wire[103]!=0 {
        return QualityEchoV4{},ErrQualityEchoV4
    }
    var plain [QualityHealthV3Size]byte
    copy(plain[:],wire[:QualityHealthV3Size])
    plain[0]=QualityHealthV3Version
    plain[102]=0
    clear(plain[68:84])
    clear(plain[96:100])
    report,err:=DecodeQualityHealthV3(plain[:])
    if err!=nil {return QualityEchoV4{},ErrQualityEchoV4}
    result:=QualityEchoV4{
        Report:report, State:QualityFeedbackState(wire[102]),
        ReceiverUnique:binary.BigEndian.Uint64(wire[68:76]),
        ReceiverLate:binary.BigEndian.Uint32(wire[76:80]),
        ReceiverDuplicate:binary.BigEndian.Uint32(wire[80:84]),
        EstimatedMissing:binary.BigEndian.Uint32(wire[96:100]),
        SourceReportSeq:binary.BigEndian.Uint64(wire[104:112]),
        SourceGeneration:binary.BigEndian.Uint64(wire[112:120]),
        SourceLastPN:binary.BigEndian.Uint64(wire[120:128]),
    }
    if !validQualityEchoV4(result) {return QualityEchoV4{},ErrQualityEchoV4}
    return result,nil
}

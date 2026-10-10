package datapath

import (
    "encoding/binary"
    "errors"
    "time"
)

// QualityHealthV3 is a reserved N1 format for the *encrypted* KindHealth
// plaintext of authenticated admission V3 lanes. The legacy 9-byte health v1
// remains the sole live on-wire payload until the bounded N1 sender/receiver
// and feedback fencing are implemented and qualified. Do not emit this
// structure on a V2 or unqualified lane.
const (
    QualityHealthV3Version byte = 2
    QualityHealthV3Size = 104 // strictly bounded below the 128-byte target
)

const (
    QualityFlagWindowValid byte = 1 << iota
    QualityFlagLossEstimated // estimated outer loss, never measured raw WAN loss
    QualityFlagCapacityLimited
    QualityFlagInsufficient
    QualityFlagStale
)

const qualityFlagsMask = QualityFlagWindowValid | QualityFlagLossEstimated |
    QualityFlagCapacityLimited | QualityFlagInsufficient | QualityFlagStale

var ErrQualityHealthV3 = errors.New("datapath: invalid V3 quality health payload")

// QualityHealthReport contains bounded counters from ONE authenticated lane
// incarnation and direction; it does not derive loss from Retransmitted/Fresh.
//
// TxSuccess counts successfully emitted DATA records only, excluding control.
// Seal PN may legitimately skip numbers due to local send/encode failures or
// control records. RxUnique counts authenticated first-unique DATA records
// only: never recursively count this quality/health control as WAN traffic.
// A downstream estimator must align the matching sender watermark and matured windows
// before labelling EstimatedMissing, never call it physical raw-wire loss.
//
// AgeMillis/RTTMillis use 0xffff to mean UNKNOWN, *never* silently zero.
// Generation and nonce fence reused report sequences after lane rotation.
type QualityHealthReport struct {
    Flags             byte
    IdleFor           time.Duration
    ReportSeq         uint64
    IncarnationNonce  [16]byte
    Generation        uint64
    WindowFirstPN     uint64
    WindowLastPN      uint64
    TxSuccess         uint64
    RxUnique          uint64
    RxLate            uint32
    RxDuplicate       uint32
    FECRecovered      uint32
    LocalDrops        uint32
    AgeMillis         uint16
    RTTMillis         uint16
    EstimatedMissing  uint32
    PeakMissingBurst  uint16
}

func validQualityHealthV3(m QualityHealthReport) bool {
    if m.Flags & ^byte(qualityFlagsMask) != 0 ||
        m.IdleFor < 0 || m.IdleFor > MaxHealthIdle ||
        m.IdleFor%time.Millisecond != 0 ||
        m.ReportSeq == 0 || m.Generation == 0 ||
        m.IncarnationNonce == ([16]byte{}) {
        return false
    }
    if m.Flags & QualityFlagWindowValid != 0 {
        if m.WindowFirstPN > m.WindowLastPN || m.TxSuccess == 0 {
            return false
        }
    } else if m.WindowFirstPN != 0 || m.WindowLastPN != 0 ||
        m.TxSuccess != 0 || m.RxUnique != 0 || m.RxLate != 0 ||
        m.RxDuplicate != 0 || m.FECRecovered != 0 ||
        m.EstimatedMissing != 0 || m.PeakMissingBurst != 0 {
        return false
    }
    // A missing-watermark/insufficient summary must not claim a physical
    // loss estimate. An estimate is also invalid without a sample window.
    if m.Flags & QualityFlagLossEstimated != 0 {
        if m.Flags & QualityFlagWindowValid == 0 { return false }
    } else if m.EstimatedMissing != 0 || m.PeakMissingBurst != 0 {
        return false
    }
    if m.PeakMissingBurst > 0 && uint32(m.PeakMissingBurst) > m.EstimatedMissing {
        return false
    }
    return true
}

// EncodeQualityHealthV3 produces only protected plaintext, never a LINK/FEC
// shard. Caller must authenticate/seal it with KindHealth and rate-limit it.
func EncodeQualityHealthV3(m QualityHealthReport) ([QualityHealthV3Size]byte, error) {
    var wire [QualityHealthV3Size]byte
    if !validQualityHealthV3(m) { return wire, ErrQualityHealthV3 }
    wire[0], wire[1] = QualityHealthV3Version, m.Flags
    // bytes 2..3 and 102..103 reserved zero (future incompatible extensions).
    binary.BigEndian.PutUint64(wire[4:12], uint64(m.IdleFor/time.Millisecond))
    binary.BigEndian.PutUint64(wire[12:20], m.ReportSeq)
    copy(wire[20:36], m.IncarnationNonce[:])
    binary.BigEndian.PutUint64(wire[36:44], m.Generation)
    binary.BigEndian.PutUint64(wire[44:52], m.WindowFirstPN)
    binary.BigEndian.PutUint64(wire[52:60], m.WindowLastPN)
    binary.BigEndian.PutUint64(wire[60:68], m.TxSuccess)
    binary.BigEndian.PutUint64(wire[68:76], m.RxUnique)
    binary.BigEndian.PutUint32(wire[76:80], m.RxLate)
    binary.BigEndian.PutUint32(wire[80:84], m.RxDuplicate)
    binary.BigEndian.PutUint32(wire[84:88], m.FECRecovered)
    binary.BigEndian.PutUint32(wire[88:92], m.LocalDrops)
    binary.BigEndian.PutUint16(wire[92:94], m.AgeMillis)
    binary.BigEndian.PutUint16(wire[94:96], m.RTTMillis)
    binary.BigEndian.PutUint32(wire[96:100], m.EstimatedMissing)
    binary.BigEndian.PutUint16(wire[100:102], m.PeakMissingBurst)
    return wire, nil
}

func DecodeQualityHealthV3(wire []byte) (QualityHealthReport, error) {
    if len(wire) != QualityHealthV3Size || wire[0] != QualityHealthV3Version ||
        wire[2] != 0 || wire[3] != 0 || wire[102] != 0 || wire[103] != 0 {
        return QualityHealthReport{}, ErrQualityHealthV3
    }
    ms := binary.BigEndian.Uint64(wire[4:12])
    if ms > uint64(MaxHealthIdle/time.Millisecond) {
        return QualityHealthReport{}, ErrQualityHealthV3
    }
    m := QualityHealthReport{
        Flags:wire[1], IdleFor:time.Duration(ms)*time.Millisecond,
        ReportSeq:binary.BigEndian.Uint64(wire[12:20]),
        Generation:binary.BigEndian.Uint64(wire[36:44]),
        WindowFirstPN:binary.BigEndian.Uint64(wire[44:52]),
        WindowLastPN:binary.BigEndian.Uint64(wire[52:60]),
        TxSuccess:binary.BigEndian.Uint64(wire[60:68]),
        RxUnique:binary.BigEndian.Uint64(wire[68:76]),
        RxLate:binary.BigEndian.Uint32(wire[76:80]),
        RxDuplicate:binary.BigEndian.Uint32(wire[80:84]),
        FECRecovered:binary.BigEndian.Uint32(wire[84:88]),
        LocalDrops:binary.BigEndian.Uint32(wire[88:92]),
        AgeMillis:binary.BigEndian.Uint16(wire[92:94]),
        RTTMillis:binary.BigEndian.Uint16(wire[94:96]),
        EstimatedMissing:binary.BigEndian.Uint32(wire[96:100]),
        PeakMissingBurst:binary.BigEndian.Uint16(wire[100:102]),
    }
    copy(m.IncarnationNonce[:], wire[20:36])
    if !validQualityHealthV3(m) { return QualityHealthReport{}, ErrQualityHealthV3 }
    return m, nil
}

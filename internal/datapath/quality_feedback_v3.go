package datapath

import (
    "errors"
    "sync"
    "time"

    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

var (
    ErrQualityFeedbackIdentity = errors.New("datapath: quality report lane incarnation mismatch")
    ErrQualityFeedbackReplay   = errors.New("datapath: old or repeated quality report")
)

// QualityFeedbackState is a conservative read-only controller input. Only
// QualityFeedbackEstimated allows future N2 loss-based decisions; all other
// states HOLD. In particular, a zero counter without paired sender watermarks
// is UNKNOWN, never an observed 0% WAN loss.
type QualityFeedbackState uint8

const (
    QualityFeedbackUnknown QualityFeedbackState = iota
    QualityFeedbackEstimated
    QualityFeedbackInsufficient
    QualityFeedbackCapacity
    QualityFeedbackStale
)

// QualityFeedbackMailbox holds exactly ONE report for an authenticated lane
// incarnation. It has no queue, timer, goroutine, business send callback or
// per-data-record invocation. The *caller* must first authenticate/decrypt a
// KindHealth record using the matching runtime lane and then call Accept;
// this mailbox is not a substitute for admission or TLS record authentication.
type QualityFeedbackMailbox struct {
    mu sync.Mutex
    ref logicaltunnel.LaneRef
    nonce [16]byte
    accepted bool
    lastSeq uint64
    peerGeneration uint64 // independent of this endpoint's local LaneRef.Generation
    report QualityHealthReport
    received time.Time
}

func NewQualityFeedbackMailbox(ref logicaltunnel.LaneRef, nonce [16]byte) (*QualityFeedbackMailbox, error) {
    if ref.ID == 0 || ref.Generation == 0 || nonce == ([16]byte{}) {
        return nil, ErrQualityFeedbackIdentity
    }
    return &QualityFeedbackMailbox{ref:ref, nonce:nonce}, nil
}

// Accept decodes first, then validates the exact live lane identity and a
// strictly advancing report seq before publishing the one latest copy.
// Rejected duplicates/rotations do not touch the accepted snapshot.
func (m *QualityFeedbackMailbox) Accept(ref logicaltunnel.LaneRef, plaintext []byte, now time.Time) error {
    if m == nil || now.IsZero() {
        return ErrQualityFeedbackIdentity
    }
    report, err := DecodeQualityHealthV3(plaintext)
    if err != nil { return err }
    return m.AcceptDecoded(ref, report, now)
}

// AcceptDecoded must receive a report already authenticated and decoded by
// the matching live Lane's protected KindHealth receive path. This routine
// revalidates the structure before any publication; never call it for raw
// untrusted network data.
func (m *QualityFeedbackMailbox) AcceptDecoded(ref logicaltunnel.LaneRef, report QualityHealthReport, now time.Time) error {
    if m == nil || now.IsZero() {return ErrQualityFeedbackIdentity}
    if !validQualityHealthV3(report) {return ErrQualityHealthV3}
    m.mu.Lock()
    defer m.mu.Unlock()
    // Peers maintain independent local LaneRef.Generation counters: their
    // numeric generations need not be equal. Pin the authenticated sender's
    // generation on the FIRST report and reject later changes on that nonce.
    // ref matches our local incarnation; nonce binds both peers by admission.
    if ref != m.ref || report.IncarnationNonce != m.nonce ||
        (m.accepted && report.Generation != m.peerGeneration) {
        return ErrQualityFeedbackIdentity
    }
    if m.accepted && (report.ReportSeq <= m.lastSeq || now.Before(m.received)) {
        return ErrQualityFeedbackReplay
    }
    m.accepted = true
    m.peerGeneration = report.Generation
    m.lastSeq = report.ReportSeq
    m.report = report
    m.received = now
    return nil
}

// Snapshot is an O(1) read of the latest immutable report. Defaults to the
// N1 minimum stale boundary 10s; callers may pass max(10s, 4*SRTT). A stale
// or incomplete report is never presented as reliable estimated physical loss.
func (m *QualityFeedbackMailbox) Snapshot(now time.Time, staleAfter time.Duration) (QualityHealthReport, QualityFeedbackState) {
    if m == nil { return QualityHealthReport{}, QualityFeedbackUnknown }
    if staleAfter < 10*time.Second { staleAfter = 10*time.Second }
    m.mu.Lock()
    if !m.accepted {
        m.mu.Unlock()
        return QualityHealthReport{}, QualityFeedbackUnknown
    }
    report, received := m.report, m.received
    m.mu.Unlock()
    if now.Before(received) || !now.Before(received.Add(staleAfter)) ||
        report.Flags & QualityFlagStale != 0 {
        return report, QualityFeedbackStale
    }
    if report.Flags & QualityFlagCapacityLimited != 0 {
        return report, QualityFeedbackCapacity
    }
    if report.Flags & QualityFlagInsufficient != 0 {
        return report, QualityFeedbackInsufficient
    }
    if report.Flags & (QualityFlagWindowValid|QualityFlagLossEstimated) !=
        QualityFlagWindowValid|QualityFlagLossEstimated ||
        report.AgeMillis == 0xffff {
        return report, QualityFeedbackUnknown
    }
    return report, QualityFeedbackEstimated
}

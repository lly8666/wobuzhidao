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
    m.mu.Lock()
    defer m.mu.Unlock()
    if ref != m.ref || report.Generation != m.ref.Generation ||
        report.IncarnationNonce != m.nonce {
        return ErrQualityFeedbackIdentity
    }
    if m.accepted && (report.ReportSeq <= m.lastSeq || now.Before(m.received)) {
        return ErrQualityFeedbackReplay
    }
    m.accepted = true
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

package runtimeowner

import (
    "bytes"
    "crypto/sha256"
    "fmt"
    "net/netip"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/faketcp"
    "github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

// TestN0SameSeqFullSealedWireOnForcedRepair covers a deliberately missing
// FakeTCP ACK: the original sealed TLS-like record is retransmitted at its
// original TCP sequence offset without re-encrypting/re-padding/re-sharding.
// It uses the real production Lane + Runtime code and inspects every byte of
// the emitted ciphertext payload. Outer IP ID, TCP ACK/window and checksums
// are intentionally *not* asserted identical: they may change legitimately.
// The three-client privileged netns test is a separate, complementary scope.
func TestN0SameSeqFullSealedWireOnForcedRepair(t *testing.T) {
    profiles := []struct {
        name   string
        parity int
    }{
        {"normal-off", 0},
        {"normal-fixed20-4", 4},
        {"game-fixed20-20-record", 20},
    }
    directions := []struct {
        name string
        role datapath.Role
    }{
        {"c2s", datapath.RoleClient},
        {"s2c", datapath.RoleServer},
    }

    for _, profile := range profiles {
        for _, direction := range directions {
            t.Run(fmt.Sprintf("%s/%s", profile.name, direction.name), func(t *testing.T) {
                lease := runtimeLease(t)
                leaseAddr, err := lease.Config.LeaseIPv4()
                if err != nil {
                    t.Fatal(err)
                }
                owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
                if err != nil {
                    t.Fatal(err)
                }
                sent := make([]faketcp.Segment, 0, 8)
                emit := func(seg faketcp.Segment) error {
                    // Independent, owned snapshot: detect accidental mutation
                    // of the original by a pooled buffer or later write.
                    seg.Payload = bytes.Clone(seg.Payload)
                    sent = append(sent, seg)
                    return nil
                }
                runtime, err := New(owner, nil)
                if err != nil {
                    t.Fatal(err)
                }
                defer runtime.Close()
                clientCfg, serverCfg := transportPair(emit, emit, 1, 40000)
                transport := clientCfg
                if direction.role == datapath.RoleServer {
                    transport = serverCfg
                }
                lane := runtimeLane(t, direction.role, lease, profile.parity, 93)
                snapshot, err := runtime.AttachInitial(1, lane, transport)
                if err != nil {
                    t.Fatal(err)
                }
                t0 := time.Unix(9000, 0)
                src, dst := leaseAddr, netip.MustParseAddr("203.0.113.9")
                if direction.role == datapath.RoleServer {
                    src, dst = dst, src
                }
                packet := runtimeIPv4(src, dst, []byte("N0-full-sealed-wire-payload"))
                records, err := owner.NormalOutbound(packet, t0)
                if err != nil {
                    t.Fatal(err)
                }
                if len(records) == 0 {
                    t.Fatal("no sealed source record produced")
                }
                originalCiphertext := bytes.Clone(records[0].Wire)
                if len(originalCiphertext) <= tlsrecord.FixedWireOverhead ||
                    originalCiphertext[0] != tlsrecord.OuterType ||
                    originalCiphertext[1] != 3 || originalCiphertext[2] != 3 {
                    t.Fatalf("not an entire TLS-like ciphertext record: len=%d prefix=%x",
                        len(originalCiphertext), originalCiphertext[:min(len(originalCiphertext), 5)])
                }
                if err := runtime.SendNormal(records, t0); err != nil {
                    t.Fatal(err)
                }
                if len(sent) == 0 {
                    t.Fatal("sealed source was not emitted")
                }
                first := sent[0]
                if first.Seq != transport.SendNext || !bytes.Equal(first.Payload, originalCiphertext) {
                    t.Fatalf("fresh FakeTCP record mismatch seq=%d want=%d", first.Seq, transport.SendNext)
                }
                freshPacket := faketcp.MarshalSegment(first, 123, faketcp.PacketPersonaLegacy)
                if !bytes.Equal(freshPacket[len(freshPacket)-len(first.Payload):], originalCiphertext) {
                    t.Fatal("fully serialized outer packet did not contain original complete sealed record")
                }

                // No peer ACK is delivered. Use the real bounded repair timer,
                // *not* a second Seal call or fabricated repair callback.
                if err := runtime.Tick(t0.Add(1100 * time.Millisecond)); err != nil {
                    t.Fatal(err)
                }
                originalHash := sha256.Sum256(originalCiphertext)
                matched := 0
                for _, seg := range sent[1:] {
                    if seg.Seq != first.Seq {
                        continue // independent FEC parity/source records have new Seq
                    }
                    matched++
                    repairHash := sha256.Sum256(seg.Payload)
                    if len(seg.Payload) != len(originalCiphertext) ||
                        repairHash != originalHash || !bytes.Equal(seg.Payload, originalCiphertext) {
                        t.Fatalf("same TCP Seq was re-sealed/modified: parity=%d role=%s fresh=%x repair=%x",
                            profile.parity, direction.name, originalHash, repairHash)
                    }
                    repairPacket := faketcp.MarshalSegment(seg, 124, faketcp.PacketPersonaLegacy)
                    if !bytes.Equal(repairPacket[len(repairPacket)-len(seg.Payload):], originalCiphertext) {
                        t.Fatal("repaired fully serialized TCP packet changed the sealed record bytes")
                    }
                }
                if matched == 0 {
                    t.Fatal("no controlled retransmission of original Seq after due timer")
                }
                stats, ok := runtime.TransportStats(snapshot.Ref)
                if !ok || stats.Retransmitted == 0 || stats.RepairSucceeded == 0 {
                    t.Fatalf("real bounded repair not accounted: stats=%+v ok=%v", stats, ok)
                }

                // A truly *new* record with the identical inner payload must
                // advance PN and ciphertext; a retransmission must not.
                nextRecords, err := owner.NormalOutbound(packet, t0.Add(1200*time.Millisecond))
                if err != nil || len(nextRecords) == 0 {
                    t.Fatalf("fresh source record after repair: n=%d err=%v", len(nextRecords), err)
                }
                if bytes.Equal(nextRecords[0].Wire, originalCiphertext) {
                    t.Fatal("new record reused a previous ciphertext/PN")
                }
            })
        }
    }
}

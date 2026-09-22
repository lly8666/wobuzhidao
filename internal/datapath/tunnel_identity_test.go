package datapath

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func datapathLeaseManager(t *testing.T) *logicaltunnel.Manager {
	t.Helper()
	m, err := logicaltunnel.ParseManager("10.77.0.0/29", []string{"0.0.0.0/0"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func datapathInstallation(t *testing.T, raw string) logicaltunnel.InstallationID {
	t.Helper()
	id, err := logicaltunnel.ParseInstallationID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func leasedTestLane(t *testing.T, role Role, parity int, seed byte, lease logicaltunnel.Lease) *Lane {
	t.Helper()
	cfg := pairConfig(role, parity)
	cfg.TunnelID = lease.Config.TunnelID.Bytes()
	cfg.IncarnationNonce[0] = seed
	cfg.Keys.C2S.AEADKey[0] ^= seed
	cfg.Keys.C2S.HPKey[0] ^= seed
	cfg.Keys.C2S.IV[0] ^= seed
	cfg.Keys.S2C.AEADKey[0] ^= seed
	cfg.Keys.S2C.HPKey[0] ^= seed
	cfg.Keys.S2C.IV[0] ^= seed
	lane, err := NewLane(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return lane
}

func TestLeasedOwnerPreservesIdentityAcrossReplacementDormantAndWake(t *testing.T) {
	manager := datapathLeaseManager(t)
	lease, err := manager.Acquire("solo", datapathInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewLeasedTunnelOwner(lease, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	leaseAddr, err := lease.Config.LeaseIPv4()
	if err != nil {
		t.Fatal(err)
	}

	initial := leasedTestLane(t, RoleClient, 4, 1, lease)
	snap, err := owner.AttachInitial(1, initial)
	if err != nil {
		t.Fatal(err)
	}
	a, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	b, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Outbound(businessIPv4Packet(leaseAddr, leaseAddr, []byte("flow-a")), time.Unix(300, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Outbound(businessIPv4Packet(leaseAddr, leaseAddr, []byte("flow-b")), time.Unix(300, int64(time.Millisecond))); err != nil {
		t.Fatal(err)
	}

	candidate := leasedTestLane(t, RoleClient, 20, 2, lease)
	if err := owner.BeginSameIDReplacement(snap.Ref, candidate); err != nil {
		t.Fatal(err)
	}
	fresh, err := owner.PromoteSameIDReplacement(snap.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Ref.Generation <= snap.Ref.Generation || fresh.ParityShards != 20 {
		t.Fatalf("replacement old=%+v fresh=%+v", snap, fresh)
	}
	if got, ok := owner.Lease(); !ok || got.Config.TunnelID != lease.Config.TunnelID || got.Config.Address4 != lease.Config.Address4 {
		t.Fatalf("replacement changed lease got=%+v ok=%v want=%+v", got, ok, lease)
	}

	if _, err := owner.Dormant(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Outbound([]byte("dormant"), time.Unix(301, 0)); !errors.Is(err, ErrTunnelDormant) {
		t.Fatalf("dormant flow err=%v", err)
	}
	if got, ok := owner.Lease(); !ok || got.Config.TunnelID != lease.Config.TunnelID || got.Config.Address4 != lease.Config.Address4 {
		t.Fatalf("dormant changed lease got=%+v ok=%v", got, ok)
	}
	if lookedUp, err := manager.Lookup(lease.Config.TunnelID); err != nil || lookedUp.Config.Address4 != lease.Config.Address4 {
		t.Fatalf("dormant released manager lease lookup=%+v err=%v", lookedUp, err)
	}

	wake := leasedTestLane(t, RoleClient, 4, 3, lease)
	wakeSnap, err := owner.AttachInitial(1, wake)
	if err != nil {
		t.Fatal(err)
	}
	if wakeSnap.Ref.Generation <= fresh.Ref.Generation {
		t.Fatalf("wake generation fresh=%+v wake=%+v", fresh.Ref, wakeSnap.Ref)
	}
	if _, err := b.Outbound(businessIPv4Packet(leaseAddr, leaseAddr, []byte("awake")), time.Unix(302, 0)); err != nil {
		t.Fatalf("preserved flow did not reuse wake lane: %v", err)
	}
}

func TestLeasedOwnerRejectsAnotherInstallationBeforeMembershipMutation(t *testing.T) {
	manager := datapathLeaseManager(t)
	a, err := manager.Acquire("solo", datapathInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := manager.Acquire("solo", datapathInstallation(t, "ffeeddccbbaa99887766554433221100"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewLeasedTunnelOwner(a, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()

	wrong := leasedTestLane(t, RoleClient, 0, 4, b)
	if _, err := owner.AttachInitial(1, wrong); !errors.Is(err, ErrTunnelMismatch) {
		wrong.Close()
		t.Fatalf("other installation lane err=%v", err)
	}
	wrong.Close()
	if st := owner.Stats(); st.ActiveLogicalLanes != 0 || st.PhysicalLanes != 0 {
		t.Fatalf("identity rejection mutated membership=%+v", st)
	}

	right := leasedTestLane(t, RoleClient, 0, 5, a)
	if _, err := owner.AttachInitial(1, right); err != nil {
		t.Fatal(err)
	}
}

func TestLeasedAdmissionHandoffRequiresExactManagerTunnelID(t *testing.T) {
	manager := datapathLeaseManager(t)
	lease, err := manager.Acquire("solo", datapathInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := manager.Acquire("solo", datapathInstallation(t, "ffeeddccbbaa99887766554433221100"))
	if err != nil {
		t.Fatal(err)
	}

	syn := faketcp.Segment{
		SrcIP: [4]byte{10, 0, 0, 1}, DstIP: [4]byte{10, 0, 0, 2},
		SrcPort: 40000, DstPort: 443,
		Seq: 100, Flags: faketcp.FlagSYN, Window: 65535,
		MSS: 1200, MSSSet: true,
	}
	assoc, err := faketcp.NewServerAssociation(syn, 9000, 100*time.Millisecond, func(faketcp.Segment) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer assoc.Close()

	session := &realityfront.ServerAdmissionSession{
		Negotiated: realityfront.AdmissionResult{
			RecordVersion: realityfront.RecordVersionV2,
			TunnelID:      other.Config.TunnelID.Bytes(),
			ClientLimit:   1100,
			ServerLimit:   1000,
			Keys:          testKeys(),
		},
	}
	params := ServerLaneParams{
		ConnectionMTU:   1500,
		TxIPv4HeaderLen: 20,
		TxTCPHeaderLen:  20,
		RxIPv4HeaderLen: 20,
		RxTCPHeaderLen:  20,
		ParityShards:    0,
	}
	if _, err := ServerLaneConfigFromLeasedAdmission(session, assoc, lease, params); !errors.Is(err, ErrTunnelMismatch) {
		t.Fatalf("cross-installation admission err=%v", err)
	}

	session.Negotiated.TunnelID = lease.Config.TunnelID.Bytes()
	cfg, err := ServerLaneConfigFromLeasedAdmission(session, assoc, lease, params)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cfg.TunnelID, lease.Config.TunnelID.Bytes()) {
		t.Fatalf("handoff tunnel id=%x want=%x", cfg.TunnelID, lease.Config.TunnelID.Bytes())
	}
	lane, err := NewLane(cfg)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewLeasedTunnelOwner(lease, 1, 2)
	if err != nil {
		lane.Close()
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := owner.AttachInitial(1, lane); err != nil {
		lane.Close()
		t.Fatalf("matching admission lane rejected: %v", err)
	}
}

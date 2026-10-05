package datapath

import (
	"bytes"
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestGameQualifiedSubsetDoesNotWaitOrEncodeExcludedLanes(t *testing.T) {
	lease := gameLease(t)
	pair := newGameOwnerPair(t, lease, 4)
	addr, _ := lease.Config.LeaseIPv4()
	packet := businessIPv4Packet(netip.MustParseAddr("1.1.1.1"), addr, []byte("ready-sibling"))
	now := time.Unix(600, 0)
	for _, mask := range []uint8{0, 0x10} {
		if _, err := pair.server.GameOutboundOnLanes(packet, now, mask); !errors.Is(err, ErrLaneUnavailable) {
			t.Fatalf("mask=%x error=%v", mask, err)
		}
	}
	out, err := pair.server.GameOutboundOnLanes(packet, now, 1)
	if err != nil || out.PacketID != 1 || len(out.Lanes) != 1 || out.Lanes[0].Ref.ID != 1 {
		t.Fatalf("subset=%+v error=%v", out, err)
	}
	got := deliverGameLane(t, pair.client, pair.clientRef[1], out.Lanes[0].Records, now)
	if len(got.Datagrams) != 1 || !bytes.Equal(got.Datagrams[0], packet) {
		t.Fatalf("ready lane did not deliver independently: %+v", got)
	}
	for id := uint8(2); id <= 4; id++ {
		if stats := pair.serverLane[id].Stats(); stats.OutboundDatagrams != 0 || stats.OutboundRecords != 0 {
			t.Fatalf("excluded lane=%d encoded=%+v", id, stats)
		}
	}
	full, err := pair.server.GameOutbound(packet, now.Add(time.Millisecond))
	if err != nil || full.PacketID != 2 || len(full.Lanes) != 4 {
		t.Fatalf("full racing not restored: %+v %v", full, err)
	}
	for _, lane := range full.Lanes {
		result := deliverGameLane(t, pair.client, pair.clientRef[lane.Ref.ID], lane.Records, now)
		if lane.Ref.ID == 1 && len(result.Datagrams) != 1 || lane.Ref.ID != 1 && len(result.Datagrams) != 0 {
			t.Fatalf("first arrival/dedup lane=%d: %+v", lane.Ref.ID, result)
		}
	}
}

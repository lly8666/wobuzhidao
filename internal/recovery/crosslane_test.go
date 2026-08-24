package recovery

import (
	"errors"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

type crossLaneTestLanes struct{}

func (crossLaneTestLanes) ActiveLaneIDs() []protocol.LaneID  { return []protocol.LaneID{1, 2} }
func (crossLaneTestLanes) SendOn(protocol.LaneID, any) error { return nil }

func TestCrossLaneSafeTreatsACKedSubrangeGapAsStale(t *testing.T) {
	s := NewStreamSender(10)
	if err := s.Track(protocol.DataFrame{FlowID: 1, Offset: 0, TransmissionID: 1, Payload: []byte("abcd")}, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Track(protocol.DataFrame{FlowID: 1, Offset: 4, TransmissionID: 2, Payload: []byte("efgh")}, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 1, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 4}}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReinjectGapCrossLaneSafe(protocol.GapHintFrame{FlowID: 1, Kind: protocol.AckStream, Start: 0, End: 4}, crossLaneTestLanes{})
	if err != nil || len(got) != 0 {
		t.Fatalf("stale ACKed gap got=%#v err=%v", got, err)
	}
}

func TestCrossLaneSafePreservesRealUnknownGap(t *testing.T) {
	s := NewStreamSender(10)
	if err := s.Track(protocol.DataFrame{FlowID: 1, Offset: 4, TransmissionID: 1, Payload: []byte("efgh")}, 2); err != nil {
		t.Fatal(err)
	}
	_, err := s.ReinjectGapCrossLaneSafe(protocol.GapHintFrame{FlowID: 1, Kind: protocol.AckStream, Start: 0, End: 4}, crossLaneTestLanes{})
	if !errors.Is(err, ErrUnknownGap) {
		t.Fatalf("err=%v", err)
	}
}

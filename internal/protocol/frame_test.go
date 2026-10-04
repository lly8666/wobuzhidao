package protocol

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func roundTrip(t *testing.T, in any) {
	t.Helper()
	a, err := MarshalFrame(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b, err := MarshalFrame(in)
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("encoding is not deterministic\n%x\n%x", a, b)
	}
	out, err := UnmarshalFrame(a)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("roundtrip mismatch\nin:  %#v\nout: %#v", in, out)
	}
}

func TestRoundTrips(t *testing.T) {
	t.Run("data", func(t *testing.T) {
		roundTrip(t, DataFrame{FlowID: 7, Offset: 4096, TransmissionID: 91, FIN: true, Payload: []byte("hello")})
	})
	t.Run("datagram", func(t *testing.T) {
		roundTrip(t, DatagramFrame{FlowID: 8, DatagramID: 44, TransmissionID: 92, Payload: []byte{1, 2, 3, 4}})
	})
	t.Run("ack-stream", func(t *testing.T) {
		roundTrip(t, AckFrame{FlowID: 7, Kind: AckStream, Ranges: []Range{{Start: 0, End: 4096}, {Start: 8192, End: 12288}}})
	})
	t.Run("gap-datagram", func(t *testing.T) {
		roundTrip(t, GapHintFrame{FlowID: 8, Kind: AckDatagram, Start: 101, End: 102})
	})
}

func TestLogicalIdentityIndependentFromTransmission(t *testing.T) {
	original := DataFrame{FlowID: 5, Offset: 9000, TransmissionID: 1, Payload: []byte("same logical bytes")}
	reinject := original
	reinject.TransmissionID = 2
	a, _ := MarshalFrame(original)
	b, _ := MarshalFrame(reinject)
	if bytes.Equal(a, b) {
		t.Fatal("different transmission attempts must encode differently")
	}
	pa, _ := UnmarshalFrame(a)
	pb, _ := UnmarshalFrame(b)
	da := pa.(DataFrame)
	db := pb.(DataFrame)
	if da.FlowID != db.FlowID || da.Offset != db.Offset || !bytes.Equal(da.Payload, db.Payload) {
		t.Fatal("reinjection changed logical identity")
	}
	if da.TransmissionID == db.TransmissionID {
		t.Fatal("transmission identity did not change")
	}
}

func TestMalformedFrames(t *testing.T) {
	valid, err := MarshalFrame(DataFrame{FlowID: 1, Offset: 2, TransmissionID: 3, Payload: []byte("abc")})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(valid); i++ {
		if _, err := UnmarshalFrame(valid[:i]); err == nil {
			t.Fatalf("prefix length %d unexpectedly decoded", i)
		}
	}

	cases := [][]byte{
		append(append([]byte{}, valid...), 0),
		{2, byte(FrameData), 0, 0},
		{1, 99, 0, 0},
		{1, byte(FrameDatagram), 1, 0},
	}
	for i, c := range cases {
		if _, err := UnmarshalFrame(c); err == nil {
			t.Fatalf("case %d unexpectedly decoded", i)
		}
	}
}

func TestValidation(t *testing.T) {
	if _, err := MarshalFrame(AckFrame{FlowID: 1, Kind: AckStream}); !errors.Is(err, ErrLimit) {
		t.Fatalf("empty ack ranges: %v", err)
	}
	if _, err := MarshalFrame(AckFrame{FlowID: 1, Kind: AckStream, Ranges: []Range{{Start: 9, End: 9}}}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("bad range: %v", err)
	}
	if _, err := MarshalFrame(AckFrame{FlowID: 1, Kind: AckStream, Ranges: []Range{{Start: 10, End: 20}, {Start: 19, End: 30}}}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("overlap: %v", err)
	}
	big := make([]byte, MaxPayload+1)
	if _, err := MarshalFrame(DatagramFrame{FlowID: 1, DatagramID: 1, TransmissionID: 1, Payload: big}); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversize: %v", err)
	}
	if _, err := MarshalFrame(GapHintFrame{FlowID: 1, Kind: AckKind(99), Start: 1, End: 2}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("kind: %v", err)
	}
}

func FuzzUnmarshalFrame(f *testing.F) {
	seeds := []any{
		DataFrame{FlowID: 1, Offset: 2, TransmissionID: 3, Payload: []byte("seed")},
		DatagramFrame{FlowID: 2, DatagramID: 4, TransmissionID: 5, Payload: []byte{1, 2}},
		AckFrame{FlowID: 1, Kind: AckStream, Ranges: []Range{{Start: 0, End: 10}}},
		GapHintFrame{FlowID: 1, Kind: AckStream, Start: 10, End: 20},
	}
	for _, s := range seeds {
		b, _ := MarshalFrame(s)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = UnmarshalFrame(data) })
}

func TestAckFINAndDatagramIDRangeBoundary(t *testing.T) {
	// Empty reliable stream closure still needs a representable logical ACK.
	roundTrip(t, AckFrame{FlowID: 90, Kind: AckStream, FIN: true})
	if _, err := MarshalFrame(AckFrame{FlowID: 90, Kind: AckDatagram, FIN: true}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("DATAGRAM FIN ack: %v", err)
	}
	if _, err := MarshalFrame(DatagramFrame{FlowID: 1, DatagramID: DatagramID(MaxDatagramID + 1), TransmissionID: 1}); !errors.Is(err, ErrLimit) {
		t.Fatalf("unacknowledgeable datagram id: %v", err)
	}
	// The largest allowed datagram ID has a half-open ACK endpoint at MaxValue.
	roundTrip(t, DatagramFrame{FlowID: 1, DatagramID: DatagramID(MaxDatagramID), TransmissionID: 1, Payload: []byte{}})
	roundTrip(t, AckFrame{FlowID: 1, Kind: AckDatagram, Ranges: []Range{{Start: MaxDatagramID, End: MaxValue}}})
}

package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"testing"
)

type fragmentReader struct {
	data []byte
	step int
}

func (r *fragmentReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := r.step
	if n <= 0 || n > len(r.data) {
		n = len(r.data)
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

type shortWriter struct {
	buf  bytes.Buffer
	step int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	n := w.step
	if n <= 0 || n > len(p) {
		n = len(p)
	}
	return w.buf.Write(p[:n])
}

func TestReadFrameFragmentedAndCoalesced(t *testing.T) {
	frames := []any{
		DataFrame{FlowID: 1, Offset: 0, TransmissionID: 1, Payload: []byte("hello")},
		DatagramFrame{FlowID: 2, DatagramID: 9, TransmissionID: 2, Payload: []byte("dgram")},
		AckFrame{FlowID: 1, Kind: AckStream, Ranges: []Range{{Start: 0, End: 5}}},
		GapHintFrame{FlowID: 1, Kind: AckStream, Start: 5, End: 10},
	}
	var wire []byte
	for _, frame := range frames {
		b, err := MarshalFrame(frame)
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, b...)
	}

	for _, step := range []int{1, 2, 3, 7, len(wire)} {
		r := &fragmentReader{data: append([]byte(nil), wire...), step: step}
		for i, want := range frames {
			got, err := ReadFrame(r)
			if err != nil {
				t.Fatalf("step=%d frame=%d: %v", step, i, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("step=%d frame=%d got=%#v want=%#v", step, i, got, want)
			}
		}
		if _, err := ReadFrame(r); !errors.Is(err, io.EOF) {
			t.Fatalf("step=%d final err=%v want EOF", step, err)
		}
	}
}

func TestWriteFrameHandlesShortWrites(t *testing.T) {
	want := DataFrame{FlowID: 3, Offset: 99, TransmissionID: 7, FIN: true, Payload: []byte("payload")}
	w := &shortWriter{step: 2}
	if err := WriteFrame(w, want); err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalFrame(w.buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
}

func TestReadFrameRejectsOversizeBeforeBody(t *testing.T) {
	var v [10]byte
	n := binary.PutUvarint(v[:], MaxFrameBody+1)
	wire := append([]byte{Version1, byte(FrameData), 0}, v[:n]...)
	if _, err := ReadFrame(bytes.NewReader(wire)); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversize err=%v", err)
	}
}

func TestReadFrameRejectsBadOrTruncatedInput(t *testing.T) {
	if _, err := ReadFrame(bytes.NewReader([]byte{Version1})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short fixed header err=%v", err)
	}
	badVarint := append([]byte{Version1, byte(FrameData), 0}, bytes.Repeat([]byte{0x80}, 10)...)
	if _, err := ReadFrame(bytes.NewReader(badVarint)); !errors.Is(err, ErrMalformed) {
		t.Fatalf("bad varint err=%v", err)
	}
	valid, err := MarshalFrame(DataFrame{FlowID: 1, Offset: 0, TransmissionID: 1, Payload: []byte("abc")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFrame(bytes.NewReader(valid[:len(valid)-1])); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated body err=%v", err)
	}
}

func FuzzReadFrame(f *testing.F) {
	seeds := []any{
		DataFrame{FlowID: 1, Offset: 0, TransmissionID: 1, Payload: []byte("seed")},
		DatagramFrame{FlowID: 2, DatagramID: 3, TransmissionID: 4, Payload: []byte("d")},
		AckFrame{FlowID: 1, Kind: AckStream, Ranges: []Range{{Start: 0, End: 4}}},
	}
	for _, frame := range seeds {
		b, _ := MarshalFrame(frame)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ReadFrame(bytes.NewReader(data))
	})
}

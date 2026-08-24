package control

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	cases := []any{
		Hello{MinProtocol: 1, MaxProtocol: 3},
		Accept{Protocol: 2},
		Error{Code: ErrorNoCommonVersion, Message: "no common protocol version"},
	}
	for _, tc := range cases {
		b, err := Marshal(tc)
		if err != nil {
			t.Fatalf("Marshal(%T): %v", tc, err)
		}
		got, err := Unmarshal(b)
		if err != nil {
			t.Fatalf("Unmarshal(%T): %v", tc, err)
		}
		if !reflect.DeepEqual(got, tc) {
			t.Fatalf("round trip %T: got %#v want %#v", tc, got, tc)
		}
	}
}

func TestNegotiation(t *testing.T) {
	got := Negotiate(Hello{MinProtocol: 1, MaxProtocol: 3}, 1, 2)
	if want := (Accept{Protocol: 2}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	got = Negotiate(Hello{MinProtocol: 3, MaxProtocol: 4}, 1, 2)
	e, ok := got.(Error)
	if !ok || e.Code != ErrorNoCommonVersion {
		t.Fatalf("got %#v", got)
	}
}

func TestRejectMalformed(t *testing.T) {
	good, err := Marshal(Hello{MinProtocol: 1, MaxProtocol: 1})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"short", []byte("WBDC"), ErrMalformed},
		{"bad-magic", append([]byte("NOPE"), good[4:]...), ErrMalformed},
		{"unknown-frame-version", mutate(good, 4, 2), ErrUnsupported},
		{"unknown-type", mutate(good, 5, 99), ErrUnsupported},
		{"truncated-body", good[:len(good)-1], ErrMalformed},
		{"trailing-byte", append(append([]byte(nil), good...), 0), ErrMalformed},
		{"hello-zero-min", mutate(good, 9, 0), ErrMalformed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Unmarshal(tc.data)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestLimitsAndUTF8(t *testing.T) {
	if _, err := Marshal(Error{Code: ErrorPolicy, Message: strings.Repeat("x", MaxErrorMessageLen+1)}); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversize err=%v", err)
	}
	bad := []byte{'W', 'B', 'D', 'C', 1, byte(TypeError), 0, 3, 0, 1, 0xff}
	if _, err := Unmarshal(bad); !errors.Is(err, ErrMalformed) {
		t.Fatalf("utf8 err=%v", err)
	}
	oversizedHeader := []byte{'W', 'B', 'D', 'C', 1, byte(TypeError), 0x04, 0x01}
	if _, err := Unmarshal(oversizedHeader); !errors.Is(err, ErrLimit) {
		t.Fatalf("limit err=%v", err)
	}
}

func TestEveryTruncationRejectsWithoutPanic(t *testing.T) {
	b, err := Marshal(Error{Code: ErrorPolicy, Message: "policy"})
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < len(b); n++ {
		if _, err := Unmarshal(b[:n]); err == nil {
			t.Fatalf("prefix len %d unexpectedly accepted", n)
		}
	}
}

func FuzzUnmarshal(f *testing.F) {
	seeds := [][]byte{
		{}, []byte("WBDC"),
		mustMarshal(Hello{MinProtocol: 1, MaxProtocol: 1}),
		mustMarshal(Accept{Protocol: 1}),
		mustMarshal(Error{Code: ErrorPolicy, Message: "x"}),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = Unmarshal(b) })
}

func mutate(src []byte, idx int, value byte) []byte {
	out := append([]byte(nil), src...)
	out[idx] = value
	return out
}
func mustMarshal(v any) []byte {
	b, err := Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

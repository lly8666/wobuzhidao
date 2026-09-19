package control

import (
	"errors"
	"reflect"
	"testing"
)

func TestConfigRoundTripFixedModes(t *testing.T) {
	for _, mode := range []ProtectionMode{ProtectionNormal, ProtectionWeak15, ProtectionWeak2} {
		for _, in := range []any{Config{Mode: mode}, ConfigOK{Mode: mode}} {
			wire, err := MarshalExtended(in)
			if err != nil {
				t.Fatal(err)
			}
			got, err := UnmarshalExtended(wire)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, in) {
				t.Fatalf("got %#v want %#v", got, in)
			}
		}
	}
}

func TestConfigRejectsAutoZeroUnknownAndMalformed(t *testing.T) {
	for _, mode := range []ProtectionMode{0, ProtectionAutoReserved, 99} {
		if _, err := MarshalExtended(Config{Mode: mode}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("mode %d err=%v", mode, err)
		}
	}
	for _, wire := range [][]byte{
		{'W', 'B', 'D', 'C', 1, byte(TypeConfig), 0, 0},
		{'W', 'B', 'D', 'C', 1, byte(TypeConfig), 0, 2, 1, 2},
		{'W', 'B', 'D', 'C', 1, byte(TypeConfig), 0, 1, byte(ProtectionAutoReserved)},
		{'W', 'B', 'D', 'C', 1, byte(TypeConfigOK), 0, 1, 99},
	} {
		if _, err := UnmarshalExtended(wire); err == nil {
			t.Fatalf("accepted %x", wire)
		}
	}
}

func TestProtectionModeString(t *testing.T) {
	want := map[ProtectionMode]string{ProtectionNormal: "normal", ProtectionWeak15: "weak-1.5x", ProtectionWeak2: "weak-2x", ProtectionAutoReserved: "auto"}
	for mode, s := range want {
		if mode.String() != s {
			t.Fatalf("%d=%q", mode, mode.String())
		}
	}
}

func TestConfigServerSessionOneShotAndOrdering(t *testing.T) {
	s, err := NewConfigServerSession(1, 1, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := mustExtended(t, Config{Mode: ProtectionWeak15})
	reply, err := s.HandleWire(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertErrorCode(t, reply, ErrorUnexpectedState)

	hello := mustExtended(t, Hello{MinProtocol: 1, MaxProtocol: 1})
	if _, err = s.HandleWire(hello, 2); err != nil {
		t.Fatal(err)
	}
	reply, err = s.HandleWire(cfg, 3)
	if err != nil {
		t.Fatal(err)
	}
	assertErrorCode(t, reply, ErrorAuthRequired)

	auth := mustExtended(t, Auth{Token: []byte("secret")})
	if _, err = s.HandleWire(auth, 4); err != nil {
		t.Fatal(err)
	}
	reply, err = s.HandleWire(cfg, 5)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalExtended(reply)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, ConfigOK{Mode: ProtectionWeak15}) {
		t.Fatalf("got %#v", got)
	}
	st := s.Stats()
	if !st.Configured || st.ProtectionMode != ProtectionWeak15 || st.State != StateEstablished || !st.Authenticated {
		t.Fatalf("stats %#v", st)
	}

	reply, err = s.HandleWire(cfg, 6)
	if err != nil {
		t.Fatal(err)
	}
	assertErrorCode(t, reply, ErrorUnexpectedState)
}

func TestConfigStatsIncludeWireCounts(t *testing.T) {
	s, _ := NewConfigServerSession(1, 1, nil)
	hello := mustExtended(t, Hello{MinProtocol: 1, MaxProtocol: 1})
	helloReply, _ := s.HandleWire(hello, 10)
	cfg := mustExtended(t, Config{Mode: ProtectionWeak2})
	cfgReply, _ := s.HandleWire(cfg, 20)
	st := s.Stats()
	if st.ControlRX != 2 || st.ControlTX != 2 {
		t.Fatalf("counts %#v", st)
	}
	if st.ControlRXBytes != uint64(len(hello)+len(cfg)) || st.ControlTXBytes != uint64(len(helloReply)+len(cfgReply)) {
		t.Fatalf("bytes %#v", st)
	}
	if !st.Configured || st.ProtectionMode != ProtectionWeak2 {
		t.Fatalf("config %#v", st)
	}
}

func mustExtended(t *testing.T, v any) []byte {
	t.Helper()
	b, err := MarshalExtended(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func assertErrorCode(t *testing.T, wire []byte, code ErrorCode) {
	t.Helper()
	v, err := UnmarshalExtended(wire)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := v.(Error)
	if !ok || e.Code != code {
		t.Fatalf("got %#v want error %d", v, code)
	}
}

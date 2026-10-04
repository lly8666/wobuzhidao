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
		Auth{Token: []byte{0, 1, 2, 3}},
		AuthOK{},
	}
	for _, tc := range cases {
		b, err := Marshal(tc)
		if err != nil { t.Fatalf("Marshal(%T): %v", tc, err) }
		got, err := Unmarshal(b)
		if err != nil { t.Fatalf("Unmarshal(%T): %v", tc, err) }
		if !reflect.DeepEqual(got, tc) { t.Fatalf("round trip %T: got %#v want %#v", tc, got, tc) }
	}
}

func TestNegotiation(t *testing.T) {
	got := Negotiate(Hello{MinProtocol: 1, MaxProtocol: 3}, 1, 2)
	if want := (Accept{Protocol: 2}); !reflect.DeepEqual(got, want) { t.Fatalf("got %#v want %#v", got, want) }
	got = Negotiate(Hello{MinProtocol: 3, MaxProtocol: 4}, 1, 2)
	e, ok := got.(Error)
	if !ok || e.Code != ErrorNoCommonVersion { t.Fatalf("got %#v", got) }
}

func TestRejectMalformed(t *testing.T) {
	good, err := Marshal(Hello{MinProtocol: 1, MaxProtocol: 1})
	if err != nil { t.Fatal(err) }
	cases := []struct{name string; data []byte; want error}{
		{"short", []byte("WBDC"), ErrMalformed},
		{"bad-magic", append([]byte("NOPE"), good[4:]...), ErrMalformed},
		{"unknown-frame-version", mutate(good, 4, 2), ErrUnsupported},
		{"unknown-type", mutate(good, 5, 99), ErrUnsupported},
		{"truncated-body", good[:len(good)-1], ErrMalformed},
		{"trailing-byte", append(append([]byte(nil), good...), 0), ErrMalformed},
		{"hello-zero-min", mutate(good, 9, 0), ErrMalformed},
		{"auth-ok-nonempty", []byte{'W','B','D','C',1,byte(TypeAuthOK),0,1,1}, ErrMalformed},
		{"ping-wrong-size", []byte{'W','B','D','C',1,byte(TypePing),0,1,1}, ErrMalformed},
	}
	for _, tc := range cases { t.Run(tc.name, func(t *testing.T){ _,err:=Unmarshal(tc.data); if !errors.Is(err,tc.want){t.Fatalf("err=%v want %v",err,tc.want)} }) }
}

func TestLimitsAndUTF8(t *testing.T) {
	if _, err := Marshal(Error{Code: ErrorPolicy, Message: strings.Repeat("x", MaxErrorMessageLen+1)}); !errors.Is(err, ErrLimit) { t.Fatalf("oversize err=%v", err) }
	if _, err := Marshal(Auth{Token: make([]byte, MaxTokenLen+1)}); !errors.Is(err, ErrLimit) { t.Fatalf("auth oversize err=%v", err) }
	if _, err := Marshal(Auth{}); !errors.Is(err, ErrMalformed) { t.Fatalf("empty auth err=%v", err) }
	bad := []byte{'W','B','D','C',1,byte(TypeError),0,3,0,1,0xff}
	if _, err := Unmarshal(bad); !errors.Is(err, ErrMalformed) { t.Fatalf("utf8 err=%v", err) }
	oversizedHeader := []byte{'W','B','D','C',1,byte(TypeError),0x04,0x01}
	if _, err := Unmarshal(oversizedHeader); !errors.Is(err, ErrLimit) { t.Fatalf("limit err=%v", err) }
}

func TestServerSessionAuthDisabled(t *testing.T) {
	s, err := NewServerSession(1, 1, nil); if err != nil { t.Fatal(err) }
	got := s.Handle(Hello{MinProtocol: 1, MaxProtocol: 1})
	if !reflect.DeepEqual(got, Accept{Protocol: 1}) { t.Fatalf("got %#v", got) }
	if s.State() != StateEstablished { t.Fatalf("state=%v", s.State()) }
	dup := s.Handle(Auth{Token: []byte("unused")})
	if e, ok := dup.(Error); !ok || e.Code != ErrorUnexpectedState { t.Fatalf("duplicate got %#v", dup) }
}

func TestServerSessionAuthSuccess(t *testing.T) {
	s, err := NewServerSession(1, 1, []byte("secret")); if err != nil { t.Fatal(err) }
	if got := s.Handle(Hello{MinProtocol: 1, MaxProtocol: 1}); !reflect.DeepEqual(got, Accept{Protocol: 1}) { t.Fatalf("hello got %#v", got) }
	if s.State() != StateAwaitAuth { t.Fatalf("state=%v", s.State()) }
	if got := s.Handle(Auth{Token: []byte("secret")}); !reflect.DeepEqual(got, AuthOK{}) { t.Fatalf("auth got %#v", got) }
	if s.State() != StateEstablished { t.Fatalf("state=%v", s.State()) }
}

func TestServerSessionWrongTokenFailsClosed(t *testing.T) {
	s, err := NewServerSession(1, 1, []byte("secret")); if err != nil { t.Fatal(err) }
	_ = s.Handle(Hello{MinProtocol: 1, MaxProtocol: 1})
	got := s.Handle(Auth{Token: []byte("wrong")})
	if e, ok := got.(Error); !ok || e.Code != ErrorAuthFailed { t.Fatalf("got %#v", got) }
	if s.State() != StateFailed { t.Fatalf("state=%v", s.State()) }
	got = s.Handle(Auth{Token: []byte("secret")})
	if e, ok := got.(Error); !ok || e.Code != ErrorUnexpectedState { t.Fatalf("retry got %#v", got) }
}

func TestServerSessionOrdering(t *testing.T) {
	s, err := NewServerSession(1, 1, []byte("secret")); if err != nil { t.Fatal(err) }
	got := s.Handle(Auth{Token: []byte("secret")})
	if e, ok := got.(Error); !ok || e.Code != ErrorUnexpectedState { t.Fatalf("pre-hello got %#v", got) }
	if s.State() != StateAwaitHello { t.Fatalf("state=%v", s.State()) }
	_ = s.Handle(Hello{MinProtocol: 1, MaxProtocol: 1})
	got = s.Handle(Hello{MinProtocol: 1, MaxProtocol: 1})
	if e, ok := got.(Error); !ok || e.Code != ErrorAuthRequired { t.Fatalf("repeated hello got %#v", got) }
	if s.State() != StateAwaitAuth { t.Fatalf("state=%v", s.State()) }
}

func TestServerSessionUnsupportedVersionFails(t *testing.T) {
	s, err := NewServerSession(1, 1, nil); if err != nil { t.Fatal(err) }
	got := s.Handle(Hello{MinProtocol: 2, MaxProtocol: 2})
	if e, ok := got.(Error); !ok || e.Code != ErrorNoCommonVersion { t.Fatalf("got %#v", got) }
	if s.State() != StateFailed { t.Fatalf("state=%v", s.State()) }
}

func TestEveryTruncationRejectsWithoutPanic(t *testing.T) {
	b, err := Marshal(Error{Code: ErrorPolicy, Message: "policy"}); if err != nil { t.Fatal(err) }
	for n := 0; n < len(b); n++ { if _, err := Unmarshal(b[:n]); err == nil { t.Fatalf("prefix len %d unexpectedly accepted", n) } }
}

func FuzzUnmarshal(f *testing.F) {
	seeds := [][]byte{{}, []byte("WBDC"), mustMarshal(Hello{MinProtocol:1,MaxProtocol:1}), mustMarshal(Accept{Protocol:1}), mustMarshal(Error{Code:ErrorPolicy,Message:"x"}), mustMarshal(Auth{Token:[]byte("token")}), mustMarshal(AuthOK{})}
	for _, s := range seeds { f.Add(s) }
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = Unmarshal(b) })
}

func mutate(src []byte, idx int, value byte) []byte { out:=append([]byte(nil),src...); out[idx]=value; return out }
func mustMarshal(v any) []byte { b,err:=Marshal(v); if err!=nil { panic(err) }; return b }

func TestPingPongRoundTrip(t *testing.T) {
	for _, tc := range []any{Ping{Nonce: 0x0102030405060708}, Pong{Nonce: 99}} {
		wire, err := Marshal(tc); if err != nil { t.Fatal(err) }
		got, err := Unmarshal(wire); if err != nil { t.Fatal(err) }
		if !reflect.DeepEqual(got, tc) { t.Fatalf("got %#v want %#v", got, tc) }
	}
}

func TestPingRequiresEstablished(t *testing.T) {
	s, err := NewServerSession(1, 1, []byte("secret")); if err != nil { t.Fatal(err) }
	got := s.Handle(Ping{Nonce: 7})
	if e, ok := got.(Error); !ok || e.Code != ErrorUnexpectedState { t.Fatalf("pre-hello got %#v", got) }
	_ = s.Handle(Hello{MinProtocol: 1, MaxProtocol: 1})
	got = s.Handle(Ping{Nonce: 8})
	if e, ok := got.(Error); !ok || e.Code != ErrorAuthRequired { t.Fatalf("pre-auth got %#v", got) }
}

func TestEstablishedPingEchoAndStats(t *testing.T) {
	s, err := NewServerSession(1, 1, nil); if err != nil { t.Fatal(err) }
	helloWire := mustMarshal(Hello{MinProtocol: 1, MaxProtocol: 1})
	reply, err := s.HandleWire(helloWire, 100); if err != nil { t.Fatal(err) }
	if _, err := Unmarshal(reply); err != nil { t.Fatal(err) }
	pingWire := mustMarshal(Ping{Nonce: 0xdeadbeef})
	pongWire, err := s.HandleWire(pingWire, 150); if err != nil { t.Fatal(err) }
	pong, err := Unmarshal(pongWire); if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(pong, Pong{Nonce: 0xdeadbeef}) { t.Fatalf("pong=%#v", pong) }
	st := s.Stats()
	if st.State != StateEstablished || !st.Authenticated || st.AuthRequired { t.Fatalf("stats=%+v", st) }
	if st.ControlRX != 2 || st.ControlTX != 2 || st.PingsReceived != 1 || st.PongsSent != 1 { t.Fatalf("stats=%+v", st) }
	if st.ControlRXBytes != uint64(len(helloWire)+len(pingWire)) || st.ControlTXBytes != uint64(len(reply)+len(pongWire)) { t.Fatalf("byte stats=%+v", st) }
	if st.LastActivity != 150 { t.Fatalf("last=%d", st.LastActivity) }
}

func TestIdleExpiredBoundary(t *testing.T) {
	s, err := NewServerSession(1, 1, nil); if err != nil { t.Fatal(err) }
	_, err = s.HandleWire(mustMarshal(Hello{MinProtocol: 1, MaxProtocol: 1}), 1000); if err != nil { t.Fatal(err) }
	if s.IdleExpired(1099, 100) { t.Fatal("expired before boundary") }
	if !s.IdleExpired(1100, 100) { t.Fatal("not expired at exact boundary") }
	if s.IdleExpired(900, 100) { t.Fatal("clock regression must not expire") }
	if s.IdleExpired(999999, 0) { t.Fatal("zero idle disables expiry") }
}

func TestCloseRoundTripAndValidation(t *testing.T) {
	want := Close{Reason: CloseIdleTimeout, Detail: "idle"}
	wire, err := Marshal(want); if err != nil { t.Fatal(err) }
	got, err := Unmarshal(wire); if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(got, want) { t.Fatalf("got %#v want %#v", got, want) }
	if _, err := Marshal(Close{Reason: 99}); !errors.Is(err, ErrUnsupported) { t.Fatalf("unknown reason err=%v", err) }
	if _, err := Marshal(Close{Reason: ClosePolicy, Detail: strings.Repeat("x", MaxErrorMessageLen+1)}); !errors.Is(err, ErrLimit) { t.Fatalf("detail limit err=%v", err) }
	badUTF8 := []byte{'W','B','D','C',1,byte(TypeClose),0,3,0,byte(ClosePolicy),0xff}
	if _, err := Unmarshal(badUTF8); !errors.Is(err, ErrMalformed) { t.Fatalf("utf8 err=%v", err) }
}

func TestPeerCloseTransitionsClosed(t *testing.T) {
	s, err := NewServerSession(1, 1, nil); if err != nil { t.Fatal(err) }
	_ = s.Handle(Hello{MinProtocol: 1, MaxProtocol: 1})
	got := s.Handle(Close{Reason: CloseNormal, Detail: "done"})
	if !reflect.DeepEqual(got, Close{Reason: CloseNormal, Detail: "done"}) { t.Fatalf("got %#v", got) }
	if s.State() != StateClosed || s.Stats().CloseReason != CloseNormal { t.Fatalf("stats=%+v", s.Stats()) }
	got = s.Handle(Ping{Nonce: 1})
	if e, ok := got.(Error); !ok || e.Code != ErrorUnexpectedState { t.Fatalf("after close got %#v", got) }
}

func TestExpireIdleClosesSession(t *testing.T) {
	s, err := NewServerSession(1, 1, nil); if err != nil { t.Fatal(err) }
	_, err = s.HandleWire(mustMarshal(Hello{MinProtocol: 1, MaxProtocol: 1}), 1000); if err != nil { t.Fatal(err) }
	if s.ExpireIdle(1099, 100) { t.Fatal("closed early") }
	if !s.ExpireIdle(1100, 100) { t.Fatal("did not close at boundary") }
	if s.State() != StateClosed || s.Stats().CloseReason != CloseIdleTimeout { t.Fatalf("stats=%+v", s.Stats()) }
}

func TestReconnectAllowed(t *testing.T) {
	cases := map[CloseReason]bool{CloseNormal:false, CloseIdleTimeout:true, CloseAuthFailure:false, ClosePolicy:false, CloseProtocolError:false, CloseTransportTransient:true, CloseReason(99):false}
	for r,want := range cases { if got:=ReconnectAllowed(r); got!=want { t.Fatalf("reason=%d got=%v want=%v",r,got,want) } }
}

func TestBackoffBoundaries(t *testing.T) {
	cases := []struct{attempt uint32; want uint64}{{0,100},{1,200},{2,400},{3,800},{4,1000},{1000,1000}}
	for _,tc := range cases { got,err:=Backoff(tc.attempt,100,1000); if err!=nil || got!=tc.want { t.Fatalf("attempt=%d got=%d err=%v want=%d",tc.attempt,got,err,tc.want) } }
	if _,err:=Backoff(0,0,100); !errors.Is(err,ErrMalformed) { t.Fatalf("zero min err=%v",err) }
	if _,err:=Backoff(0,200,100); !errors.Is(err,ErrMalformed) { t.Fatalf("range err=%v",err) }
	got,err:=Backoff(64,1,^(uint64(0))); if err!=nil || got!=^(uint64(0)) { t.Fatalf("overflow saturation got=%d err=%v",got,err) }
}

package control

import (
	"errors"
	"reflect"
	"testing"
)

func fixed20x20Link() LinkConfig {
	return LinkConfig{
		FECMode: FECFixed, Scheduler: FECSchedulerTailRS,
		DataShards: 20, ParityShards: 20, LaneCount: 1,
		FlushMillis: 8, MTU: 1400,
	}
}

func offLink() LinkConfig {
	return LinkConfig{FECMode: FECOff, Scheduler: FECSchedulerNone, LaneCount: 1, MTU: 1400}
}

func TestLinkWireRoundTrip(t *testing.T) {
	for _, in := range []any{
		LinkInit{MinProtocol: 1, MaxProtocol: 2, Config: fixed20x20Link()},
		LinkAccept{Protocol: 1, AuthRequired: true, Config: offLink()},
		LinkAccept{Protocol: 1, AuthRequired: false, Config: fixed20x20Link()},
	} {
		wire, err := MarshalLink(in)
		if err != nil {
			t.Fatal(err)
		}
		got, err := UnmarshalLink(wire)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, in) {
			t.Fatalf("got %#v want %#v", got, in)
		}
	}
}

func TestCurrentLinkPolicyAdmitsOnlyLiveProfiles(t *testing.T) {
	p := CurrentLinkPolicy()
	if err := p.Validate(offLink()); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(fixed20x20Link()); err != nil {
		t.Fatal(err)
	}

	bad := fixed20x20Link()
	bad.ParityShards = 10
	if err := p.Validate(bad); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("20:10 should remain unsupported by live WBD codec, err=%v", err)
	}
	bad = fixed20x20Link()
	bad.Scheduler = FECSchedulerCausal
	if err := p.Validate(bad); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("causal simulator profile must not be negotiated live, err=%v", err)
	}
	bad = fixed20x20Link()
	bad.LaneCount = 2
	if err := p.Validate(bad); !errors.Is(err, ErrLimit) {
		t.Fatalf("two lanes must remain unsupported, err=%v", err)
	}
}

func TestLinkConfigShapeRejectsOffWithRepairFields(t *testing.T) {
	bad := offLink()
	bad.ParityShards = 1
	if _, err := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: bad}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err=%v", err)
	}
}

func TestLinkServerLocksExactConfigBeforeAuth(t *testing.T) {
	s, err := NewLinkServerSession(1, 1, []byte("secret"), CurrentLinkPolicy())
	if err != nil {
		t.Fatal(err)
	}
	init := LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: fixed20x20Link()}
	wire, _ := MarshalLink(init)
	replyWire, err := s.HandleWire(wire, 1)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := UnmarshalLink(replyWire)
	if err != nil {
		t.Fatal(err)
	}
	accept, ok := reply.(LinkAccept)
	if !ok {
		t.Fatalf("got %#v", reply)
	}
	if !accept.AuthRequired {
		t.Fatal("LINK_ACCEPT did not advertise required authentication")
	}
	if err := ValidateLinkAccept(init, accept); err != nil {
		t.Fatal(err)
	}
	if s.State() != StateAwaitAuth {
		t.Fatalf("state=%v", s.State())
	}
	st := s.Stats()
	if !st.Configured || st.Config != init.Config || st.LastActivity != 1 {
		t.Fatalf("stats=%#v", st)
	}

	authWire, _ := MarshalLink(Auth{Token: []byte("secret")})
	authReplyWire, err := s.HandleWire(authWire, 2)
	if err != nil {
		t.Fatal(err)
	}
	authReply, err := UnmarshalLink(authReplyWire)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := authReply.(AuthOK); !ok {
		t.Fatalf("got %#v", authReply)
	}
	if s.State() != StateEstablished || s.Stats().LastActivity != 2 {
		t.Fatalf("stats=%#v", s.Stats())
	}
}

func TestLinkServerAdvertisesAuthDisabled(t *testing.T) {
	s, err := NewLinkServerSession(1, 1, nil, CurrentLinkPolicy())
	if err != nil {
		t.Fatal(err)
	}
	init := LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: offLink()}
	wire, _ := MarshalLink(init)
	replyWire, err := s.HandleWire(wire, 1)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := UnmarshalLink(replyWire)
	if err != nil {
		t.Fatal(err)
	}
	accept, ok := reply.(LinkAccept)
	if !ok || accept.AuthRequired {
		t.Fatalf("got %#v", reply)
	}
	if s.State() != StateEstablished {
		t.Fatalf("state=%v", s.State())
	}
}

func TestLinkServerRejectedProposalPoisonsAssociation(t *testing.T) {
	s, _ := NewLinkServerSession(1, 1, nil, CurrentLinkPolicy())
	bad := fixed20x20Link()
	bad.ParityShards = 10
	wire, _ := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: bad})
	replyWire, err := s.HandleWire(wire, 1)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := UnmarshalLink(replyWire)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := reply.(Error)
	if !ok || e.Code != ErrorPolicy {
		t.Fatalf("got %#v", reply)
	}
	if s.State() != StateFailed || s.Stats().Configured {
		t.Fatalf("rejected proposal did not poison association: %#v", s.Stats())
	}

	good, _ := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: offLink()})
	retryWire, err := s.HandleWire(good, 2)
	if err != nil {
		t.Fatal(err)
	}
	retry, _ := UnmarshalLink(retryWire)
	if e, ok := retry.(Error); !ok || e.Code != ErrorUnexpectedState {
		t.Fatalf("same association accepted/replied unexpectedly to retry: %#v", retry)
	}
	if s.State() != StateFailed {
		t.Fatalf("state=%v", s.State())
	}
}

func TestLinkServerRequiresLinkInitAsFirstFrame(t *testing.T) {
	s, _ := NewLinkServerSession(1, 1, []byte("secret"), CurrentLinkPolicy())
	wire, _ := MarshalLink(Auth{Token: []byte("secret")})
	replyWire, err := s.HandleWire(wire, 1)
	if err != nil {
		t.Fatal(err)
	}
	reply, _ := UnmarshalLink(replyWire)
	if e, ok := reply.(Error); !ok || e.Code != ErrorUnexpectedState {
		t.Fatalf("got %#v", reply)
	}
	if s.State() != StateFailed {
		t.Fatalf("state=%v", s.State())
	}
}

func TestLinkServerRejectsAnyPostSetupConfigChange(t *testing.T) {
	s, _ := NewLinkServerSession(1, 1, nil, CurrentLinkPolicy())
	init := LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: offLink()}
	wire, _ := MarshalLink(init)
	if _, err := s.HandleWire(wire, 1); err != nil {
		t.Fatal(err)
	}
	if s.State() != StateEstablished {
		t.Fatalf("state=%v", s.State())
	}

	change, _ := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: fixed20x20Link()})
	replyWire, err := s.HandleWire(change, 2)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := UnmarshalLink(replyWire)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := reply.(Error)
	if !ok || e.Code != ErrorUnexpectedState {
		t.Fatalf("got %#v", reply)
	}
	if s.State() != StateFailed || s.Stats().Config != init.Config {
		t.Fatalf("post-setup change did not force reconnect: %#v", s.Stats())
	}
}

func TestValidateLinkAcceptRejectsServerRewrite(t *testing.T) {
	init := LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: fixed20x20Link()}
	accept := LinkAccept{Protocol: 1, AuthRequired: true, Config: fixed20x20Link()}
	if err := ValidateLinkAccept(init, accept); err != nil {
		t.Fatal(err)
	}
	accept.Config.MTU = 1300
	if err := ValidateLinkAccept(init, accept); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err=%v", err)
	}
}

package control

import (
	"errors"
	"testing"
)

func demoWitness(v byte) [DemoWitnessLen]byte {
	var out [DemoWitnessLen]byte
	for i := range out {
		out[i] = v + byte(i)
	}
	return out
}

func TestDemoFramesRoundTrip(t *testing.T) {
	w := demoWitness(1)
	for _, frame := range []any{DemoBind{Witness: w}, DemoBindOK{Witness: w}} {
		wire, err := MarshalDemo(frame)
		if err != nil {
			t.Fatal(err)
		}
		got, err := UnmarshalDemo(wire)
		if err != nil {
			t.Fatal(err)
		}
		switch want := frame.(type) {
		case DemoBind:
			if got != want {
				t.Fatalf("got=%#v want=%#v", got, want)
			}
		case DemoBindOK:
			if got != want {
				t.Fatalf("got=%#v want=%#v", got, want)
			}
		}
	}
}

func TestDemoBoundStartupRequiresWitnessBeforeLinkInit(t *testing.T) {
	cfg := LinkConfig{FECMode: FECOff, Scheduler: FECSchedulerNone, LaneCount: 1, MTU: 1400}
	w := demoWitness(7)
	client, err := NewDemoLinkClientSession(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg}, []byte("secret"), w)
	if err != nil {
		t.Fatal(err)
	}
	verified := 0
	server, err := NewDemoReliableLinkServerSession(1, 1, []byte("secret"), CurrentLinkPolicy(), func(got [DemoWitnessLen]byte) error {
		verified++
		if got != w {
			t.Fatalf("verify got=%x want=%x", got, w)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	bind, err := client.RetryWire()
	if err != nil {
		t.Fatal(err)
	}
	if f, err := UnmarshalDemo(bind); err != nil {
		t.Fatal(err)
	} else if _, ok := f.(DemoBind); !ok {
		t.Fatalf("first client frame=%T want DemoBind", f)
	}
	bindOK, err := server.HandleWire(bind, 1)
	if err != nil {
		t.Fatal(err)
	}
	linkInit, err := client.HandleWire(bindOK)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := UnmarshalLink(linkInit); err != nil {
		t.Fatal(err)
	} else if _, ok := f.(LinkInit); !ok {
		t.Fatalf("after bind=%T want LinkInit", f)
	}
	accept, err := server.HandleWire(linkInit, 2)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := client.HandleWire(accept)
	if err != nil {
		t.Fatal(err)
	}
	if len(auth) == 0 {
		t.Fatal("expected encrypted AUTH startup datagram")
	}
	authOK, err := server.HandleWire(auth, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.HandleWire(authOK); err != nil {
		t.Fatal(err)
	}
	if !client.Established() || server.State() != StateEstablished {
		t.Fatalf("client established=%v server=%v", client.Established(), server.State())
	}
	if verified != 1 {
		t.Fatalf("witness verifier calls=%d want 1", verified)
	}
}

func TestDemoBindRetryIsIdempotentButChangeFails(t *testing.T) {
	w := demoWitness(11)
	server, err := NewDemoReliableLinkServerSession(1, 1, nil, CurrentLinkPolicy(), func([DemoWitnessLen]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := MarshalDemo(DemoBind{Witness: w})
	first, err := server.HandleWire(wire, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.HandleWire(wire, 2)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("retry did not return byte-identical DEMO_BIND_OK")
	}
	other, _ := MarshalDemo(DemoBind{Witness: demoWitness(12)})
	if _, err := server.HandleWire(other, 3); err != nil {
		t.Fatal(err)
	}
	if server.State() != StateFailed {
		t.Fatalf("changed witness state=%v want failed", server.State())
	}
}

func TestDemoBindRejectsMissingOrInvalidWitness(t *testing.T) {
	cfg := LinkConfig{FECMode: FECOff, Scheduler: FECSchedulerNone, LaneCount: 1, MTU: 1400}
	server, err := NewDemoReliableLinkServerSession(1, 1, nil, CurrentLinkPolicy(), func([DemoWitnessLen]byte) error { return errors.New("expired") })
	if err != nil {
		t.Fatal(err)
	}
	link, _ := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg})
	wire, err := server.HandleWire(link, 1)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := UnmarshalLink(wire); err != nil {
		t.Fatal(err)
	} else if _, ok := f.(Error); !ok {
		t.Fatalf("missing bind reply=%T", f)
	}

	server, _ = NewDemoReliableLinkServerSession(1, 1, nil, CurrentLinkPolicy(), func([DemoWitnessLen]byte) error { return errors.New("expired") })
	bind, _ := MarshalDemo(DemoBind{Witness: demoWitness(9)})
	wire, err = server.HandleWire(bind, 2)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := UnmarshalLink(wire); err != nil {
		t.Fatal(err)
	} else if e, ok := f.(Error); !ok || e.Code != ErrorAuthFailed {
		t.Fatalf("rejected bind reply=%#v", f)
	}
}

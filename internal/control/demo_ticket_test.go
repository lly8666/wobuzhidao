package control

import "testing"

func TestDemoTicketStartupDoesNotRepeatAccountPasswordOrBearer(t *testing.T) {
	cfg := LinkConfig{FECMode: FECOff, Scheduler: FECSchedulerNone, LaneCount: 1, MTU: 1400}
	ticket := demoWitness(51)
	client, err := NewDemoTicketLinkClientSession(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg}, ticket)
	if err != nil {
		t.Fatal(err)
	}
	verified := 0
	server, err := NewDemoTicketReliableLinkServerSession(1, 1, CurrentLinkPolicy(), func(got [DemoWitnessLen]byte) error {
		verified++
		if got != ticket {
			t.Fatalf("ticket=%x want=%x", got, ticket)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	bind, _ := client.RetryWire()
	bindOK, err := server.HandleWire(bind, 1)
	if err != nil {
		t.Fatal(err)
	}
	linkInit, err := client.HandleWire(bindOK)
	if err != nil {
		t.Fatal(err)
	}
	accept, err := server.HandleWire(linkInit, 2)
	if err != nil {
		t.Fatal(err)
	}
	next, err := client.HandleWire(accept)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 0 {
		t.Fatal("ticket-authenticated startup unexpectedly requested bearer AUTH")
	}
	if !client.Established() || server.State() != StateEstablished || verified != 1 {
		t.Fatalf("client=%v server=%v verified=%d", client.Established(), server.State(), verified)
	}
}

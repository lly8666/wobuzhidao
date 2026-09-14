package control

import (
	"fmt"
	"sync"
	"testing"
)

func testOffLinkConfig(mtu uint16) LinkConfig {
	return LinkConfig{
		FECMode:   FECOff,
		Scheduler: FECSchedulerNone,
		LaneCount: 1,
		MTU:       mtu,
	}
}

func TestLinkConfigValidatorRejectsBeforeAccept(t *testing.T) {
	cfg := testOffLinkConfig(1201)
	calls := 0
	server, err := NewLinkServerSessionWithValidator(1, 1, nil, CurrentLinkPolicy(), func(got LinkConfig) error {
		calls++
		if got != cfg {
			return fmt.Errorf("validator got config %#v want %#v", got, cfg)
		}
		return fmt.Errorf("%w: carrier ceiling exceeded", ErrLimit)
	})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	replyWire, err := server.HandleWire(wire, 1)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := UnmarshalLink(replyWire)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reply.(LinkAccept); ok {
		t.Fatal("carrier-invalid LINK_INIT was accepted")
	}
	policyErr, ok := reply.(Error)
	if !ok || policyErr.Code != ErrorPolicy {
		t.Fatalf("reply=%T %#v want ErrorPolicy", reply, reply)
	}
	if calls != 1 {
		t.Fatalf("validator calls=%d want=1", calls)
	}
	if server.State() != StateFailed {
		t.Fatalf("state=%v want failed", server.State())
	}
	if server.Stats().Configured {
		t.Fatal("rejected LINK_INIT became configured")
	}
}

func TestReliableLinkStartupDifferentMTUsWithThirtyPercentLossConcurrent(t *testing.T) {
	mtus := []uint16{616, 940, 1240, 1372}
	var wg sync.WaitGroup
	errCh := make(chan error, len(mtus))
	for i, mtu := range mtus {
		i, mtu := i, mtu
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runLossyReliableStartup(mtu, i); err != nil {
				errCh <- fmt.Errorf("mtu=%d: %w", mtu, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestReliableLinkStartupOverCeilingNeverAcceptsWithThirtyPercentLoss(t *testing.T) {
	const ceiling uint16 = 1372
	validator := func(cfg LinkConfig) error {
		if cfg.MTU > ceiling {
			return fmt.Errorf("%w: mtu %d exceeds carrier ceiling %d", ErrLimit, cfg.MTU, ceiling)
		}
		return nil
	}
	server, err := NewReliableLinkServerSessionWithValidator(1, 1, nil, CurrentLinkPolicy(), validator)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewLinkClientSession(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: testOffLinkConfig(ceiling + 1)}, nil)
	if err != nil {
		t.Fatal(err)
	}

	accepted := false
	clientTX, serverTX := 0, 0
	for round := 0; round < 40 && client.State() != LinkClientFailed; round++ {
		wire, err := client.RetryWire()
		if err != nil {
			break
		}
		clientTX++
		if dropThirtyPercent(clientTX, 2) {
			continue
		}
		reply, err := server.HandleWire(wire, uint64(round+1))
		if err != nil {
			continue
		}
		if frame, err := UnmarshalLink(reply); err == nil {
			if _, ok := frame.(LinkAccept); ok {
				accepted = true
			}
		}
		serverTX++
		if dropThirtyPercent(serverTX, 5) {
			continue
		}
		_, _ = client.HandleWire(reply)
	}
	if accepted {
		t.Fatal("over-ceiling config produced LINK_ACCEPT under loss/retry")
	}
	if server.State() != StateFailed {
		t.Fatalf("server state=%v want failed", server.State())
	}
	if server.Stats().Configured {
		t.Fatal("over-ceiling config became configured")
	}
	if client.Established() {
		t.Fatal("over-ceiling client established")
	}
}

func runLossyReliableStartup(mtu uint16, seed int) error {
	const ceiling uint16 = 1372
	validator := func(cfg LinkConfig) error {
		if cfg.MTU > ceiling {
			return fmt.Errorf("%w: mtu %d exceeds carrier ceiling %d", ErrLimit, cfg.MTU, ceiling)
		}
		return nil
	}
	server, err := NewReliableLinkServerSessionWithValidator(1, 1, nil, CurrentLinkPolicy(), validator)
	if err != nil {
		return err
	}
	client, err := NewLinkClientSession(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: testOffLinkConfig(mtu)}, nil)
	if err != nil {
		return err
	}
	clientTX, serverTX := 0, 0
	for round := 0; round < 80; round++ {
		if client.Established() && server.State() == StateEstablished {
			accept, ok := client.Accept()
			if !ok || accept.Config.MTU != mtu {
				return fmt.Errorf("accepted mtu=%d ok=%v want=%d", accept.Config.MTU, ok, mtu)
			}
			return nil
		}
		wire, err := client.RetryWire()
		if err != nil {
			return err
		}
		if len(wire) == 0 {
			continue
		}
		clientTX++
		if dropThirtyPercent(clientTX, seed) {
			continue
		}
		reply, err := server.HandleWire(wire, uint64(round+1))
		if err != nil {
			return err
		}
		if len(reply) == 0 {
			continue
		}
		serverTX++
		if dropThirtyPercent(serverTX, seed+3) {
			continue
		}
		if _, err := client.HandleWire(reply); err != nil {
			return err
		}
	}
	return fmt.Errorf("startup did not converge: client=%v server=%v client_tx=%d server_tx=%d", client.State(), server.State(), clientTX, serverTX)
}

func dropThirtyPercent(sequence, seed int) bool {
	switch (sequence + seed) % 10 {
	case 1, 4, 8:
		return true
	default:
		return false
	}
}

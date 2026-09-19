package control

import (
	"errors"
	"testing"
)

func TestReliableStartupNoAuth(t *testing.T) {
	init := LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: offLink()}
	c, err := NewLinkClientSession(init, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewReliableLinkServerSession(1, 1, nil, CurrentLinkPolicy())
	if err != nil {
		t.Fatal(err)
	}

	start, _ := c.RetryWire()
	acceptWire, err := s.HandleWire(start, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.HandleWire(acceptWire); err != nil {
		t.Fatal(err)
	}
	if !c.Established() || s.State() != StateEstablished {
		t.Fatalf("client=%d server=%d", c.State(), s.State())
	}
}

func TestReliableStartupAuthAndLostResponses(t *testing.T) {
	init := LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: fixed20x20Link()}
	c, err := NewLinkClientSession(init, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewReliableLinkServerSession(1, 1, []byte("secret"), CurrentLinkPolicy())
	if err != nil {
		t.Fatal(err)
	}

	// First LINK_ACCEPT is "lost". Exact LINK_INIT retry must be idempotent.
	start1, _ := c.RetryWire()
	accept1, err := s.HandleWire(start1, 1)
	if err != nil {
		t.Fatal(err)
	}
	start2, _ := c.RetryWire()
	accept2, err := s.HandleWire(start2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if string(accept1) != string(accept2) {
		t.Fatal("duplicate LINK_INIT did not return byte-identical LINK_ACCEPT")
	}

	authWire, err := c.HandleWire(accept2)
	if err != nil {
		t.Fatal(err)
	}
	if c.State() != LinkClientAwaitAuthOK {
		t.Fatalf("state=%d", c.State())
	}

	// First AUTH_OK is lost. Repeating the exact AUTH must be idempotent even
	// though the server already entered Established.
	authOK1, err := s.HandleWire(authWire, 3)
	if err != nil {
		t.Fatal(err)
	}
	authRetry, _ := c.RetryWire()
	authOK2, err := s.HandleWire(authRetry, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(authOK1) != string(authOK2) {
		t.Fatal("duplicate AUTH did not return byte-identical AUTH_OK")
	}
	if _, err := c.HandleWire(authOK2); err != nil {
		t.Fatal(err)
	}
	if !c.Established() || s.State() != StateEstablished {
		t.Fatalf("client=%d server=%d", c.State(), s.State())
	}
}

func TestReliableStartupDifferentRetryRequiresReconnect(t *testing.T) {
	s, err := NewReliableLinkServerSession(1, 1, []byte("secret"), CurrentLinkPolicy())
	if err != nil {
		t.Fatal(err)
	}
	first, _ := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: fixed20x20Link()})
	if _, err := s.HandleWire(first, 1); err != nil {
		t.Fatal(err)
	}
	changed, _ := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: offLink()})
	replyWire, err := s.HandleWire(changed, 2)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := UnmarshalLink(replyWire)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := reply.(Error); !ok || e.Code != ErrorUnexpectedState {
		t.Fatalf("got %#v", reply)
	}
	if s.State() != StateFailed {
		t.Fatalf("state=%d", s.State())
	}
}

func TestClientFailsWhenAuthRequiredWithoutToken(t *testing.T) {
	init := LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: offLink()}
	c, _ := NewLinkClientSession(init, nil)
	acceptWire, _ := MarshalLink(LinkAccept{Protocol: 1, AuthRequired: true, Config: offLink()})
	if _, err := c.HandleWire(acceptWire); !errors.Is(err, ErrLinkStartupFailed) {
		t.Fatalf("err=%v", err)
	}
	if c.State() != LinkClientFailed {
		t.Fatalf("state=%d", c.State())
	}
}

func TestClientDuplicateAcceptResendsSameAuth(t *testing.T) {
	init := LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: fixed20x20Link()}
	c, _ := NewLinkClientSession(init, []byte("secret"))
	acceptWire, _ := MarshalLink(LinkAccept{Protocol: 1, AuthRequired: true, Config: fixed20x20Link()})
	auth1, err := c.HandleWire(acceptWire)
	if err != nil {
		t.Fatal(err)
	}
	auth2, err := c.HandleWire(acceptWire)
	if err != nil {
		t.Fatal(err)
	}
	if string(auth1) != string(auth2) {
		t.Fatal("duplicate LINK_ACCEPT changed AUTH bytes")
	}
}

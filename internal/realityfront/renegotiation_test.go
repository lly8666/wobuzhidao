package realityfront

import (
	"bytes"
	"net"
	"testing"

	utls "github.com/refraction-networking/utls"
)

func TestFirefox120KeepsRenegotiationExtensionBytesButDisablesPolicy(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	defer clientConn.Close()
	defer peerConn.Close()

	got, err := newFirefox120Client(clientConn, ClientConfig{
		ServerName: "target.test",
		RouteKey:   []byte("0123456789abcdef0123456789abcdef"),
	})
	if err != nil {
		t.Fatal(err)
	}

	refConn, refPeer := net.Pipe()
	defer refConn.Close()
	defer refPeer.Close()
	ref := utls.UClient(
		refConn,
		&utls.Config{ServerName: "target.test", InsecureSkipVerify: true},
		utls.HelloFirefox_120,
	)
	if err := ref.BuildHandshakeState(); err != nil {
		t.Fatal(err)
	}

	var gotReneg, refReneg *utls.RenegotiationInfoExtension
	for _, ext := range got.Extensions {
		if v, ok := ext.(*utls.RenegotiationInfoExtension); ok {
			gotReneg = v
		}
	}
	for _, ext := range ref.Extensions {
		if v, ok := ext.(*utls.RenegotiationInfoExtension); ok {
			refReneg = v
		}
	}
	if gotReneg == nil || refReneg == nil {
		t.Fatal("Firefox120 renegotiation_info extension missing")
	}
	if gotReneg.Renegotiation != utls.RenegotiateNever {
		t.Fatalf("got policy=%v want RenegotiateNever", gotReneg.Renegotiation)
	}
	if refReneg.Renegotiation != utls.RenegotiateOnceAsClient {
		t.Fatalf("reference policy=%v want RenegotiateOnceAsClient", refReneg.Renegotiation)
	}

	gotWire := make([]byte, gotReneg.Len())
	refWire := make([]byte, refReneg.Len())
	_, _ = gotReneg.Read(gotWire)
	_, _ = refReneg.Read(refWire)
	if !bytes.Equal(gotWire, refWire) {
		t.Fatalf("renegotiation_info wire changed: got=%x ref=%x", gotWire, refWire)
	}
}

package windowsclient

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
)

func n4NativeUDPTestRequest(t *testing.T) ([]byte,netip.Addr,netip.Addr) {
	t.Helper()
	lease:=netip.MustParseAddr("10.66.0.2")
	remote:=netip.MustParseAddr("10.3.4.5")
	p,err:=makeDirectUDPResponse(lease,remote,41000,443,[]byte{1,2,3,4,5},0x1234,1249)
	if err!=nil {t.Fatal(err)}
	return p,lease,remote
}

func TestN4NativeUDPValidTupleAndBytePerfectReturn(t *testing.T) {
	p,lease,remote:=n4NativeUDPTestRequest(t)
	original:=append([]byte(nil),p...)
	d,err:=parseDirectUDPIPv4(p,lease,1249)
	if err!=nil {t.Fatal(err)}
	if d.remote!=netip.AddrPortFrom(remote,443)||d.sourcePort!=41000||
		!bytes.Equal(d.payload,[]byte{1,2,3,4,5})||d.dontFragment {
		t.Fatalf("wrong parsed native UDP tuple: %+v",d)
	}
	resp,err:=makeDirectUDPResponse(remote,lease,443,41000,[]byte{8,7,6},0x5678,1249)
	if err!=nil {t.Fatal(err)}
	if packetChecksum(resp[:20])!=0 || udpChecksum(remote,lease,resp[20:])!=0 {
		t.Fatal("invalid generated IPv4/UDP checksum")
	}
	if got:=binary.BigEndian.Uint16(resp[20:22]);got!=443 {t.Fatalf("reply sport=%d",got)}
	if got:=binary.BigEndian.Uint16(resp[22:24]);got!=41000 {t.Fatalf("reply dport=%d",got)}
	if !bytes.Equal(resp[28:],[]byte{8,7,6}) || !bytes.Equal(p,original) {
		t.Fatal("response tuple/payload or caller request buffer corrupted")
	}
}

func TestN4NativeUDPRejectsSpoofBadLengthsChecksumsAndFragments(t *testing.T) {
	p,lease,_:=n4NativeUDPTestRequest(t)
	bad:=func(f func([]byte)) []byte {
		x:=append([]byte(nil),p...)
		f(x)
		return x
	}
	cases:=[]struct{name string;p []byte;scope netip.Addr;want error}{
		{"spoof",p,netip.MustParseAddr("10.66.0.3"),ErrN4UDPInvalid},
		{"IPv4 checksum",bad(func(x []byte){x[8]++}),lease,ErrN4UDPInvalid},
		{"UDP checksum",bad(func(x []byte){x[28]^=0x55}),lease,ErrN4UDPInvalid},
		{"UDP length",bad(func(x []byte){binary.BigEndian.PutUint16(x[24:26],100)}),lease,ErrN4UDPInvalid},
		{"IPv4 length",bad(func(x []byte){binary.BigEndian.PutUint16(x[2:4],1)}),lease,ErrN4UDPInvalid},
		{"zero port",bad(func(x []byte){x[20]=0;x[21]=0}),lease,ErrN4UDPInvalid},
		{"not UDP",bad(func(x []byte){x[9]=6}),lease,ErrN4UDPInvalid},
		{"first fragment",bad(func(x []byte){binary.BigEndian.PutUint16(x[6:8],0x2000);binary.BigEndian.PutUint16(x[10:12],0);binary.BigEndian.PutUint16(x[10:12],packetChecksum(x[:20]))}),lease,ErrN4UDPUnsupported},
		{"late fragment",bad(func(x []byte){binary.BigEndian.PutUint16(x[6:8],1);binary.BigEndian.PutUint16(x[10:12],0);binary.BigEndian.PutUint16(x[10:12],packetChecksum(x[:20]))}),lease,ErrN4UDPUnsupported},
	}
	for _,tc:=range cases {
		t.Run(tc.name,func(t *testing.T){
			if _,err:=parseDirectUDPIPv4(tc.p,tc.scope,1249);!errors.Is(err,tc.want) {
				t.Fatalf("got %v want %v",err,tc.want)
			}
		})
	}
}

func TestN4NativeUDPDFAndMTUHardLimit(t *testing.T) {
	p,lease,remote:=n4NativeUDPTestRequest(t)
	binary.BigEndian.PutUint16(p[6:8],0x4000)
	binary.BigEndian.PutUint16(p[10:12],0)
	binary.BigEndian.PutUint16(p[10:12],packetChecksum(p[:20]))
	d,err:=parseDirectUDPIPv4(p,lease,1249)
	if err!=nil || !d.dontFragment {t.Fatalf("DF lost err=%v",err)}
	if _,err:=parseDirectUDPIPv4(p,lease,len(p)-1);!errors.Is(err,ErrN4UDPInvalid) {
		t.Fatalf("TUN MTU not enforced: %v",err)
	}
	if _,err:=makeDirectUDPResponse(remote,lease,443,41000,make([]byte,1222),1,1249);
		!errors.Is(err,ErrN4UDPUnsupported) {t.Fatalf("oversize packet accepted: %v",err)}
}

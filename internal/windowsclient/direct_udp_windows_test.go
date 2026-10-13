//go:build windows

package windowsclient

import (
	"errors"
	"net/netip"
	"testing"
)

func TestN4NativeUDPWindowsConstructorAndGenerationCloseFence(t *testing.T) {
	lease:=netip.MustParseAddr("10.66.0.2")
	physical:=netip.MustParseAddr("192.0.2.2")
	writer:=&fakeWriter{}
	for _,tc:=range []struct{name string;lease,physical netip.Addr;iface uint32;generation uint64;mtu int}{
		{"no iface",lease,physical,0,5,1249},
		{"no generation",lease,physical,12,0,1249},
		{"source tunnel leak",lease,lease,12,5,1249},
		{"MTU oversize",lease,physical,12,5,9001},
		{"MTU missing",lease,physical,12,5,0},
	}{
		t.Run(tc.name,func(t *testing.T){
			if _,err:=NewNativeUDPDirect(tc.lease,tc.physical,tc.iface,tc.generation,tc.mtu,writer);!errors.Is(err,ErrInvalidBinding) {
				t.Fatalf("unsafe source/interface accepted: %v",err)
			}
		})
	}
	backend,err:=NewNativeUDPDirect(lease,physical,12,5,1249,writer)
	if err!=nil {t.Fatal(err)}
	p,_,_:=n4NativeUDPTestRequest(t)
	if err:=backend.ForwardDirectIPv4(p,11,5);!errors.Is(err,ErrTUNFlowScope) {
		t.Fatalf("wrong physical interface accepted: %v",err)
	}
	if err:=backend.ForwardDirectIPv4(p,12,6);!errors.Is(err,ErrTUNFlowScope) {
		t.Fatalf("retired generation accepted: %v",err)
	}
	if err:=backend.Close();err!=nil {t.Fatal(err)}
	if err:=backend.Close();err!=nil {t.Fatal(err)}
	if err:=backend.ForwardDirectIPv4(p,12,5);!errors.Is(err,ErrTUNDirectBackendUnavailable) {
		t.Fatalf("closed backend accepted packet: %v",err)
	}
	if backend.FlowCount()!=0 {t.Fatalf("owned flow count after Close=%d",backend.FlowCount())}
}

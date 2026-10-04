package realityfront

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"
)

func TestLeaseAllocationRunsAfterCredentialsOnly(t *testing.T) {
	cert:=makeServerCert(t);key:=[]byte("0123456789abcdef0123456789abcdef")
	assoc,peer:=newAssociationPeer(t);defer peer.Close();defer assoc.Close()
	called:=false;done:=make(chan error,1)
	go func(){_,err:=EstablishServer(context.Background(),assoc,ServerAdmissionConfig{TLS:ServerConfig{ServerName:"target.test",RouteKey:key,TLSConfig:&tls.Config{Certificates:[]tls.Certificate{cert}},Timeout:3*time.Second},ExpectedUsername:"shared",ExpectedPassword:"correct",ServerLimit:1250,AllocateLease:func(AdmissionRequest)(string,error){called=true;return "10.66.0.2/32",nil}});done<-err}()
	_,err:=EstablishClient(context.Background(),peer,ClientAdmissionConfig{AutoLease:true,DesiredLanes:1,InstallationID:[]byte("installation-001"),TunnelID:[]byte("0123456789abcdef"),TLS:ClientConfig{ServerName:"target.test",RouteKey:key,Timeout:3*time.Second},Username:"shared",Password:"wrong",ClientLimit:1300})
	if !errors.Is(err,ErrAdmissionAuth){t.Fatal(err)}
	if err:=<-done;!errors.Is(err,ErrAdmissionAuth){t.Fatal(err)};if called{t.Fatal("unauthenticated request allocated an address")}
}

func TestLeaseExtensionRoundTripAndLegacyLengths(t *testing.T) {
	req:=AdmissionRequest{RecordVersion:RecordVersionV2,LaneID:3,ClientLimit:1300,TunnelID:[]byte("0123456789abcdef"),Username:"shared",Password:"password"}
	legacy,err:=marshalAdmissionRequest(req);if err!=nil{t.Fatal(err)}
	req.AutoLease=true;req.DesiredLanes=4;req.InstallationID=[]byte("installation-001")
	wire,err:=marshalAdmissionRequest(req);if err!=nil{t.Fatal(err)};if len(wire)!=len(legacy)+17{t.Fatal("extension size")}
	parsed,err:=readAdmissionRequest(bytes.NewReader(wire));if err!=nil || !parsed.AutoLease || parsed.DesiredLanes!=4 || !bytes.Equal(req.InstallationID,parsed.InstallationID){t.Fatal("bad request",err)}
	reply,err:=marshalAdmissionReply(AdmissionResult{RecordVersion:RecordVersionV2,LaneID:3,ClientLimit:1300,ServerLimit:1250,TunnelID:req.TunnelID,Lease4:"10.66.0.9/32"});if err!=nil{t.Fatal(err)}
	got,err:=readAdmissionReply(bytes.NewReader(reply),req);if err!=nil || got.Lease4!="10.66.0.9/32"{t.Fatal("bad reply",err)}
	if _,err:=readAdmissionReply(bytes.NewReader(reply[:len(reply)-1]),req);err==nil{t.Fatal("truncated lease accepted")}
	req.DesiredLanes=2;if _,err:=marshalAdmissionRequest(req);err==nil{t.Fatal("lane exceeds declared mode")}
}

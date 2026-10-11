package runtimeowner

import (
	"errors"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func TestN2ProtectedPolicyControllerBindingIsExactPerIncarnation(t *testing.T) {
	// Separate runtime owners emulate independent accepted customer Tunnels;
	// their lane ID may both be 1, but nonce/mode/min MUST NOT leak.
	for _,tc:=range []struct{mode,min,seed uint8}{
		{realityfront.FECAuto,0,73},
		{realityfront.FECAggressive,4,74},
	} {
		lease:=runtimeLease(t)
		owner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
		if err!=nil {t.Fatal(err)}
		r,err:=New(owner,nil)
		if err!=nil {t.Fatal(err)}
		p:=realityfront.AdmissionPolicy{
			Schema:realityfront.PolicySchemaV3,
			Transport:realityfront.TransportNormal,
			DesiredLanes:1,FECMode:tc.mode,
			InitialParity:20,MinParity:tc.min,MaxParity:20,
			Cipher:realityfront.CipherChaCha,Quality:1,
		}
		if err:=p.SupportedNow();err!=nil {
			t.Fatalf("protected Normal auto policy still unsupported: %v",err)
		}
		var nonce [16]byte
		nonce[0]=tc.seed
		negotiated:=realityfront.AdmissionResult{
			RecordVersion:realityfront.RecordVersionV3,
			Policy:p,ClientLimit:1300,ServerLimit:1250,
			TunnelID:lease.Config.TunnelID.Bytes(),
			Keys:runtimeKeys(),IncarnationNonce:nonce,
		}
		lane,_,err:=datapath.N2ClientLaneFromProtectedPolicyForTests(
			&realityfront.ClientAdmissionSession{Negotiated:negotiated},
			datapath.ClientLaneParams{ConnectionMTU:1500,PeerMSS:1400,PeerMSSSet:true})
		if err!=nil {t.Fatal(err)}
		cCfg,_:=transportPair(func(faketcp.Segment)error{return nil},
			func(faketcp.Segment)error{return nil},1,33300+uint32(tc.seed))
		snap,err:=r.AttachInitial(1,lane,cCfg)
		if err!=nil {t.Fatal(err)}
		if err:=r.ConfigureN2ProtectedPolicyForTests(snap.Ref,negotiated);!errors.Is(err,ErrTransportConfig) {
			t.Fatalf("unconfigured quality accepted controller: %v",err)
		}
		if err:=r.ConfigureQualityV3(snap.Ref,nonce);err!=nil {t.Fatal(err)}
		foreign:=negotiated
		foreign.Policy.MinParity=8
		if err:=r.ConfigureN2ProtectedPolicyForTests(snap.Ref,foreign);!errors.Is(err,datapath.ErrTunnelMismatch) {
			t.Fatalf("other customer's policy used on local incarnation: %v",err)
		}
		foreign=negotiated
		foreign.IncarnationNonce[0]++
		if err:=r.ConfigureN2ProtectedPolicyForTests(snap.Ref,foreign);!errors.Is(err,ErrTransportConfig) {
			t.Fatalf("other incarnation nonce bound to quality: %v",err)
		}
		if err:=r.ConfigureN2ProtectedPolicyForTests(snap.Ref,negotiated);err!=nil {
			t.Fatalf("exact protected policy rejected: %v",err)
		}
		if err:=r.ConfigureN2ProtectedPolicyForTests(snap.Ref,negotiated);!errors.Is(err,ErrTransportConfig) {
			t.Fatalf("second controller replaced first: %v",err)
		}
		if _,ok:=r.N2ProposalForTests(snap.Ref);ok {
			t.Fatal("unobserved line quality manufactured a decision")
		}
		if _,err:=r.Dormant();err!=nil {t.Fatal(err)}
		if err:=r.ConfigureN2ProtectedPolicyForTests(snap.Ref,negotiated);err==nil {
			t.Fatal("retired generation accepted controller after dormant")
		}
		r.Close()
	}
}

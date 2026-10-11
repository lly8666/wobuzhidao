package datapath

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func n2V3CandidatePolicy(mode uint8,min uint8) realityfront.AdmissionPolicy {
	return realityfront.AdmissionPolicy{
		Schema:realityfront.PolicySchemaV3,
		Transport:realityfront.TransportNormal,
		DesiredLanes:1,FECMode:mode,
		InitialParity:20,MinParity:min,MaxParity:20,
		Cipher:realityfront.CipherChaCha,Quality:1,
	}
}

func TestN2ProtectedPerClientPolicyCandidatePreservesWireBudgetAndLateMixedDelivery(t *testing.T) {
	for _,tc:=range []struct{mode,min uint8; advice adaptivefec.Mode}{
		{realityfront.FECAuto,0,adaptivefec.Auto},
		{realityfront.FECAggressive,4,adaptivefec.Aggressive},
	} {
		policy:=n2V3CandidatePolicy(tc.mode,tc.min)
		if err:=policy.SupportedNow();!errors.Is(err,realityfront.ErrAdmissionUnsupported) {
			t.Fatalf("unqualified auto inadvertently advertised %v",err)
		}
		var nonce [16]byte
		nonce[0]=tc.mode
		original:=realityfront.AdmissionResult{
			RecordVersion:realityfront.RecordVersionV3,
			Policy:policy,ClientLimit:1300,ServerLimit:1250,
			IncarnationNonce:nonce,Keys:testKeys(),
			TunnelID:[]byte("0123456789abcdef"),
		}
		cl:=&realityfront.ClientAdmissionSession{Negotiated:original}
		sl:=&realityfront.ServerAdmissionSession{Negotiated:original}
		syn:=faketcp.Segment{
			SrcIP:[4]byte{192,0,2,5},DstIP:[4]byte{198,51,100,10},
			SrcPort:40000, DstPort:443,
			Seq:80,Flags:faketcp.FlagSYN,Window:65535,
			MSS:1400,MSSSet:true,
		}
		assoc,err:=faketcp.NewServerAssociation(syn,9000,
			100*time.Millisecond,func(faketcp.Segment)error{return nil})
		if err!=nil{t.Fatal(err)}
		paramsC:=ClientLaneParams{ConnectionMTU:1500,PeerMSS:1400,PeerMSSSet:true}
		paramsS:=ServerLaneParams{ConnectionMTU:1500}
		client,cc,err:=N2ClientLaneFromProtectedPolicyForTests(cl,paramsC)
		if err!=nil{assoc.Close();t.Fatal(err)}
		server,sc,err:=N2ServerLaneFromProtectedPolicyForTests(sl,assoc,paramsS)
		if err!=nil{client.Close();assoc.Close();t.Fatal(err)}
		if cc.Mode!=tc.advice||sc.Mode!=tc.advice||cc.MinParity!=int(tc.min)||
			sc.MinParity!=int(tc.min)||cc.InitialParity!=20||sc.MaxParity!=20 {
			t.Fatalf("per-client controller mismatch C=%+v S=%+v",cc,sc)
		}
		if original.Policy!=cl.Negotiated.Policy||
			original.Policy!=sl.Negotiated.Policy {
			t.Fatal("protected negotiated policy replaced by local MTU shadow")
		}
		if client.TxBudget().ParityShards!=20 || server.RxBudget().ParityShards!=20 ||
			client.TxBudget().RecordWireMTU!=1250 || server.TxBudget().RecordWireMTU!=1300 ||
			client.RxBudget().RecordWireMTU!=1300 {
			t.Fatalf("max geometry or direction lost Ctx=%+v Crx=%+v Stx=%+v",
				client.TxBudget(),client.RxBudget(),server.TxBudget())
		}
		initialBudget:=client.TxBudget()
		at:=time.Unix(81000,0)
		originalWire,err:=client.Outbound([]byte("old-first-lost"),at)
		if err!=nil||len(originalWire)!=1||!originalWire[0].FEC.Enabled {t.Fatalf("initial systematic %v",err)}
		if err:=client.SwitchN2ParityForTests(0);err!=nil{t.Fatal(err)}
		fresh,err:=client.Outbound([]byte("new-off-first"),at.Add(time.Millisecond))
		if err!=nil||len(fresh)!=1||fresh[0].FEC.Enabled {t.Fatalf("off fresh blocked %v",err)}
		received,err:=server.InboundPayload(fresh[0].Wire,at.Add(time.Millisecond))
		if err!=nil||len(received.RecordErrors)!=0||len(received.PathErrors)!=0||
			len(received.Datagrams)!=1||!bytes.Equal(received.Datagrams[0],[]byte("new-off-first")) {
			t.Fatalf("new FEC-off failed immediate authenticated delivery %+v %v",received,err)
		}
		late,err:=client.FlushDue(at.Add(32*time.Millisecond))
		if err!=nil||len(late)!=1||late[0].FEC.ConfiguredParity!=20 {
			t.Fatalf("old original deadline/geometry lost %d %v",len(late),err)
		}
		recovered,err:=server.InboundPayload(late[0].Wire,at.Add(32*time.Millisecond))
		if err!=nil||len(recovered.Datagrams)!=1||
			!bytes.Equal(recovered.Datagrams[0],[]byte("old-first-lost")) {
			t.Fatalf("delayed parity did not first-deliver old group %+v %v",recovered,err)
		}
		if next:=client.TxBudget();next!=initialBudget {
			t.Fatalf("switch changed negotiated max FEC MTU %+v -> %+v",initialBudget,next)
		}
		client.Close();server.Close();assoc.Close()
	}
}

func TestN2ProtectedAutoCandidateRejectsGameMissingQualityAndUnimplementedCipher(t *testing.T) {
	base:=n2V3CandidatePolicy(realityfront.FECAuto,4)
	for _,modify:=range []func(*realityfront.AdmissionPolicy){
		func(p *realityfront.AdmissionPolicy){p.Transport=realityfront.TransportGame;p.DesiredLanes=4},
		func(p *realityfront.AdmissionPolicy){p.Quality=0},
		func(p *realityfront.AdmissionPolicy){p.Cipher=realityfront.CipherAES128},
		func(p *realityfront.AdmissionPolicy){p.Cipher=realityfront.CipherAES256},
		func(p *realityfront.AdmissionPolicy){p.InitialParity=4},
		func(p *realityfront.AdmissionPolicy){p.MinParity=3},
		func(p *realityfront.AdmissionPolicy){p.FECMode=realityfront.FECFixed;p.FixedParity=4;p.InitialParity=0;p.MinParity=0;p.MaxParity=0},
	} {
		p:=base
		modify(&p)
		if _,err:=N2PreparedPolicyForTests(p);!errors.Is(err,ErrAdmissionHandoff) {
			t.Fatalf("invalid or unimplemented policy accepted: %+v, err %v",p,err)
		}
	}
	// An unprotected V2, even with something resembling an auto policy,
	// cannot construct a test dynamic path; preserve fixed legacy admission.
	v2:=&realityfront.ClientAdmissionSession{
		Negotiated:realityfront.AdmissionResult{
			RecordVersion:realityfront.RecordVersionV2,Policy:base,
		},
	}
	if l,_,err:=N2ClientLaneFromProtectedPolicyForTests(v2,ClientLaneParams{});l!=nil||
		!errors.Is(err,ErrAdmissionHandoff) {
		t.Fatalf("V2 promoted to auto path %v",err)
	}
}

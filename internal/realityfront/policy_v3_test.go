package realityfront

import (
    "bytes"
    "context"
    "crypto/tls"
    "errors"
    "testing"
    "time"
)
func fixedV3Policy(parity uint8) AdmissionPolicy {
    p:=AdmissionPolicy{Schema:1,Transport:TransportNormal,DesiredLanes:1,FECMode:FECFixed,
        FixedParity:parity,Cipher:CipherChaCha}
    if parity==0 {p.FECMode=FECOff}
    return p
}
func v3TestRequest(p AdmissionPolicy) AdmissionRequest {
    return AdmissionRequest{RecordVersion:RecordVersionV3,LaneID:1,TunnelID:[]byte("0123456789abcdef"),
        ClientLimit:1400,Username:"x",Password:"y",Policy:p}
}
func TestV3PolicyWireAndFailures(t *testing.T) {
    for _,parity:=range []uint8{0,4,8,10,12,16,20} {
        req:=v3TestRequest(fixedV3Policy(parity))
        b,err:=marshalAdmissionRequest(req);if err!=nil{t.Fatal(err)}
        if len(b)!=15+16+1+1+10 {t.Fatalf("size=%d",len(b))}
        r,err:=readAdmissionRequest(bytes.NewReader(b));if err!=nil{t.Fatal(err)}
        if r.Policy!=req.Policy {t.Fatal("policy changed on request")}
        response:=AdmissionResult{RecordVersion:RecordVersionV3,LaneID:1,
            TunnelID:req.TunnelID,ClientLimit:1400,ServerLimit:1350,Policy:req.Policy}
        rb,err:=marshalAdmissionReply(response);if err!=nil{t.Fatal(err)}
        got,err:=readAdmissionReply(bytes.NewReader(rb),req)
        if err!=nil||got.Policy!=req.Policy{t.Fatalf("reply %v %#v",err,got.Policy)}
        for cut:=0;cut<len(b);cut++ {
            if _,err:=readAdmissionRequest(bytes.NewReader(b[:cut]));err==nil{t.Fatal("trunc accepted")}
        }
        for cut:=0;cut<len(rb);cut++ {
            if _,err:=readAdmissionReply(bytes.NewReader(rb[:cut]),req);err==nil{t.Fatal("reply trunc accepted")}
        }
        modified:=bytes.Clone(rb);modified[len(modified)-2]=2
        if _,err:=readAdmissionReply(bytes.NewReader(modified),req);!errors.Is(err,ErrAdmissionParams) {
            t.Fatalf("unexpected mismatch error %v",err)
        }
    }
    cases:=[]AdmissionPolicy{
        {Schema:1,Transport:TransportNormal,DesiredLanes:1,FECMode:FECFixed,FixedParity:3,Cipher:1},
        {Schema:1,Transport:TransportGame,DesiredLanes:2,FECMode:FECAuto,InitialParity:20,MinParity:4,MaxParity:20,Cipher:1},
        {Schema:2,Transport:1,DesiredLanes:1,FECMode:1,FixedParity:4,Cipher:1},
        {Schema:1,Transport:1,DesiredLanes:1,FECMode:1,FixedParity:4,Cipher:9},
        {Schema:1,Transport:1,DesiredLanes:1,FECMode:1,FixedParity:4,Cipher:1,Quality:2},
    }
    for _,p:=range cases {if _,err:=marshalAdmissionRequest(v3TestRequest(p));!errors.Is(err,ErrAdmissionParams){t.Fatalf("accepted %#v: %v",p,err)}}
    for _,p:=range []AdmissionPolicy{
        {Schema:1,Transport:1,DesiredLanes:1,FECMode:FECAuto,InitialParity:20,MinParity:4,MaxParity:20,Cipher:1},
        {Schema:1,Transport:1,DesiredLanes:1,FECMode:FECFixed,FixedParity:4,Cipher:CipherAES128},
        {Schema:1,Transport:1,DesiredLanes:1,FECMode:FECFixed,FixedParity:4,Cipher:CipherAES256},
        {Schema:1,Transport:1,DesiredLanes:1,FECMode:FECFixed,FixedParity:4,Cipher:1,Quality:1},
    } {if err:=p.SupportedNow();!errors.Is(err,ErrAdmissionUnsupported){t.Fatalf("advertised unsupported %#v: %v",p,err)}}
}
func TestV3ProtectedFixedAdmissionKeys(t *testing.T) {
    cert:=makeServerCert(t)
    routeKey:=[]byte("0123456789abcdef0123456789abcdef")
    assoc,peer:=newAssociationPeer(t)
    defer assoc.Close();defer peer.Close()
    type answer struct{ sess *ServerAdmissionSession; err error }
    done:=make(chan answer,1)
    go func(){
        session,err:=EstablishServer(context.Background(),assoc,ServerAdmissionConfig{
            TLS:ServerConfig{ServerName:"target.test",RouteKey:routeKey,
                TLSConfig:&tls.Config{Certificates:[]tls.Certificate{cert}},Timeout:3*time.Second},
            ExpectedUsername:"solo",ExpectedPassword:"secret",ServerLimit:1400})
        done<-answer{session,err}
    }()
    c,err:=EstablishClient(context.Background(),peer,ClientAdmissionConfig{
        TLS:ClientConfig{ServerName:"target.test",RouteKey:routeKey,Timeout:3*time.Second},
        Username:"solo",Password:"secret",TunnelID:[]byte("0123456789abcdef"),
        ClientLimit:1400,Policy:fixedV3Policy(4)})
    if err!=nil{t.Fatal(err)}
    s:= <-done
    if s.err!=nil {t.Fatal(s.err)}
    if c.Negotiated.RecordVersion!=RecordVersionV3||s.sess.Negotiated.RecordVersion!=RecordVersionV3||
        c.Negotiated.Policy!=s.sess.Negotiated.Policy||c.Negotiated.Policy.EffectiveParity()!=4{
        t.Fatal("protected V3 policy mismatch")
    }
    if c.Negotiated.Keys != s.sess.Negotiated.Keys ||
        c.Negotiated.Keys.C2S==c.Negotiated.Keys.S2C {t.Fatal("V3 exporter direction mismatch")}
}

func TestFixedPolicyForClientNoImplicitAutoMigration(t *testing.T) {
    for _,lanes:=range []int{1,2,4} {
        for _,parity:=range []int{0,4,8,10,12,16,20} {
            p,err:=FixedPolicyForClient(lanes,parity)
            if err!=nil {t.Fatalf("lanes=%d parity=%d: %v",lanes,parity,err)}
            if p.EffectiveParity()!=parity ||
                (lanes==1 && p.Transport!=TransportNormal) ||
                (lanes>1 && p.Transport!=TransportGame) ||
                (parity==0 && p.FECMode!=FECOff) ||
                (parity>0 && p.FECMode!=FECFixed) ||
                p.Cipher!=CipherChaCha || p.Quality!=0 {
                t.Fatalf("silent migration: %+v",p)
            }
        }
    }
    for _,tc:=range [][2]int{{0,4},{5,4},{1,-1},{1,9},{2,21}} {
        if _,err:=FixedPolicyForClient(tc[0],tc[1]);!errors.Is(err,ErrAdmissionParams){
            t.Fatalf("invalid fixed policy %+v accepted %v",tc,err)
        }
    }
}

func TestV3AuthenticatedPolicyAllowListRejectsBeforeLeaseAllocation(t *testing.T) {
    cert:=makeServerCert(t)
    key:=[]byte("0123456789abcdef0123456789abcdef")
    assoc,peer:=newAssociationPeer(t)
    defer peer.Close();defer assoc.Close()
    type result struct {err error;allocated bool;validated bool}
    done:=make(chan result,1)
    go func(){
        allocated:=false
        validated:=false
        _,err:=EstablishServer(context.Background(),assoc,ServerAdmissionConfig{
            TLS:ServerConfig{ServerName:"target.test",RouteKey:key,
                TLSConfig:&tls.Config{Certificates:[]tls.Certificate{cert}},Timeout:3*time.Second},
            ExpectedUsername:"solo",ExpectedPassword:"secret",ServerLimit:1400,
            RequireV3:true,
            PolicyAllow:func(p AdmissionPolicy)error{
                validated=true
                if p.FixedParity==8 {return errors.New("profile not allowed")}
                return nil
            },
            AllocateLease:func(req AdmissionRequest)(string,error){
                allocated=true
                return "10.66.0.17/32",nil
            },
        })
        done<-result{err,allocated,validated}
    }()
    p:=fixedV3Policy(8)
    _,err:=EstablishClient(context.Background(),peer,ClientAdmissionConfig{
        TLS:ClientConfig{ServerName:"target.test",RouteKey:key,Timeout:3*time.Second},
        Username:"solo",Password:"secret",TunnelID:[]byte("0123456789abcdef"),
        ClientLimit:1400,AutoLease:true,DesiredLanes:1,
        InstallationID:bytes.Repeat([]byte{0x12},16),Policy:p})
    if !errors.Is(err,ErrAdmissionParams){t.Fatalf("client rejection %v",err)}
    got:= <-done
    if !errors.Is(got.err,ErrAdmissionUnsupported) || !got.validated || got.allocated {
        t.Fatalf("allowlist bypass: %+v",got)
    }
}
func TestRequireV3RejectsLegacyV2WithoutAllocating(t *testing.T) {
    cert:=makeServerCert(t)
    key:=[]byte("0123456789abcdef0123456789abcdef")
    assoc,peer:=newAssociationPeer(t)
    defer peer.Close();defer assoc.Close()
    type result struct {err error;allocated bool}
    done:=make(chan result,1)
    go func(){
        allocated:=false
        _,err:=EstablishServer(context.Background(),assoc,ServerAdmissionConfig{
            TLS:ServerConfig{ServerName:"target.test",RouteKey:key,
                TLSConfig:&tls.Config{Certificates:[]tls.Certificate{cert}},Timeout:3*time.Second},
            ExpectedUsername:"solo",ExpectedPassword:"secret",ServerLimit:1400,
            RequireV3:true,
            AllocateLease:func(req AdmissionRequest)(string,error){
                allocated=true
                return "10.66.0.17/32",nil
            },
        })
        done<-result{err,allocated}
    }()
    _,err:=EstablishClient(context.Background(),peer,ClientAdmissionConfig{
        TLS:ClientConfig{ServerName:"target.test",RouteKey:key,Timeout:3*time.Second},
        Username:"solo",Password:"secret",TunnelID:[]byte("0123456789abcdef"),
        ClientLimit:1400,AutoLease:true,DesiredLanes:1,
        InstallationID:bytes.Repeat([]byte{0x12},16)})
    if !errors.Is(err,ErrAdmissionVersion){t.Fatalf("client version rejection %v",err)}
    got:= <-done
    if !errors.Is(got.err,ErrAdmissionVersion) || got.allocated {
        t.Fatalf("legacy admission unexpectedly accepted: %+v",got)
    }
}

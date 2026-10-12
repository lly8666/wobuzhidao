package realityfront

import (
    "bytes"
    "context"
    "crypto/tls"
    "errors"
    "testing"
    "time"
)

// This is real TLS-protected client/server admission using existing FakeTCP
// fixture; no test fabricates a server "accepted" policy without authentication.
func TestN3AESProtectedAdmissionBindsCipherExporterOnBothPeers(t *testing.T) {
    cert:=makeServerCert(t)
    routeKey:=[]byte("0123456789abcdef0123456789abcdef")
    var keysBySuite=make(map[uint8]bool)
    for _,cipher:=range []uint8{CipherAES128,CipherAES256} {
        policy,err:=PolicyForClientWithInitialAndCipher(1,"fixed",4,4,20,cipher)
        if err!=nil {t.Fatal(err)}
        assoc,peer:=newAssociationPeer(t)
        type result struct{session *ServerAdmissionSession;err error}
        done:=make(chan result,1)
        go func(){
            s,e:=EstablishServer(context.Background(),assoc,ServerAdmissionConfig{
                TLS:ServerConfig{ServerName:"target.test",RouteKey:routeKey,
                    TLSConfig:&tls.Config{Certificates:[]tls.Certificate{cert}},Timeout:3*time.Second},
                ExpectedUsername:"solo",ExpectedPassword:"secret",ServerLimit:1400})
            done<-result{s,e}
        }()
        client,err:=EstablishClient(context.Background(),peer,ClientAdmissionConfig{
            TLS:ClientConfig{ServerName:"target.test",RouteKey:routeKey,Timeout:3*time.Second},
            Username:"solo",Password:"secret",TunnelID:[]byte("0123456789abcdef"),
            ClientLimit:1400,Policy:policy})
        if err!=nil {assoc.Close();peer.Close();t.Fatalf("AES%d client admit: %v",cipher,err)}
        server:= <-done
        if server.err!=nil {assoc.Close();peer.Close();t.Fatalf("AES%d server admit: %v",cipher,server.err)}
        c,s:=client.Negotiated,server.session.Negotiated
        if c.RecordVersion!=RecordVersionV3 || s.RecordVersion!=RecordVersionV3 ||
           c.Policy!=policy || s.Policy!=policy || c.Keys!=s.Keys ||
           c.Keys.C2S==c.Keys.S2C {
            t.Fatalf("authenticated AES%d suite/directional HKDF mismatch",cipher)
        }
        if keysBySuite[cipher] {t.Fatal("duplicate suite")}
        keysBySuite[cipher]=true
        assoc.Close();peer.Close()
    }
}

func TestN3AESPolicyRejectsSuiteDowngradeAndLegacyImplicitUpgrade(t *testing.T) {
    p128,err:=PolicyForClientWithInitialAndCipher(1,"fixed",4,4,20,CipherAES128)
    if err!=nil {t.Fatal(err)}
    p256:=p128
    p256.Cipher=CipherAES256
    req:=v3TestRequest(p128)
    reply:=AdmissionResult{RecordVersion:RecordVersionV3,LaneID:1,
        TunnelID:req.TunnelID,ClientLimit:1400,ServerLimit:1350,Policy:p256}
    data,err:=marshalAdmissionReply(reply)
    if err!=nil {t.Fatal(err)}
    if _,err:=readAdmissionReply(bytes.NewReader(data),req);!errors.Is(err,ErrAdmissionParams) {
        t.Fatalf("V3 downgrade/suite mismatch admitted: %v",err)
    }
    for _,bad:=range []uint8{0,4,255} {
        if _,err:=PolicyForClientWithInitialAndCipher(1,"fixed",4,4,20,bad);err==nil {
            t.Fatalf("accepted invalid suite %d",bad)
        }
    }
    for _,legacy:=range []string{"","legacy"} {
        if _,err:=PolicyForClientWithInitialAndCipher(1,legacy,4,4,20,CipherAES256);!errors.Is(err,ErrAdmissionParams){
            t.Fatalf("implicit legacy AES migration: %v",err)
        }
    }
    for _,p:=range []struct{mode string;parity,min,initial int}{
        {"off",0,4,20},{"auto",0,4,20},{"auto",0,0,0},
        {"auto-aggressive",0,4,20},
    } {
        q,err:=PolicyForClientWithInitialAndCipher(1,p.mode,p.parity,p.min,p.initial,CipherAES256)
        if err!=nil||q.Cipher!=CipherAES256||q.SupportedNow()!=nil {
            t.Fatalf("N3 mode %s cipher bind denied: %+v %v",p.mode,q,err)
        }
    }
}

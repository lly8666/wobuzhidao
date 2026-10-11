package realityfront

import (
    "context"
    "crypto/tls"
    "testing"
    "time"

)

func TestV3ProtectedNormalAutoAndAggressiveOnceNegotiatedBothDirections(t *testing.T) {
    for _,tc:=range []struct{mode string;min int}{
        {"auto",0},{"auto-aggressive",4},
    } {
        t.Run(tc.mode,func(t *testing.T){
            p,err:=PolicyForClient(1,tc.mode,0,tc.min)
            if err!=nil {t.Fatal(err)}
            cert:=makeServerCert(t)
            key:=[]byte("0123456789abcdef0123456789abcdef")
            assoc,peer:=newAssociationPeer(t)
            defer assoc.Close()
            defer peer.Close()
            type answer struct{session *ServerAdmissionSession;err error}
            done:=make(chan answer,1)
            go func(){
                sess,err:=EstablishServer(context.Background(),assoc,ServerAdmissionConfig{
                    TLS:ServerConfig{ServerName:"target.test",RouteKey:key,
                        TLSConfig:&tls.Config{Certificates:[]tls.Certificate{cert}},
                        Timeout:3*time.Second},
                    ExpectedUsername:"solo",ExpectedPassword:"secret",ServerLimit:1400,
                    RequireV3:true,
                })
                done<-answer{sess,err}
            }()
            client,err:=EstablishClient(context.Background(),peer,ClientAdmissionConfig{
                TLS:ClientConfig{ServerName:"target.test",RouteKey:key,Timeout:3*time.Second},
                Username:"solo",Password:"secret",TunnelID:[]byte("0123456789abcdef"),
                ClientLimit:1400,Policy:p,
            })
            if err!=nil {t.Fatal(err)}
            svr:= <-done
            if svr.err!=nil {t.Fatal(svr.err)}
            if svr.session==nil || client.Negotiated.RecordVersion!=RecordVersionV3||
                svr.session.Negotiated.RecordVersion!=RecordVersionV3 ||
                client.Negotiated.Policy!=p || svr.session.Negotiated.Policy!=p ||
                client.Negotiated.Policy.EffectiveParity()!=20 ||
                svr.session.Negotiated.Policy.EffectiveParity()!=20 {
                t.Fatalf("V3 authenticated per-customer auto mismatch")
            }
            if client.Negotiated.Keys!=svr.session.Negotiated.Keys ||
                client.Negotiated.Keys.C2S==client.Negotiated.Keys.S2C {
                t.Fatal("client/server exporter or directional keys mismatch")
            }
            // AEAD record cipher remains ChaCha until N3, no silent AES,
            // and no fallback to unauthenticated auto negotiation.
            if client.Negotiated.Policy.Cipher!=CipherChaCha ||
                client.Negotiated.Policy.Quality!=1 {
                t.Fatal("protected quality/cipher mismatch")
            }
        })
    }
}

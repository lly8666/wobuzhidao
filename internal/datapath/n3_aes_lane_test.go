package datapath

import (
    "bytes"
    "errors"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/realityfront"
    "github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

// N3-B: AES is not public admission-enabled yet. This test exercises the
// exact product Lane encode/decode path with explicit version/suite metadata,
// not merely isolated tlsrecord constructors or fake ciphertext.
func TestN3V3SuiteBoundLaneAllFixedAndOffFirstDelivery(t *testing.T) {
    for _,suite := range []uint8{tlsrecord.RecordCipherChaCha,
        tlsrecord.RecordCipherAES128,tlsrecord.RecordCipherAES256} {
        for _,parity := range []int{0,4,20} {
            clientConfig:=pairConfig(RoleClient,parity)
            serverConfig:=pairConfig(RoleServer,parity)
            var nonce [16]byte
            nonce[0]=byte(17+suite)
            keys,err:=tlsrecord.DeriveKeysV3(bytes.Repeat([]byte{0x71},32),nonce,suite)
            if err!=nil {t.Fatal(err)}
            for _,cfg:=range []*LaneConfig{&clientConfig,&serverConfig} {
                cfg.RecordVersion=realityfront.RecordVersionV3
                cfg.RecordCipher=suite
                cfg.Keys=keys
                cfg.IncarnationNonce=nonce
            }
            client,err:=NewLane(clientConfig)
            if err!=nil {t.Fatalf("client cipher=%d parity=%d: %v",suite,parity,err)}
            server,err:=NewLane(serverConfig)
            if err!=nil {client.Close();t.Fatalf("server cipher=%d parity=%d: %v",suite,parity,err)}
            now:=time.Unix(9000,0)
            upstream:=[]byte{0x45,0,0,28,1,byte(suite),byte(parity),0,64,17,0,0,10,0,0,1,10,0,0,2,1,2,3,4,5,6,7,8}
            outbound,err:=client.Outbound(upstream,now)
            if err!=nil {t.Fatal(err)}
            got:=deliverRecords(t,server,outbound,now)
            if len(got)!=1 || !bytes.Equal(got[0],upstream) {
                t.Fatalf("C2S suite=%d parity=%d got=%x",suite,parity,got)
            }
            downstream:=bytes.Clone(upstream)
            downstream[4]=2
            outbound,err=server.Outbound(downstream,now)
            if err!=nil {t.Fatal(err)}
            got=deliverRecords(t,client,outbound,now)
            if len(got)!=1 || !bytes.Equal(got[0],downstream) {
                t.Fatalf("S2C suite=%d parity=%d got=%x",suite,parity,got)
            }
            if client.Config().RecordCipher!=suite || server.Config().RecordCipher!=suite {
                t.Fatal("record suite not immutable and pinned on lane")
            }
            client.Close()
            server.Close()
        }
    }
}

func TestN3LaneCipherRefusesLegacyOrMismatchedPolicy(t *testing.T) {
    cfg:=pairConfig(RoleClient,0)
    cfg.RecordCipher=tlsrecord.RecordCipherAES128
    if _,err:=NewLane(cfg);!errors.Is(err,ErrConfigMismatch) {
        t.Fatalf("legacy lane admitted arbitrary cipher: %v",err)
    }
    cfg.RecordVersion=realityfront.RecordVersionV3
    for _,bad:=range []uint8{0,4,255} {
        cfg.RecordCipher=bad
        if _,err:=NewLane(cfg);!errors.Is(err,ErrConfigMismatch) {
            t.Fatalf("accepted V3 bad cipher %d: %v",bad,err)
        }
    }
    cfg.RecordVersion=realityfront.RecordVersionV2
    cfg.RecordCipher=tlsrecord.RecordCipherChaCha
    if _,err:=NewLane(cfg);!errors.Is(err,ErrConfigMismatch) {
        t.Fatalf("V2 must not silently re-key through V3: %v",err)
    }
    cfg.RecordVersion=0
    cfg.RecordCipher=0
    lane,err:=NewLane(cfg)
    if err!=nil {t.Fatalf("legacy lane broken: %v",err)}
    lane.Close()
}

func TestN3AuthenticatedAdmissionCipherSelectorDoesNotGuessFromV2(t *testing.T) {
    for _,suite:=range []uint8{realityfront.CipherChaCha,realityfront.CipherAES128,realityfront.CipherAES256} {
        n:=realityfront.AdmissionResult{RecordVersion:realityfront.RecordVersionV3,
            Policy:realityfront.AdmissionPolicy{Cipher:suite}}
        if got:=admittedV3RecordCipher(n);got!=suite {t.Fatalf("suite %d got %d",suite,got)}
        if got:=admittedV3RecordVersion(n);got!=realityfront.RecordVersionV3 {
            t.Fatalf("version got %d",got)
        }
        n.RecordVersion=realityfront.RecordVersionV2
        if admittedV3RecordCipher(n)!=0 || admittedV3RecordVersion(n)!=0 {
            t.Fatal("V2 bypass selected V3 record path")
        }
    }
}

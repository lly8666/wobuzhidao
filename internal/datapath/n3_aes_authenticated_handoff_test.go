package datapath

import (
    "bytes"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/faketcp"
    "github.com/lly8666/wobuzhidao/internal/realityfront"
    "github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

// Authenticated handoff logic is exercised for both peer roles. The V3
// sessions here are pre-negotiated immutable results; the separate
// realityfront N3 handshake test proves the protected peer agreement.
func TestN3AuthenticatedAESClientServerLaneHandoffFixedOffAuto(t *testing.T) {
    syn:=faketcp.Segment{SrcIP:[4]byte{10,0,0,1},DstIP:[4]byte{10,0,0,2},
        SrcPort:40000,DstPort:443,Seq:100,Flags:faketcp.FlagSYN,
        Window:65535,MSS:1200,MSSSet:true}
    assoc,err:=faketcp.NewServerAssociation(syn,9000,100*time.Millisecond,func(faketcp.Segment)error{return nil})
    if err!=nil {t.Fatal(err)}
    defer assoc.Close()
    for _,cipher:=range []uint8{realityfront.CipherAES128,realityfront.CipherAES256} {
        for _,spec:=range []struct{mode string;parity,min,initial int}{
            {"off",0,4,20},{"fixed",4,4,20},{"auto",0,4,20},
            {"auto",0,0,0},{"auto-aggressive",0,4,20},
        } {
            p,err:=realityfront.PolicyForClientWithInitialAndCipher(1,spec.mode,
                spec.parity,spec.min,spec.initial,cipher)
            if err!=nil {t.Fatal(err)}
            var nonce [16]byte
            nonce[0]=byte(cipher)
            pair,err:=tlsrecord.DeriveKeysV3(bytes.Repeat([]byte{0xb2},32),nonce,cipher)
            if err!=nil {t.Fatal(err)}
            result:=realityfront.AdmissionResult{
                RecordVersion:realityfront.RecordVersionV3,Policy:p,
                ClientLimit:1400,ServerLimit:1350,Keys:pair,
                TunnelID:[]byte("0123456789abcdef"),IncarnationNonce:nonce}
            clientSession:=&realityfront.ClientAdmissionSession{Negotiated:result}
            serverSession:=&realityfront.ServerAdmissionSession{Negotiated:result}
            cc,err:=ClientLaneConfigFromAdmission(clientSession,ClientLaneParams{
                ConnectionMTU:1500,PeerMSS:1200,PeerMSSSet:true})
            if err!=nil {t.Fatalf("client %s AES%d: %v",spec.mode,cipher,err)}
            sc,err:=ServerLaneConfigFromAdmission(serverSession,assoc,ServerLaneParams{ConnectionMTU:1500})
            if err!=nil {t.Fatalf("server %s AES%d: %v",spec.mode,cipher,err)}
            if cc.RecordCipher!=cipher||sc.RecordCipher!=cipher||
               cc.RecordVersion!=realityfront.RecordVersionV3||sc.RecordVersion!=realityfront.RecordVersionV3 {
                t.Fatalf("authenticated suite missing %s AES%d",spec.mode,cipher)
            }
            c,err:=NewLane(cc);if err!=nil {t.Fatal(err)}
            s,err:=NewLane(sc);if err!=nil {c.Close();t.Fatal(err)}
            now:=time.Unix(1234,0)
            packet:=[]byte{0x45,0,0,28,1,cipher,0,0,64,17,0,0,10,0,0,1,10,0,0,2,1,2,3,4,5,6,7,8}
            c2s,err:=c.Outbound(packet,now);if err!=nil {t.Fatal(err)}
            up:=deliverRecords(t,s,c2s,now)
            if len(up)!=1||!bytes.Equal(up[0],packet) {
                t.Fatalf("AES%d mode%s C2S first packet delivery lost",cipher,spec.mode)
            }
            packet[4]=2
            s2c,err:=s.Outbound(packet,now);if err!=nil {t.Fatal(err)}
            down:=deliverRecords(t,c,s2c,now)
            if len(down)!=1||!bytes.Equal(down[0],packet) {
                t.Fatalf("AES%d mode%s S2C first packet delivery lost",cipher,spec.mode)
            }
            s.Close();c.Close()
        }
    }
}

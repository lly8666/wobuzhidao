package datapath

import (
    "bytes"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/faketcp"
    "github.com/lly8666/wobuzhidao/internal/realityfront"
)

// These use the actual lane sealer/FEC encoder/decoder. The first source
// record must be delivered in both directions before parity flush and without
// probing a previous business record for the effective profile.
func TestV3ProtectedPolicyFirstBidirectionalRecords(t *testing.T) {
    syn:=faketcp.Segment{SrcIP:[4]byte{10,0,0,1}, DstIP:[4]byte{10,0,0,2},
        SrcPort:40000,DstPort:443,Seq:100,Flags:faketcp.FlagSYN,Window:65535,
        MSS:1200,MSSSet:true}
    assoc,err:=faketcp.NewServerAssociation(syn,9000,100*time.Millisecond,func(faketcp.Segment)error{return nil})
    if err!=nil{t.Fatal(err)}
    defer assoc.Close()
    for _,parity:=range []uint8{0,4,20} {
        policy:=realityfront.AdmissionPolicy{Schema:1,Transport:realityfront.TransportNormal,
            DesiredLanes:1,FECMode:realityfront.FECFixed,FixedParity:parity,Cipher:realityfront.CipherChaCha}
        if parity==0{policy.FECMode=realityfront.FECOff}
        n:=realityfront.AdmissionResult{RecordVersion:realityfront.RecordVersionV3,
            Policy:policy,TunnelID:[]byte("0123456789abcdef"),ClientLimit:1400,ServerLimit:1350,Keys:testKeys()}
        n.IncarnationNonce[0]=byte(parity+1)
        clientSession:=&realityfront.ClientAdmissionSession{Negotiated:n}
        serverSession:=&realityfront.ServerAdmissionSession{Negotiated:n}
        clientConfig,err:=ClientLaneConfigFromAdmission(clientSession,ClientLaneParams{
            ConnectionMTU:1500,PeerMSS:1200,PeerMSSSet:true,ParityShards:16})
        if err!=nil{t.Fatal(err)}
        serverConfig,err:=ServerLaneConfigFromAdmission(serverSession,assoc,ServerLaneParams{
            ConnectionMTU:1500,ParityShards:8})
        if err!=nil{t.Fatal(err)}
        if clientConfig.ParityShards!=int(parity)||serverConfig.ParityShards!=int(parity)||
            clientConfig.TxMTU.ParityShards!=int(parity)||serverConfig.RxMTU.ParityShards!=int(parity)||
            clientConfig.FlushAfter!=serverConfig.FlushAfter{
            t.Fatalf("parity=%d client=%+v server=%+v",parity,clientConfig,serverConfig)
        }
        c,err:=NewLane(clientConfig);if err!=nil{t.Fatal(err)}
        s,err:=NewLane(serverConfig);if err!=nil{c.Close();t.Fatal(err)}
        now:=time.Unix(9000,0)
        upstream:=[]byte{0x45,0,0,28,1,byte(parity),0,0,64,17,0,0,10,0,0,1,10,0,0,2,1,2,3,4,5,6,7,8}
        outbound,err:=c.Outbound(upstream,now);if err!=nil{t.Fatal(err)}
        received:=deliverRecords(t,s,outbound,now)
        if len(received)!=1||!bytes.Equal(received[0],upstream){t.Fatalf("first C2S parity=%d received=%x",parity,received)}
        downstream:=[]byte{0x45,0,0,28,2,byte(parity),0,0,64,17,0,0,10,0,0,2,10,0,0,1,8,7,6,5,4,3,2,1}
        outbound,err=s.Outbound(downstream,now);if err!=nil{t.Fatal(err)}
        received=deliverRecords(t,c,outbound,now)
        if len(received)!=1||!bytes.Equal(received[0],downstream){t.Fatalf("first S2C parity=%d received=%x",parity,received)}
        c.Close();s.Close()
    }
}

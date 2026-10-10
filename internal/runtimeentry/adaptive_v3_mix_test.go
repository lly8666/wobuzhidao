package runtimeentry

import (
    "context"
    "crypto/tls"
    "encoding/hex"
    "net/netip"
    "sync"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/faketcp"
    "github.com/lly8666/wobuzhidao/internal/linuxserver"
    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
    "github.com/lly8666/wobuzhidao/internal/platformflow"
    "github.com/lly8666/wobuzhidao/internal/realityfront"
)

// This uses the existing real TLS/uTLS, FakeTCP, LINK and FEC runtime
// over the deterministic in-memory segment carrier. It does not claim
// physical netns, Windows native TUN or loss-performance qualification.
func TestV3ThreeClientsIndependentFirstBothDirectionsAndRotation(t *testing.T) {
    cert:=runtimeCertificate(t)
    key:=[]byte("0123456789abcdef0123456789abcdef")
    base,ep:=memorySegmentPair()
    mux,err:=NewSegmentMux(base.io())
    if err!=nil{t.Fatal(err)}
    defer mux.Close()
    writer:=newMemoryPacketWriter()
    router,err:=linuxserver.NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/24"),16,writer)
    if err!=nil{t.Fatal(err)}
    registry,err:=logicaltunnel.NewLeaseRegistry(netip.MustParsePrefix("10.66.0.0/24"),16)
    if err!=nil{t.Fatal(err)}
    server,err:=NewLifecycleServer(LifecycleServerConfig{
        ServerConfig:ServerConfig{
            IO:ep.io(),ListenPort:443,TickInterval:10*time.Millisecond,MaxFlows:128,
            Admission:realityfront.ServerAdmissionConfig{
                TLS:realityfront.ServerConfig{ServerName:"target.test",RouteKey:key,
                    TLSConfig:&tls.Config{Certificates:[]tls.Certificate{cert}},Timeout:3*time.Second},
                ExpectedUsername:"mix",ExpectedPassword:"password",ServerLimit:1300,RequireV3:true,
                AllocateLease:func(req realityfront.AdmissionRequest)(string,error){
                    inst,err:=logicaltunnel.ParseInstallationID(hex.EncodeToString(req.InstallationID))
                    if err!=nil{return "",err}
                    id,err:=logicaltunnel.TunnelIDFromBytes(req.TunnelID)
                    if err!=nil{return "",err}
                    l,err:=registry.Acquire(req.Username,inst,id)
                    if err!=nil{return "",err}
                    return l.Config.Address4,nil
                },
            },
            LookupLease:registry.Lookup,Router:router,Service:platformflow.DefaultServerConfig(),
            Lane:datapath.ServerLaneParams{ConnectionMTU:1500,
                TxIPv4HeaderLen:20,TxTCPHeaderLen:20,RxIPv4HeaderLen:20,RxTCPHeaderLen:20,
                ParityShards:0}, // deliberately not the mixed requested profiles
        },DesiredLanes:4,
    })
    if err!=nil{t.Fatal(err)}
    defer server.Close()
    ctx,cancel:=context.WithTimeout(context.Background(),16*time.Second)
    defer cancel()
    done:=make(chan error,1)
    go func(){done<-server.Run(ctx)}()
    defer func(){cancel();select{case <-done:case <-time.After(3*time.Second):}}()

    cases:=[]struct{lanes,parity int}{{1,0},{1,4},{2,20}}
    clients:=make([]*TunnelClient,len(cases))
    leases:=make([]logicaltunnel.Lease,len(cases))
    policies:=make([]realityfront.AdmissionPolicy,len(cases))
    receives:=make([]chan []byte,len(cases))
    for i:=range receives{receives[i]=make(chan []byte,4)}
    var group sync.WaitGroup
    for i,tc:=range cases{
        i,tc:=i,tc
        group.Add(1)
        go func(){
            defer group.Done()
            var inst logicaltunnel.InstallationID
            inst[0]=byte(i+1)
            tunnelID:=logicaltunnel.DerivedTunnelID("mix",inst)
            p,err:=realityfront.FixedPolicyForClient(tc.lanes,tc.parity)
            if err!=nil{t.Error(err);return}
            policies[i]=p
            client,err:=DialTunnelClient(ctx,TunnelClientConfig{
                Lease:logicaltunnel.Lease{Account:"mix",InstallationID:inst,
                    Config:logicaltunnel.TunnelConfig{TunnelID:tunnelID,Address4:"0.0.0.0/32",
                        Routes4:[]string{"0.0.0.0/0"}}},
                DesiredLanes:tc.lanes,MaxFlows:128,
                OpenLane:func(_ uint8,inc uint64)(SegmentIO,faketcp.ClientFlow,error){
                    port,err:=RotatingSourcePort(uint16(40000+i*4096),inc)
                    if err!=nil{return SegmentIO{},faketcp.ClientFlow{},err}
                    flow:=faketcp.ClientFlow{LocalIP:[4]byte{192,0,2,1},
                        PeerIP:[4]byte{198,51,100,2},LocalPort:port,PeerPort:443}
                    io,err:=mux.Open(flow)
                    return io,flow,err
                },
                Admission:realityfront.ClientAdmissionConfig{
                    AutoLease:true,InstallationID:inst[:],TLS:realityfront.ClientConfig{
                        ServerName:"target.test",RouteKey:key,Timeout:3*time.Second},
                    Username:"mix",Password:"password",TunnelID:tunnelID.Bytes(),
                    ClientLimit:1300,Policy:p,
                },
                Lane:datapath.ClientLaneParams{ConnectionMTU:1500,
                    TxIPv4HeaderLen:20,TxTCPHeaderLen:20,RxIPv4HeaderLen:20,RxTCPHeaderLen:20,
                    ParityShards:16}, // deliberately distinct from per-client policy
                Deliver:func(pkts [][]byte,_ time.Time)error{
                    for _,pkt:=range pkts{receives[i]<-append([]byte(nil),pkt...)}
                    return nil
                },
                TickInterval:10*time.Millisecond,
            })
            if err!=nil{t.Error(err);return}
            clients[i]=client
            lease,_:=client.Owner().Lease()
            leases[i]=lease
        }()
    }
    group.Wait()
    for i,c:=range clients{
        if c==nil{t.Fatalf("V3 client %d missing",i)}
        defer c.Close()
    }
    seenAddresses:=make(map[string]struct{})
    for i,c:=range clients{
        lease:=leases[i]
        addr,err:=lease.Config.LeaseIPv4();if err!=nil{t.Fatal(err)}
        if _,ok:=seenAddresses[lease.Config.Address4];ok{t.Fatal("leases crossed")}
        seenAddresses[lease.Config.Address4]=struct{}{}
        server.mu.Lock()
        tunnel:=server.byTunnel[lease.Config.TunnelID]
        var policy realityfront.AdmissionPolicy
        if tunnel!=nil{policy=tunnel.admissionPolicy}
        server.mu.Unlock()
        if tunnel==nil||policy!=policies[i]{t.Fatalf("server V3 client %d policy=%+v want=%+v",i,policy,policies[i])}

        up:=ipv4Packet(addr.As4(),[4]byte{8,8,8,8},17)
        if err:=c.SendPacket(ctx,up,time.Now());err!=nil{t.Fatal(err)}
        if got:=writer.waitPacket(t,3*time.Second);string(got)!=string(up){t.Fatal("first upstream crossed or altered")}

        waitLifecycle(t,3*time.Second,func()bool{return server.TunnelQualified(lease.Config.TunnelID)})
        down:=ipv4Packet([4]byte{8,8,8,8},addr.As4(),17)
        if err:=server.RoutePacket(down,time.Now());err!=nil{t.Fatal(err)}
        select{
        case got:= <-receives[i]:
            if string(got)!=string(down){t.Fatal("first downstream crossed or altered")}
        case <-time.After(3*time.Second):t.Fatal("first downstream timeout")
        }
    }
    // The authenticated FEC policy must remain frozen through generation
    // replacement; old/new S2C can first-deliver independently.
    if err:=clients[1].RotateOldest(ctx);err!=nil{t.Fatal(err)}
    server.mu.Lock()
    rotated:=server.byTunnel[leases[1].Config.TunnelID]
    policy:=rotated.admissionPolicy
    server.mu.Unlock()
    if policy!=policies[1]{t.Fatal("rotation silently changed V3 policy")}
}

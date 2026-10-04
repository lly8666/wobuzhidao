package dnsroute

import (
 "encoding/binary"
 "io"
 "net"
 "net/netip"
 "sync"
 "testing"
 "time"
 "golang.org/x/net/dns/dnsmessage"
)

func message(t *testing.T, id uint16, response bool, name string, code dnsmessage.RCode) []byte {
 t.Helper(); n,e:=dnsmessage.NewName(name);if e!=nil{t.Fatal(e)}
 m:=dnsmessage.Message{Header:dnsmessage.Header{ID:id,Response:response,RecursionDesired:true,RCode:code},Questions:[]dnsmessage.Question{{Name:n,Type:dnsmessage.TypeA,Class:dnsmessage.ClassINET}}}
 b,e:=m.Pack();if e!=nil{t.Fatal(e)};return b
}
func TestUDPFailoverValidationAndRecovery(t *testing.T) {
 now:=time.Now();client:=netip.MustParseAddrPort("10.40.0.2:1234");original:=netip.MustParseAddrPort("223.5.5.5:53")
 servers,_:=ParseServers(DefaultServers);var sent []netip.AddrPort;var replies int
 r,e:=New(servers,func(c,p netip.AddrPort,b []byte,n time.Time)error{sent=append(sent,p);return nil},func(p,c netip.AddrPort,b []byte)error{if p!=original||c!=client{t.Fatal("original endpoint not restored")};replies++;return nil},func(netip.AddrPort)(net.Conn,error){return nil,ErrQuery});if e!=nil{t.Fatal(e)};defer r.Close()
 q:=message(t,1,false,"example.com.",0);if e=r.Query(client,original,q,now);e!=nil{t.Fatal(e)};q[0]=99 // ownership
 r.Tick(now.Add(time.Second));if len(sent)!=1{t.Fatal("premature retry")}
 r.Tick(now.Add(1500*time.Millisecond));r.Tick(now.Add(2*time.Second));if len(sent)!=2||sent[1].Addr()!=servers[1]{t.Fatal("backup not attempted exactly once")}
 r.Answer(sent[1],client,message(t,1,true,"wrong.com.",0),now.Add(2*time.Second));if replies!=0{t.Fatal("wrong question accepted")}
 answer:=message(t,1,true,"EXAMPLE.com.",0);r.Answer(sent[1],client,answer,now.Add(2*time.Second));r.Answer(sent[0],client,answer,now.Add(2*time.Second));if replies!=1{t.Fatal("duplicate delivery")}
 r.Query(client,original,message(t,2,false,"example.com.",0),now.Add(3*time.Second));if sent[len(sent)-1].Addr()!=servers[1]{t.Fatal("healthy backup not preferred")}
 r.Tick(now.Add(5*time.Second));if sent[len(sent)-1].Addr()!=servers[0]{t.Fatal("backup's backup missing")}
 r.Answer(sent[len(sent)-1],client,message(t,2,true,"example.com.",0),now.Add(5*time.Second));if replies!=2{t.Fatal("recovered primary failed")}
 r.Query(client,original,message(t,3,false,"example.com.",0),now.Add(6*time.Second));if sent[len(sent)-1].Addr()!=servers[0]{t.Fatal("primary recovery preference")}
 r.Answer(sent[len(sent)-1],client,message(t,3,true,"example.com.",dnsmessage.RCodeNameError),now.Add(6*time.Second));if replies!=3{t.Fatal("NXDOMAIN must not retry")}
}

func TestUDPBoundedExpiryAndErrors(t *testing.T){
 servers,_:=ParseServers(DefaultServers);now:=time.Now();var sends int
 r,_:=New(servers,func(netip.AddrPort,netip.AddrPort,[]byte,time.Time)error{sends++;return nil},func(netip.AddrPort,netip.AddrPort,[]byte)error{return nil},func(netip.AddrPort)(net.Conn,error){return nil,ErrQuery});defer r.Close()
 c:=netip.MustParseAddrPort("10.0.0.1:2000");p:=netip.MustParseAddrPort("192.168.1.1:53")
 for i:=0;i<MaxPending;i++{if e:=r.Query(c,p,message(t,uint16(i),false,"example.com.",0),now);e!=nil{t.Fatal(e)}}
 if e:=r.Query(c,p,message(t,600,false,"example.com.",0),now);e==nil{t.Fatal("unbounded pending")}
 r.Tick(now.Add(6*time.Second));if len(r.pending)!=0||r.bytes!=0{t.Fatal("expired state retained")}
 r.Query(c,p,message(t,601,false,"example.com.",0),now.Add(7*time.Second));n:=sends
 r.Answer(netip.AddrPortFrom(servers[0],53),c,message(t,601,true,"example.com.",dnsmessage.RCodeServerFailure),now.Add(7*time.Second));if sends!=n+1{t.Fatal("SERVFAIL did not fail over")}
 if _,e:=ParseServers("1.1.1.1,8.8.8.8,9.9.9.9");e==nil{t.Fatal("too many servers")};if _,e:=ParseServers("::1");e==nil{t.Fatal("IPv6 resolver accepted")}
 if e:=r.Query(c,p,[]byte{1,2},now);e==nil{t.Fatal("malformed query accepted")}
}

func TestTCPFailoverFramingAndClose(t *testing.T){
 servers,_:=ParseServers(DefaultServers);var mu sync.Mutex;var attempts []netip.Addr
 r,_:=New(servers,func(netip.AddrPort,netip.AddrPort,[]byte,time.Time)error{return nil},func(netip.AddrPort,netip.AddrPort,[]byte)error{return nil},func(p netip.AddrPort)(net.Conn,error){
  mu.Lock();attempts=append(attempts,p.Addr());mu.Unlock();if p.Addr()==servers[0]{return nil,ErrQuery}
  a,b:=net.Pipe();go func(){defer b.Close();var h [2]byte;if _,e:=io.ReadFull(b,h[:]);e!=nil{return};q:=make([]byte,int(binary.BigEndian.Uint16(h[:])));if _,e:=io.ReadFull(b,q);e!=nil{return};q[2]|=0x80;binary.BigEndian.PutUint16(h[:],uint16(len(q)));b.Write(h[:]);b.Write(q)}();return a,nil
 });c,d:=net.Pipe();r.ServeTCP(d);defer c.Close();c.SetDeadline(time.Now().Add(3*time.Second))
 q:=message(t,77,false,"example.com.",0);wire:=make([]byte,2+len(q));binary.BigEndian.PutUint16(wire,uint16(len(q)));copy(wire[2:],q);if _,e:=c.Write(wire);e!=nil{t.Fatal(e)};var h [2]byte;if _,e:=io.ReadFull(c,h[:]);e!=nil{t.Fatal(e)};a:=make([]byte,int(binary.BigEndian.Uint16(h[:])));if _,e:=io.ReadFull(c,a);e!=nil{t.Fatal(e)};if _,_,e:=inspect(a,true);e!=nil{t.Fatal(e)}
 mu.Lock();if len(attempts)!=2||attempts[1]!=servers[1]{t.Fatal("TCP failover missing")};mu.Unlock()
 done:=make(chan struct{});go func(){r.Close();close(done)}();select{case <-done:case <-time.After(2*time.Second):t.Fatal("Close leaked TCP handler")}
}

func TestConcurrentQueriesAndRetirement(t *testing.T){
 servers,_:=ParseServers(DefaultServers);r,_:=New(servers,func(netip.AddrPort,netip.AddrPort,[]byte,time.Time)error{return nil},func(netip.AddrPort,netip.AddrPort,[]byte)error{return nil},func(netip.AddrPort)(net.Conn,error){return nil,ErrQuery});defer r.Close();q:=message(t,1,false,"example.com.",0);var wg sync.WaitGroup
 for i:=0;i<8;i++{wg.Add(1);go func(i int){defer wg.Done();for j:=0;j<100;j++{now:=time.Now();r.Query(netip.AddrPortFrom(netip.MustParseAddr("10.0.0.1"),uint16(2000+i)),netip.MustParseAddrPort("192.168.1.1:53"),q,now);r.Tick(now)}}(i)};wg.Wait()
}

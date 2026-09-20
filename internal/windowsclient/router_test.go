package windowsclient

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

type fakeWriter struct {
	packets [][]byte
	short   bool
}

func (w *fakeWriter) WritePacket(p []byte) (int, error) {
	w.packets = append(w.packets, append([]byte(nil), p...))
	if w.short { return len(p)-1, nil }
	return len(p), nil
}

type fakeOwner struct {
	lease logicaltunnel.Lease
	stats datapath.TunnelOwnerStats
	normalCalls int
	gameCalls int
	last []byte
}

func (o *fakeOwner) Lease() (logicaltunnel.Lease, bool) { return o.lease.Clone(), true }
func (o *fakeOwner) Stats() datapath.TunnelOwnerStats { return o.stats }
func (o *fakeOwner) NormalOutbound(p []byte, _ time.Time) ([]datapath.WireRecord, error) {
	o.normalCalls++
	o.last=append([]byte(nil),p...)
	return []datapath.WireRecord{{PN:1,Wire:[]byte{1}}},nil
}
func (o *fakeOwner) GameOutbound(p []byte, _ time.Time) (datapath.GameOutboundResult,error) {
	o.gameCalls++
	o.last=append([]byte(nil),p...)
	return datapath.GameOutboundResult{PacketID:9},nil
}

func testLease(t *testing.T, idHex, addr string) logicaltunnel.Lease {
	t.Helper()
	id,err:=logicaltunnel.ParseTunnelID(idHex)
	if err!=nil { t.Fatal(err) }
	return logicaltunnel.Lease{Account:"acct",Config:logicaltunnel.TunnelConfig{TunnelID:id,Address4:addr+"/32"}}
}

func ipv4Packet(src,dst netip.Addr,payload []byte) []byte {
	p:=make([]byte,20+len(payload))
	p[0]=0x45
	binary.BigEndian.PutUint16(p[2:4],uint16(len(p)))
	copy(p[12:16],src.AsSlice())
	copy(p[16:20],dst.AsSlice())
	copy(p[20:],payload)
	return p
}

func TestRouterSourceFenceAndModeDispatch(t *testing.T) {
	lease:=testLease(t,"00112233445566778899aabbccddeeff","10.66.0.7")
	writer:=&fakeWriter{}
	normal:=&fakeOwner{lease:lease,stats:datapath.TunnelOwnerStats{DesiredLanes:1}}
	r,err:=NewRouter(normal,writer)
	if err!=nil { t.Fatal(err) }
	valid:=ipv4Packet(netip.MustParseAddr("10.66.0.7"),netip.MustParseAddr("1.1.1.1"),[]byte("q"))
	out,err:=r.RouteFromTUN(valid,time.Unix(1,0))
	if err!=nil { t.Fatal(err) }
	if out.IsGame || len(out.Normal)!=1 || normal.normalCalls!=1 || !bytes.Equal(normal.last,valid) { t.Fatalf("normal out=%+v calls=%d",out,normal.normalCalls) }

	spoof:=ipv4Packet(netip.MustParseAddr("10.66.0.8"),netip.MustParseAddr("1.1.1.1"),nil)
	if _,err:=r.RouteFromTUN(spoof,time.Unix(2,0)); !errors.Is(err,logicaltunnel.ErrSourceSpoof) { t.Fatalf("spoof err=%v",err) }
	if normal.normalCalls!=1 { t.Fatalf("spoof reached owner calls=%d",normal.normalCalls) }

	game:=&fakeOwner{lease:lease,stats:datapath.TunnelOwnerStats{DesiredLanes:3}}
	gr,_:=NewRouter(game,writer)
	gout,err:=gr.RouteFromTUN(valid,time.Unix(3,0))
	if err!=nil { t.Fatal(err) }
	if !gout.IsGame || gout.Game.PacketID!=9 || game.gameCalls!=1 { t.Fatalf("game out=%+v calls=%d",gout,game.gameCalls) }
}

func TestRouterInboundDestinationFenceAndShortWrite(t *testing.T) {
	lease:=testLease(t,"00112233445566778899aabbccddeeff","10.66.0.7")
	writer:=&fakeWriter{}
	owner:=&fakeOwner{lease:lease,stats:datapath.TunnelOwnerStats{DesiredLanes:1}}
	r,_:=NewRouter(owner,writer)

	valid:=ipv4Packet(netip.MustParseAddr("8.8.8.8"),netip.MustParseAddr("10.66.0.7"),[]byte("r"))
	if err:=r.DeliverFromOwner([][]byte{valid}); err!=nil { t.Fatal(err) }
	if len(writer.packets)!=1 || !bytes.Equal(writer.packets[0],valid) { t.Fatalf("packets=%x",writer.packets) }

	wrong:=ipv4Packet(netip.MustParseAddr("8.8.8.8"),netip.MustParseAddr("10.66.0.8"),nil)
	if err:=r.DeliverFromOwner([][]byte{wrong}); !errors.Is(err,ErrDestinationLeak) { t.Fatalf("wrong dst err=%v",err) }
	if len(writer.packets)!=1 { t.Fatal("cross-lease packet reached Wintun") }

	v6:=make([]byte,40); v6[0]=0x60
	if err:=r.DeliverFromOwner([][]byte{v6}); !errors.Is(err,ErrInvalidIPv4) { t.Fatalf("IPv6 err=%v",err) }

	short:=&fakeWriter{short:true}
	sr,_:=NewRouter(owner,short)
	if err:=sr.DeliverFromOwner([][]byte{valid}); !errors.Is(err,ErrShortTUNWrite) { t.Fatalf("short err=%v",err) }
}

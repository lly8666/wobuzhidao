//go:build linux

package openwrtclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/linuxserver"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

type serviceRejectWriter struct {
	mu      sync.Mutex
	packets int
}

func (w *serviceRejectWriter) WritePacket(p []byte) (int, error) {
	w.mu.Lock()
	w.packets++
	w.mu.Unlock()
	return len(p), nil
}

func (w *serviceRejectWriter) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.packets
}

func TestPrivilegedOpenWrtSocketTunnelAdapter(t *testing.T) {
	if os.Getenv("WBD_P4_OPENWRT_TPROXY_NET") != "1" {
		t.Skip("set WBD_P4_OPENWRT_TPROXY_NET=1 inside the root netns harness")
	}
	if os.Geteuid() != 0 {
		t.Fatal("OpenWrt socket tunnel qualification requires root")
	}
	clientNS := os.Getenv("WBD_P4_OPENWRT_CLIENT_NS")
	targetNS := os.Getenv("WBD_P4_OPENWRT_TARGET_NS")
	if clientNS == "" || targetNS == "" {
		t.Fatal("missing client/target namespace names")
	}

	target := startSocketTunnelTarget(t, targetNS)
	defer func() {
		_ = target.Process.Kill()
		_, _ = target.Process.Wait()
	}()
	waitSocketTunnelTarget(t)

	lease := socketTunnelLease(t)
	clientOwner, err := datapath.NewLeasedTunnelOwner(lease, 1, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer clientOwner.Close()
	serverOwner, err := datapath.NewLeasedTunnelOwner(lease, 1, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer serverOwner.Close()

	clientLane := socketTunnelLane(t, datapath.RoleClient, lease)
	serverLane := socketTunnelLane(t, datapath.RoleServer, lease)
	clientSnap, err := clientOwner.AttachInitial(1, clientLane)
	if err != nil {
		t.Fatal(err)
	}
	serverSnap, err := serverOwner.AttachInitial(1, serverLane)
	if err != nil {
		t.Fatal(err)
	}

	writer := &serviceRejectWriter{}
	router, err := linuxserver.NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 4, writer)
	if err != nil {
		t.Fatal(err)
	}
	token, err := router.Register(serverOwner)
	if err != nil {
		t.Fatal(err)
	}

	var adapter *SocketAdapter
	serverChannel, err := platformflow.NewTunnelChannel(serverOwner, func(out platformflow.Outbound) error {
		if out.IsGame {
			return errors.New("privileged socket contract expected Normal mode")
		}
		now := time.Now()
		for _, record := range out.Normal {
			result, err := clientOwner.InboundPayload(clientSnap.Ref, record.Wire, now)
			if err != nil {
				return err
			}
			if len(result.RecordErrors) != 0 || len(result.PathErrors) != 0 {
				return fmt.Errorf("client inbound record=%v path=%v", result.RecordErrors, result.PathErrors)
			}
			if adapter == nil || adapter.client == nil {
				return errors.New("client socket adapter unavailable")
			}
			for _, packet := range result.Datagrams {
				handled, err := adapter.client.HandleServicePacket(packet, now)
				if err != nil {
					return err
				}
				if !handled {
					return errors.New("server response was not a platform service packet")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	serverService, err := platformflow.NewServer(serverChannel, platformflow.DefaultServerConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer serverService.Close()
	if err := router.SetServiceHandler(token, serverService); err != nil {
		t.Fatal(err)
	}

	clientChannel, err := platformflow.NewTunnelChannel(clientOwner, func(out platformflow.Outbound) error {
		if out.IsGame {
			return errors.New("privileged socket contract expected Normal mode")
		}
		now := time.Now()
		for _, record := range out.Normal {
			result, err := serverOwner.InboundPayload(serverSnap.Ref, record.Wire, now)
			if err != nil {
				return err
			}
			if len(result.RecordErrors) != 0 || len(result.PathErrors) != 0 {
				return fmt.Errorf("server inbound record=%v path=%v", result.RecordErrors, result.PathErrors)
			}
			if err := router.DeliverFromOwnerAt(token, result.Datagrams, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	clientCfg := platformflow.DefaultClientConfig()
	clientCfg.UDPIdle = 10 * time.Second
	clientCfg.TCP.IdleTimeout = 10 * time.Second
	adapter, err = OpenSocketAdapter(SocketConfig{
		ListenPort: 12345, Channel: clientChannel, Client: clientCfg,
		MaxReplySockets: 32, TickInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()

	plan, err := BuildNetworkPlan(12345, 0x66, 1066, 1066, netip.MustParseAddr("10.20.0.3"))
	if err != nil {
		t.Fatal(err)
	}
	networkRuntime, err := OpenRuntime(plan)
	if err != nil {
		t.Fatal(err)
	}
	defer networkRuntime.Close()

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- adapter.Run(ctx) }()
	defer func() {
		cancel()
		_ = adapter.Close()
		select {
		case err := <-runDone:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrSocketClosed) {
				t.Errorf("socket adapter exit: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("socket adapter did not stop")
		}
	}()

	tcpOut := runClientPython(t, clientNS, `
import socket
s=socket.create_connection(("10.20.0.2",8088),timeout=4)
s.sendall(b"through-tunnel-owner")
s.shutdown(socket.SHUT_WR)
s.settimeout(5)
out=b""
while True:
    part=s.recv(4096)
    if not part: break
    out += part
assert out == b"TUNNELTCP:through-tunnel-owner", out
print("SOCKET_TUNNEL_TCP_OK", flush=True)
`)
	if !strings.Contains(tcpOut, "SOCKET_TUNNEL_TCP_OK") {
		t.Fatalf("TCP output=%q", tcpOut)
	}

	udpOut := runClientPython(t, clientNS, `
import socket

def seen(data,prefix):
    assert data.startswith(prefix),(data,prefix)
    return int(data.rsplit(b":SEEN=",1)[1])

s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(("10.10.0.2",0))
s.settimeout(5)

s.sendto(b"one",("10.20.0.2",5355))
data,peer=s.recvfrom(65535)
assert peer == ("10.20.0.2",5355),peer
pa=seen(data,b"A:one")

s.sendto(b"two",("10.20.0.4",5356))
data,peer=s.recvfrom(65535)
assert peer == ("10.20.0.4",5356),peer
pb=seen(data,b"B:two")
assert pa == pb and pa != 0,(pa,pb)
print("SOCKET_TUNNEL_UDP_EIM_OK",pa,flush=True)

s.sendto(str(pa).encode(),("10.20.0.3",5454))
ack,peer=s.recvfrom(65535)
assert peer == ("10.20.0.3",5454) and ack == b"CONTROL_OK",(peer,ack)
data,peer=s.recvfrom(65535)
assert peer == ("10.20.0.5",7777),peer
assert data == b"UNSOLICITED",data
print("SOCKET_TUNNEL_UDP_EIF_OK",flush=True)
`)
	if !strings.Contains(udpOut, "SOCKET_TUNNEL_UDP_EIM_OK") || !strings.Contains(udpOut, "SOCKET_TUNNEL_UDP_EIF_OK") {
		t.Fatalf("UDP output=%q", udpOut)
	}

	if writer.Count() != 0 {
		t.Fatalf("platform service packet leaked to shared TUN writes=%d", writer.Count())
	}
	if st := clientOwner.Stats(); st.ActiveLogicalLanes != 1 || st.PhysicalLanes != 1 {
		t.Fatalf("client owner lane growth=%+v", st)
	}
	if st := serverOwner.Stats(); st.ActiveLogicalLanes != 1 || st.PhysicalLanes != 1 {
		t.Fatalf("server owner lane growth=%+v", st)
	}

	fmt.Println("WBD_P4_OPENWRT_SOCKET_TUNNEL_PASS tcp=1 udp=1 eim=1 eif=1 tunnelowner=1 lanes=1 service_tun_leak=0 ipv6=NOT_IMPLEMENTED")
}

func socketTunnelLease(t *testing.T) logicaltunnel.Lease {
	t.Helper()
	id, err := logicaltunnel.ParseTunnelID("11223344556677889900aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	return logicaltunnel.Lease{
		Account: "openwrt-socket-contract",
		Config: logicaltunnel.TunnelConfig{TunnelID: id, Address4: "10.66.0.9/32"},
	}
}

func socketTunnelKeys() tlsrecord.KeyPair {
	var keys tlsrecord.KeyPair
	for i := range keys.C2S.AEADKey {
		keys.C2S.AEADKey[i] = byte(i + 1)
		keys.C2S.HPKey[i] = byte(0x80 + i)
		keys.S2C.AEADKey[i] = byte(0x40 + i)
		keys.S2C.HPKey[i] = byte(0xc0 + i)
	}
	for i := range keys.C2S.IV {
		keys.C2S.IV[i] = byte(0x10 + i)
		keys.S2C.IV[i] = byte(0x30 + i)
	}
	return keys
}

func socketTunnelLane(t *testing.T, role datapath.Role, lease logicaltunnel.Lease) *datapath.Lane {
	t.Helper()
	mtu := func(limit int) pathmtu.Config {
		return pathmtu.Config{
			ConnectionMTU: 1500, IPv4HeaderLen: 20, TCPHeaderLen: 20,
			PeerMSS: faketcp.DefaultMSS, PeerMSSSet: true,
			RecordWireLimit: limit,
		}
	}
	cfg := datapath.LaneConfig{
		Role: role, TunnelID: lease.Config.TunnelID.Bytes(),
		ClientRecordLimit: 1300, ServerRecordLimit: 1250,
		Keys: socketTunnelKeys(),
	}
	cfg.IncarnationNonce[0] = 71
	if role == datapath.RoleClient {
		cfg.TxMTU = mtu(cfg.ServerRecordLimit)
		cfg.RxMTU = mtu(cfg.ClientRecordLimit)
	} else {
		cfg.TxMTU = mtu(cfg.ClientRecordLimit)
		cfg.RxMTU = mtu(cfg.ServerRecordLimit)
	}
	lane, err := datapath.NewLane(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return lane
}

func startSocketTunnelTarget(t *testing.T, ns string) *exec.Cmd {
	t.Helper()
	code := `
import socket,threading,time

def tcp():
    s=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
    s.bind(("10.20.0.2",8088)); s.listen(16)
    while True:
        c,peer=s.accept()
        try:
            if peer[0] != "10.20.0.1":
                c.close(); continue
            data=b""
            while True:
                part=c.recv(4096)
                if not part: break
                data += part
            c.sendall(b"TUNNELTCP:"+data)
            try: c.shutdown(socket.SHUT_WR)
            except OSError: pass
        finally:
            c.close()

def udp(addr,port,tag):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    s.bind((addr,port))
    while True:
        data,peer=s.recvfrom(65535)
        s.sendto(tag+b":"+data+b":SEEN="+str(peer[1]).encode(),peer)

def control():
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    s.bind(("10.20.0.3",5454))
    injector=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    injector.bind(("10.20.0.5",7777))
    while True:
        data,peer=s.recvfrom(128)
        mapped=int(data.decode())
        s.sendto(b"CONTROL_OK",peer)
        time.sleep(.02)
        injector.sendto(b"UNSOLICITED",("10.20.0.1",mapped))

threading.Thread(target=tcp,daemon=True).start()
threading.Thread(target=udp,args=("10.20.0.2",5355,b"A"),daemon=True).start()
threading.Thread(target=udp,args=("10.20.0.4",5356,b"B"),daemon=True).start()
threading.Thread(target=control,daemon=True).start()
while True: time.sleep(3600)
`
	cmd := exec.Command("ip", "netns", "exec", ns, "python3", "-u", "-c", code)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func waitSocketTunnelTarget(t *testing.T) {
	t.Helper()
	for i := 0; i < 50; i++ {
		conn, err := net.DialTimeout("tcp4", "10.20.0.2:8088", 100*time.Millisecond)
		if err == nil {
			if tcp, ok := conn.(*net.TCPConn); ok {
				_, writeErr := tcp.Write([]byte("ready"))
				closeErr := tcp.CloseWrite()
				_ = tcp.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				buf := make([]byte, 64)
				n, readErr := tcp.Read(buf)
				_ = tcp.Close()
				if writeErr == nil && closeErr == nil && readErr == nil && string(buf[:n]) == "TUNNELTCP:ready" {
					return
				}
			} else {
				_ = conn.Close()
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("socket tunnel target did not become ready")
}

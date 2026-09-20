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
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPrivilegedOpenWrtTPROXYRuntime(t *testing.T) {
	if os.Getenv("WBD_P4_OPENWRT_TPROXY_NET") != "1" {
		t.Skip("set WBD_P4_OPENWRT_TPROXY_NET=1 inside the root netns harness")
	}
	if os.Geteuid() != 0 {
		t.Fatal("OpenWrt TPROXY qualification requires root")
	}
	clientNS := os.Getenv("WBD_P4_OPENWRT_CLIENT_NS")
	targetNS := os.Getenv("WBD_P4_OPENWRT_TARGET_NS")
	if clientNS == "" || targetNS == "" {
		t.Fatal("missing client/target namespace names")
	}

	target := startTargetServices(t, targetNS)
	defer func() {
		_ = target.Process.Kill()
		_, _ = target.Process.Wait()
	}()
	waitForTarget(t, clientNS)

	mustCommand(t, "nft", "add", "table", "inet", "wbd_foreign")
	defer exec.Command("nft", "delete", "table", "inet", "wbd_foreign").Run()
	mustCommand(t, "ip", "-4", "rule", "add", "priority", "2000", "from", "10.10.0.0/24", "lookup", "main")
	defer exec.Command("ip", "-4", "rule", "del", "priority", "2000").Run()

	tcpListener := transparentTCPListener(t, 12345)
	defer tcpListener.Close()
	udpListener := transparentUDPListener(t, 12345)
	defer udpListener.Close()

	plan, err := BuildNetworkPlan(
		12345,
		0x66,
		1066,
		1066,
		netip.MustParseAddr("10.20.0.3"),
	)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := OpenRuntime(plan)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = rt.Close()
		}
	}()

	assertKernelOwnership(t, plan, true)
	if _, err := OpenRuntime(plan); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("second runtime must refuse existing state: %v", err)
	}

	tcpAccepted := make(chan error, 1)
	go func() {
		conn, err := tcpListener.Accept()
		if err != nil {
			tcpAccepted <- err
			return
		}
		defer conn.Close()
		local := conn.LocalAddr().(*net.TCPAddr)
		if local.IP.String() != "10.20.0.2" || local.Port != 8080 {
			tcpAccepted <- fmt.Errorf("transparent TCP local=%s want=10.20.0.2:8080", local)
			return
		}
		buf := make([]byte, 64)
		n, err := conn.Read(buf)
		if err != nil {
			tcpAccepted <- err
			return
		}
		if string(buf[:n]) != "capture-tcp" {
			tcpAccepted <- fmt.Errorf("transparent TCP payload=%q", buf[:n])
			return
		}
		_, err = conn.Write([]byte("TPROXYTCP"))
		tcpAccepted <- err
	}()
	out := runClientPython(t, clientNS, `
import socket
s=socket.create_connection(("10.20.0.2",8080),timeout=3)
s.sendall(b"capture-tcp")
data=s.recv(64)
assert data == b"TPROXYTCP", data
print("TCP_CAPTURE_OK", flush=True)
`)
	if !strings.Contains(out, "TCP_CAPTURE_OK") {
		t.Fatalf("tcp client output=%q", out)
	}
	select {
	case err := <-tcpAccepted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("transparent TCP listener did not receive captured flow")
	}

	udpReceived := make(chan error, 1)
	go func() {
		_ = udpListener.SetReadDeadline(time.Now().Add(3 * time.Second))
		buf := make([]byte, 64)
		n, peer, err := udpListener.ReadFromUDP(buf)
		if err != nil {
			udpReceived <- err
			return
		}
		if peer.IP.String() != "10.10.0.2" {
			udpReceived <- fmt.Errorf("transparent UDP peer=%s", peer)
			return
		}
		if string(buf[:n]) != "capture-udp" {
			udpReceived <- fmt.Errorf("transparent UDP payload=%q", buf[:n])
			return
		}
		udpReceived <- nil
	}()
	runClientPython(t, clientNS, `
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.sendto(b"capture-udp",("10.20.0.2",5353))
print("UDP_SENT", flush=True)
`)
	select {
	case err := <-udpReceived:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("transparent UDP listener did not receive captured datagram")
	}

	out = runClientPython(t, clientNS, `
import socket
s=socket.create_connection(("10.20.0.3",8081),timeout=3)
s.sendall(b"underlay-tcp")
assert s.recv(64) == b"UNDERLAYTCP", "underlay tcp"
u=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); u.settimeout(3)
u.sendto(b"underlay-udp",("10.20.0.3",5354))
data,peer=u.recvfrom(64)
assert data == b"UNDERLAYUDP" and peer == ("10.20.0.3",5354), (data,peer)
print("UNDERLAY_BYPASS_OK", flush=True)
`)
	if !strings.Contains(out, "UNDERLAY_BYPASS_OK") {
		t.Fatalf("underlay client output=%q", out)
	}

	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	assertKernelOwnership(t, plan, false)
	mustCommand(t, "nft", "list", "table", "inet", "wbd_foreign")
	if rules := mustCommand(t, "ip", "-4", "rule", "show"); !strings.Contains(rules, "2000:") {
		t.Fatalf("foreign policy rule was removed: %s", rules)
	}

	out = runClientPython(t, clientNS, `
import socket
s=socket.create_connection(("10.20.0.2",8080),timeout=3)
s.sendall(b"plain-tcp")
assert s.recv(64) == b"DIRECTTCP", "direct tcp"
u=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); u.settimeout(3)
u.sendto(b"plain-udp",("10.20.0.2",5353))
data,peer=u.recvfrom(64)
assert data == b"DIRECTUDP" and peer == ("10.20.0.2",5353), (data,peer)
print("CLEANUP_RESTORE_OK", flush=True)
`)
	if !strings.Contains(out, "CLEANUP_RESTORE_OK") {
		t.Fatalf("cleanup client output=%q", out)
	}

	fmt.Println("WBD_P4_OPENWRT_TPROXY_PASS tcp=1 udp=1 underlay_bypass=1 cleanup=owned-only ipv6=NOT_IMPLEMENTED")
}

func transparentTCPListener(t *testing.T, port int) *net.TCPListener {
	t.Helper()
	lc := net.ListenConfig{Control: transparentControl}
	ln, err := lc.Listen(context.Background(), "tcp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	return ln.(*net.TCPListener)
}

func transparentUDPListener(t *testing.T, port int) *net.UDPConn {
	t.Helper()
	lc := net.ListenConfig{Control: transparentControl}
	pc, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	return pc.(*net.UDPConn)
}

func transparentControl(_, _ string, c syscall.RawConn) error {
	var sockErr error
	if err := c.Control(func(fd uintptr) {
		sockErr = unix.SetsockoptInt(int(fd), unix.SOL_IP, unix.IP_TRANSPARENT, 1)
	}); err != nil {
		return err
	}
	return sockErr
}

func startTargetServices(t *testing.T, ns string) *exec.Cmd {
	t.Helper()
	code := `
import socket,threading,time

def tcp(addr,port,reply):
    s=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
    s.bind((addr,port)); s.listen(16)
    while True:
        c,_=s.accept()
        try:
            c.recv(64)
            c.sendall(reply)
        finally:
            c.close()

def udp(addr,port,reply):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    s.bind((addr,port))
    while True:
        _,peer=s.recvfrom(64)
        s.sendto(reply,peer)

for args in [
    ("10.20.0.2",8080,b"DIRECTTCP"),
    ("10.20.0.3",8081,b"UNDERLAYTCP"),
]:
    threading.Thread(target=tcp,args=args,daemon=True).start()
for args in [
    ("10.20.0.2",5353,b"DIRECTUDP"),
    ("10.20.0.3",5354,b"UNDERLAYUDP"),
]:
    threading.Thread(target=udp,args=args,daemon=True).start()
while True: time.sleep(3600)
`
	cmd := exec.Command("ip", "netns", "exec", ns, "python3", "-u", "-c", code)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func waitForTarget(t *testing.T, clientNS string) {
	t.Helper()
	for i := 0; i < 30; i++ {
		cmd := exec.Command(
			"ip", "netns", "exec", clientNS,
			"python3", "-c",
			`import socket; s=socket.create_connection(("10.20.0.3",8081),timeout=.2); s.sendall(b"x"); assert s.recv(64)==b"UNDERLAYTCP"`,
		)
		if err := cmd.Run(); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("target services did not become ready")
}

func runClientPython(t *testing.T, ns, code string) string {
	t.Helper()
	out, err := exec.Command("ip", "netns", "exec", ns, "python3", "-c", code).CombinedOutput()
	if err != nil {
		t.Fatalf("client python: %v: %s", err, out)
	}
	return string(out)
}

func assertKernelOwnership(t *testing.T, plan NetworkPlan, want bool) {
	t.Helper()
	tableErr := exec.Command("nft", "list", "table", plan.NFTFamily, plan.NFTTable).Run()
	if (tableErr == nil) != want {
		t.Fatalf("nft table present=%v want=%v", tableErr == nil, want)
	}
	rules := mustCommand(t, "ip", "-4", "rule", "show")
	hasRule := strings.Contains(rules, strconv.FormatUint(uint64(plan.Priority), 10)+":") &&
		strings.Contains(rules, fmt.Sprintf("fwmark 0x%x", plan.Mark))
	if hasRule != want {
		t.Fatalf("policy rule present=%v want=%v rules=%s", hasRule, want, rules)
	}
	routes, err := routeTableState(plan.Table)
	if err != nil {
		t.Fatal(err)
	}
	hasRoute := strings.Contains(routes, "local default dev lo") ||
		strings.Contains(routes, "local 0.0.0.0/0 dev lo")
	if hasRoute != want {
		t.Fatalf("local route present=%v want=%v routes=%s", hasRoute, want, routes)
	}
}

func mustCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
	return string(out)
}
